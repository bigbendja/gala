<script lang="ts">
	import { untrack } from 'svelte';
	import Grid from '$lib/components/Grid.svelte';
	import { listItems } from '$lib/api/client';
	import type { Item } from '$lib/api/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let items = $state<Item[]>(untrack(() => data.items));
	let total = $state(untrack(() => data.total));
	let loading = $state(false);

	async function loadMore() {
		if (loading || items.length >= total) return;
		loading = true;
		try {
			const page = await listItems({ wished: true, limit: 60, offset: items.length });
			items = [...items, ...page.items];
			total = page.total;
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>GALA — Deseos</title>
</svelte:head>

<section class="section">
	<h1>Deseos</h1>
	{#if data.error}
		<p class="muted">No se pudo cargar tu lista de deseos: {data.error}</p>
	{:else}
		<Grid
			{items}
			{total}
			{loading}
			variant="wish"
			emptyText="Nada en tu lista de deseos todavía. Márcalo con “+ Mi lista” desde una ficha."
			onLoadMore={loadMore}
		/>
	{/if}
</section>

<style>
	.section {
		max-width: 1236px;
		margin: 0 auto;
		padding: 34px 28px 60px;
	}
	h1 {
		font-family: var(--display);
		font-size: 26px;
		font-weight: 700;
		margin-bottom: 22px;
	}
	.muted {
		color: var(--muted);
	}
</style>
