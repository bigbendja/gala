<script lang="ts">
	import BrowseView from '$lib/components/BrowseView.svelte';
</script>

<svelte:head>
	<title>GALA — Series</title>
</svelte:head>

<BrowseView
	kind="series"
	title="Series"
	emptyTitle="Aún no hay series"
	emptyText="Añade una carpeta o pega enlaces: GALA agrupa los episodios por temporada."
	errorTitle="No se pudieron cargar las series"
/>
