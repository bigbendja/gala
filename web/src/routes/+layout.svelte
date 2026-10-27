<script lang="ts">
	import '$lib/styles/global.css';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import favicon from '$lib/assets/favicon.svg';
	import { PIN_REQUIRED_EVENT } from '$lib/api/client';
	import Topbar from '$lib/components/Topbar.svelte';
	import Footer from '$lib/components/Footer.svelte';
	import IngestModal from '$lib/components/IngestModal.svelte';
	import LanPanel from '$lib/components/LanPanel.svelte';
	import PinGate from '$lib/components/PinGate.svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	let ingestOpen = $state(false);
	let lanOpen = $state(false);
	// La píldora de la topbar refleja el estado real; el panel lo actualiza.
	let lanEnabledOverride = $state<boolean | null>(null);
	const lanEnabled = $derived(lanEnabledOverride ?? data.lanEnabled);

	// Modo LAN: este navegador es el del PC, o un dispositivo de la red.
	const isLocal = $derived(data.session.local);
	// Sesión de PIN que caducó con la app abierta (cambiaron el PIN, etc.).
	let relocked = $state(false);
	const locked = $derived(
		relocked || (!data.session.local && data.session.pin_required && !data.session.authenticated)
	);
	// Ajustes y "Por confirmar" son del dueño: desde otro dispositivo no hay
	// nada que hacer ahí (el backend además lo rechaza).
	const ownerOnlyRoute = $derived(
		!isLocal && (page.url.pathname.startsWith('/ajustes') || page.url.pathname.startsWith('/pendientes'))
	);

	onMount(() => {
		const onPin = () => (relocked = true);
		window.addEventListener(PIN_REQUIRED_EVENT, onPin);
		return () => window.removeEventListener(PIN_REQUIRED_EVENT, onPin);
	});

	async function unlocked() {
		relocked = false;
		await invalidateAll();
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>GALA — Tu cine, tus fuentes</title>
</svelte:head>

{#if locked}
	<PinGate onUnlocked={unlocked} />
{:else}
	<Topbar
		onOpenIngest={() => (ingestOpen = true)}
		pendingCount={data.pendingCount}
		local={isLocal}
		{lanEnabled}
		onOpenLan={() => (lanOpen = true)}
	/>
	{#if ownerOnlyRoute}
		<main class="owner-only">
			<h1>Esto se maneja desde el PC</h1>
			<p>
				Los ajustes y la gestión de la galería solo están disponibles en el ordenador donde corre GALA.
				Desde aquí puedes ver tu galería y reproducir.
			</p>
			<a href="/">Volver a la galería</a>
		</main>
	{:else}
		{@render children()}
	{/if}
	<Footer />

	{#if isLocal}
		<IngestModal
			open={ingestOpen}
			onClose={() => (ingestOpen = false)}
			onIngested={() => invalidateAll()}
		/>
		<LanPanel
			open={lanOpen}
			onClose={() => (lanOpen = false)}
			onChanged={(enabled) => (lanEnabledOverride = enabled)}
		/>
	{/if}
{/if}

<style>
	.owner-only {
		max-width: 520px;
		margin: 14vh auto 0;
		padding: 0 20px;
		text-align: center;
		display: flex;
		flex-direction: column;
		gap: 12px;
		align-items: center;
	}
	.owner-only h1 {
		font-family: var(--display);
		font-size: 26px;
		font-weight: 800;
	}
	.owner-only p {
		color: var(--muted);
	}
	.owner-only a {
		color: var(--accent);
	}
</style>
