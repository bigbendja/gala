<script lang="ts">
	// Tarjeta horizontal (16:9) de las vistas de exploración (mockup Vistas2):
	// el fondo es el backdrop de TMDB, el título va encima abajo a la izquierda
	// y debajo una línea con año [· N temporadas], nota de TMDB, salud y NUEVO.
	// Sin backdrop: el póster recortado; sin nada: un degradado sobrio. Nunca
	// un hueco. No enseña idioma (GALA aún no lo sabe por fuente).
	import { imgUrl } from '$lib/api/client';
	import type { Health, Item } from '$lib/api/types';
	import { displayTitle, formatRating } from '$lib/format';
	import HealthDot from './HealthDot.svelte';

	interface Props {
		item: Item;
	}
	let { item }: Props = $props();

	const backdrop = $derived(imgUrl(item.backdrop_path));
	const poster = $derived(imgUrl(item.poster_path));
	const art = $derived(backdrop ?? poster);
	const paletteIndex = $derived((item.id % 8) + 1);
	const title = $derived(displayTitle(item));
	const rating = $derived(formatRating(item.vote_average));

	const NEW_WINDOW_HOURS = 48;
	const isNew = $derived(Date.now() - new Date(item.added_at).getTime() < NEW_WINDOW_HOURS * 3600 * 1000);

	const MAX_DOTS = 3;
	const dots = $derived.by(() => {
		const all: Health[] = [];
		for (const [h, n] of Object.entries(item.health_summary ?? {})) {
			for (let i = 0; i < (n ?? 0); i++) all.push(h as Health);
		}
		return all.slice(0, MAX_DOTS);
	});

	const yearLine = $derived(
		[item.year, item.kind === 'series' && item.season_count ? `${item.season_count} temp.` : '']
			.filter((x) => !!x)
			.join(' · ')
	);

	// Si la imagen falla al cargar, se cae al degradado (nunca un hueco roto).
	let failed = $state(false);
</script>

<a class="card" href="/item/{item.id}">
	{#if isNew}<span class="badge">NUEVO</span>{/if}
	<div class="thumb p{paletteIndex}" class:poster-fallback={!backdrop && !!poster}>
		{#if art && !failed}
			<img src={art} alt="" loading="lazy" onerror={() => (failed = true)} />
		{/if}
		<span class="title">{title}</span>
		<div class="play" aria-hidden="true"><span>▶</span></div>
	</div>
	<div class="info">
		{#if yearLine}<span>{yearLine}</span>{/if}
		{#if rating}<span class="star" title="Nota de TMDB">★ {rating}</span>{/if}
		{#if dots.length > 0}
			<span class="dots">
				{#each dots as h, i (i)}<HealthDot health={h} size={6} />{/each}
			</span>
		{/if}
	</div>
</a>

<style>
	.card {
		position: relative;
		display: block;
		border-radius: 12px;
		overflow: hidden;
		background: var(--surface);
		border: 1px solid var(--line);
		transition: 0.16s;
		cursor: pointer;
		text-decoration: none;
		color: inherit;
	}
	.card:hover,
	.card:focus-visible {
		transform: translateY(-4px);
		border-color: #4d3a22;
		box-shadow: 0 14px 34px rgba(0, 0, 0, 0.5);
	}
	.thumb {
		aspect-ratio: 16 / 9;
		position: relative;
		display: flex;
		align-items: flex-end;
		padding: 13px 14px;
	}
	.thumb img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	/* Póster recortado como respaldo: se enseña la franja alta, donde suele
	   estar lo reconocible. */
	.poster-fallback img {
		object-position: center 22%;
	}
	.thumb::after {
		content: '';
		position: absolute;
		inset: 0;
		background: linear-gradient(180deg, transparent 35%, rgba(10, 7, 4, 0.88));
		pointer-events: none;
	}
	.title {
		position: relative;
		z-index: 1;
		font-family: var(--display);
		font-weight: 800;
		font-size: 18px;
		line-height: 1.08;
		text-shadow: 0 2px 12px rgba(0, 0, 0, 0.5);
		overflow: hidden;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}
	.play {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		z-index: 2;
		opacity: 0;
		transition: 0.18s;
	}
	.card:hover .play,
	.card:focus-visible .play {
		opacity: 1;
	}
	.play span {
		width: 48px;
		height: 48px;
		border-radius: 50%;
		background: rgba(16, 12, 9, 0.6);
		border: 1px solid rgba(244, 237, 227, 0.4);
		display: grid;
		place-items: center;
		font-size: 18px;
		color: #fff;
		backdrop-filter: blur(4px);
	}
	.info {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 9px 13px 11px;
		font-size: 12px;
		color: var(--muted);
		min-height: 36px;
	}
	.info .star {
		color: var(--accent);
		font-weight: 600;
	}
	.dots {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.badge {
		position: absolute;
		top: 10px;
		left: 10px;
		z-index: 3;
		font-family: var(--mono);
		font-size: 10px;
		letter-spacing: 0.05em;
		background: rgba(16, 12, 9, 0.6);
		border-radius: 6px;
		padding: 3px 7px;
		color: #e8d9c5;
		backdrop-filter: blur(4px);
	}
	/* Degradados de respaldo sin imagen (mockup: p1..p8). */
	.p1 {
		background: linear-gradient(135deg, #59331a, #241206 65%, #0e0804);
	}
	.p2 {
		background: linear-gradient(135deg, #173a33, #0a1a17 65%, #060d0b);
	}
	.p3 {
		background: linear-gradient(135deg, #3a1f3f, #170b1c 65%, #0a0510);
	}
	.p4 {
		background: linear-gradient(135deg, #1d3050, #0c1424 65%, #05080f);
	}
	.p5 {
		background: linear-gradient(135deg, #5c1f1f, #230a0a 65%, #0f0404);
	}
	.p6 {
		background: linear-gradient(135deg, #4d4416, #1e1a06 65%, #0d0b03);
	}
	.p7 {
		background: linear-gradient(135deg, #20423b, #0d1d1a 65%, #060f0d);
	}
	.p8 {
		background: linear-gradient(135deg, #513a5e, #1f1426 65%, #0e0813);
	}
	@media (hover: none) {
		.play {
			display: none;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.card,
		.play {
			transition: none;
		}
	}
</style>
