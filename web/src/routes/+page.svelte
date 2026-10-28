<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getHomeSettings, listGenres, listItems } from '$lib/api/client';
	import type { GenreCount, Item, LanSession } from '$lib/api/types';
	import { galleryVisible } from '$lib/gallery';
	import { errorMessage } from '$lib/format';
	import { DOCUMENTARY_GENRE, genreRows, playableMovies } from '$lib/home';
	import HeroSlider from '$lib/components/HeroSlider.svelte';
	import HomeSection from '$lib/components/HomeSection.svelte';
	import StateBlock from '$lib/components/StateBlock.svelte';

	// Un dispositivo de la red (Modo LAN) no gestiona la galería: no ve el botón
	// de añadir contenido (el backend además lo rechazaría).
	const isLocal = $derived((page.data as { session?: LanSession }).session?.local !== false);

	// La Home se compone de secciones que cargan solas. Aquí solo se pide lo
	// que decide la estructura: ¿hay algo en la biblioteca?, ¿se ve el hero?,
	// qué géneros tienen contenido, y los títulos que alimentan el hero.
	let phase = $state<'loading' | 'ready' | 'empty' | 'error'>('loading');
	let errorText = $state('');
	let heroPool = $state<Item[]>([]);
	let heroEnabled = $state(true);
	let genres = $state<GenreCount[]>([]);

	async function boot() {
		phase = 'loading';
		try {
			const [pool, settings, genreList] = await Promise.all([
				listItems({ parent_id: 'null', limit: 200 }),
				getHomeSettings().catch(() => ({ hero: true })),
				listGenres().catch(() => [] as GenreCount[])
			]);
			heroPool = pool.items;
			heroEnabled = settings.hero;
			genres = genreList;
			phase = pool.total === 0 ? 'empty' : 'ready';
		} catch (err) {
			errorText = errorMessage(err, 'No se pudo cargar la galería');
			phase = 'error';
		}
	}
	onMount(boot);

	function openIngest() {
		window.dispatchEvent(new CustomEvent('gala:open-ingest'));
	}

	const rows = $derived(genreRows(genres));
</script>

<svelte:head>
	<title>GALA — Tu galería</title>
</svelte:head>

{#if phase === 'error'}
	<StateBlock
		tone="error"
		title="No se pudo cargar la galería"
		text={errorText}
		actionLabel="Reintentar"
		onAction={boot}
	/>
{:else if phase === 'empty'}
	{#if isLocal}
		<StateBlock
			title="Aún no hay nada aquí"
			text="Añade una carpeta o pega enlaces para empezar tu galería."
			actionLabel="＋ Añadir"
			onAction={openIngest}
		/>
	{:else}
		<StateBlock
			title="Aún no hay nada aquí"
			text="La galería está vacía. El contenido se añade desde el PC donde corre GALA."
		/>
	{/if}
{:else}
	{#if phase === 'loading'}
		<div class="hero-skel skeleton" aria-busy="true" aria-label="Cargando"></div>
	{:else if heroEnabled && heroPool.length > 0}
		<HeroSlider items={heroPool} />
	{/if}

	<HomeSection
		title="Continuar viendo"
		subtitle="en este dispositivo"
		variant="wide"
		lazy={false}
		fetchPage={(offset) => listItems({ in_progress: true, limit: 30, offset })}
	/>
	<HomeSection
		title="Películas"
		showCount
		linkHref="/peliculas"
		fetchPage={(offset) => listItems({ kind: 'movie', parent_id: 'null', limit: 40, offset })}
		filter={playableMovies}
	/>
	<HomeSection
		title="Series"
		showCount
		linkHref="/series"
		fetchPage={(offset) => listItems({ kind: 'series', parent_id: 'null', limit: 40, offset })}
	/>
	<HomeSection
		title="Documentales"
		fetchPage={(offset) => listItems({ genre: DOCUMENTARY_GENRE, parent_id: 'null', limit: 40, offset })}
		filter={playableMovies}
	/>
	<HomeSection
		title="Añadidas recientemente"
		fetchPage={(offset) => listItems({ parent_id: 'null', limit: 40, offset })}
		filter={galleryVisible}
	/>
	<HomeSection
		title="Mis Listas"
		linkHref="/deseos"
		variant="wish"
		fetchPage={(offset) => listItems({ wished: true, limit: 40, offset })}
	/>
	{#each rows as g (g.name)}
		<HomeSection
			title={g.name}
			subtitle="{g.count} {g.count === 1 ? 'título' : 'títulos'}"
			fetchPage={(offset) => listItems({ genre: g.name, parent_id: 'null', limit: 40, offset })}
			filter={playableMovies}
		/>
	{/each}
	<div class="end-space"></div>
{/if}

<style>
	.hero-skel {
		height: 440px;
		border-radius: 0;
	}
	.end-space {
		height: 40px;
	}
	@media (max-width: 900px) {
		.hero-skel {
			height: 360px;
		}
	}
</style>
