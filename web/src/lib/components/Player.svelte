<script lang="ts">
	import Hls from 'hls.js';
	import { onDestroy, untrack } from 'svelte';
	import { getPlay, putPlayback, reportSourceFailed, startHLS, streamUrl } from '$lib/api/client';
	import type { Item, PlayResponse, PlaySourceView, Progress } from '$lib/api/types';
	import { displayTitle, providerLabel } from '$lib/format';
	import { isPlayable, pickNextSource } from '$lib/player';
	import HealthDot from './HealthDot.svelte';

	interface Props {
		item: Item;
		initialPlay: PlayResponse;
		initialProgress: Progress | null;
	}
	let { item, initialPlay, initialProgress }: Props = $props();

	const title = $derived(displayTitle(item));

	// Semilla intencional de estado local (untrack): a partir de aquí el
	// reproductor vive de sus propios eventos (fallback, elección manual,
	// timeupdate), no de recargas de las props (mismo patrón que el resto de
	// M10a/M10a-fix, Svelte state_referenced_locally).
	let play = $state(untrack(() => initialPlay));
	let currentSourceId = $state<number | null>(
		untrack(() => initialPlay.chosen_source_id ?? initialPlay.sources[0]?.source_id ?? null)
	);
	let excluded = $state(new Set<number>());
	let fallbackMessage = $state('');
	let exhausted = $state(false);

	let videoEl: HTMLVideoElement | undefined = $state();
	let hls: Hls | null = null;

	// Posición conocida del ARCHIVO ORIGINAL (no de la sesión HLS, que puede
	// empezar su propio reloj en 0 tras un -ss del lado servidor — ver
	// remuxStartOffset). Arranca en el progreso guardado si no está ya visto
	// (Brief M10b §1: "reanudar desde progress").
	let knownPosition = $state(
		untrack(() => (initialProgress && !initialProgress.watched ? initialProgress.position_s : 0))
	);
	let knownDuration = $state<number | undefined>(untrack(() => initialProgress?.duration_s));
	let remuxStartOffset = 0;
	let lastSavedAt = 0;

	const currentSource = $derived(play.sources.find((s) => s.source_id === currentSourceId) ?? null);
	const hasAnySource = $derived(play.sources.length > 0);

	// "Analizando" es transitorio (Brief M4): reintenta /play unas cuantas
	// veces antes de rendirse con un mensaje, en vez de dejarlo colgado.
	let analyzingAttempts = $state(0);
	const MAX_ANALYZING_ATTEMPTS = 15; // ~45s a 3s por intento

	$effect(() => {
		if (currentSource?.mode !== 'analyzing') {
			analyzingAttempts = 0;
			return;
		}
		if (analyzingAttempts >= MAX_ANALYZING_ATTEMPTS) return;
		const t = setTimeout(async () => {
			try {
				play = await getPlay(item.id);
			} catch {
				// se ignora: se reintenta en el siguiente tick igualmente
			}
			analyzingAttempts++;
		}, 3000);
		return () => clearTimeout(t);
	});

	// Montar/desmontar el <video> (o la sesión hls.js) según la fuente activa.
	$effect(() => {
		const src = currentSource;
		teardownHls();
		if (!videoEl || !src) return;
		if (src.mode === 'direct') {
			remuxStartOffset = 0;
			videoEl.src = streamUrl(src.source_id);
		} else if (src.mode === 'remux') {
			setupRemux(src, videoEl);
		} else {
			videoEl.removeAttribute('src');
		}
	});

	onDestroy(() => {
		teardownHls();
		savePosition();
	});

	function teardownHls() {
		if (hls) {
			hls.destroy();
			hls = null;
		}
	}

	async function setupRemux(src: PlaySourceView, video: HTMLVideoElement) {
		const offset = knownPosition;
		try {
			const { playlist_url } = await startHLS(src.source_id, offset || undefined);
			if (currentSource?.source_id !== src.source_id) return; // cambiaste de fuente mientras arrancaba
			remuxStartOffset = offset;
			if (Hls.isSupported()) {
				const instance = new Hls();
				instance.on(Hls.Events.ERROR, (_evt, data) => {
					if (data.fatal) handleSourceError();
				});
				instance.loadSource(playlist_url);
				instance.attachMedia(video);
				hls = instance;
			} else if (video.canPlayType('application/vnd.apple.mpegurl')) {
				video.src = playlist_url;
			} else {
				fallbackMessage = 'Este navegador no puede reproducir este formato (ni siquiera remuxado).';
			}
		} catch {
			handleSourceError();
		}
	}

	async function handleSourceError() {
		const failed = currentSource;
		if (!failed) return;
		excluded.add(failed.source_id);
		reportSourceFailed(failed.source_id).catch(() => {});
		const next = pickNextSource(play.sources, excluded);
		if (next) {
			fallbackMessage = 'La fuente falló, probando la siguiente…';
			currentSourceId = next.source_id;
		} else {
			fallbackMessage = '';
			exhausted = true;
		}
	}

	function selectSource(src: PlaySourceView) {
		fallbackMessage = '';
		exhausted = false;
		currentSourceId = src.source_id;
	}

	function onLoadedMetadata() {
		if (!videoEl || currentSource?.mode !== 'direct' || knownPosition <= 0) return;
		videoEl.currentTime = knownPosition;
	}

	function onTimeUpdate() {
		if (!videoEl) return;
		knownPosition = remuxStartOffset + videoEl.currentTime;
		knownDuration = remuxStartOffset ? knownDuration : videoEl.duration || knownDuration;
		const now = Date.now();
		if (now - lastSavedAt > 10_000) {
			lastSavedAt = now;
			savePosition();
		}
	}

	function savePosition() {
		if (knownPosition <= 0) return;
		putPlayback(item.id, knownPosition, knownDuration).catch(() => {});
	}

	async function copyLink() {
		if (!currentSource) return;
		await navigator.clipboard.writeText(location.origin + currentSource.url);
	}
	async function copyPath() {
		if (currentSource?.path) await navigator.clipboard.writeText(currentSource.path);
	}

	function modeLabel(mode: PlaySourceView['mode']): string {
		switch (mode) {
			case 'direct':
				return 'Directo';
			case 'remux':
				return 'Remux';
			case 'analyzing':
				return 'Analizando';
			case 'reject':
				return 'No compatible';
		}
	}
</script>

<div class="player">
	<header>
		<a class="back" href="/item/{item.id}">← Volver a la ficha</a>
		<h1>{title}</h1>
	</header>

	{#if !play.ffmpeg_available}
		<p class="notice">
			ffmpeg no está disponible en este equipo: los archivos que necesiten conversión no podrán
			reproducirse (solo direct play).
		</p>
	{/if}

	<div class="stage">
		{#if fallbackMessage}
			<p class="notice warn">{fallbackMessage}</p>
		{/if}

		{#if !hasAnySource}
			<div class="empty"><p>Este título todavía no tiene ninguna fuente.</p></div>
		{:else if exhausted}
			<div class="empty">
				<p>Ninguna fuente disponible pudo reproducirse.</p>
				<p class="muted">Puedes elegir otra manualmente abajo si queda alguna, o inténtalo más tarde.</p>
			</div>
		{:else if currentSource?.mode === 'reject'}
			<div class="reject">
				<p>No se puede reproducir esta fuente en el navegador.</p>
				{#if currentSource.reason}<p class="reason">{currentSource.reason}</p>{/if}
				<div class="actions">
					<button class="btn-ghost" onclick={copyLink}>Copiar enlace</button>
					{#if currentSource.path}
						<button class="btn-ghost" onclick={copyPath}>Copiar ruta del archivo</button>
					{/if}
				</div>
			</div>
		{:else if currentSource?.mode === 'analyzing'}
			<div class="analyzing">
				<div class="spinner" aria-hidden="true"></div>
				<p>Analizando el archivo…</p>
			</div>
		{:else}
			<!-- svelte-ignore a11y_media_has_caption -->
			<video
				bind:this={videoEl}
				controls
				autoplay
				onerror={handleSourceError}
				onpause={savePosition}
				ontimeupdate={onTimeUpdate}
				onloadedmetadata={onLoadedMetadata}
			></video>
		{/if}
	</div>

	{#if play.sources.length > 1}
		<div class="sources">
			<div class="sources-head">FUENTES</div>
			{#each play.sources as src (src.source_id)}
				<button
					class="source-row"
					class:active={src.source_id === currentSourceId}
					onclick={() => selectSource(src)}
				>
					<HealthDot health={src.health} size={8} />
					<span class="tag" class:local={src.provider === 'localfs'}>{providerLabel(src.provider)}</span>
					<span class="mode" class:not-ready={!isPlayable(src.mode)}>{modeLabel(src.mode)}</span>
					<span class="quality">{src.quality_label ?? '—'}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.player {
		max-width: 1100px;
		margin: 0 auto;
		padding: 28px 28px 60px;
	}
	header {
		margin-bottom: 18px;
	}
	.back {
		display: inline-block;
		color: var(--muted);
		text-decoration: none;
		font-size: 13.5px;
		margin-bottom: 10px;
	}
	.back:hover {
		color: var(--text);
	}
	header h1 {
		font-family: var(--display);
		font-size: clamp(22px, 3vw, 32px);
		font-weight: 800;
	}
	.notice {
		background: rgba(242, 169, 59, 0.1);
		border: 1px solid rgba(242, 169, 59, 0.3);
		border-radius: var(--r);
		padding: 10px 16px;
		font-size: 13.5px;
		color: var(--accent);
		margin-bottom: 14px;
	}
	.notice.warn {
		position: absolute;
		top: 12px;
		left: 50%;
		transform: translateX(-50%);
		z-index: 2;
		white-space: nowrap;
	}
	.stage {
		position: relative;
		aspect-ratio: 16 / 9;
		background: #000;
		border-radius: var(--r);
		overflow: hidden;
		border: 1px solid var(--line);
		display: flex;
		align-items: center;
		justify-content: center;
	}
	video {
		width: 100%;
		height: 100%;
		background: #000;
	}
	.empty,
	.reject,
	.analyzing {
		text-align: center;
		padding: 0 28px;
		color: var(--text);
	}
	.reject .reason {
		color: var(--muted);
		font-size: 13.5px;
		margin-top: 8px;
	}
	.muted {
		color: var(--muted);
		font-size: 13.5px;
	}
	.actions {
		display: flex;
		gap: 12px;
		justify-content: center;
		margin-top: 18px;
	}
	.btn-ghost {
		padding: 10px 18px;
		border-radius: 10px;
		border: 1px solid var(--line);
		color: var(--text);
		background: rgba(255, 255, 255, 0.03);
		font-weight: 500;
		font-size: 13.5px;
	}
	.btn-ghost:hover {
		background: var(--surface);
	}
	.spinner {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		border: 3px solid var(--line);
		border-top-color: var(--accent);
		margin: 0 auto 14px;
		animation: spin 0.9s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.spinner {
			animation: none;
		}
	}
	.sources {
		margin-top: 20px;
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.2);
		overflow: hidden;
	}
	.sources-head {
		font-family: var(--mono);
		font-size: 11.5px;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--muted);
		padding: 10px 16px;
		border-bottom: 1px solid var(--line);
	}
	.source-row {
		display: flex;
		align-items: center;
		gap: 12px;
		width: 100%;
		padding: 11px 16px;
		border-bottom: 1px solid rgba(51, 38, 25, 0.5);
		font-size: 13.5px;
		text-align: left;
		background: none;
	}
	.source-row:last-child {
		border-bottom: none;
	}
	.source-row:hover {
		background: var(--surface);
	}
	.source-row.active {
		background: var(--surface-2);
	}
	.source-row .tag {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--bg);
		background: var(--muted);
		border-radius: 5px;
		padding: 2px 8px;
		font-weight: 500;
	}
	.source-row .tag.local {
		background: var(--accent);
		color: #1a1207;
	}
	.source-row .mode {
		color: var(--muted);
	}
	.source-row .mode.not-ready {
		color: var(--bad);
	}
	.source-row .quality {
		margin-left: auto;
		color: var(--muted);
	}
</style>
