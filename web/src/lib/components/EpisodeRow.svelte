<script lang="ts">
	// Fila de episodio dentro de una temporada (Sistema de Diseño GALA §3.1):
	// miniatura (o respaldo sobrio), número, título real, sinopsis breve, estado
	// de salud y progreso. El nombre crudo "Serie T1E01" no se enseña nunca.
	import { imgUrl } from '$lib/api/client';
	import type { Health, Item } from '$lib/api/types';
	import HealthDot from './HealthDot.svelte';

	interface Props {
		episode: Item;
		/** Número de temporada para el código "T1 · E3" (el episodio no lo trae). */
		seasonNumber?: number;
	}
	let { episode, seasonNumber }: Props = $props();

	const still = $derived(imgUrl(episode.poster_path));
	const number = $derived(episode.episode_number);
	const code = $derived(
		seasonNumber && number ? `T${seasonNumber} · E${number}` : number ? `E${number}` : ''
	);
	// El título provisional que pone M3 antes de pedir el detalle a TMDB
	// ("Serie T1E01") se sustituye por "Episodio N" mientras llega.
	const looksRaw = $derived(/\sT\d+E\d+$/.test(episode.title));
	const title = $derived(looksRaw && number ? `Episodio ${number}` : episode.title);

	const playable = $derived(episode.source_count > 0);
	const dots = $derived.by(() => {
		const all: Health[] = [];
		for (const [h, n] of Object.entries(episode.health_summary ?? {})) {
			for (let i = 0; i < (n ?? 0); i++) all.push(h as Health);
		}
		return all.slice(0, 3);
	});
	const pct = $derived.by(() => {
		const p = episode.progress;
		if (!p || !p.duration_s || p.duration_s <= 0) return null;
		return Math.min(100, Math.round((p.position_s / p.duration_s) * 100));
	});
	const watched = $derived(!!episode.progress?.watched);
</script>

<li class="ep">
	<a class="thumb" href="/item/{episode.id}" tabindex="-1" aria-hidden="true">
		{#if still}
			<img src={still} alt="" loading="lazy" />
		{:else}
			<span class="fallback">{number ?? '·'}</span>
		{/if}
		{#if pct !== null && !watched}
			<span class="bar"><i style:width="{pct}%"></i></span>
		{/if}
	</a>

	<a class="body" href="/item/{episode.id}">
		<span class="line">
			{#if code}<span class="code">{code}</span>{/if}
			<span class="title">{title}</span>
			{#if watched}<span class="seen">Visto</span>{/if}
		</span>
		{#if episode.overview}<span class="overview">{episode.overview}</span>{/if}
	</a>

	<div class="side">
		{#if dots.length > 0}
			<span class="dots">
				{#each dots as h, i (i)}<HealthDot health={h} size={7} />{/each}
			</span>
		{:else}
			<span class="nosrc">Sin fuente</span>
		{/if}
		{#if playable}
			<a class="play" href="/ver/{episode.id}" aria-label="Reproducir {code} {title}">▶</a>
		{:else}
			<span class="play off" title="Este episodio aún no tiene fuente" aria-hidden="true">▶</span>
		{/if}
	</div>
</li>

<style>
	.ep {
		display: grid;
		grid-template-columns: 132px minmax(0, 1fr) auto;
		gap: 14px;
		align-items: center;
		padding: 10px 14px 10px 10px;
		border-bottom: 1px solid rgba(51, 38, 25, 0.5);
	}
	.ep:last-child {
		border-bottom: none;
	}
	.ep:hover {
		background: var(--surface);
	}
	.thumb {
		position: relative;
		display: block;
		aspect-ratio: 16 / 9;
		border-radius: 9px;
		overflow: hidden;
		background: linear-gradient(150deg, #3a2412, #1a0f06 70%);
		box-shadow: 0 0 0 1px var(--line);
	}
	.thumb img {
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
		align-items: center;
		justify-content: center;
		font-family: var(--display);
		font-weight: 800;
		font-size: 30px;
		color: rgba(244, 237, 227, 0.28);
	}
	.bar {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 3px;
		background: rgba(0, 0, 0, 0.55);
	}
	.bar i {
		display: block;
		height: 100%;
		background: var(--accent);
	}
	.body {
		display: flex;
		flex-direction: column;
		gap: 3px;
		min-width: 0;
		text-decoration: none;
		color: inherit;
	}
	.line {
		display: flex;
		align-items: baseline;
		gap: 10px;
		min-width: 0;
	}
	.code {
		font-size: 12px;
		font-weight: 600;
		color: var(--muted);
		flex-shrink: 0;
		white-space: nowrap;
	}
	.title {
		font-weight: 600;
		font-size: 14.5px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.seen {
		font-size: 11px;
		color: var(--ok);
		flex-shrink: 0;
	}
	.overview {
		color: var(--muted);
		font-size: 13px;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.side {
		display: flex;
		align-items: center;
		gap: 12px;
	}
	.dots {
		display: flex;
		gap: 4px;
	}
	.nosrc {
		font-size: 12px;
		color: var(--muted);
		white-space: nowrap;
	}
	.play {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		border-radius: 50%;
		border: 1px solid var(--line);
		color: var(--text);
		text-decoration: none;
		font-size: 13px;
		background: rgba(255, 255, 255, 0.03);
		transition: 0.15s;
	}
	a.play:hover {
		background: var(--text);
		color: #17110a;
	}
	.play.off {
		opacity: 0.3;
	}
	@media (max-width: 600px) {
		.ep {
			grid-template-columns: 96px minmax(0, 1fr);
			gap: 10px;
		}
		.side {
			grid-column: 1 / -1;
			justify-content: space-between;
		}
	}
</style>
