<script lang="ts">
	import BrowseView from '$lib/components/BrowseView.svelte';
</script>

<svelte:head>
	<title>GALA — Películas</title>
</svelte:head>

<BrowseView
	kind="movie"
	title="Películas"
	emptyTitle="Aún no hay películas"
	emptyText="Añade una carpeta o pega enlaces: GALA reconoce las películas solas."
	errorTitle="No se pudieron cargar las películas"
/>
