<script lang="ts">
	import { page } from '$app/state';

	interface Props {
		onOpenIngest: () => void;
		/** Items "Por confirmar" reales (Brief M10b §3); 0 = la píldora no se
		 *  muestra — nada que confirmar no es un estado que anunciar. */
		pendingCount?: number;
	}
	let { onOpenIngest, pendingCount = 0 }: Props = $props();
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
		<a href="/" class:active={page.url.pathname === '/'}>Inicio</a>
		<!-- Películas/Series/Listas/Deseos: visuales por ahora — el
		     enrutado de M10a/M10b se limita a "/", "/item/{id}", "/ver/{id}"
		     y "/pendientes" (CLAUDE.md). -->
		<span class="inert" title="Todavía no navega a ningún sitio">Películas</span>
		<span class="inert" title="Todavía no navega a ningún sitio">Series</span>
		<span class="inert" title="Todavía no navega a ningún sitio">Listas</span>
		<span class="inert" title="Todavía no navega a ningún sitio">Deseos</span>
	</nav>

	<div class="spacer"></div>

	{#if pendingCount > 0}
		<a class="pending-pill" href="/pendientes">{pendingCount} por confirmar</a>
	{/if}

	<div class="search" role="search" title="Búsqueda: llega en M10c">🔍 Buscar en tu galería…</div>

	<button class="lan-pill" disabled title="Modo LAN: llega en M7">
		<span class="lan-dot"></span> Modo LAN
	</button>

	<button class="btn-add" onclick={onOpenIngest}>＋ Añadir</button>
</header>

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
	.nav a,
	.nav .inert {
		color: var(--muted);
		text-decoration: none;
		padding: 7px 12px;
		border-radius: 9px;
		font-weight: 500;
		transition: 0.15s;
		cursor: default;
	}
	.nav a {
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
	.search {
		display: flex;
		align-items: center;
		gap: 8px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 10px;
		padding: 8px 14px;
		color: var(--muted);
		min-width: 220px;
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
		opacity: 0.65;
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

	@media (max-width: 900px) {
		.nav,
		.search {
			display: none;
		}
		.topbar {
			padding: 12px 16px;
			gap: 12px;
		}
	}
</style>
