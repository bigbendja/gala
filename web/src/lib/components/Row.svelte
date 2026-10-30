<script lang="ts">
	import { tick } from 'svelte';
	import Card from './Card.svelte';
	import LandscapeCard from './LandscapeCard.svelte';
	import type { Item } from '$lib/api/types';

	interface Props {
		title: string;
		subtitle?: string;
		linkHref?: string;
		linkText?: string;
		items: Item[];
		/** Total real en el backend (para saber si hay más páginas que pedir). */
		total?: number;
		variant?: 'default' | 'wide' | 'wish' | 'landscape';
		loading?: boolean;
		/** Se llama cuando el scroll se acerca al final de lo ya cargado. */
		onNeedMore?: () => void;
	}
	let {
		title,
		subtitle,
		linkHref,
		linkText,
		items,
		total,
		variant = 'default',
		loading = false,
		onNeedMore
	}: Props = $props();

	// Virtualización SIN librerías (Brief M10a §5/§7): grid-auto-flow:column
	// con grid-auto-columns fijo reserva el ancho de scroll completo aunque
	// solo se rendericen unos pocos <Card> con su grid-column explícito — el
	// resto de columnas quedan vacías pero ocupando su hueco. Así una fila
	// de 5.000 items scrollea fluido sin montar 5.000 nodos DOM a la vez.
	const gap = 14;
	const cardWidth = $derived(variant === 'wide' ? 300 : variant === 'landscape' ? 320 : 168);
	const step = $derived(cardWidth + gap);
	const buffer = 4; // tarjetas extra a cada lado para que no "parpadee" en scroll rápido

	let container: HTMLDivElement | undefined = $state();
	let scrollLeft = $state(0);
	let clientWidth = $state(0);
	let scrollWidth = $state(0);

	function measure() {
		if (!container) return;
		scrollLeft = container.scrollLeft;
		clientWidth = container.clientWidth;
		scrollWidth = container.scrollWidth;
	}

	function onScroll() {
		measure();
	}

	$effect(() => {
		// Depende de items.length/loading para reevaluar "¿hace falta pedir
		// más?" cada vez que llega una página nueva, no solo al hacer scroll.
		items.length;
		loading;
		measure();
		if (!onNeedMore || loading || !container) return;
		const nearEnd =
			container.scrollLeft + container.clientWidth > container.scrollWidth - step * 6;
		if (nearEnd && items.length < (total ?? items.length)) onNeedMore();
	});

	const range = $derived.by(() => {
		const start = Math.max(0, Math.floor(scrollLeft / step) - buffer);
		const end = Math.min(items.length, Math.ceil((scrollLeft + clientWidth) / step) + buffer);
		return { start, end: Math.max(start, end) };
	});

	const visible = $derived(items.slice(range.start, range.end));

	// El ancho de scroll depende de las tarjetas YA pintadas: se vuelve a medir
	// cuando cambia cuántas hay (al medir el ancho del contenedor se pintan
	// más), para que las flechas sepan si hay más a la derecha.
	$effect(() => {
		void visible.length;
		void container;
		tick().then(measure);
	});

	// Flechas ‹ › (Sistema de Diseño GALA §5): además del scroll táctil y del
	// teclado. Se desplazan casi una "página" de tarjetas.
	const canPrev = $derived(scrollLeft > 4);
	const canNext = $derived(scrollWidth - (scrollLeft + clientWidth) > 4);

	function page(dir: -1 | 1) {
		if (!container) return;
		const by = Math.max(step, Math.floor(clientWidth / step) * step - step);
		container.scrollBy({ left: dir * by, behavior: 'smooth' });
	}
</script>

<svelte:window onresize={measure} />

<section class="section" class:link-always={variant === 'landscape'} aria-label={title}>
	<div class="sec-head">
		<h2>{title}</h2>
		{#if subtitle}<span class="count">{subtitle}</span>{/if}
		{#if linkHref}<a class="link" href={linkHref}>{linkText ?? 'Ver todo →'}</a>{/if}
	</div>

	{#if loading && items.length === 0}
		<div class="row" class:wide={variant === 'wide'} class:landscape={variant === 'landscape'}>
			{#each { length: 6 } as _, i (i)}
				<div
					class="skeleton card-skeleton"
					class:wide-skel={variant === 'wide'}
					class:land-skel={variant === 'landscape'}
				></div>
			{/each}
		</div>
	{:else}
		<div class="carousel">
			<button
				class="arrow prev"
				class:show={canPrev}
				onclick={() => page(-1)}
				aria-label="Anterior en {title}"
				tabindex={canPrev ? 0 : -1}
				disabled={!canPrev}>‹</button
			>
			<div
				class="row"
				class:wide={variant === 'wide'}
				class:landscape={variant === 'landscape'}
				bind:this={container}
				onscroll={onScroll}
			>
				{#each visible as item, i (item.id)}
					<div style:grid-column={range.start + i + 1}>
						{#if variant === 'landscape'}
							<LandscapeCard {item} />
						{:else}
							<Card {item} {variant} />
						{/if}
					</div>
				{/each}
			</div>
			<button
				class="arrow next"
				class:show={canNext}
				onclick={() => page(1)}
				aria-label="Siguiente en {title}"
				tabindex={canNext ? 0 : -1}
				disabled={!canNext}>›</button
			>
		</div>
	{/if}
</section>

<style>
	/* Medidas del mockup de la Home: contenedor de 1296px, cabecera y tarjetas
	   alineadas a 28px, tarjeta 168px, hueco 14px. */
	.section {
		max-width: 1296px;
		margin: 0 auto;
		padding: 0 0 34px;
	}
	.sec-head {
		display: flex;
		align-items: baseline;
		gap: 12px;
		margin-bottom: 14px;
		padding: 0 28px;
	}
	.sec-head h2 {
		font-family: var(--display);
		font-size: 20px;
		font-weight: 600;
	}
	.sec-head .count {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--muted);
	}
	.sec-head .link {
		margin-left: auto;
		color: var(--accent);
		font-size: 13px;
		font-weight: 500;
		text-decoration: none;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.section.link-always .sec-head .link,
	.section:hover .sec-head .link,
	.sec-head .link:focus-visible {
		opacity: 1;
	}
	@media (hover: none) {
		.sec-head .link {
			opacity: 1;
		}
	}
	.sec-head .link:hover {
		text-decoration: underline;
	}
	.carousel {
		position: relative;
	}
	.row {
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: 168px;
		gap: 14px;
		overflow-x: auto;
		padding: 4px 28px 10px;
		scrollbar-width: none;
	}
	.row::-webkit-scrollbar {
		display: none;
	}
	.row.wide {
		grid-auto-columns: 300px;
	}
	.row.landscape {
		grid-auto-columns: 320px;
	}
	.card-skeleton {
		aspect-ratio: 2 / 3;
	}
	.card-skeleton.wide-skel {
		aspect-ratio: 16 / 10;
	}
	.card-skeleton.land-skel {
		aspect-ratio: 16 / 11;
	}
	/* Flechas (mockup de la Home): bandas a los lados con degradado hacia el
	   fondo; aparecen al pasar el ratón o enfocar la fila y, en táctil, siempre
	   (donde no hay "hover"). Ancho táctil de 52px. */
	.arrow {
		position: absolute;
		top: 0;
		bottom: 14px;
		width: 52px;
		z-index: 3;
		display: grid;
		place-items: center;
		color: var(--text);
		font-size: 30px;
		line-height: 1;
		opacity: 0;
		pointer-events: none;
		transition: opacity 0.2s, color 0.15s;
	}
	.arrow.prev {
		left: 0;
		background: linear-gradient(90deg, var(--bg) 30%, transparent);
	}
	.arrow.next {
		right: 0;
		background: linear-gradient(270deg, var(--bg) 30%, transparent);
	}
	.carousel:hover .arrow.show,
	.carousel:focus-within .arrow.show {
		opacity: 1;
		pointer-events: auto;
	}
	.arrow:hover {
		color: var(--accent);
	}
	@media (hover: none) {
		.arrow.show {
			opacity: 0.9;
			pointer-events: auto;
		}
	}
	@media (max-width: 900px) {
		.sec-head,
		.row {
			padding-left: 16px;
			padding-right: 16px;
		}
		.row {
			grid-auto-columns: 132px;
		}
		.row.wide {
			grid-auto-columns: 240px;
		}
		.row.landscape {
			grid-auto-columns: 280px;
		}
	}
</style>
