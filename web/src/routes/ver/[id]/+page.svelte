<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import Player from '$lib/components/Player.svelte';
	import StateBlock from '$lib/components/StateBlock.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<svelte:head>
	<title>{data.item ? `Reproduciendo ${data.item.title} — GALA` : 'GALA'}</title>
</svelte:head>

{#if data.loadError}
	<StateBlock
		tone="error"
		title="No se pudo cargar el reproductor"
		text={data.loadError}
		actionLabel="Reintentar"
		onAction={() => invalidateAll()}
	/>
{:else if data.item && data.play}
	<Player item={data.item} initialPlay={data.play} initialProgress={data.progress} />
{/if}
