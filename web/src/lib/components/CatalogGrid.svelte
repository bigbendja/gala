<script lang="ts">
	// Listado completo de una sección (Películas, Series, Deseos): carga sus
	// datos en el propio componente para que la página (con su cabecera) se
	// pinte al instante y el contenido entre con skeleton (Sistema de Diseño
	// GALA §2.3); vacío = invitación a actuar (§2.2); error = qué pasó y
	// cómo reintentar (§2.1).
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import type { Item, LanSession, ListResult } from '$lib/api/types';
	import { errorMessage } from '$lib/format';
	import Grid from './Grid.svelte';
	import StateBlock from './StateBlock.svelte';

	interface Props {
		title: string;
		fetchPage: (offset: number) => Promise<ListResult>;
		filter?: (items: Item[]) => Item[];
		variant?: 'default' | 'wish';
		emptyTitle: string;
		emptyText: string;
		/** Mostrar el botón "Añadir" en el vacío (solo tiene sentido en el PC). */
		emptyAddButton?: boolean;
		errorTitle: string;
	}
	let {
		title,
		fetchPage,
		filter = (items) => items,
		variant = 'default',
		emptyTitle,
		emptyText,
		emptyAddButton = true,
		errorTitle
	}: Props = $props();

	const isLocal = $derived((page.data as { session?: LanSession }).session?.local !== false);

	let items = $state<Item[]>([]);
	let rawOffset = $state(0);
	let rawTotal = $state(0);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state<string | null>(null);

	async function loadFirst() {
		loading = true;
		error = null;
		try {
			const res = await fetchPage(0);
			items = filter(res.items);
			rawOffset = res.items.length;
			rawTotal = res.total;
		} catch (err) {
			error = errorMessage(err, errorTitle);
		} finally {
			loading = false;
		}
	}
	onMount(loadFirst);

	async function loadMore() {
		if (loadingMore || rawOffset >= rawTotal) return;
		loadingMore = true;
		try {
			const res = await fetchPage(rawOffset);
			items = [...items, ...filter(res.items)];
			rawOffset += res.items.length;
			rawTotal = res.total;
		} catch (err) {
			error = errorMessage(err, errorTitle);
		} finally {
			loadingMore = false;
		}
	}

	function openIngest() {
		window.dispatchEvent(new CustomEvent('gala:open-ingest'));
	}
</script>

<section class="section">
	<div class="head">
		<h1>{title}</h1>
		{#if !loading && !error && items.length > 0}
			<span class="count">{rawTotal} {rawTotal === 1 ? 'título' : 'títulos'}</span>
		{/if}
	</div>

	{#if error && items.length === 0}
		<StateBlock compact tone="error" title="No se pudo cargar" text={error} actionLabel="Reintentar" onAction={loadFirst} />
	{:else if !loading && items.length === 0}
		<StateBlock
			compact
			title={emptyTitle}
			text={emptyText}
			actionLabel={emptyAddButton && isLocal ? '＋ Añadir' : undefined}
			onAction={openIngest}
		/>
	{:else}
		<Grid {items} total={rawTotal} loading={loading || loadingMore} {variant} onLoadMore={loadMore} />
		{#if error}
			<p class="inline-error" role="alert">{error}</p>
		{/if}
	{/if}
</section>

<style>
	.section {
		max-width: 1236px;
		margin: 0 auto;
		padding: 34px 28px 60px;
	}
	.head {
		display: flex;
		align-items: baseline;
		gap: 14px;
		margin-bottom: 22px;
	}
	h1 {
		font-family: var(--display);
		font-size: 26px;
		font-weight: 700;
	}
	.count {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--muted);
	}
	.inline-error {
		margin-top: 14px;
		color: var(--bad);
		font-size: 13.5px;
	}
	@media (max-width: 900px) {
		.section {
			padding-left: 16px;
			padding-right: 16px;
		}
	}
</style>
