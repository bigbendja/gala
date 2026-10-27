<script lang="ts">
	import CatalogGrid from '$lib/components/CatalogGrid.svelte';
	import { listItems } from '$lib/api/client';
</script>

<svelte:head>
	<title>GALA — Deseos</title>
</svelte:head>

<CatalogGrid
	title="Deseos"
	variant="wish"
	fetchPage={(offset) => listItems({ wished: true, limit: 60, offset })}
	emptyTitle="Tu lista de deseos está vacía"
	emptyText="Marca los títulos que quieras ver con «+ Mi lista» desde su ficha."
	emptyAddButton={false}
	errorTitle="No se pudo cargar tu lista de deseos"
/>
