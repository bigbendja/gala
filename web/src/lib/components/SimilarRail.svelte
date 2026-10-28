<script lang="ts">
	// "Más como esto": títulos que TMDB recomienda. Los que ya tienes enlazan a
	// su ficha; los que no, se enseñan apagados y con su motivo (no hay forma
	// de desearlos sin tenerlos todavía: eso es M8).
	import { imgUrl } from '$lib/api/client';
	import type { SimilarTitle } from '$lib/api/types';

	interface Props {
		items: SimilarTitle[];
	}
	let { items }: Props = $props();

	// Los que ya tienes, primero (orden estable dentro de cada grupo).
	const ordered = $derived([...items.filter((s) => s.item_id), ...items.filter((s) => !s.item_id)]);

	let failed = $state(new Set<number>());
	function markFailed(id: number) {
		failed = new Set(failed).add(id);
	}
</script>

<ul class="similar" aria-label="Títulos similares">
	{#each ordered as s (s.tmdb_id)}
		{@const src = s.poster && !failed.has(s.tmdb_id) ? imgUrl(s.poster) : null}
		<li>
			<svelte:element
				this={s.item_id ? 'a' : 'div'}
				class="scard"
				class:out={!s.item_id}
				href={s.item_id ? `/item/${s.item_id}` : undefined}
				title={s.item_id ? undefined : 'Este título no está en tu galería'}
			>
				<div class="sp">
					{#if src}
						<img {src} alt="" loading="lazy" onerror={() => markFailed(s.tmdb_id)} />
					{:else}
						<b>{s.title}</b>
					{/if}
					{#if !s.item_id}<span class="tag">Fuera de tu galería</span>{/if}
				</div>
				<div class="sinfo">
					<span class="t">{s.title}</span>
					<span class="y">{s.year ?? ''}</span>
				</div>
			</svelte:element>
		</li>
	{/each}
</ul>

<style>
	.similar {
		list-style: none;
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: 150px;
		gap: 14px;
		overflow-x: auto;
		padding: 4px 4px 12px;
		scrollbar-width: thin;
		scrollbar-color: var(--line) transparent;
	}
	.scard {
		display: block;
		border-radius: 10px;
		overflow: hidden;
		border: 1px solid var(--line);
		background: var(--surface);
		text-decoration: none;
		color: inherit;
		transition: 0.15s;
	}
	a.scard:hover {
		transform: translateY(-3px);
		border-color: #4d3a22;
		box-shadow: 0 14px 30px rgba(0, 0, 0, 0.5);
	}
	.scard.out {
		opacity: 0.62;
		border-style: dashed;
	}
	.sp {
		position: relative;
		aspect-ratio: 2 / 3;
		display: flex;
		align-items: flex-end;
		padding: 11px;
		font-family: var(--display);
		font-weight: 800;
		font-size: 15px;
		line-height: 1.1;
		background: linear-gradient(150deg, #3a2412, #170d05 70%);
	}
	.sp img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.tag {
		position: absolute;
		left: 8px;
		right: 8px;
		bottom: 8px;
		z-index: 1;
		padding: 3px 7px;
		border-radius: 6px;
		background: rgba(16, 12, 9, 0.78);
		color: var(--muted);
		font-family: var(--body);
		font-weight: 500;
		font-size: 10.5px;
		line-height: 1.2;
		text-align: center;
	}
	.sinfo {
		display: flex;
		flex-direction: column;
		gap: 1px;
		padding: 8px 10px 10px;
		font-size: 12.5px;
		min-height: 52px;
	}
	.t {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.y {
		color: var(--muted);
		font-size: 11.5px;
	}
</style>
