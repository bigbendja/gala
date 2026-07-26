<script lang="ts">
	import Player from '$lib/components/Player.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<svelte:head>
	<title>{data.item ? `Reproduciendo ${data.item.title} — GALA` : 'GALA'}</title>
</svelte:head>

{#if data.loadError}
	<div class="state-page">
		<p>No se pudo cargar el reproductor.</p>
		<p class="muted">{data.loadError}</p>
	</div>
{:else if data.item && data.play}
	<Player item={data.item} initialPlay={data.play} initialProgress={data.progress} />
{/if}

<style>
	.state-page {
		max-width: 560px;
		margin: 15vh auto;
		text-align: center;
		padding: 0 28px;
	}
	.muted {
		color: var(--muted);
		font-size: 14px;
	}
</style>
