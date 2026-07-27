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
			const page = await listItems({ kind: 'movie', confirmed: true, limit: 60, offset: items.length });
			items = [...items, ...page.items];
			total = page.total;
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>GALA — Películas</title>
</svelte:head>

<section class="section">
	<h1>Películas</h1>
	{#if data.error}
		<p class="muted">No se pudieron cargar las películas: {data.error}</p>
	{:else}
		<Grid {items} {total} {loading} emptyText="Todavía no tienes películas en tu galería." onLoadMore={loadMore} />
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
