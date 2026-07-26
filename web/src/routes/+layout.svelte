<script lang="ts">
	import '$lib/styles/global.css';
	import type { Snippet } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import favicon from '$lib/assets/favicon.svg';
	import Topbar from '$lib/components/Topbar.svelte';
	import Footer from '$lib/components/Footer.svelte';
	import IngestModal from '$lib/components/IngestModal.svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	let ingestOpen = $state(false);
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>GALA — Tu cine, tus fuentes</title>
</svelte:head>

<Topbar onOpenIngest={() => (ingestOpen = true)} pendingCount={data.pendingCount} />
{@render children()}
<Footer />

<IngestModal
	open={ingestOpen}
	onClose={() => (ingestOpen = false)}
	onIngested={() => invalidateAll()}
/>
