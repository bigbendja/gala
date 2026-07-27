<script lang="ts">
	import Card from './Card.svelte';
	import type { Item } from '$lib/api/types';

	interface Props {
		items: Item[];
		total: number;
		loading?: boolean;
		emptyText?: string;
		variant?: 'default' | 'wish';
		onLoadMore?: () => void;
	}
	let { items, total, loading = false, emptyText = 'Nada por aquí todavía.', variant = 'default', onLoadMore }: Props = $props();

	const hasMore = $derived(items.length < total);
</script>

{#if loading && items.length === 0}
	<div class="grid">
		{#each { length: 12 } as _, i (i)}
			<div class="skeleton"></div>
		{/each}
	</div>
{:else if items.length === 0}
	<p class="empty">{emptyText}</p>
{:else}
	<div class="grid">
		{#each items as item (item.id)}
			<Card {item} {variant} />
		{/each}
	</div>
	{#if hasMore}
		<div class="more">
			<button class="btn-load" onclick={onLoadMore} disabled={loading}>
				{loading ? 'Cargando…' : 'Cargar más'}
			</button>
		</div>
	{/if}
{/if}

<style>
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: 16px;
	}
	.skeleton {
		aspect-ratio: 2 / 3;
		border-radius: 12px;
		background: linear-gradient(100deg, var(--surface) 40%, var(--surface-2) 50%, var(--surface) 60%);
		background-size: 200% 100%;
		animation: shimmer 1.4s ease-in-out infinite;
	}
	@media (prefers-reduced-motion: reduce) {
		.skeleton {
			animation: none;
		}
	}
	@keyframes shimmer {
		from {
			background-position: 200% 0;
		}
		to {
			background-position: -200% 0;
		}
	}
	.empty {
		color: var(--muted);
		padding: 40px 0;
		text-align: center;
	}
	.more {
		display: flex;
		justify-content: center;
		padding: 26px 0 8px;
	}
	.btn-load {
		background: var(--surface);
		border: 1px solid var(--line);
		color: var(--text);
		padding: 10px 22px;
		border-radius: 10px;
		font-weight: 500;
		font-size: 14px;
		transition: 0.15s;
	}
	.btn-load:hover:not(:disabled) {
		background: var(--surface-2);
	}
	.btn-load:disabled {
		opacity: 0.6;
	}

	@media (max-width: 480px) {
		.grid {
			grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
			gap: 10px;
		}
	}
</style>
