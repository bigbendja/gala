<script lang="ts">
	import { page } from '$app/state';
	import SearchBox from './SearchBox.svelte';

	interface Props {
		onOpenIngest: () => void;
		/** Items "Por confirmar" reales (Brief M10b §3); 0 = la píldora no se
		 *  muestra — nada que confirmar no es un estado que anunciar. */
		pendingCount?: number;
		/** true = este navegador es el del PC que ejecuta GALA. Un dispositivo de
		 *  la red (Modo LAN) solo ve galería y reproducción: sin Añadir, Ajustes,
		 *  Por confirmar ni el panel de Modo LAN (Brief M7 §5). */
		local?: boolean;
		/** Modo LAN activo (la píldora lo refleja). */
		lanEnabled?: boolean;
		onOpenLan?: () => void;
	}
	let {
		onOpenIngest,
		pendingCount = 0,
		local = true,
		lanEnabled = false,
		onOpenLan = () => {}
	}: Props = $props();

	let mobileNavOpen = $state(false);

	const navLinks = [
		{ href: '/', label: 'Inicio' },
		{ href: '/peliculas', label: 'Películas' },
		{ href: '/series', label: 'Series' },
		{ href: '/listas', label: 'Listas' },
		{ href: '/deseos', label: 'Deseos' }
	];

	function isActive(href: string): boolean {
		return href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
	}

	function closeMobileNav() {
		mobileNavOpen = false;
	}
</script>

<header class="topbar">
	<a class="brand" href="/">
		<svg width="26" height="26" viewBox="0 0 26 26" fill="none" aria-hidden="true">
			<circle cx="13" cy="13" r="11.5" stroke="#F2A93B" stroke-width="2.4" />
			<path d="M10.5 8.5L18 13l-7.5 4.5v-9z" fill="#F2A93B" />
		</svg>
		GALA
	</a>

	<nav class="nav" aria-label="Principal">
		{#each navLinks as link (link.href)}
			<a href={link.href} class:active={isActive(link.href)}>{link.label}</a>
		{/each}
	</nav>

	<div class="spacer"></div>

	{#if local && pendingCount > 0}
		<a class="pending-pill" href="/pendientes">{pendingCount} por confirmar</a>
	{/if}

	<div class="search-slot">
		<SearchBox />
	</div>

	{#if local}
		<a class="settings-link" href="/ajustes" title="Ajustes" aria-label="Ajustes">⚙️</a>

		<button
			class="lan-pill"
			class:off={!lanEnabled}
			onclick={onOpenLan}
			title={lanEnabled ? 'Modo LAN activo: ver código QR' : 'Activar Modo LAN'}
		>
			<span class="lan-dot"></span>
			{lanEnabled ? 'Modo LAN · activo' : 'Modo LAN'}
		</button>

		<button class="btn-add" onclick={onOpenIngest}>＋ Añadir</button>
	{/if}

	<button
		class="hamburger"
		aria-label={mobileNavOpen ? 'Cerrar menú' : 'Abrir menú'}
		aria-expanded={mobileNavOpen}
		onclick={() => (mobileNavOpen = !mobileNavOpen)}
	>
		{mobileNavOpen ? '✕' : '☰'}
	</button>
</header>

{#if mobileNavOpen}
	<div class="mobile-nav" role="navigation" aria-label="Principal (móvil)">
		<div class="mobile-search"><SearchBox onNavigate={closeMobileNav} /></div>
		{#each navLinks as link (link.href)}
			<a href={link.href} class:active={isActive(link.href)} onclick={closeMobileNav}>{link.label}</a>
		{/each}
		{#if local}
			{#if pendingCount > 0}
				<a href="/pendientes" onclick={closeMobileNav}>{pendingCount} por confirmar</a>
			{/if}
			<a href="/ajustes" onclick={closeMobileNav}>⚙️ Ajustes</a>
			<button
				class="mobile-lan"
				onclick={() => {
					closeMobileNav();
					onOpenLan();
				}}>{lanEnabled ? 'Modo LAN · activo' : 'Modo LAN'}</button
			>
			<button
				class="mobile-add"
				onclick={() => {
					closeMobileNav();
					onOpenIngest();
				}}>＋ Añadir</button
			>
		{/if}
	</div>
{/if}

<style>
	.topbar {
		position: sticky;
		top: 0;
		z-index: 50;
		display: flex;
		align-items: center;
		gap: 22px;
		padding: 14px 28px;
		background: linear-gradient(180deg, rgba(16, 12, 9, 0.97), rgba(16, 12, 9, 0.88));
		backdrop-filter: blur(10px);
		border-bottom: 1px solid var(--line);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		font-family: var(--display);
		font-weight: 800;
		font-size: 22px;
		letter-spacing: 0.04em;
		text-decoration: none;
		color: var(--text);
	}
	.brand svg {
		display: block;
	}
	.nav {
		display: flex;
		gap: 4px;
		margin-left: 6px;
	}
	.nav a {
		color: var(--muted);
		text-decoration: none;
		padding: 7px 12px;
		border-radius: 9px;
		font-weight: 500;
		transition: 0.15s;
		cursor: pointer;
	}
	.nav a:hover {
		color: var(--text);
		background: var(--surface);
	}
	.nav a.active {
		color: var(--text);
		background: var(--surface-2);
	}
	.spacer {
		flex: 1;
	}
	.search-slot {
		display: flex;
	}
	.settings-link {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 34px;
		height: 34px;
		border-radius: 10px;
		font-size: 15px;
		text-decoration: none;
		transition: 0.15s;
	}
	.settings-link:hover {
		background: var(--surface);
	}
	.hamburger {
		display: none;
		align-items: center;
		justify-content: center;
		width: 38px;
		height: 38px;
		border-radius: 10px;
		background: var(--surface);
		border: 1px solid var(--line);
		font-size: 16px;
	}
	.lan-pill {
		display: flex;
		align-items: center;
		gap: 8px;
		background: rgba(111, 207, 142, 0.1);
		border: 1px solid rgba(111, 207, 142, 0.3);
		border-radius: 99px;
		padding: 7px 14px;
		font-size: 13px;
		color: var(--ok);
		font-weight: 500;
		transition: 0.15s;
		cursor: pointer;
	}
	.lan-pill:hover {
		background: rgba(111, 207, 142, 0.18);
	}
	.lan-pill.off {
		background: transparent;
		border-color: var(--line);
		color: var(--muted);
	}
	.lan-pill.off .lan-dot {
		background: var(--off);
		box-shadow: none;
	}
	.lan-dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--ok);
		box-shadow: 0 0 8px var(--ok);
	}
	.btn-add {
		display: flex;
		align-items: center;
		gap: 8px;
		background: var(--accent);
		color: #1a1207;
		font-weight: 600;
		padding: 9px 18px;
		border-radius: 10px;
		font-size: 14px;
		transition: 0.15s;
		box-shadow: 0 4px 18px rgba(242, 169, 59, 0.25);
	}
	.btn-add:hover {
		background: #ffbf58;
		transform: translateY(-1px);
	}
	.pending-pill {
		display: flex;
		align-items: center;
		background: rgba(242, 169, 59, 0.12);
		border: 1px solid rgba(242, 169, 59, 0.35);
		border-radius: 99px;
		padding: 7px 14px;
		font-size: 13px;
		color: var(--accent);
		font-weight: 500;
		text-decoration: none;
		transition: 0.15s;
		white-space: nowrap;
	}
	.pending-pill:hover {
		background: rgba(242, 169, 59, 0.2);
	}

	.mobile-nav {
		display: none;
	}

	@media (max-width: 900px) {
		.nav,
		.search-slot,
		.settings-link,
		.lan-pill {
			display: none;
		}
		.topbar {
			padding: 12px 16px;
			gap: 10px;
		}
		.btn-add {
			padding: 9px 12px;
		}
		.hamburger {
			display: flex;
		}
		.mobile-nav {
			display: flex;
			flex-direction: column;
			gap: 2px;
			padding: 14px 16px 20px;
			background: rgba(16, 12, 9, 0.98);
			border-bottom: 1px solid var(--line);
		}
		.mobile-search {
			margin-bottom: 10px;
		}
		.mobile-search :global(.search-box) {
			width: 100%;
			min-width: 0;
		}
		.mobile-nav a {
			color: var(--text);
			text-decoration: none;
			padding: 12px 10px;
			border-radius: 8px;
			font-weight: 500;
			font-size: 15px;
		}
		.mobile-nav a.active {
			background: var(--surface-2);
		}
		.mobile-nav a:active {
			background: var(--surface);
		}
		.mobile-lan {
			text-align: left;
			color: var(--text);
			padding: 12px 10px;
			border-radius: 8px;
			font-weight: 500;
			font-size: 15px;
		}
		.mobile-lan:active {
			background: var(--surface);
		}
		.mobile-add {
			margin-top: 10px;
			background: var(--accent);
			color: #1a1207;
			font-weight: 600;
			padding: 13px 18px;
			border-radius: 10px;
			font-size: 15px;
		}
	}
</style>
