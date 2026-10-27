<script lang="ts">
	import CatalogGrid from '$lib/components/CatalogGrid.svelte';
	import { listItems } from '$lib/api/client';
</script>

<svelte:head>
	<title>GALA — Series</title>
</svelte:head>

<CatalogGrid
	title="Series"
	fetchPage={(offset) => listItems({ kind: 'series', parent_id: 'null', limit: 60, offset })}
	emptyTitle="Aún no hay series"
	emptyText="Añade una carpeta o pega enlaces: GALA agrupa los episodios por temporada."
	errorTitle="No se pudieron cargar las series"
/>
