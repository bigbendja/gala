<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { deleteItem, imgUrl, setWished } from '$lib/api/client';
	import type { LanSession } from '$lib/api/types';
	import CastRail from '$lib/components/CastRail.svelte';
	import EpisodeRow from '$lib/components/EpisodeRow.svelte';
	import FileSpecs from '$lib/components/FileSpecs.svelte';
	import SimilarRail from '$lib/components/SimilarRail.svelte';
	import SourceStack from '$lib/components/SourceStack.svelte';
	import StateBlock from '$lib/components/StateBlock.svelte';
	import {
		displayTitle,
		episodeCode,
		errorMessage,
		formatRating,
		formatRuntime,
		isUnidentified
	} from '$lib/format';
	import { nextEpisode, seriesStatusText, seriesSummary } from '$lib/series';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const item = $derived(data.item);
	const details = $derived(data.details);

	const isLocal = $derived((page.data as { session?: LanSession }).session?.local !== false);

	const isEpisode = $derived(item?.kind === 'episode');
	const isSeries = $derived(item?.kind === 'series');
	const backdrop = $derived(item ? imgUrl(item.backdrop_path) : null);
	const poster = $derived(item ? imgUrl(item.poster_path) : null);
	const title = $derived(item ? displayTitle(item) : '');
	const unidentified = $derived(item ? isUnidentified(item) : false);
	const code = $derived(item && isEpisode ? episodeCode(item) : '');

	// Una serie no tiene fuente propia: su estado y su "Reproducir" salen de
	// sus episodios (Sistema de Diseño GALA §3.1).
	const orderedEpisodes = $derived(data.seasons.flatMap((s) => data.episodesBySeason[s.id] ?? []));
	const summary = $derived(seriesSummary(data.episodesBySeason));
	const next = $derived(isSeries ? nextEpisode(orderedEpisodes) : null);
	const playHref = $derived(
		!item ? '' : isSeries ? (next ? `/ver/${next.id}` : '') : item.source_count > 0 || item.sources?.length ? `/ver/${item.id}` : ''
	);
	const canPlay = $derived(playHref !== '');
	const playText = $derived.by(() => {
		if (!isSeries) return '▶ Reproducir';
		if (!next) return '▶ Reproducir';
		const started = next.progress && !next.progress.watched && next.progress.position_s > 0;
		return `${started ? '▶ Continuar' : '▶ Empezar'} ${episodeCode(next, seasonOf(next.parent_id))}`.trim();
	});
	function seasonOf(parentId: number | undefined): number | undefined {
		return data.seasons.find((s) => s.id === parentId)?.season_number;
	}

	// Ficha enriquecida: solo para lo identificado (un título sin identificar
	// no tiene reparto ni similares; sus secciones no se muestran vacías).
	const rich = $derived(!!details?.has_details && !unidentified);
	const backLink = $derived.by(() => {
		if (!item) return null;
		if (isEpisode) return item.series_id && item.series_title ? { href: `/item/${item.series_id}`, text: `← ${item.series_title}` } : null;
		if (item.kind === 'movie') return { href: '/peliculas', text: '← Volver a Películas' };
		if (isSeries) return { href: '/series', text: '← Volver a Series' };
		return { href: '/', text: '← Volver a la galería' };
	});
	const originalLine = $derived.by(() => {
		if (!item) return '';
		const parts: string[] = [];
		if (item.original_title && item.original_title !== item.title) parts.push(`Título original: ${item.original_title}`);
		if (details?.countries?.length) parts.push(details.countries.slice(0, 2).join(' · '));
		return parts.join(' · ');
	});
	const runtime = $derived(formatRuntime(details?.runtime_min));
	const rating = $derived(formatRating(details?.vote_average));
	// El archivo de la fuente que se reproduce primero (la mejor analizada).
	const mainFile = $derived(details?.files?.[0] ?? null);
	const showSpecs = $derived(!isSeries && !!item && (mainFile !== null || !!item.added_at));

	let wished = $state(false);
	let wishPending = $state(false);
	$effect(() => {
		wished = item?.wished ?? false;
	});
	async function toggleWished() {
		if (!item || wishPending) return;
		const nextValue = !wished;
		wishPending = true;
		try {
			await setWished(item.id, nextValue);
			wished = nextValue;
		} catch {
			// el botón se queda como estaba; se puede reintentar
		} finally {
			wishPending = false;
		}
	}

	// Descartar un título sin identificar (solo el dueño, con confirmación).
	let confirmingDiscard = $state(false);
	let discardPending = $state(false);
	let discardError = $state<string | null>(null);
	async function discard() {
		if (!item || discardPending) return;
		discardPending = true;
		discardError = null;
		try {
			await deleteItem(item.id);
			await goto('/');
		} catch (err) {
			discardError = errorMessage(err, 'No se pudo descartar');
			discardPending = false;
		}
	}

	const identifyNote = $derived.by(() => {
		switch (item?.identify_status) {
			case 'pending':
				return 'GALA está identificando este título…';
			case 'review':
				return 'GALA no estuvo segura de qué título es. Puedes elegirlo tú.';
			case 'error':
				return 'No se pudo identificar. Puedes elegir el título tú.';
			case 'junk':
				return 'GALA lo detectó como un vídeo suelto (no una película ni serie) y no lo ofrece para identificar.';
			default:
				return 'Aún no está identificado: se enseña con el nombre del archivo.';
		}
	});
	const canIdentify = $derived(
		isLocal && (item?.identify_status === 'review' || item?.identify_status === 'error')
	);
</script>

<svelte:head>
	<title>{item ? `${title} — GALA` : 'GALA'}</title>
</svelte:head>

{#if data.itemError}
	<StateBlock
		tone="error"
		title="No se pudo cargar esta ficha"
		text={data.itemError}
		actionLabel="Reintentar"
		onAction={() => invalidateAll()}
	/>
{:else if item}
	<section class="hero">
		<div class="backdrop" class:has-img={!!backdrop} style:background-image={backdrop ? `url(${backdrop})` : undefined}></div>
		<div class="hero-grid" class:ep-layout={isEpisode}>
			<div class="art" class:still={isEpisode}>
				{#if poster}
					<img src={poster} alt="" />
				{:else}
					<div class="fallback"><b>{isEpisode ? code || title : title}</b></div>
				{/if}
			</div>

			<div class="h-info">
				{#if backLink}
					<a class="back" href={backLink.href}>{backLink.text}</a>
				{/if}
				<h1>{title}</h1>
				{#if isEpisode && code}
					<p class="code">{code}</p>
				{/if}
				{#if unidentified}
					<p class="unidentified-tag">Sin identificar</p>
				{/if}
				{#if originalLine}
					<p class="orig">{originalLine}</p>
				{/if}
				<div class="meta">
					{#if item.rating_cert}<span class="cert">{item.rating_cert}+</span>{/if}
					{#if item.year}<span>{item.year}</span>{/if}
					{#if runtime}<span>{runtime}</span>{/if}
					{#if rating}<span class="star" title="Nota de TMDB">★ {rating}</span>{/if}
					{#each item.genres as g (g)}<span class="genre">{g}</span>{/each}
				</div>
				{#if item.overview}<p class="synopsis">{item.overview}</p>{/if}

				{#if unidentified}
					<p class="identify-note">{identifyNote}</p>
				{/if}

				<div class="actions">
					{#if canPlay}
						<a class="btn-play" href={playHref}>{playText}</a>
					{:else}
						<span
							class="btn-play off"
							aria-disabled="true"
							title={isSeries ? 'Ninguno de sus episodios tiene fuente todavía' : 'Este título aún no tiene fuente'}
							>{playText}</span
						>
					{/if}
					{#if !isEpisode}
						<button class="btn-ic" class:active={wished} disabled={wishPending} onclick={toggleWished}>
							{wished ? '✓ En tu lista' : '＋ Mi lista'}
						</button>
					{/if}
					{#if unidentified && isLocal}
						{#if canIdentify}
							<a class="btn-ic" href="/pendientes#item-{item.id}">Identificar</a>
						{/if}
						{#if !confirmingDiscard}
							<button class="btn-ic danger" onclick={() => (confirmingDiscard = true)}>Descartar</button>
						{/if}
					{/if}
				</div>

				{#if confirmingDiscard}
					<div class="discard" role="alertdialog" aria-label="Confirmar descarte">
						<p>¿Descartar este título? Se quita de tu galería junto con sus fuentes; los archivos del disco no se tocan.</p>
						<div class="discard-actions">
							<button class="btn btn-quiet-danger btn-sm" disabled={discardPending} onclick={discard}>
								{discardPending ? 'Descartando…' : 'Sí, descartar'}
							</button>
							<button class="btn btn-secondary btn-sm" disabled={discardPending} onclick={() => (confirmingDiscard = false)}>
								Cancelar
							</button>
						</div>
						{#if discardError}<p class="err" role="alert">{discardError}</p>{/if}
					</div>
				{/if}

				{#if isSeries}
					<p class="series-status" class:none={summary.available === 0}>{seriesStatusText(summary)}</p>
				{:else if item.sources && item.sources.length > 0}
					<SourceStack sources={item.sources} />
				{:else}
					<p class="no-sources">Este título aún no tiene fuentes.</p>
				{/if}
			</div>
		</div>
	</section>

	<main class="body">
		{#if isSeries}
			<section class="section">
				<h2>Temporadas</h2>
				{#if data.seasons.length === 0}
					<StateBlock compact title="Sin temporadas todavía" text="Aparecerán cuando GALA identifique sus episodios." />
				{:else}
					<div class="seasons">
						{#each data.seasons as season (season.id)}
							{@const eps = data.episodesBySeason[season.id] ?? []}
							<div class="season">
								<h3>
									{season.season_number ? `Temporada ${season.season_number}` : displayTitle(season)}
									<span class="season-count">{eps.length} {eps.length === 1 ? 'episodio' : 'episodios'}</span>
								</h3>
								{#if eps.length > 0}
									<ol class="episodes">
										{#each eps as ep (ep.id)}
											<EpisodeRow episode={ep} seasonNumber={season.season_number} />
										{/each}
									</ol>
								{:else}
									<p class="muted">Sin episodios catalogados todavía.</p>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</section>
		{/if}

		{#if rich && details && details.cast.length > 0}
			<section class="section">
				<h2>Reparto</h2>
				<CastRail cast={details.cast} />
			</section>
		{/if}

		{#if (rich && details && details.crew.length > 0) || showSpecs}
			<section class="section">
				<div class="twocol" class:single={!(rich && details && details.crew.length > 0) || !showSpecs}>
					{#if rich && details && details.crew.length > 0}
						<div>
							<h2>Equipo</h2>
							<div class="crew">
								{#each details.crew as group (group.role)}
									<div class="crew-item">
										<span class="role">{group.role}</span>
										<span class="name">{group.names.join(' · ')}</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
					{#if showSpecs}
						<div>
							<h2>Este archivo</h2>
							<FileSpecs file={mainFile} addedAt={item.added_at} />
						</div>
					{/if}
				</div>
			</section>
		{/if}

		{#if rich && details && details.similar.length > 0}
			<section class="section">
				<h2>Más como esto</h2>
				<SimilarRail items={details.similar} />
			</section>
		{/if}
	</main>
{/if}

<style>
	/* Hero con backdrop atmosférico (mockup de la Ficha): la imagen apaisada de
	   TMDB a toda la anchura, con degradado hacia el fondo para que el texto
	   se lea; sin backdrop, un degradado cálido sobrio. */
	.hero {
		position: relative;
		min-height: 560px;
		overflow: hidden;
		isolation: isolate;
	}
	.backdrop {
		position: absolute;
		inset: 0;
		z-index: -2;
		background: radial-gradient(120% 100% at 72% 15%, #3d2a14 0%, #1d1409 38%, #0d0a06 72%);
		background-size: cover;
		background-position: center 22%;
	}
	.backdrop.has-img {
		background-color: #0d0a06;
	}
	.backdrop::after {
		content: '';
		position: absolute;
		inset: 0;
		background:
			linear-gradient(90deg, var(--bg) 0%, rgba(16, 12, 9, 0.82) 38%, rgba(16, 12, 9, 0.28) 70%, transparent 100%),
			linear-gradient(0deg, var(--bg) 1%, transparent 52%);
	}
	.hero-grid {
		max-width: 1200px;
		margin: 0 auto;
		padding: 48px 28px 40px;
		display: grid;
		grid-template-columns: 260px minmax(0, 1fr);
		gap: 44px;
		align-items: end;
		position: relative;
	}
	.hero-grid.ep-layout {
		grid-template-columns: 380px minmax(0, 1fr);
	}
	.art {
		aspect-ratio: 2 / 3;
		border-radius: 14px;
		overflow: hidden;
		position: relative;
		box-shadow:
			0 30px 70px rgba(0, 0, 0, 0.65),
			0 8px 20px rgba(0, 0, 0, 0.4),
			0 0 0 1px var(--line);
		background: linear-gradient(160deg, #4a3016 0%, #1e1207 55%, #0d0a06 100%);
	}
	.art.still {
		aspect-ratio: 16 / 9;
	}
	.art img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.fallback {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: flex-end;
		padding: 18px;
		font-family: var(--display);
		font-weight: 800;
		font-size: 22px;
		line-height: 1.1;
		background: radial-gradient(90% 60% at 30% 20%, rgba(242, 169, 59, 0.3), transparent 60%);
	}
	.h-info {
		/* Un track de grid "1fr" sigue teniendo min-width:auto por defecto: sin
		   esto, el contenido sin ajuste de línea de SourceStack (rutas largas)
		   fuerza el track — y con él toda la página — más ancho que el viewport
		   en móvil, incluso ya colapsado a una sola columna. */
		min-width: 0;
	}
	.back {
		display: inline-flex;
		align-items: center;
		color: var(--muted);
		text-decoration: none;
		font-size: 13.5px;
		margin-bottom: 16px;
	}
	.back:hover {
		color: var(--text);
	}
	h1 {
		font-family: var(--display);
		font-size: clamp(34px, 4.4vw, 52px);
		font-weight: 800;
		line-height: 1.03;
		letter-spacing: -0.01em;
		margin-bottom: 6px;
		overflow-wrap: anywhere;
		text-shadow: 0 2px 24px rgba(0, 0, 0, 0.5);
	}
	.code {
		color: var(--accent);
		font-weight: 600;
		font-size: 14px;
		margin-bottom: 6px;
	}
	.unidentified-tag {
		display: inline-block;
		font-size: 13px;
		color: var(--muted);
		font-style: italic;
		margin-bottom: 8px;
	}
	.orig {
		color: var(--muted);
		font-size: 14px;
		margin-bottom: 14px;
	}
	.meta {
		display: flex;
		gap: 12px;
		align-items: center;
		flex-wrap: wrap;
		color: var(--muted);
		font-size: 14px;
		margin: 10px 0 16px;
	}
	.meta .cert {
		border: 1px solid var(--line);
		border-radius: 5px;
		padding: 1px 7px;
		font-size: 12px;
	}
	.meta .star {
		color: var(--accent);
		font-weight: 600;
	}
	.meta .genre {
		background: var(--surface-2);
		border-radius: 99px;
		padding: 3px 11px;
		font-size: 12.5px;
		color: #d9cab8;
	}
	.synopsis {
		max-width: 620px;
		color: #e0d2c0;
		font-size: 15.5px;
		margin-bottom: 22px;
	}
	.identify-note {
		color: var(--muted);
		max-width: 560px;
		font-size: 14px;
		margin-bottom: 18px;
	}
	.actions {
		display: flex;
		gap: 12px;
		margin-bottom: 22px;
		flex-wrap: wrap;
		align-items: center;
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
		padding: 12px 30px;
		border-radius: 11px;
		text-decoration: none;
		transition: 0.15s;
	}
	a.btn-play:hover {
		transform: translateY(-1px);
		box-shadow: 0 8px 24px rgba(244, 237, 227, 0.15);
	}
	.btn-play.off {
		opacity: 0.4;
		cursor: default;
	}
	.btn-ic {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-height: 46px;
		padding: 12px 20px;
		border-radius: 11px;
		border: 1px solid rgba(244, 237, 227, 0.2);
		background: rgba(255, 255, 255, 0.06);
		color: var(--text);
		font-weight: 500;
		font-size: 15px;
		text-decoration: none;
		transition: 0.15s;
	}
	.btn-ic:hover:not(:disabled) {
		background: rgba(255, 255, 255, 0.12);
	}
	.btn-ic.active {
		border-color: rgba(242, 169, 59, 0.4);
		color: var(--accent);
		background: rgba(242, 169, 59, 0.1);
	}
	.btn-ic.danger {
		color: var(--muted);
	}
	.btn-ic.danger:hover {
		color: var(--bad);
		border-color: rgba(225, 96, 79, 0.45);
	}
	.btn-ic:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.discard {
		margin-bottom: 16px;
		max-width: 560px;
		padding: 14px 16px;
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.35);
		font-size: 14px;
	}
	.discard-actions {
		display: flex;
		gap: 10px;
		margin-top: 12px;
	}
	.err {
		margin-top: 10px;
		color: var(--bad);
		font-size: 13.5px;
	}
	.no-sources,
	.series-status {
		color: var(--muted);
		font-size: 14px;
	}
	.series-status.none {
		color: var(--text);
	}

	/* Secciones del cuerpo. */
	.body {
		max-width: 1200px;
		margin: 0 auto;
		padding: 6px 28px 60px;
	}
	.section {
		margin-top: 40px;
	}
	.section h2 {
		font-family: var(--display);
		font-size: 21px;
		font-weight: 600;
		margin-bottom: 18px;
	}
	.seasons {
		display: flex;
		flex-direction: column;
		gap: 26px;
		max-width: 860px;
	}
	.season h3 {
		display: flex;
		align-items: baseline;
		gap: 12px;
		font-family: var(--display);
		font-size: 18px;
		font-weight: 600;
		margin-bottom: 10px;
	}
	.season-count {
		font-family: var(--body);
		font-size: 13px;
		font-weight: 400;
		color: var(--muted);
	}
	.episodes {
		list-style: none;
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.2);
		overflow: hidden;
	}
	.muted {
		color: var(--muted);
		font-size: 14px;
	}
	.twocol {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 40px;
	}
	.twocol.single {
		grid-template-columns: minmax(0, 560px);
	}
	.crew-item {
		display: flex;
		justify-content: space-between;
		gap: 16px;
		padding: 10px 0;
		border-bottom: 1px solid var(--line);
		font-size: 14px;
	}
	.crew-item .role {
		color: var(--muted);
		flex-shrink: 0;
	}
	.crew-item .name {
		font-weight: 500;
		text-align: right;
	}

	@media (max-width: 900px) {
		.hero {
			min-height: 0;
		}
		.hero-grid,
		.hero-grid.ep-layout {
			grid-template-columns: 1fr;
			gap: 22px;
			padding: 24px 16px 28px;
		}
		.art:not(.still) {
			width: 150px;
		}
		.backdrop::after {
			background:
				linear-gradient(90deg, rgba(16, 12, 9, 0.7) 0%, rgba(16, 12, 9, 0.35) 100%),
				linear-gradient(0deg, var(--bg) 4%, transparent 60%);
		}
		.body {
			padding: 4px 16px 48px;
		}
		.twocol,
		.twocol.single {
			grid-template-columns: 1fr;
			gap: 28px;
		}
	}
</style>
