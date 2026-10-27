<script lang="ts">
	// Barra fina de progreso al navegar entre rutas (mientras SvelteKit espera
	// los datos de la ruta nueva): el usuario nunca duda de si la app se colgó.
	import { navigating } from '$app/state';

	const active = $derived(navigating.to !== null);
</script>

<div class="nav-progress" class:active aria-hidden="true"><i></i></div>

<style>
	.nav-progress {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		height: 2px;
		z-index: 100;
		pointer-events: none;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.nav-progress.active {
		opacity: 1;
	}
	.nav-progress i {
		display: block;
		height: 100%;
		width: 40%;
		background: var(--accent);
		box-shadow: 0 0 10px rgba(242, 169, 59, 0.6);
		animation: gala-nav 1.1s ease-in-out infinite;
	}
	@keyframes gala-nav {
		from {
			transform: translateX(-100%);
		}
		to {
			transform: translateX(260%);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.nav-progress i {
			animation: none;
			width: 100%;
		}
	}
</style>
