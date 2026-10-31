<script lang="ts">
	import '$lib/styles/global.css';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import favicon from '$lib/assets/favicon.svg';
	import { ADMIN_LOGIN_EVENT, PIN_REQUIRED_EVENT } from '$lib/api/client';
	import AdminLogin from '$lib/components/AdminLogin.svelte';
	import Topbar from '$lib/components/Topbar.svelte';
	import Footer from '$lib/components/Footer.svelte';
	import IngestModal from '$lib/components/IngestModal.svelte';
	import LanPanel from '$lib/components/LanPanel.svelte';
	import NavProgress from '$lib/components/NavProgress.svelte';
	import PinGate from '$lib/components/PinGate.svelte';
	import StateBlock from '$lib/components/StateBlock.svelte';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	let ingestOpen = $state(false);
	let lanOpen = $state(false);

	// Recuento de "Por confirmar" y estado del Modo LAN: llegan como promesa
	// (el layout no espera por ellos para pintar la estructura). Hasta que
	// llegan valen 0/false: la píldora simplemente aparece después.
	let pendingCount = $state(0);
	let lanFromServer = $state(false);
	// La píldora de la topbar refleja el estado real; el panel lo actualiza.
	let lanEnabledOverride = $state<boolean | null>(null);
	const lanEnabled = $derived(lanEnabledOverride ?? lanFromServer);

	$effect(() => {
		const promise = data.chrome;
		let alive = true;
		promise.then((c) => {
			if (!alive) return;
			pendingCount = c.pendingCount;
			lanFromServer = c.lanEnabled;
			lanEnabledOverride = null;
		});
		return () => {
			alive = false;
		};
	});

	// Modo LAN: este navegador es el del PC, o un dispositivo de la red.
	const isLocal = $derived(data.session.local);
	// Sesión de PIN que caducó con la app abierta (cambiaron el PIN, etc.).
	let relocked = $state(false);
	const locked = $derived(
		relocked || (!data.session.local && data.session.pin_required && !data.session.authenticated)
	);
	// Administrar (Ajustes, "Por confirmar", Listas) es del PC, o de un
	// dispositivo de la red que inició sesión de administrador (M-Servidor).
	// Sin sesión, esas rutas enseñan el acceso de administrador.
	const isAdmin = $derived(isLocal || !!data.auth?.authenticated);
	const adminRoute = $derived(
		['/ajustes', '/pendientes', '/listas'].some((p) => page.url.pathname.startsWith(p))
	);
	const needsAdminLogin = $derived(!isAdmin && adminRoute);

	onMount(() => {
		// Retira el cascarón de arranque (app.html): ya hay app de verdad.
		document.getElementById('gala-shell')?.remove();
		const onPin = () => (relocked = true);
		const onOpenIngest = () => (ingestOpen = true);
		// La sesión de administrador caducó o se cerró con la app abierta.
		// Solo si creíamos tener sesión: si ya se sabe que no, no hay nada que recargar.
		const onAdminLogin = () => {
			if (isAdmin && !isLocal) void invalidateAll();
		};
		window.addEventListener(ADMIN_LOGIN_EVENT, onAdminLogin);
		window.addEventListener(PIN_REQUIRED_EVENT, onPin);
		window.addEventListener('gala:open-ingest', onOpenIngest);
		return () => {
			window.removeEventListener(ADMIN_LOGIN_EVENT, onAdminLogin);
			window.removeEventListener(PIN_REQUIRED_EVENT, onPin);
			window.removeEventListener('gala:open-ingest', onOpenIngest);
		};
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

<NavProgress />

{#if locked}
	<PinGate onUnlocked={unlocked} />
{:else}
	<div class="app">
		<Topbar
			onOpenIngest={() => (ingestOpen = true)}
			{pendingCount}
			local={isLocal}
			admin={isAdmin}
			{lanEnabled}
			onOpenLan={() => (lanOpen = true)}
		/>
		<div class="app-main">
			{#if needsAdminLogin}
				<AdminLogin status={data.auth} onDone={unlocked} />
			{:else}
				{@render children()}
			{/if}
		</div>
		<Footer />
	</div>

	{#if isAdmin}
		<IngestModal
			open={ingestOpen}
			onClose={() => (ingestOpen = false)}
			onIngested={() => invalidateAll()}
			onOpenSettings={() => {
				ingestOpen = false;
				goto('/ajustes#carpetas');
			}}
		/>
	{/if}
	{#if isLocal}
		<LanPanel
			open={lanOpen}
			onClose={() => (lanOpen = false)}
			onChanged={(enabled) => (lanEnabledOverride = enabled)}
		/>
	{/if}
{/if}
