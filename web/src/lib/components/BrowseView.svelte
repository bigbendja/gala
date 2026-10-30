<script lang="ts">
	// Vista de exploración de Películas y Series (mockup Vistas2, vista «Filas»):
	// barra de filtros arriba y una sección por género con su carrusel de
	// tarjetas horizontales. Filtros y orden viven en la URL (?genero=&anio=&orden=)
	// para poder compartir, recargar y volver atrás.
	//
	// Orden de las secciones: por nº de títulos (el género con más, primero) y,
	// a igualdad, alfabético. Un título con varios géneros sale en cada uno de
	// ellos (como en el mockup). Una sección sin resultados no se pinta.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { listFacets, listItems } from '$lib/api/client';
	import type { Facets, LanSession, SortOrder } from '$lib/api/types';
	import { errorMessage } from '$lib/format';
	import BrowseFilters from './BrowseFilters.svelte';
	import HomeSection from './HomeSection.svelte';
	import StateBlock from './StateBlock.svelte';

	interface Props {
		kind: 'movie' | 'series';
		title: string;
		emptyTitle: string;
		emptyText: string;
		errorTitle: string;
	}
	let { kind, title, emptyTitle, emptyText, errorTitle }: Props = $props();

	const isLocal = $derived((page.data as { session?: LanSession }).session?.local !== false);

	const SORTS: SortOrder[] = ['recent', 'year', 'title', 'rating'];
	const params = $derived(page.url.searchParams);
	const genre = $derived(params.get('genero') ?? '');
	const year = $derived(Number(params.get('anio')) || 0);
	const sort = $derived<SortOrder>(
		SORTS.includes(params.get('orden') as SortOrder) ? (params.get('orden') as SortOrder) : 'recent'
	);

	let facets = $state<Facets | null>(null);
	let totalAll = $state<number | null>(null);
	let totalFiltered = $state<number | null>(null);
	let error = $state<string | null>(null);

	async function boot() {
		error = null;
		try {
			const [f, all] = await Promise.all([
				listFacets(kind),
				listItems({ kind, playable: true, parent_id: 'null', limit: 1 })
			]);
			facets = f;
			totalAll = all.total;
		} catch (err) {
			error = errorMessage(err, errorTitle);
		}
	}
	onMount(boot);

	// Cuántos títulos quedan con el filtro actual (cabecera y «N de M»). Una
	// respuesta vieja nunca pisa a una más nueva.
	let ticket = 0;
	$effect(() => {
		const g = genre;
		const y = year;
		if (totalAll === null) return;
		const mine = ++ticket;
		listItems({ kind, genre: g || undefined, year: y || undefined, playable: true, parent_id: 'null', limit: 1 })
			.then((r) => mine === ticket && (totalFiltered = r.total))
			.catch(() => mine === ticket && (totalFiltered = null));
	});

	const sections = $derived.by(() => {
		if (!facets) return [];
		if (genre) return [{ name: genre, count: 0 }];
		return facets.genres;
	});
	const filtersOn = $derived(!!genre || !!year);
	const shown = $derived(totalFiltered ?? totalAll);
	const counter = $derived(
		filtersOn && totalFiltered !== null && totalAll !== null ? `${totalFiltered} de ${totalAll}` : ''
	);
	const headCount = $derived(shown === null ? '' : `${shown} ${shown === 1 ? 'título' : 'títulos'}`);

	function urlWith(next: { genre?: string; year?: number; sort?: SortOrder }): string {
		const q = new URLSearchParams();
		const g = next.genre ?? genre;
		const y = next.year ?? year;
		const s = next.sort ?? sort;
		if (g) q.set('genero', g);
		if (y) q.set('anio', String(y));
		if (s !== 'recent') q.set('orden', s);
		const qs = q.toString();
		return page.url.pathname + (qs ? `?${qs}` : '');
	}
	function change(next: { genre?: string; year?: number; sort?: SortOrder }) {
		void goto(urlWith(next), { replaceState: true, keepFocus: true, noScroll: true });
	}
	function clear() {
		void goto(page.url.pathname, { replaceState: true, keepFocus: true, noScroll: true });
	}
	function openIngest() {
		window.dispatchEvent(new CustomEvent('gala:open-ingest'));
	}
</script>

<div class="wrap">
	<div class="phead">
		<h1>{title}</h1>
		{#if headCount}<span class="count">{headCount}</span>{/if}
	</div>

	{#if error}
		<StateBlock tone="error" title={errorTitle} text={error} actionLabel="Reintentar" onAction={boot} />
	{:else if facets === null || totalAll === null}
		<div class="skeleton filters-skel" aria-busy="true" aria-label="Cargando"></div>
		<div class="skeleton head-skel"></div>
		<div class="skel-row">
			{#each { length: 4 } as _, i (i)}<div class="skeleton card-skel"></div>{/each}
		</div>
	{:else if totalAll === 0}
		{#if isLocal}
			<StateBlock title={emptyTitle} text={emptyText} actionLabel="＋ Añadir" onAction={openIngest} />
		{:else}
			<StateBlock title={emptyTitle} text="El contenido se añade desde el PC donde corre GALA." />
		{/if}
	{:else}
		<div class="fwrap">
			<BrowseFilters {facets} {genre} {year} {sort} {counter} onChange={change} onClear={clear} />
		</div>
		{#if totalFiltered === 0}
			<StateBlock
				compact
				title="Sin resultados con estos filtros"
				text="Prueba con otro género o año."
				actionLabel="Limpiar filtros"
				onAction={clear}
			/>
		{:else}
			{#key `${genre}|${year}|${sort}`}
				{#each sections as g, i (g.name)}
					<HomeSection
						title={g.name}
						variant="landscape"
						showCount
						countStyle="number"
						lazy={i > 1}
						idleStart={false}
						linkHref={genre ? undefined : urlWith({ genre: g.name })}
						fetchPage={(offset) =>
							listItems({
								kind,
								genre: g.name,
								year: year || undefined,
								sort,
								playable: true,
								parent_id: 'null',
								limit: 40,
								offset
							})}
					/>
				{/each}
			{/key}
		{/if}
	{/if}
</div>

<style>
	.wrap {
		max-width: 1296px;
		margin: 0 auto;
		padding: 22px 0 70px;
	}
	.phead,
	.fwrap,
	.filters-skel {
		margin-left: 28px;
		margin-right: 28px;
	}
	.phead {
		display: flex;
		align-items: center;
		gap: 14px;
		margin-bottom: 16px;
		flex-wrap: wrap;
	}
	h1 {
		font-family: var(--display);
		font-size: 30px;
		font-weight: 800;
	}
	.count {
		font-family: var(--mono);
		font-size: 13px;
		color: var(--muted);
	}
	.filters-skel {
		height: 62px;
		border-radius: 12px;
		margin-bottom: 26px;
	}
	.head-skel {
		width: 180px;
		height: 24px;
		margin: 0 28px 14px;
		border-radius: 8px;
	}
	.skel-row {
		display: flex;
		gap: 14px;
		overflow: hidden;
		padding: 4px 28px 10px;
	}
	.card-skel {
		flex: 0 0 320px;
		aspect-ratio: 16 / 11;
	}
	@media (max-width: 760px) {
		.phead,
		.fwrap,
		.filters-skel {
			margin-left: 16px;
			margin-right: 16px;
		}
		.head-skel {
			margin-left: 16px;
		}
		.skel-row {
			padding-left: 16px;
		}
		.card-skel {
			flex-basis: 280px;
		}
	}
</style>
