<script lang="ts">
	import { imgUrl } from '$lib/api/client';
	import type { Item, Volume } from '$lib/api/types';
	import { addedEyebrow } from '$lib/format';
	import SourceStack from './SourceStack.svelte';

	interface Props {
		item: Item;
		volumes: Volume[];
	}
	let { item, volumes }: Props = $props();

	// Etiqueta real del disco de origen, si la fuente principal es localfs y
	// el volumen tiene Label conocido (Brief M10a: "no inventar" — sin
	// volumen/label real, se omite esa parte del eyebrow).
	const diskLabel = $derived.by(() => {
		const src = (item.sources ?? []).find((s) => s.provider === 'localfs' && s.volume_id);
		if (!src) return null;
		const vol = volumes.find((v) => v.id === src.volume_id);
		return vol?.label ?? null;
	});

	const eyebrow = $derived(
		diskLabel ? `${addedEyebrow(item.added_at)} · desde tu disco «${diskLabel}»` : addedEyebrow(item.added_at)
	);

	const backdrop = $derived(imgUrl(item.backdrop_path));
	const poster = $derived(imgUrl(item.poster_path));
</script>

<section class="hero">
	{#if backdrop}
		<div class="hero-bg" style:background-image="url({backdrop})"></div>
	{/if}
	<div class="hero-inner">
		<div>
			<div class="eyebrow">{eyebrow}</div>
			<h1>{item.title}</h1>
			<div class="hero-meta">
				{#if item.rating_cert}<span class="cert">{item.rating_cert}+</span>{/if}
				{#if item.year}<span>{item.year}</span>{/if}
				{#if item.genres.length}<span>{item.genres.join(' · ')}</span>{/if}
			</div>
			{#if item.overview}<p>{item.overview}</p>{/if}
			<div class="hero-actions">
				<a class="btn-play" href="/item/{item.id}">▶ Reproducir</a>
				<button class="btn-ghost" disabled title="Llega en M8">＋ Mi lista</button>
				<a class="btn-ghost" href="/item/{item.id}">Detalles</a>
			</div>

			{#if item.sources && item.sources.length > 0}
				<SourceStack sources={item.sources} />
			{/if}
		</div>

		<a class="hero-poster" href="/item/{item.id}">
			{#if poster}
				<img src={poster} alt="" />
			{:else}
				<div class="art"><b>{item.title}</b></div>
			{/if}
		</a>
	</div>
</section>

<style>
	.hero {
		position: relative;
		padding: 64px 28px 40px;
		overflow: hidden;
		isolation: isolate;
	}
	.hero::before {
		content: '';
		position: absolute;
		inset: 0;
		z-index: -2;
		background: radial-gradient(120% 90% at 78% 8%, #3d2a14 0%, #221709 42%, var(--bg) 78%);
	}
	.hero-bg {
		position: absolute;
		inset: 0;
		z-index: -3;
		background-size: cover;
		background-position: center 20%;
		opacity: 0.4;
		-webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.7), transparent 85%);
		mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.7), transparent 85%);
	}
	/* haz de proyector: la firma visual del mockup */
	.hero::after {
		content: '';
		position: absolute;
		top: -20%;
		right: 6%;
		width: 60%;
		height: 150%;
		z-index: -1;
		pointer-events: none;
		background: conic-gradient(
			from 208deg at 100% 0%,
			transparent 0deg,
			rgba(242, 169, 59, 0.1) 8deg,
			rgba(242, 169, 59, 0.03) 22deg,
			transparent 34deg
		);
		animation: beam 9s ease-in-out infinite alternate;
	}
	@keyframes beam {
		from {
			opacity: 0.7;
		}
		to {
			opacity: 1;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.hero::after {
			animation: none;
		}
	}
	.hero-inner {
		max-width: 1180px;
		margin: 0 auto;
		display: grid;
		grid-template-columns: 1fr 300px;
		gap: 48px;
		align-items: center;
	}
	.eyebrow {
		font-family: var(--mono);
		font-size: 12px;
		letter-spacing: 0.14em;
		color: var(--accent);
		text-transform: uppercase;
		margin-bottom: 14px;
	}
	.hero h1 {
		font-family: var(--display);
		font-size: clamp(34px, 4.6vw, 58px);
		font-weight: 800;
		line-height: 1.02;
		letter-spacing: -0.01em;
		margin-bottom: 16px;
	}
	.hero-meta {
		display: flex;
		gap: 14px;
		align-items: center;
		color: var(--muted);
		font-size: 14px;
		margin-bottom: 14px;
		flex-wrap: wrap;
	}
	.hero-meta .cert {
		border: 1px solid var(--line);
		border-radius: 5px;
		padding: 1px 7px;
		font-size: 12px;
	}
	.hero p {
		color: var(--muted);
		max-width: 560px;
		margin-bottom: 22px;
	}
	.hero-actions {
		display: flex;
		gap: 12px;
		align-items: center;
		flex-wrap: wrap;
	}
	.btn-play {
		display: flex;
		align-items: center;
		gap: 10px;
		background: var(--text);
		color: #17110a;
		font-weight: 600;
		font-size: 15px;
		padding: 13px 26px;
		border-radius: 11px;
		transition: 0.15s;
		text-decoration: none;
	}
	.btn-play:hover {
		transform: translateY(-1px);
		box-shadow: 0 8px 24px rgba(244, 237, 227, 0.15);
	}
	.btn-ghost {
		padding: 13px 20px;
		border-radius: 11px;
		border: 1px solid var(--line);
		color: var(--text);
		background: rgba(255, 255, 255, 0.03);
		font-weight: 500;
		transition: 0.15s;
		text-decoration: none;
	}
	.btn-ghost:hover {
		background: var(--surface);
	}
	.btn-ghost:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.hero-poster {
		display: block;
		aspect-ratio: 2 / 3;
		border-radius: 16px;
		overflow: hidden;
		position: relative;
		box-shadow: 0 24px 60px rgba(0, 0, 0, 0.55), 0 0 0 1px var(--line);
		background: linear-gradient(160deg, #4a3016 0%, #1e1207 55%, #0d0a06 100%);
	}
	.hero-poster img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.hero-poster .art {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: flex-end;
		padding: 22px;
		font-family: var(--display);
		font-weight: 800;
		font-size: 30px;
		line-height: 1.05;
		background: radial-gradient(90% 60% at 30% 20%, rgba(242, 169, 59, 0.35), transparent 60%);
	}

	@media (max-width: 900px) {
		.hero-inner {
			grid-template-columns: 1fr;
			gap: 28px;
		}
		.hero-poster {
			display: none;
		}
		.hero {
			padding-left: 16px;
			padding-right: 16px;
		}
	}
</style>
