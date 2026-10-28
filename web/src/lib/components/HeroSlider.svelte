<script lang="ts">
	// Hero-slider de la Home (mockup GALA_Mockup_Home): el backdrop apaisado de
	// TMDB ocupa toda la anchura, con degradado hacia el fondo; el texto abajo a
	// la izquierda; filtros arriba a la derecha; puntos-barra y flechas. Rota
	// solo (se detiene con ratón, foco, pestaña oculta y prefers-reduced-motion).
	// Datos reales de la API; un título sin fondo no entra.
	import { getItemDetails, imgUrl, setWished } from '$lib/api/client';
	import type { Item, ItemDetails } from '$lib/api/types';
	import { addedEyebrow, displayTitle, formatRating, formatRuntime } from '$lib/format';
	import { HERO_MODES, pickHero, playHref, playLabel, type HeroMode } from '$lib/home';

	interface Props {
		/** Títulos de nivel superior ya cargados (la API, sin filtrar). */
		items: Item[];
		/** Milisegundos entre diapositivas. */
		intervalMs?: number;
	}
	let { items, intervalMs = 6000 }: Props = $props();

	const STORAGE_KEY = 'gala.home.heroMode';
	function loadMode(): HeroMode {
		try {
			const v = localStorage.getItem(STORAGE_KEY);
			if (v === 'destacados' || v === 'nuevos' || v === 'sin_ver') return v;
		} catch {
			// sin almacenamiento (ventana privada…): se usa el valor por defecto
		}
		return 'destacados';
	}

	let mode = $state<HeroMode>(loadMode());
	let index = $state(0);
	let hovering = $state(false);
	let focusInside = $state(false);
	let tabHidden = $state(false);
	let reducedMotion = $state(false);

	const slides = $derived(pickHero(items, mode));
	const activeIndex = $derived(slides.length ? Math.min(index, slides.length - 1) : 0);

	function setMode(next: HeroMode) {
		mode = next;
		index = 0;
		try {
			localStorage.setItem(STORAGE_KEY, next);
		} catch {
			// idem
		}
	}

	function go(i: number) {
		if (!slides.length) return;
		index = (i + slides.length) % slides.length;
	}

	// Rotación automática: se detiene al pasar el ratón o enfocar el hero, con
	// la pestaña oculta y con prefers-reduced-motion (Sistema §6).
	$effect(() => {
		const mq =
			typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
		const onChange = () => (reducedMotion = !!mq?.matches);
		onChange();
		mq?.addEventListener('change', onChange);
		const onVis = () => (tabHidden = document.hidden);
		document.addEventListener('visibilitychange', onVis);
		return () => {
			mq?.removeEventListener('change', onChange);
			document.removeEventListener('visibilitychange', onVis);
		};
	});

	$effect(() => {
		if (reducedMotion || hovering || focusInside || tabHidden || slides.length < 2) return;
		const t = setInterval(() => go(activeIndex + 1), intervalMs);
		return () => clearInterval(t);
	});

	// Duración y nota de TMDB de cada diapositiva (la lista no las trae): una
	// petición pequeña por título, una sola vez.
	let detailsMap = $state<Record<number, ItemDetails | null>>({});
	const requested = new Set<number>();
	$effect(() => {
		let alive = true;
		for (const s of slides) {
			if (requested.has(s.id)) continue;
			requested.add(s.id);
			getItemDetails(s.id)
				.then((d) => alive && (detailsMap = { ...detailsMap, [s.id]: d }))
				.catch(() => alive && (detailsMap = { ...detailsMap, [s.id]: null }));
		}
		return () => {
			alive = false;
			// Si el efecto se relanza antes de que llegue, se vuelve a pedir.
			for (const s of slides) if (!(s.id in detailsMap)) requested.delete(s.id);
		};
	});

	// "+ Mi lista" por diapositiva: estado propio sembrado con el real.
	let wishedOverride = $state<Record<number, boolean>>({});
	let wishPending = $state(false);
	function isWished(it: Item): boolean {
		return wishedOverride[it.id] ?? it.wished;
	}
	async function toggleWished(it: Item) {
		if (wishPending) return;
		const next = !isWished(it);
		wishPending = true;
		try {
			await setWished(it.id, next);
			wishedOverride = { ...wishedOverride, [it.id]: next };
		} catch {
			// el botón se queda como estaba; el usuario puede reintentar
		} finally {
			wishPending = false;
		}
	}

	function onKey(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft') {
			e.preventDefault();
			go(activeIndex - 1);
		} else if (e.key === 'ArrowRight') {
			e.preventDefault();
			go(activeIndex + 1);
		}
	}

	function eyebrow(it: Item): string {
		const kind = it.kind === 'series' ? 'Serie' : 'Película';
		if (mode === 'nuevos') return `${addedEyebrow(it.added_at)} · ${kind}`;
		if (mode === 'sin_ver') return `Sin ver · ${kind}`;
		return `Destacado · ${kind}`;
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<section
	class="hero"
	aria-roledescription="carrusel"
	aria-label="Títulos destacados"
	onmouseenter={() => (hovering = true)}
	onmouseleave={() => (hovering = false)}
	onfocusin={() => (focusInside = true)}
	onfocusout={() => (focusInside = false)}
	onkeydown={onKey}
>
	{#each slides as it, i (it.id)}
		{@const active = i === activeIndex}
		{@const bg = imgUrl(it.backdrop_path)}
		{@const d = detailsMap[it.id]}
		{@const runtime = formatRuntime(d?.runtime_min)}
		{@const rating = formatRating(d?.vote_average)}
		<div
			class="slide"
			class:active
			role="group"
			aria-roledescription="diapositiva"
			aria-label="{i + 1} de {slides.length}"
			aria-hidden={!active}
			inert={!active}
		>
			<div class="slide-bg" style:background-image={bg ? `url(${bg})` : undefined}></div>
			<div class="slide-inner">
				<div class="eyebrow">{eyebrow(it)}</div>
				<svelte:element this={active ? 'h1' : 'p'} class="ttl">{displayTitle(it)}</svelte:element>
				<div class="hero-meta">
					{#if it.rating_cert}<span class="cert">{it.rating_cert}+</span>{/if}
					{#if it.year}<span>{it.year}</span>{/if}
					{#if runtime}<span>{runtime}</span>{/if}
					{#if it.genres.length}<span>{it.genres.slice(0, 2).join(' · ')}</span>{/if}
					{#if rating}<span class="star" title="Nota de TMDB">★ {rating}</span>{/if}
				</div>
				{#if it.overview}<p class="overview">{it.overview}</p>{/if}
				<div class="hero-actions">
					<a class="btn-play" href={playHref(it)}>{playLabel(it)}</a>
					<button class="btn-ghost" class:on={isWished(it)} disabled={wishPending} onclick={() => toggleWished(it)}>
						{isWished(it) ? '✓ En tu lista' : '＋ Mi lista'}
					</button>
					<a class="btn-ghost" href="/item/{it.id}">Detalles</a>
				</div>
			</div>
		</div>
	{/each}

	{#if slides.length === 0}
		<div class="slide active empty">
			<div class="slide-bg"></div>
			<div class="slide-inner">
				<p class="none-title">Nada que enseñar con este filtro</p>
				<p class="none-text">
					{mode === 'sin_ver'
						? 'Ya has empezado o visto todo lo destacable. Prueba con «Nuevos».'
						: 'Todavía no hay títulos con imagen de fondo. Se completan al identificarlos.'}
				</p>
			</div>
		</div>
	{/if}

	<div class="hero-filters" role="group" aria-label="Qué se muestra en el destacado">
		<span class="flabel">Destacar</span>
		{#each HERO_MODES as m (m.id)}
			<button class="chip" class:on={mode === m.id} aria-pressed={mode === m.id} onclick={() => setMode(m.id)}>
				{m.label}
			</button>
		{/each}
	</div>

	{#if slides.length > 1}
		<button class="hero-arrow prev" onclick={() => go(activeIndex - 1)} aria-label="Título anterior">‹</button>
		<button class="hero-arrow next" onclick={() => go(activeIndex + 1)} aria-label="Título siguiente">›</button>
		<div class="hero-dots" role="group" aria-label="Elegir título">
			{#each slides as s, i (s.id)}
				<button
					class="dot"
					class:on={i === activeIndex}
					aria-label="Título {i + 1} de {slides.length}: {displayTitle(s)}"
					aria-current={i === activeIndex}
					onclick={() => go(i)}
				></button>
			{/each}
		</div>
	{/if}
</section>

<style>
	.hero {
		position: relative;
		height: clamp(440px, 56vw, 620px);
		overflow: hidden;
		isolation: isolate;
		background: #0d0a06;
	}
	.slide {
		position: absolute;
		inset: 0;
		opacity: 0;
		transition: opacity 1s ease;
		pointer-events: none;
	}
	.slide.active {
		opacity: 1;
		pointer-events: auto;
	}
	.slide-bg {
		position: absolute;
		inset: 0;
		background-color: #0d0a06;
		background-image: radial-gradient(120% 90% at 75% 20%, #4a3016, #1e1207 55%, #0d0a06);
		background-size: cover;
		background-position: center 28%;
	}
	/* Degradado hacia el fondo: texto legible a la izquierda, imagen a la
	   derecha, y el borde inferior se funde con la página. */
	.slide-bg::after {
		content: '';
		position: absolute;
		inset: 0;
		background:
			linear-gradient(90deg, var(--bg) 0%, rgba(16, 12, 9, 0.75) 38%, rgba(16, 12, 9, 0.1) 70%, transparent 100%),
			linear-gradient(0deg, var(--bg) 2%, transparent 42%);
	}
	@media (prefers-reduced-motion: reduce) {
		.slide {
			transition: none;
		}
	}
	.slide-inner {
		position: relative;
		z-index: 2;
		height: 100%;
		max-width: 640px;
		margin-left: max(28px, calc((100vw - 1240px) / 2));
		padding: 0 28px 54px;
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
	}
	.eyebrow {
		font-family: var(--mono);
		font-size: 12px;
		letter-spacing: 0.14em;
		color: var(--accent);
		text-transform: uppercase;
		margin-bottom: 12px;
	}
	.ttl {
		font-family: var(--display);
		font-size: clamp(34px, 4.6vw, 56px);
		font-weight: 800;
		line-height: 1.02;
		letter-spacing: -0.01em;
		margin-bottom: 14px;
		overflow-wrap: anywhere;
		text-shadow: 0 2px 28px rgba(0, 0, 0, 0.55);
	}
	.hero-meta {
		display: flex;
		gap: 14px;
		align-items: center;
		color: var(--muted);
		font-size: 14px;
		margin-bottom: 12px;
		flex-wrap: wrap;
	}
	.hero-meta .cert {
		border: 1px solid var(--line);
		border-radius: 5px;
		padding: 1px 7px;
		font-size: 12px;
	}
	.hero-meta .star {
		color: var(--accent);
		font-weight: 600;
	}
	.overview {
		color: #d9cab8;
		max-width: 540px;
		margin-bottom: 20px;
		font-size: 15.5px;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.hero-actions {
		display: flex;
		gap: 12px;
		align-items: center;
		flex-wrap: wrap;
	}
	.btn-play {
		display: inline-flex;
		align-items: center;
		gap: 10px;
		min-height: 46px;
		background: var(--text);
		color: #17110a;
		font-weight: 600;
		font-size: 15px;
		padding: 12px 28px;
		border-radius: 11px;
		transition: 0.15s;
		text-decoration: none;
	}
	.btn-play:hover {
		transform: translateY(-1px);
		box-shadow: 0 8px 24px rgba(244, 237, 227, 0.15);
	}
	.btn-ghost {
		display: inline-flex;
		align-items: center;
		min-height: 46px;
		padding: 12px 20px;
		border-radius: 11px;
		border: 1px solid rgba(244, 237, 227, 0.2);
		color: var(--text);
		background: rgba(255, 255, 255, 0.06);
		font-weight: 500;
		font-size: 15px;
		text-decoration: none;
		transition: 0.15s;
	}
	.btn-ghost:hover:not(:disabled) {
		background: rgba(255, 255, 255, 0.12);
	}
	.btn-ghost.on {
		border-color: rgba(242, 169, 59, 0.4);
		color: var(--accent);
		background: rgba(242, 169, 59, 0.1);
	}
	.btn-ghost:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.none-title {
		font-family: var(--display);
		font-size: 26px;
		font-weight: 600;
		margin-bottom: 6px;
	}
	.none-text {
		color: var(--muted);
		max-width: 46ch;
	}

	/* Filtros arriba a la derecha. */
	.hero-filters {
		position: absolute;
		top: 22px;
		right: max(28px, calc((100vw - 1240px) / 2));
		z-index: 3;
		display: flex;
		gap: 8px;
		align-items: center;
	}
	.flabel {
		font-family: var(--mono);
		font-size: 11px;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--muted);
		margin-right: 4px;
	}
	.chip {
		min-height: 36px;
		font-size: 13px;
		padding: 6px 14px;
		border-radius: 99px;
		border: 1px solid var(--line);
		background: rgba(16, 12, 9, 0.6);
		color: var(--muted);
		transition: 0.15s;
		backdrop-filter: blur(6px);
	}
	.chip:hover {
		color: var(--text);
		border-color: #4d3a22;
	}
	.chip.on {
		background: var(--accent);
		color: #1a1207;
		border-color: var(--accent);
		font-weight: 600;
	}

	/* Flechas laterales (aparecen al pasar el ratón o enfocar) y puntos-barra. */
	.hero-arrow {
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		z-index: 3;
		width: 46px;
		height: 46px;
		border-radius: 50%;
		background: rgba(16, 12, 9, 0.55);
		border: 1px solid var(--line);
		display: grid;
		place-items: center;
		color: var(--text);
		font-size: 22px;
		line-height: 1;
		opacity: 0;
		transition: 0.2s;
		backdrop-filter: blur(6px);
	}
	.hero:hover .hero-arrow,
	.hero:focus-within .hero-arrow {
		opacity: 1;
	}
	.hero-arrow.prev {
		left: 20px;
	}
	.hero-arrow.next {
		right: 20px;
	}
	.hero-arrow:hover {
		background: rgba(16, 12, 9, 0.85);
		border-color: #4d3a22;
	}
	.hero-dots {
		position: absolute;
		bottom: 14px;
		right: max(28px, calc((100vw - 1240px) / 2));
		z-index: 3;
		display: flex;
		gap: 2px;
	}
	/* El botón da el área táctil (40px); la barra es lo que se ve. */
	.dot {
		position: relative;
		width: 36px;
		height: 36px;
	}
	.dot::after {
		content: '';
		position: absolute;
		top: 50%;
		left: 50%;
		width: 28px;
		height: 4px;
		margin: -2px 0 0 -14px;
		border-radius: 2px;
		background: rgba(244, 237, 227, 0.25);
		transition: 0.2s;
	}
	.dot:hover::after {
		background: rgba(244, 237, 227, 0.5);
	}
	.dot.on::after {
		width: 40px;
		margin-left: -20px;
		background: var(--accent);
	}

	@media (max-width: 900px) {
		.hero {
			height: auto;
			min-height: 480px;
		}
		.slide {
			position: absolute;
		}
		.slide-inner {
			margin-left: 0;
			max-width: none;
			padding: 0 16px 62px;
		}
		.slide-bg::after {
			background:
				linear-gradient(90deg, rgba(16, 12, 9, 0.7) 0%, rgba(16, 12, 9, 0.25) 100%),
				linear-gradient(0deg, var(--bg) 4%, transparent 62%);
		}
		.hero-filters {
			top: 12px;
			left: 16px;
			right: 16px;
			overflow-x: auto;
			scrollbar-width: none;
		}
		.flabel {
			display: none;
		}
		.hero-dots {
			right: 8px;
			bottom: 8px;
		}
		.hero-arrow {
			display: none;
		}
	}
</style>
