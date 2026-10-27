<script lang="ts">
	// Hero-slider de la Home (Sistema de Diseño GALA §5): varios títulos que
	// rotan solos, con filtros (Destacados / Nuevos / Sin ver) que deciden qué
	// los alimenta. Datos reales de la API; si un título no tiene fondo no
	// entra (nunca se inventa una imagen).
	import { fade } from 'svelte/transition';
	import { imgUrl, setWished } from '$lib/api/client';
	import type { Item } from '$lib/api/types';
	import { addedEyebrow, displayTitle } from '$lib/format';
	import { HERO_MODES, pickHero, playHref, playLabel, type HeroMode } from '$lib/home';

	interface Props {
		/** Títulos de nivel superior ya cargados (la API, sin filtrar). */
		items: Item[];
		/** Milisegundos entre diapositivas. */
		intervalMs?: number;
	}
	let { items, intervalMs = 8000 }: Props = $props();

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
	const current = $derived(slides.length ? slides[Math.min(index, slides.length - 1)] : null);

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
		const t = setInterval(() => go(index + 1), intervalMs);
		return () => clearInterval(t);
	});

	// "+ Mi lista" por diapositiva: estado propio sembrado con el real.
	let wishedOverride = $state<Record<number, boolean>>({});
	let wishPending = $state(false);
	const currentWished = $derived(current ? (wishedOverride[current.id] ?? current.wished) : false);
	async function toggleWished() {
		if (!current || wishPending) return;
		const id = current.id;
		const next = !currentWished;
		wishPending = true;
		try {
			await setWished(id, next);
			wishedOverride = { ...wishedOverride, [id]: next };
		} catch {
			// el botón se queda como estaba; el usuario puede reintentar
		} finally {
			wishPending = false;
		}
	}

	function onKey(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft') {
			e.preventDefault();
			go(index - 1);
		} else if (e.key === 'ArrowRight') {
			e.preventDefault();
			go(index + 1);
		}
	}

	const backdrop = $derived(current ? imgUrl(current.backdrop_path) : null);
	const poster = $derived(current ? imgUrl(current.poster_path) : null);
	const title = $derived(current ? displayTitle(current) : '');
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
	{#if backdrop}
		{#key current?.id}
			<div
				class="hero-bg"
				style:background-image="url({backdrop})"
				in:fade={{ duration: reducedMotion ? 0 : 450 }}
			></div>
		{/key}
	{/if}

	<div class="hero-inner">
		{#if current}
			{#key current.id}
				<div class="copy" in:fade={{ duration: reducedMotion ? 0 : 350 }}>
					<div class="eyebrow">{addedEyebrow(current.added_at)}</div>
					<h1>{title}</h1>
					<div class="hero-meta">
						{#if current.rating_cert}<span class="cert">{current.rating_cert}+</span>{/if}
						{#if current.year}<span>{current.year}</span>{/if}
						{#if current.genres.length}<span>{current.genres.slice(0, 3).join(' · ')}</span>{/if}
					</div>
					{#if current.overview}<p class="overview">{current.overview}</p>{/if}
					<div class="hero-actions">
						<a class="btn-play" href={playHref(current)}>{playLabel(current)}</a>
						<button
							class="btn btn-secondary"
							class:active={currentWished}
							disabled={wishPending}
							onclick={toggleWished}
						>
							{currentWished ? '✓ En tu lista' : '＋ Mi lista'}
						</button>
						<a class="btn btn-secondary" href="/item/{current.id}">Detalles</a>
					</div>
				</div>
			{/key}

			<a class="hero-poster" href="/item/{current.id}" tabindex="-1" aria-hidden="true">
				{#if poster}
					<img src={poster} alt="" />
				{:else}
					<div class="art"><b>{title}</b></div>
				{/if}
			</a>
		{:else}
			<div class="copy none">
				<p class="none-title">Nada que enseñar con este filtro</p>
				<p class="none-text">
					{mode === 'sin_ver'
						? 'Ya has empezado o visto todo lo destacable. Prueba con «Nuevos».'
						: 'Todavía no hay títulos con imagen de fondo. Se completan al identificarlos.'}
				</p>
			</div>
		{/if}
	</div>

	<div class="controls">
		<div class="chips" role="group" aria-label="Qué se muestra en el destacado">
			{#each HERO_MODES as m (m.id)}
				<button class="chip" class:on={mode === m.id} aria-pressed={mode === m.id} onclick={() => setMode(m.id)}>
					{m.label}
				</button>
			{/each}
		</div>

		{#if slides.length > 1}
			<div class="nav">
				<button class="arrow" onclick={() => go(index - 1)} aria-label="Título anterior">‹</button>
				<div class="dots" role="group" aria-label="Elegir título">
					{#each slides as s, i (s.id)}
						<button
							class="dot"
							class:on={i === index}
							aria-label="Título {i + 1} de {slides.length}: {displayTitle(s)}"
							aria-current={i === index}
							onclick={() => go(i)}
						></button>
					{/each}
				</div>
				<button class="arrow" onclick={() => go(index + 1)} aria-label="Título siguiente">›</button>
			</div>
		{/if}
	</div>
</section>

<style>
	.hero {
		position: relative;
		padding: 56px 28px 96px;
		overflow: hidden;
		isolation: isolate;
		min-height: 440px;
	}
	.hero::before {
		content: '';
		position: absolute;
		inset: 0;
		z-index: -2;
		background: radial-gradient(120% 90% at 78% 8%, #3d2a14 0%, #221709 42%, var(--bg) 78%);
	}
	.hero-bg {
		position: absolute;
		inset: 0;
		z-index: -3;
		background-size: cover;
		background-position: center 20%;
		opacity: 0.4;
		-webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.7), transparent 85%);
		mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.7), transparent 85%);
	}
	/* Haz de proyector: la firma visual del hero. */
	.hero::after {
		content: '';
		position: absolute;
		top: -20%;
		right: 6%;
		width: 60%;
		height: 150%;
		z-index: -1;
		pointer-events: none;
		background: conic-gradient(
			from 208deg at 100% 0%,
			transparent 0deg,
			rgba(242, 169, 59, 0.1) 8deg,
			rgba(242, 169, 59, 0.03) 22deg,
			transparent 34deg
		);
		animation: beam 9s ease-in-out infinite alternate;
	}
	@keyframes beam {
		from {
			opacity: 0.7;
		}
		to {
			opacity: 1;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.hero::after {
			animation: none;
		}
	}
	.hero-inner {
		max-width: 1180px;
		margin: 0 auto;
		display: grid;
		grid-template-columns: 1fr 280px;
		gap: 48px;
		align-items: center;
		min-height: 300px;
	}
	.copy {
		min-width: 0;
	}
	.eyebrow {
		font-size: 12px;
		font-weight: 600;
		letter-spacing: 0.14em;
		color: var(--accent);
		text-transform: uppercase;
		margin-bottom: 14px;
	}
	h1 {
		font-family: var(--display);
		font-size: clamp(34px, 4.6vw, 58px);
		font-weight: 800;
		line-height: 1.02;
		letter-spacing: -0.01em;
		margin-bottom: 16px;
		overflow-wrap: anywhere;
	}
	.hero-meta {
		display: flex;
		gap: 14px;
		align-items: center;
		color: var(--muted);
		font-size: 14px;
		margin-bottom: 14px;
		flex-wrap: wrap;
	}
	.hero-meta .cert {
		border: 1px solid var(--line);
		border-radius: 5px;
		padding: 1px 7px;
		font-size: 12px;
	}
	.overview {
		color: var(--muted);
		max-width: 560px;
		margin-bottom: 22px;
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
		min-height: 44px;
		background: var(--text);
		color: #17110a;
		font-weight: 600;
		font-size: 15px;
		padding: 12px 26px;
		border-radius: 11px;
		transition: 0.15s;
		text-decoration: none;
	}
	.btn-play:hover {
		transform: translateY(-1px);
		box-shadow: 0 8px 24px rgba(244, 237, 227, 0.15);
	}
	.hero-actions :global(.btn) {
		min-height: 44px;
		padding: 12px 20px;
		font-size: 15px;
		border-radius: 11px;
	}
	.hero-actions :global(.btn.active) {
		border-color: rgba(242, 169, 59, 0.4);
		color: var(--accent);
		background: rgba(242, 169, 59, 0.1);
	}
	.hero-poster {
		display: block;
		aspect-ratio: 2 / 3;
		border-radius: 16px;
		overflow: hidden;
		position: relative;
		box-shadow: 0 24px 60px rgba(0, 0, 0, 0.55), 0 0 0 1px var(--line);
		background: linear-gradient(160deg, #4a3016 0%, #1e1207 55%, #0d0a06 100%);
	}
	.hero-poster img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.hero-poster .art {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: flex-end;
		padding: 22px;
		font-family: var(--display);
		font-weight: 800;
		font-size: 28px;
		line-height: 1.05;
		background: radial-gradient(90% 60% at 30% 20%, rgba(242, 169, 59, 0.35), transparent 60%);
	}
	.none-title {
		font-family: var(--display);
		font-size: 24px;
		font-weight: 600;
		margin-bottom: 6px;
	}
	.none-text {
		color: var(--muted);
		max-width: 46ch;
	}

	/* Filtros y navegación, abajo del hero. */
	.controls {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 22px;
		max-width: 1180px;
		margin: 0 auto;
		padding: 0 28px;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		flex-wrap: wrap;
	}
	.chips {
		display: flex;
		gap: 8px;
	}
	.chip {
		min-height: 36px;
		padding: 6px 14px;
		border-radius: 99px;
		border: 1px solid var(--line);
		background: rgba(16, 12, 9, 0.55);
		color: var(--muted);
		font-size: 13px;
		font-weight: 500;
		transition: 0.15s;
	}
	.chip:hover {
		color: var(--text);
		background: var(--surface);
	}
	.chip.on {
		color: var(--text);
		background: var(--surface-2);
		border-color: #4d3a22;
	}
	.nav {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.arrow {
		width: 40px;
		height: 40px;
		border-radius: 50%;
		border: 1px solid var(--line);
		background: rgba(16, 12, 9, 0.55);
		color: var(--text);
		font-size: 22px;
		line-height: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: 0.15s;
	}
	.arrow:hover {
		background: var(--surface-2);
	}
	.dots {
		display: flex;
		gap: 2px;
		align-items: center;
	}
	/* El punto es pequeño; el área táctil (40px) la da el botón entero. */
	.dot {
		position: relative;
		width: 24px;
		height: 40px;
	}
	.dot::after {
		content: '';
		position: absolute;
		top: 50%;
		left: 50%;
		width: 8px;
		height: 8px;
		margin: -4px 0 0 -4px;
		border-radius: 50%;
		background: var(--off);
		transition: 0.2s;
	}
	.dot.on::after {
		width: 22px;
		margin-left: -11px;
		border-radius: 99px;
		background: var(--accent);
	}
	.dot:hover::after {
		background: var(--muted);
	}
	.dot.on:hover::after {
		background: var(--accent);
	}

	@media (max-width: 900px) {
		.hero {
			padding: 36px 16px 100px;
			min-height: 0;
		}
		.hero-inner {
			grid-template-columns: 1fr;
			gap: 20px;
			min-height: 0;
		}
		.hero-poster {
			display: none;
		}
		.controls {
			padding: 0 16px;
			bottom: 14px;
		}
		.arrow {
			display: none;
		}
	}
	@media (max-width: 480px) {
		.chip {
			padding: 6px 11px;
			font-size: 12.5px;
		}
	}
</style>
