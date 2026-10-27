<script lang="ts">
	import { candidatePosterUrl } from '$lib/api/client';
	import type { Candidate } from '$lib/api/types';

	interface Props {
		candidate: Candidate;
		onChoose: (candidate: Candidate) => void;
		disabled?: boolean;
	}
	let { candidate, onChoose, disabled = false }: Props = $props();

	const poster = $derived(candidatePosterUrl(candidate.poster_path));
	// Si la carátula no carga (sin conexión con el servidor de imágenes de
	// TMDB, que es online), se cae al respaldo sobrio: nunca un hueco.
	let imgFailed = $state(false);
	const scorePct = $derived(Math.round(candidate.score * 100));
</script>

<button class="candidate" {disabled} onclick={() => onChoose(candidate)}>
	<div class="poster">
		{#if poster && !imgFailed}
			<img src={poster} alt="" loading="lazy" onerror={() => (imgFailed = true)} />
		{:else}
			<span class="no-poster">{candidate.title.slice(0, 1)}</span>
		{/if}
	</div>
	<div class="info">
		<div class="t">{candidate.title}</div>
		<div class="m">
			{#if candidate.year}<span>{candidate.year}</span>{/if}
			<span class="kind">{candidate.kind === 'tv' ? 'Serie' : 'Película'}</span>
			<span class="score">{scorePct}%</span>
		</div>
	</div>
</button>

<style>
	.candidate {
		display: flex;
		flex-direction: column;
		width: 130px;
		border-radius: 10px;
		overflow: hidden;
		background: var(--surface);
		border: 1px solid var(--line);
		transition: 0.15s;
		text-align: left;
	}
	.candidate:hover:not(:disabled) {
		border-color: #4d3a22;
		transform: translateY(-2px);
	}
	.candidate:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.poster {
		aspect-ratio: 2 / 3;
		background: linear-gradient(160deg, #4a3016 0%, #1e1207 55%, #0d0a06 100%);
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.poster img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.no-poster {
		font-family: var(--display);
		font-weight: 800;
		font-size: 28px;
		color: var(--muted);
	}
	.info {
		padding: 8px 10px;
	}
	.t {
		font-size: 12.5px;
		font-weight: 600;
		line-height: 1.3;
		min-height: 2.6em;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.m {
		display: flex;
		gap: 6px;
		align-items: center;
		font-size: 11px;
		color: var(--muted);
		margin-top: 3px;
	}
	.score {
		margin-left: auto;
		font-weight: 600;
		color: var(--muted);
	}
</style>
