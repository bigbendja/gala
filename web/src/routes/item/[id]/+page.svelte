<script lang="ts">
	import { imgUrl, setWished } from '$lib/api/client';
	import SourceStack from '$lib/components/SourceStack.svelte';
	import { displayTitle, isUnidentified } from '$lib/format';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const item = $derived(data.item);

	const backdrop = $derived(item ? imgUrl(item.backdrop_path) : null);
	const poster = $derived(item ? imgUrl(item.poster_path) : null);
	const title = $derived(item ? displayTitle(item) : '');
	const unidentified = $derived(item ? isUnidentified(item) : false);

	// Brief M10a-fix B2/C: "+ Mi lista" ahora es funcional. Estado local
	// optimista (arranca con el wished real del item) para no esperar una
	// vuelta de red antes de reflejar el clic.
	let wished = $state(false);
	let wishPending = $state(false);
	$effect(() => {
		wished = item?.wished ?? false;
	});

	async function toggleWished() {
		if (!item || wishPending) return;
		const next = !wished;
		wishPending = true;
		try {
			await setWished(item.id, next);
			wished = next;
		} catch {
			// Se ignora: el botón se queda como estaba, el usuario puede reintentar.
		} finally {
			wishPending = false;
		}
	}
</script>

<svelte:head>
	<title>{item ? `${title} — GALA` : 'GALA'}</title>
</svelte:head>

{#if data.itemError}
	<div class="state-page">
		<p>No se pudo cargar esta ficha.</p>
		<p class="muted">{data.itemError}</p>
	</div>
{:else if item}
	<section class="detail">
		{#if backdrop}
			<div class="backdrop" style:background-image="url({backdrop})"></div>
		{/if}
		<div class="detail-inner">
			<a class="poster" href={poster ?? undefined} aria-hidden={!poster}>
				{#if poster}
					<img src={poster} alt="" />
				{:else}
					<div class="art"><b>{title}</b></div>
				{/if}
			</a>

			<div class="info">
				<h1>{title}</h1>
				{#if unidentified}
					<p class="unidentified-tag">Sin identificar</p>
				{/if}
				{#if item.original_title && item.original_title !== item.title}
					<p class="original">{item.original_title}</p>
				{/if}
				<div class="meta">
					{#if item.rating_cert}<span class="cert">{item.rating_cert}+</span>{/if}
					{#if item.year}<span>{item.year}</span>{/if}
					{#if item.genres.length}<span>{item.genres.join(' · ')}</span>{/if}
				</div>
				{#if item.overview}<p class="overview">{item.overview}</p>{/if}

				<div class="actions">
					<button class="btn-play" disabled title="Reproducir: llega en M10b">▶ Reproducir</button>
					<button class="btn-ghost" class:active={wished} disabled={wishPending} onclick={toggleWished}>
						{wished ? '✓ En tu lista' : '＋ Mi lista'}
					</button>
				</div>

				{#if item.sources && item.sources.length > 0}
					<SourceStack sources={item.sources} />
				{:else}
					<p class="muted no-sources">Sin fuentes todavía.</p>
				{/if}

				{#if item.kind === 'series'}
					<div class="seasons">
						{#if data.seasons.length === 0}
							<p class="muted">Todavía no hay temporadas catalogadas de esta serie.</p>
						{:else}
							{#each data.seasons as season (season.id)}
								<div class="season">
									<h3>
										{season.season_number ? `Temporada ${season.season_number}` : displayTitle(season)}
									</h3>
									{#if data.episodesBySeason[season.id]?.length}
										<ol class="episodes">
											{#each data.episodesBySeason[season.id] as ep (ep.id)}
												<li>
													<a href="/item/{ep.id}">
														<span class="ep-num">{ep.episode_number ?? '—'}</span>
														<span class="ep-title">{displayTitle(ep)}</span>
													</a>
												</li>
											{/each}
										</ol>
									{:else}
										<p class="muted">Sin episodios catalogados todavía.</p>
									{/if}
								</div>
							{/each}
						{/if}
					</div>
				{/if}
			</div>
		</div>
	</section>
{/if}

<style>
	.detail {
		position: relative;
		padding: 48px 28px 60px;
		isolation: isolate;
	}
	.backdrop {
		position: absolute;
		inset: 0;
		z-index: -2;
		background-size: cover;
		background-position: center 20%;
		opacity: 0.35;
		-webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.75), transparent 90%);
		mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.75), transparent 90%);
	}
	.detail::before {
		content: '';
		position: absolute;
		inset: 0;
		z-index: -3;
		background: radial-gradient(120% 90% at 78% 0%, #3d2a14 0%, #221709 42%, var(--bg) 78%);
	}
	.detail-inner {
		max-width: 1180px;
		margin: 0 auto;
		display: grid;
		grid-template-columns: 280px 1fr;
		gap: 40px;
	}
	.poster {
		display: block;
		aspect-ratio: 2 / 3;
		border-radius: 16px;
		overflow: hidden;
		position: relative;
		box-shadow: 0 24px 60px rgba(0, 0, 0, 0.55), 0 0 0 1px var(--line);
		background: linear-gradient(160deg, #4a3016 0%, #1e1207 55%, #0d0a06 100%);
	}
	.poster img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.poster .art {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: flex-end;
		padding: 22px;
		font-family: var(--display);
		font-weight: 800;
		font-size: 24px;
		line-height: 1.1;
		background: radial-gradient(90% 60% at 30% 20%, rgba(242, 169, 59, 0.35), transparent 60%);
	}
	.info h1 {
		font-family: var(--display);
		font-size: clamp(28px, 4vw, 44px);
		font-weight: 800;
		line-height: 1.05;
		letter-spacing: -0.01em;
		margin-bottom: 6px;
	}
	.unidentified-tag {
		display: inline-block;
		font-family: var(--mono);
		font-size: 12px;
		color: var(--muted);
		font-style: italic;
		margin-bottom: 8px;
	}
	.original {
		color: var(--muted);
		font-size: 14px;
		margin-bottom: 10px;
	}
	.meta {
		display: flex;
		gap: 14px;
		align-items: center;
		color: var(--muted);
		font-size: 14px;
		margin: 10px 0 14px;
		flex-wrap: wrap;
	}
	.meta .cert {
		border: 1px solid var(--line);
		border-radius: 5px;
		padding: 1px 7px;
		font-size: 12px;
	}
	.overview {
		color: var(--muted);
		max-width: 640px;
		margin-bottom: 22px;
	}
	.actions {
		display: flex;
		gap: 12px;
		margin-bottom: 8px;
	}
	.btn-play {
		display: flex;
		align-items: center;
		gap: 10px;
		background: var(--text);
		color: #17110a;
		font-weight: 600;
		font-size: 15px;
		padding: 13px 26px;
		border-radius: 11px;
	}
	.btn-play:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.btn-ghost {
		padding: 13px 20px;
		border-radius: 11px;
		border: 1px solid var(--line);
		color: var(--text);
		background: rgba(255, 255, 255, 0.03);
		font-weight: 500;
	}
	.btn-ghost.active {
		border-color: rgba(242, 169, 59, 0.4);
		color: var(--accent);
		background: rgba(242, 169, 59, 0.1);
	}
	.btn-ghost:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.no-sources {
		margin-top: 20px;
	}
	.seasons {
		margin-top: 26px;
		max-width: 640px;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}
	.season h3 {
		font-family: var(--display);
		font-size: 16px;
		font-weight: 600;
		margin-bottom: 8px;
	}
	.episodes {
		list-style: none;
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.2);
		overflow: hidden;
	}
	.episodes li {
		border-bottom: 1px solid rgba(51, 38, 25, 0.5);
	}
	.episodes li:last-child {
		border-bottom: none;
	}
	.episodes a {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 16px;
		text-decoration: none;
		color: inherit;
		font-size: 13.5px;
	}
	.episodes a:hover {
		background: var(--surface);
	}
	.ep-num {
		font-family: var(--mono);
		color: var(--muted);
		font-size: 12px;
		flex-shrink: 0;
		width: 1.5em;
		text-align: right;
	}
	.state-page {
		max-width: 560px;
		margin: 15vh auto;
		text-align: center;
		padding: 0 28px;
	}
	.muted {
		color: var(--muted);
		font-size: 14px;
	}

	@media (max-width: 700px) {
		.detail-inner {
			grid-template-columns: 1fr;
		}
		.poster {
			max-width: 220px;
		}
	}
</style>
