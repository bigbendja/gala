<script lang="ts">
	import Card from './Card.svelte';
	import type { Item } from '$lib/api/types';

	interface Props {
		title: string;
		subtitle?: string;
		linkHref?: string;
		linkText?: string;
		items: Item[];
		/** Total real en el backend (para saber si hay más páginas que pedir). */
		total?: number;
		variant?: 'default' | 'wide' | 'wish';
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
	const gap = 16;
	const cardWidth = $derived(variant === 'wide' ? 236 : 172);
	const step = $derived(cardWidth + gap);
	const buffer = 4; // tarjetas extra a cada lado para que no "parpadee" en scroll rápido

	let container: HTMLDivElement | undefined = $state();
	let scrollLeft = $state(0);
	let clientWidth = $state(0);

	function measure() {
		if (!container) return;
		scrollLeft = container.scrollLeft;
		clientWidth = container.clientWidth;
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
</script>

<svelte:window onresize={measure} />

<section class="section">
	<div class="sec-head">
		<h2>{title}</h2>
		{#if subtitle}<span class="count">{subtitle}</span>{/if}
		{#if linkHref}<a class="link" href={linkHref}>{linkText ?? 'Ver todo →'}</a>{/if}
	</div>

	{#if loading && items.length === 0}
		<div class="row" class:wide={variant === 'wide'}>
			{#each { length: 6 } as _, i (i)}
				<div class="skeleton"></div>
			{/each}
		</div>
	{:else}
		<div class="row" class:wide={variant === 'wide'} bind:this={container} onscroll={onScroll}>
			{#each visible as item, i (item.id)}
				<div style:grid-column={range.start + i + 1}>
					<Card {item} {variant} />
				</div>
			{/each}
		</div>
	{/if}
</section>

<style>
	.section {
		max-width: 1236px;
		margin: 0 auto;
		padding: 34px 28px 0;
	}
	.sec-head {
		display: flex;
		align-items: baseline;
		gap: 14px;
		margin-bottom: 16px;
	}
	.sec-head h2 {
		font-family: var(--display);
		font-size: 21px;
		font-weight: 600;
		letter-spacing: 0.01em;
	}
	.sec-head .count {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--muted);
	}
	.sec-head .link {
		margin-left: auto;
		color: var(--accent);
		font-size: 13.5px;
		font-weight: 500;
		text-decoration: none;
	}
	.row {
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: 172px;
		gap: 16px;
		overflow-x: auto;
		padding: 4px 4px 18px;
		scrollbar-width: thin;
		scrollbar-color: var(--line) transparent;
	}
	.row.wide {
		grid-auto-columns: 236px;
	}
	.skeleton {
		aspect-ratio: 2 / 3;
		border-radius: 12px;
		background: linear-gradient(100deg, var(--surface) 40%, var(--surface-2) 50%, var(--surface) 60%);
		background-size: 200% 100%;
		animation: shimmer 1.4s ease-in-out infinite;
	}
	@media (prefers-reduced-motion: reduce) {
		.skeleton {
			animation: none;
		}
	}
	@keyframes shimmer {
		from {
			background-position: 200% 0;
		}
		to {
			background-position: -200% 0;
		}
	}
	@media (max-width: 900px) {
		.row {
			grid-auto-columns: 138px;
		}
		.row.wide {
			grid-auto-columns: 200px;
		}
	}
</style>
