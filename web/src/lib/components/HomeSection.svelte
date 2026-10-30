<script lang="ts">
	// Sección de la Home que carga SUS datos sola (Sistema de Diseño GALA §2.3,
	// §5): con skeleton mientras llegan, mensaje claro con "Reintentar" si
	// falla, y oculta si está vacía (las filas por género "solo las que tengan
	// contenido"). Las filas por debajo del pliegue esperan a estar cerca de la
	// pantalla antes de pedir nada: muchas filas no cargan miles de imágenes
	// de golpe (mantiene la lazy/virtualización de M10a).
	import { onMount } from 'svelte';
	import type { Item, ListResult } from '$lib/api/types';
	import { errorMessage } from '$lib/format';
	import Row from './Row.svelte';
	import StateBlock from './StateBlock.svelte';

	interface Props {
		title: string;
		subtitle?: string;
		linkHref?: string;
		linkText?: string;
		variant?: 'default' | 'wide' | 'wish' | 'landscape';
		/** Pide una página de la API a partir de offset. */
		fetchPage: (offset: number) => Promise<ListResult>;
		/** Deja pasar solo lo que se debe enseñar (p. ej. películas con fuente). */
		filter?: (items: Item[]) => Item[];
		/** false = pide ya (arriba del pliegue); true = espera a estar cerca. */
		lazy?: boolean;
		/** Enseña "N títulos" junto al título (mockup: contador de la fila). */
		showCount?: boolean;
		/** Cómo se enseña el contador: «11 títulos» (por defecto) o solo «11». */
		countStyle?: 'full' | 'number';
		/** Red de seguridad: pedir la fila a los 1,5 s aunque no se haya visto. La
		 *  Home la usa; las vistas con muchas secciones la apagan (solo lazy). */
		idleStart?: boolean;
		/** Se avisa con el número de títulos al cargar (para decidir si hay Home). */
		onLoaded?: (count: number) => void;
	}
	let {
		title,
		subtitle,
		linkHref,
		linkText,
		variant = 'default',
		fetchPage,
		filter = (items) => items,
		lazy = true,
		showCount = false,
		countStyle = 'full',
		idleStart = true,
		onLoaded
	}: Props = $props();

	let host: HTMLDivElement | undefined = $state();
	let started = $state(false);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let items = $state<Item[]>([]);
	let rawOffset = $state(0);
	let rawTotal = $state(0);
	let loadingMore = $state(false);
	let done = $state(false);

	async function loadFirst() {
		loading = true;
		error = null;
		try {
			const page = await fetchPage(0);
			items = filter(page.items);
			rawOffset = page.items.length;
			rawTotal = page.total;
			onLoaded?.(items.length);
		} catch (err) {
			error = errorMessage(err, `No se pudo cargar «${title}»`);
		} finally {
			loading = false;
			done = true;
		}
	}

	async function loadMore() {
		if (loadingMore || rawOffset >= rawTotal) return;
		loadingMore = true;
		try {
			const page = await fetchPage(rawOffset);
			items = [...items, ...filter(page.items)];
			rawOffset += page.items.length;
			rawTotal = page.total;
		} catch {
			// la fila se queda con lo que tiene; el siguiente scroll reintenta
		} finally {
			loadingMore = false;
		}
	}

	function start() {
		if (started) return;
		started = true;
		void loadFirst();
	}

	onMount(() => {
		if (!lazy || typeof IntersectionObserver === 'undefined' || !host) {
			start();
			return;
		}
		const io = new IntersectionObserver(
			(entries) => {
				if (entries.some((e) => e.isIntersecting)) {
					start();
					io.disconnect();
				}
			},
			{ rootMargin: '500px 0px' }
		);
		io.observe(host);
		return () => io.disconnect();
	});

	// Red de seguridad: una fila que no llegó a cruzar el umbral de visibilidad
	// (pantalla alta, observador sin disparar) se pide igualmente poco después;
	// así nunca queda una fila a medias, y si no tiene nada desaparece.
	onMount(() => {
		if (!idleStart) return;
		const t = setTimeout(start, 1500);
		return () => clearTimeout(t);
	});

	function retry() {
		started = true;
		void loadFirst();
	}
</script>

<div bind:this={host}>
	{#if error}
		<section class="sec">
			<h2>{title}</h2>
			<StateBlock compact tone="error" title="No se pudo cargar esta fila" text={error} actionLabel="Reintentar" onAction={retry} />
		</section>
	{:else if !started}
		<!-- fila diferida aún sin pedir: no ocupa sitio (nada de cajas vacías) -->
	{:else if loading && items.length === 0}
		<section class="sec" aria-busy="true" aria-label="Cargando {title}">
			<div class="skeleton head-skel"></div>
			<div class="skel-row">
				{#each { length: 7 } as _, i (i)}
					<div
						class="skeleton card-skel"
						class:wide={variant === 'wide'}
						class:land={variant === 'landscape'}
					></div>
				{/each}
			</div>
		</section>
	{:else if done && items.length > 0}
		<Row
			{title}
			subtitle={subtitle ??
				(showCount ? (countStyle === 'number' ? `${rawTotal}` : `${rawTotal} ${rawTotal === 1 ? 'título' : 'títulos'}`) : undefined)}
			{linkHref}
			{linkText}
			{variant}
			{items}
			total={rawTotal}
			loading={loadingMore}
			onNeedMore={loadMore}
		/>
	{/if}
</div>

<style>
	.sec {
		max-width: 1296px;
		margin: 0 auto;
		padding: 0 28px 34px;
	}
	.sec h2 {
		font-family: var(--display);
		font-size: 20px;
		font-weight: 600;
		margin-bottom: 14px;
	}
	.head-skel {
		width: 180px;
		height: 24px;
		margin-bottom: 16px;
		border-radius: 8px;
	}
	.skel-row {
		display: flex;
		gap: 14px;
		overflow: hidden;
		padding: 4px 0 10px;
	}
	.card-skel {
		flex: 0 0 168px;
		aspect-ratio: 2 / 3;
	}
	.card-skel.wide {
		flex-basis: 300px;
		aspect-ratio: 16 / 10;
	}
	.card-skel.land {
		flex-basis: 320px;
		aspect-ratio: 16 / 11;
	}
	@media (max-width: 900px) {
		.sec {
			padding-left: 16px;
			padding-right: 16px;
		}
		.card-skel {
			flex-basis: 132px;
		}
		.card-skel.wide {
			flex-basis: 240px;
		}
	}
</style>
