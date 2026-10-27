<script lang="ts">
	import CatalogGrid from '$lib/components/CatalogGrid.svelte';
	import { listItems } from '$lib/api/client';
	import { playableMovies } from '$lib/home';
</script>

<svelte:head>
	<title>GALA — Películas</title>
</svelte:head>

<CatalogGrid
	title="Películas"
	fetchPage={(offset) => listItems({ kind: 'movie', parent_id: 'null', limit: 60, offset })}
	filter={playableMovies}
	emptyTitle="Aún no hay películas"
	emptyText="Añade una carpeta o pega enlaces: GALA reconoce las películas solas."
	errorTitle="No se pudieron cargar las películas"
/>
