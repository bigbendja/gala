<script lang="ts">
	// Reparto de la ficha (mockup de la Ficha): fotos redondas en una fila con
	// scroll horizontal. Si una foto no está (TMDB no la tiene o aún no se ha
	// descargado), iniciales sobre un color estable por nombre: nunca un hueco
	// roto.
	import { imgUrl } from '$lib/api/client';
	import type { CastMember } from '$lib/api/types';
	import { initials, nameHue } from '$lib/format';

	interface Props {
		cast: CastMember[];
	}
	let { cast }: Props = $props();

	let failed = $state(new Set<number>());
	function markFailed(id: number) {
		failed = new Set(failed).add(id);
	}
</script>

<ul class="cast" aria-label="Reparto">
	{#each cast as person (person.id)}
		{@const src = person.photo && !failed.has(person.id) ? imgUrl(person.photo) : null}
		<li class="actor">
			<div
				class="ph"
				style:background={src
					? undefined
					: `linear-gradient(145deg, hsl(${nameHue(person.name)} 42% 62%), hsl(${nameHue(person.name)} 38% 30%))`}
			>
				{#if src}
					<img {src} alt="" loading="lazy" onerror={() => markFailed(person.id)} />
				{:else}
					<span aria-hidden="true">{initials(person.name)}</span>
				{/if}
			</div>
			<div class="nm">{person.name}</div>
			{#if person.character}<div class="ch">{person.character}</div>{/if}
		</li>
	{/each}
</ul>

<style>
	.cast {
		list-style: none;
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: 124px;
		gap: 16px;
		overflow-x: auto;
		padding: 4px 4px 12px;
		scrollbar-width: thin;
		scrollbar-color: var(--line) transparent;
		scroll-snap-type: x proximity;
	}
	.actor {
		text-align: center;
		scroll-snap-align: start;
	}
	.ph {
		width: 96px;
		height: 96px;
		border-radius: 50%;
		margin: 0 auto 10px;
		display: grid;
		place-items: center;
		overflow: hidden;
		font-family: var(--display);
		font-weight: 800;
		font-size: 30px;
		color: #1a1207;
		border: 2px solid var(--line);
		background: var(--surface-2);
		box-shadow: 0 8px 22px rgba(0, 0, 0, 0.45);
	}
	.ph img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.nm {
		font-weight: 600;
		font-size: 13px;
		line-height: 1.25;
	}
	.ch {
		color: var(--muted);
		font-size: 12px;
		margin-top: 2px;
		line-height: 1.25;
	}
</style>
