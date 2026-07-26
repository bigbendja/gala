<script lang="ts">
	import { untrack } from 'svelte';
	import Hero from '$lib/components/Hero.svelte';
	import Row from '$lib/components/Row.svelte';
	import { listItems } from '$lib/api/client';
	import type { Item } from '$lib/api/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	function moviesAndSeries(items: Item[]): Item[] {
		// Brief M10a §5: "Tu galería" no muestra season/episode sueltos (esos
		// cuelgan de su serie en la ficha); tampoco kind=other (provisional,
		// aún no confirmado por M3).
		return items.filter((it) => it.kind === 'movie' || it.kind === 'series');
	}

	// Semilla intencional de estado LOCAL de paginación a partir del primer
	// load: a partir de aquí este estado vive independiente de `data` (crece
	// con loadMoreGallery), por eso se lee con untrack — no es un olvido de
	// $derived, es la semántica que queremos (Svelte, state_referenced_locally).
	let galleryItems = $state<Item[]>(untrack(() => moviesAndSeries(data.gallery?.items ?? [])));
	let galleryRawOffset = $state(untrack(() => data.gallery?.items.length ?? 0));
	let galleryRawTotal = $state(untrack(() => data.gallery?.total ?? 0));
	let galleryLoadingMore = $state(false);

	async function loadMoreGallery() {
		if (galleryLoadingMore || galleryRawOffset >= galleryRawTotal) return;
		galleryLoadingMore = true;
		try {
			const page = await listItems({ confirmed: true, limit: 200, offset: galleryRawOffset });
			galleryItems = [...galleryItems, ...moviesAndSeries(page.items)];
			galleryRawOffset += page.items.length;
			galleryRawTotal = page.total;
		} catch {
			// Se ignora: la fila se queda con lo que ya tenía; un nuevo scroll
			// vuelve a intentarlo. No hay más de un error mostrable por fila
			// sin complicar el estado; el error inicial ya se ve arriba si
			// falló la primera página (data.galleryError).
		} finally {
			galleryLoadingMore = false;
		}
	}

	const wishlistItems = $derived(data.wishlist?.items ?? []);

	// "Continuar viendo" (Brief M10a-fix B3/C): alimentada por el filtro
	// in_progress, mira todo el catálogo, no solo la página ya cargada de
	// "Tu galería" (limitación de M10a, ya resuelta). Si viene vacía —o
	// falló la petición—, la fila se oculta, nunca se muestra vacía.
	const continuingItems = $derived(data.continuing?.items ?? []);

	const isEmptyCatalog = $derived(
		!data.galleryError && !data.wishlistError && galleryItems.length === 0 && wishlistItems.length === 0
	);
</script>

<svelte:head>
	<title>GALA — Tu galería</title>
</svelte:head>

{#if data.galleryError && data.wishlistError}
	<div class="state-page">
		<p>No se pudo conectar con GALA.</p>
		<p class="muted">Comprueba que el programa sigue en marcha y recarga la página.</p>
	</div>
{:else if isEmptyCatalog}
	<div class="state-page">
		<p>Aún no hay nada.</p>
		<p class="muted">Añade una carpeta o pega enlaces para empezar tu galería.</p>
	</div>
{:else}
	{#if data.hero}
		<Hero item={data.hero} volumes={data.volumes} />
	{/if}

	{#if continuingItems.length > 0}
		<Row
			title="Continuar viendo"
			subtitle="sigue donde lo dejaste, en cualquier dispositivo"
			items={continuingItems}
			variant="wide"
		/>
	{/if}

	{#if data.galleryError}
		<section class="section">
			<div class="sec-head"><h2>Tu galería</h2></div>
			<p class="muted">No se pudo cargar tu galería: {data.galleryError}</p>
		</section>
	{:else}
		<Row
			title="Tu galería"
			subtitle="{galleryItems.length} títulos"
			items={galleryItems}
			total={galleryRawTotal}
			loading={galleryLoadingMore}
			onNeedMore={loadMoreGallery}
		/>
	{/if}

	{#if data.wishlistError}
		<section class="section">
			<div class="sec-head"><h2>Deseos</h2></div>
			<p class="muted">No se pudo cargar tu lista de deseos: {data.wishlistError}</p>
		</section>
	{:else if wishlistItems.length > 0}
		<Row
			title="Deseos"
			subtitle="se completan solos cuando llega una fuente"
			items={wishlistItems}
			variant="wish"
		/>
	{/if}
{/if}

<style>
	.section {
		max-width: 1236px;
		margin: 0 auto;
		padding: 34px 28px 0;
	}
	.sec-head {
		margin-bottom: 16px;
	}
	.sec-head h2 {
		font-family: var(--display);
		font-size: 21px;
		font-weight: 600;
	}
	.state-page {
		max-width: 560px;
		margin: 15vh auto;
		text-align: center;
		padding: 0 28px;
	}
	.state-page p {
		font-size: 16px;
	}
	.muted {
		color: var(--muted);
		font-size: 14px;
		margin-top: 6px;
	}
</style>
