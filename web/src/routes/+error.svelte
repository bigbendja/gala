<script lang="ts">
	import { page } from '$app/state';
	import StateBlock from '$lib/components/StateBlock.svelte';
	import { httpStatusText } from '$lib/format';

	const text = $derived(httpStatusText(page.status));
</script>

<svelte:head>
	<title>GALA — {text.title}</title>
</svelte:head>

<StateBlock title={text.title} text={text.hint} tone="error" actionLabel="Volver a la galería" actionHref="/" />
