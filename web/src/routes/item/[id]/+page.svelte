<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { deleteItem, imgUrl, setWished } from '$lib/api/client';
	import type { LanSession } from '$lib/api/types';
	import EpisodeRow from '$lib/components/EpisodeRow.svelte';
	import SourceStack from '$lib/components/SourceStack.svelte';
	import StateBlock from '$lib/components/StateBlock.svelte';
	import { displayTitle, episodeCode, errorMessage, isUnidentified } from '$lib/format';
	import { nextEpisode, seriesStatusText, seriesSummary } from '$lib/series';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const item = $derived(data.item);

	const isLocal = $derived((page.data as { session?: LanSession }).session?.local !== false);

	const isEpisode = $derived(item?.kind === 'episode');
	const isSeries = $derived(item?.kind === 'series');
	const backdrop = $derived(item ? imgUrl(item.backdrop_path) : null);
	const poster = $derived(item ? imgUrl(item.poster_path) : null);
	const title = $derived(item ? displayTitle(item) : '');
	const unidentified = $derived(item ? isUnidentified(item) : false);
	const code = $derived(item && isEpisode ? episodeCode(item) : '');

	// Una serie no tiene fuente propia: su estado y su "Reproducir" salen de
	// sus episodios (Sistema de Diseño GALA §3.1).
	const orderedEpisodes = $derived(data.seasons.flatMap((s) => data.episodesBySeason[s.id] ?? []));
	const summary = $derived(seriesSummary(data.episodesBySeason));
	const next = $derived(isSeries ? nextEpisode(orderedEpisodes) : null);
	const playHref = $derived(
		!item ? '' : isSeries ? (next ? `/ver/${next.id}` : '') : item.source_count > 0 || item.sources?.length ? `/ver/${item.id}` : ''
	);
	const canPlay = $derived(playHref !== '');
	const playText = $derived.by(() => {
		if (!isSeries) return '▶ Reproducir';
		if (!next) return '▶ Reproducir';
		const started = next.progress && !next.progress.watched && next.progress.position_s > 0;
		return `${started ? '▶ Continuar' : '▶ Empezar'} ${episodeCode(next, seasonOf(next.parent_id))}`.trim();
	});
	function seasonOf(parentId: number | undefined): number | undefined {
		return data.seasons.find((s) => s.id === parentId)?.season_number;
	}

	let wished = $state(false);
	let wishPending = $state(false);
	$effect(() => {
		wished = item?.wished ?? false;
	});
	async function toggleWished() {
		if (!item || wishPending) return;
		const nextValue = !wished;
		wishPending = true;
		try {
			await setWished(item.id, nextValue);
			wished = nextValue;
		} catch {
			// el botón se queda como estaba; se puede reintentar
		} finally {
			wishPending = false;
		}
	}

	// Descartar un título sin identificar (solo el dueño, con confirmación).
	let confirmingDiscard = $state(false);
	let discardPending = $state(false);
	let discardError = $state<string | null>(null);
	async function discard() {
		if (!item || discardPending) return;
		discardPending = true;
		discardError = null;
		try {
			await deleteItem(item.id);
			await goto('/');
		} catch (err) {
			discardError = errorMessage(err, 'No se pudo descartar');
			discardPending = false;
		}
	}

	const identifyNote = $derived.by(() => {
		switch (item?.identify_status) {
			case 'pending':
				return 'GALA está identificando este título…';
			case 'review':
				return 'GALA no estuvo segura de qué título es. Puedes elegirlo tú.';
			case 'error':
				return 'No se pudo identificar. Puedes elegir el título tú.';
			case 'junk':
				return 'GALA lo detectó como un vídeo suelto (no una película ni serie) y no lo ofrece para identificar.';
			default:
				return 'Aún no está identificado: se enseña con el nombre del archivo.';
		}
	});
	const canIdentify = $derived(
		isLocal && (item?.identify_status === 'review' || item?.identify_status === 'error')
	);
</script>

<svelte:head>
	<title>{item ? `${title} — GALA` : 'GALA'}</title>
</svelte:head>

{#if data.itemError}
	<StateBlock
		tone="error"
		title="No se pudo cargar esta ficha"
		text={data.itemError}
		actionLabel="Reintentar"
		onAction={() => invalidateAll()}
	/>
{:else if item}
	<section class="detail">
		{#if backdrop}
			<div class="backdrop" style:background-image="url({backdrop})"></div>
		{/if}
		<div class="detail-inner" class:ep-layout={isEpisode}>
			<div class="art" class:still={isEpisode}>
				{#if poster}
					<img src={poster} alt="" />
				{:else}
					<div class="fallback"><b>{isEpisode ? code || title : title}</b></div>
				{/if}
			</div>

			<div class="info">
				{#if isEpisode && item.series_id && item.series_title}
					<a class="crumb" href="/item/{item.series_id}">← {item.series_title}</a>
				{/if}
				<h1>{title}</h1>
				{#if isEpisode && code}
					<p class="code">{code}</p>
				{/if}
				{#if unidentified}
					<p class="unidentified-tag">Sin identificar</p>
				{/if}
				{#if item.original_title && item.original_title !== item.title}
					<p class="original">{item.original_title}</p>
				{/if}
				<div class="meta">
					{#if item.rating_cert}<span class="cert">{item.rating_cert}+</span>{/if}
					{#if item.year}<span>{item.year}</span>{/if}
					{#if item.genres.length}<span>{item.genres.join(' · ')}</span>{/if}
				</div>
				{#if item.overview}<p class="overview">{item.overview}</p>{/if}

				{#if unidentified}
					<p class="identify-note">{identifyNote}</p>
				{/if}

				<div class="actions">
					{#if canPlay}
						<a class="btn-play" href={playHref}>{playText}</a>
					{:else}
						<span
							class="btn-play off"
							aria-disabled="true"
							title={isSeries ? 'Ninguno de sus episodios tiene fuente todavía' : 'Este título aún no tiene fuente'}
							>{playText}</span
						>
					{/if}
					{#if !isEpisode}
						<button class="btn btn-secondary" class:active={wished} disabled={wishPending} onclick={toggleWished}>
							{wished ? '✓ En tu lista' : '＋ Mi lista'}
						</button>
					{/if}
					{#if unidentified && isLocal}
						{#if canIdentify}
							<a class="btn btn-secondary" href="/pendientes#item-{item.id}">Identificar</a>
						{/if}
						{#if !confirmingDiscard}
							<button class="btn btn-quiet-danger" onclick={() => (confirmingDiscard = true)}>Descartar</button>
						{/if}
					{/if}
				</div>

				{#if confirmingDiscard}
					<div class="discard" role="alertdialog" aria-label="Confirmar descarte">
						<p>¿Descartar este título? Se quita de tu galería junto con sus fuentes; los archivos del disco no se tocan.</p>
						<div class="discard-actions">
							<button class="btn btn-quiet-danger btn-sm" disabled={discardPending} onclick={discard}>
								{discardPending ? 'Descartando…' : 'Sí, descartar'}
							</button>
							<button class="btn btn-secondary btn-sm" disabled={discardPending} onclick={() => (confirmingDiscard = false)}>
								Cancelar
							</button>
						</div>
						{#if discardError}<p class="err" role="alert">{discardError}</p>{/if}
					</div>
				{/if}

				{#if isSeries}
					<p class="series-status" class:none={summary.available === 0}>{seriesStatusText(summary)}</p>
				{:else if item.sources && item.sources.length > 0}
					<SourceStack sources={item.sources} />
				{:else}
					<p class="no-sources">Este título aún no tiene fuentes.</p>
				{/if}

				{#if isSeries}
					<div class="seasons">
						{#if data.seasons.length === 0}
							<StateBlock compact title="Sin temporadas todavía" text="Aparecerán cuando GALA identifique sus episodios." />
						{:else}
							{#each data.seasons as season (season.id)}
								{@const eps = data.episodesBySeason[season.id] ?? []}
								<div class="season">
									<h2>
										{season.season_number ? `Temporada ${season.season_number}` : displayTitle(season)}
										<span class="season-count">{eps.length} {eps.length === 1 ? 'episodio' : 'episodios'}</span>
									</h2>
									{#if eps.length > 0}
										<ol class="episodes">
											{#each eps as ep (ep.id)}
												<EpisodeRow episode={ep} seasonNumber={season.season_number} />
											{/each}
										</ol>
									{:else}
										<p class="muted">Sin episodios catalogados todavía.</p>
									{/if}
								</div>
							{/each}
						{/if}
					</div>
				{/if}
			</div>
		</div>
	</section>
{/if}

<style>
	.detail {
		position: relative;
		padding: 48px 28px 60px;
		isolation: isolate;
	}
	.backdrop {
		position: absolute;
		inset: 0;
		z-index: -2;
		background-size: cover;
		background-position: center 20%;
		opacity: 0.35;
		-webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.75), transparent 90%);
		mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.75), transparent 90%);
	}
	.detail::before {
		content: '';
		position: absolute;
		inset: 0;
		z-index: -3;
		background: radial-gradient(120% 90% at 78% 0%, #3d2a14 0%, #221709 42%, var(--bg) 78%);
	}
	.detail-inner {
		max-width: 1180px;
		margin: 0 auto;
		display: grid;
		grid-template-columns: 280px 1fr;
		gap: 40px;
		align-items: start;
	}
	.detail-inner.ep-layout {
		grid-template-columns: 380px 1fr;
	}
	.art {
		display: block;
		aspect-ratio: 2 / 3;
		border-radius: 16px;
		overflow: hidden;
		position: relative;
		box-shadow: 0 24px 60px rgba(0, 0, 0, 0.55), 0 0 0 1px var(--line);
		background: linear-gradient(160deg, #4a3016 0%, #1e1207 55%, #0d0a06 100%);
	}
	.art.still {
		aspect-ratio: 16 / 9;
	}
	.art img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.fallback {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: flex-end;
		padding: 22px;
		font-family: var(--display);
		font-weight: 800;
		font-size: 24px;
		line-height: 1.1;
		background: radial-gradient(90% 60% at 30% 20%, rgba(242, 169, 59, 0.35), transparent 60%);
	}
	.info {
		/* Un track de grid "1fr" sigue teniendo min-width:auto por defecto: sin
		   esto, el contenido sin ajuste de línea de SourceStack (rutas largas)
		   fuerza el track — y con él toda la página — más ancho que el viewport
		   en móvil, incluso ya colapsado a una sola columna. */
		min-width: 0;
	}
	.crumb {
		display: inline-block;
		color: var(--muted);
		text-decoration: none;
		font-size: 14px;
		margin-bottom: 10px;
	}
	.crumb:hover {
		color: var(--text);
	}
	h1 {
		font-family: var(--display);
		font-size: clamp(28px, 4vw, 44px);
		font-weight: 800;
		line-height: 1.05;
		letter-spacing: -0.01em;
		margin-bottom: 6px;
		overflow-wrap: anywhere;
	}
	.code {
		color: var(--accent);
		font-weight: 600;
		font-size: 14px;
		margin-bottom: 6px;
	}
	.unidentified-tag {
		display: inline-block;
		font-size: 13px;
		color: var(--muted);
		font-style: italic;
		margin-bottom: 8px;
	}
	.original {
		color: var(--muted);
		font-size: 14px;
		margin-bottom: 10px;
	}
	.meta {
		display: flex;
		gap: 14px;
		align-items: center;
		color: var(--muted);
		font-size: 14px;
		margin: 10px 0 14px;
		flex-wrap: wrap;
	}
	.meta .cert {
		border: 1px solid var(--line);
		border-radius: 5px;
		padding: 1px 7px;
		font-size: 12px;
	}
	.overview {
		color: var(--muted);
		max-width: 640px;
		margin-bottom: 22px;
	}
	.identify-note {
		color: var(--muted);
		max-width: 560px;
		font-size: 14px;
		margin-bottom: 18px;
	}
	.actions {
		display: flex;
		gap: 12px;
		margin-bottom: 8px;
		flex-wrap: wrap;
		align-items: center;
	}
	.btn-play {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		min-height: 44px;
		background: var(--text);
		color: #17110a;
		font-weight: 600;
		font-size: 15px;
		padding: 12px 26px;
		border-radius: 11px;
		text-decoration: none;
		transition: 0.15s;
	}
	a.btn-play:hover {
		transform: translateY(-1px);
		box-shadow: 0 8px 24px rgba(244, 237, 227, 0.15);
	}
	.btn-play.off {
		opacity: 0.4;
		cursor: default;
	}
	.actions :global(.btn) {
		min-height: 44px;
		padding: 12px 20px;
		font-size: 15px;
		border-radius: 11px;
	}
	.actions :global(.btn.active) {
		border-color: rgba(242, 169, 59, 0.4);
		color: var(--accent);
		background: rgba(242, 169, 59, 0.1);
	}
	.discard {
		margin-top: 14px;
		max-width: 560px;
		padding: 14px 16px;
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.25);
		font-size: 14px;
	}
	.discard-actions {
		display: flex;
		gap: 10px;
		margin-top: 12px;
	}
	.err {
		margin-top: 10px;
		color: var(--bad);
		font-size: 13.5px;
	}
	.no-sources,
	.series-status {
		margin-top: 20px;
		color: var(--muted);
		font-size: 14px;
	}
	.series-status.none {
		color: var(--text);
	}
	.seasons {
		margin-top: 26px;
		max-width: 760px;
		display: flex;
		flex-direction: column;
		gap: 24px;
	}
	.season h2 {
		display: flex;
		align-items: baseline;
		gap: 12px;
		font-family: var(--display);
		font-size: 19px;
		font-weight: 600;
		margin-bottom: 10px;
	}
	.season-count {
		font-family: var(--body);
		font-size: 13px;
		font-weight: 400;
		color: var(--muted);
	}
	.episodes {
		list-style: none;
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.2);
		overflow: hidden;
	}
	.muted {
		color: var(--muted);
		font-size: 14px;
	}

	@media (max-width: 900px) {
		.detail {
			padding: 28px 16px 48px;
		}
		.detail-inner,
		.detail-inner.ep-layout {
			grid-template-columns: 1fr;
			gap: 24px;
		}
		.art:not(.still) {
			max-width: 220px;
		}
	}
</style>
