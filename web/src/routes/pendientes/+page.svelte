<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { confirmItem, deleteItem, listPending } from '$lib/api/client';
	import CandidateCard from '$lib/components/CandidateCard.svelte';
	import StateBlock from '$lib/components/StateBlock.svelte';
	import type { Candidate, PendingItem } from '$lib/api/types';
	import { displayTitle, errorMessage, friendlyIdentifyError } from '$lib/format';

	let items = $state<PendingItem[]>([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);
	// Precargado con el título limpio de cada item (Fase 0 tarea 0.4: el
	// dato ya se conoce; obligar a reescribirlo a mano era fricción sin
	// motivo). El usuario puede editarlo antes de pulsar "Usar este título".
	let manualTitles = $state<Record<number, string>>({});
	let busyIds = $state(new Set<number>());
	let errors = $state<Record<number, string>>({});
	// Brief M10c §3: descartar un "Por confirmar" que no es un título real.
	let confirmingDiscard = $state<number | null>(null);

	async function load() {
		loading = true;
		loadError = null;
		try {
			const list = await listPending();
			items = list;
			manualTitles = Object.fromEntries(list.map((it) => [it.id, displayTitle(it)]));
		} catch (err) {
			loadError = errorMessage(err, 'No se pudieron cargar los pendientes');
		} finally {
			loading = false;
		}
		// Un enlace "Identificar" desde la ficha llega con #item-{id}.
		await tick();
		if (location.hash.startsWith('#item-')) {
			document.getElementById(location.hash.slice(1))?.scrollIntoView({ block: 'center' });
		}
	}
	onMount(load);

	// Fase 0 — diagnóstico integral: `error` (M3 lo intentó y TMDB falló —
	// típicamente una clave inválida) se trata igual que `review` (mismas
	// acciones: título manual, dejar sin identificar, descartar); la cola de
	// M3 sigue reintentándolo sola si la causa era transitoria.
	const review = $derived(items.filter((it) => it.identify_status === 'review' || it.identify_status === 'error'));
	const junk = $derived(items.filter((it) => it.identify_status === 'junk'));
	// pending es lo único puramente transitorio: todavía no le ha tocado
	// turno a la cola de M3. Se cuenta para no esconder que existe.
	const inProgressCount = $derived(items.length - review.length - junk.length);

	async function resolve(item: PendingItem, body: { tmdb_id: number; kind: 'movie' | 'tv' } | { title: string }) {
		if (busyIds.has(item.id)) return;
		busyIds.add(item.id);
		delete errors[item.id];
		try {
			await confirmItem(item.id, body);
			items = items.filter((it) => it.id !== item.id);
			invalidateAll(); // refresca la píldora "N por confirmar" de la topbar
		} catch (err) {
			errors[item.id] = errorMessage(err, 'No se pudo confirmar');
		} finally {
			busyIds.delete(item.id);
		}
	}

	function chooseCandidate(item: PendingItem, candidate: Candidate) {
		resolve(item, { tmdb_id: candidate.tmdb_id, kind: candidate.kind });
	}

	function confirmManual(item: PendingItem) {
		const title = (manualTitles[item.id] ?? '').trim();
		if (!title) return;
		resolve(item, { title });
	}

	// Fase 0 tarea 0.4: "dejarlo como está" es una acción real, no un limbo:
	// confirma con el nombre de archivo limpio de siempre (catalog.
	// ApplyManualConfirm); el item sigue mostrando "Sin identificar" y ya es
	// reproducible.
	function leaveUnidentified(item: PendingItem) {
		resolve(item, { title: displayTitle(item) });
	}

	async function discard(item: PendingItem) {
		if (busyIds.has(item.id)) return;
		confirmingDiscard = null;
		busyIds.add(item.id);
		delete errors[item.id];
		try {
			await deleteItem(item.id);
			items = items.filter((it) => it.id !== item.id);
			invalidateAll();
		} catch (err) {
			errors[item.id] = errorMessage(err, 'No se pudo descartar');
		} finally {
			busyIds.delete(item.id);
		}
	}
</script>

<svelte:head>
	<title>Por confirmar — GALA</title>
</svelte:head>

<div class="page">
	<header>
		<h1>Por confirmar</h1>
		<p class="muted">
			Títulos que GALA no pudo identificar solo con el nombre del archivo. Elige el candidato correcto o
			escribe el título a mano.
		</p>
	</header>

	{#if loading}
		<div class="list" aria-busy="true" aria-label="Cargando pendientes">
			{#each { length: 2 } as _, i (i)}
				<div class="skeleton item-skel"></div>
			{/each}
		</div>
	{:else if loadError}
		<StateBlock compact tone="error" title="No se pudo cargar" text={loadError} actionLabel="Reintentar" onAction={load} />
	{:else if review.length === 0}
		<StateBlock
			compact
			title="Todo está identificado"
			text={inProgressCount > 0
				? `${inProgressCount} ${inProgressCount === 1 ? 'título más se está procesando' : 'títulos más se están procesando'} en segundo plano.`
				: 'No hay nada por confirmar ahora mismo.'}
			actionLabel="Volver a la galería"
			actionHref="/"
		/>
	{:else}
		{#if inProgressCount > 0}
			<p class="muted note">
				Además, {inProgressCount}
				{inProgressCount === 1 ? 'título sigue procesándose' : 'títulos siguen procesándose'} en segundo plano (se
				reintentan solos).
			</p>
		{/if}
		<div class="list">
			{#each review as item (item.id)}
				<div class="item" id="item-{item.id}">
					<h2>{displayTitle(item)}</h2>
					{#if item.identify_status === 'error'}
						<p class="id-error">
							No se pudo consultar TMDB para este título: {friendlyIdentifyError(item.identify_note) ??
								'sin conexión por ahora'}
						</p>
					{/if}
					{#if errors[item.id]}<p class="error" role="alert">{errors[item.id]}</p>{/if}

					{#if item.candidates && item.candidates.length > 0}
						<div class="candidates">
							{#each item.candidates as candidate (candidate.tmdb_id)}
								<CandidateCard
									{candidate}
									disabled={busyIds.has(item.id)}
									onChoose={(c) => chooseCandidate(item, c)}
								/>
							{/each}
						</div>
					{:else if item.identify_status === 'review'}
						<p class="muted">TMDB no propuso ningún candidato para este nombre. Escribe el título a mano.</p>
					{/if}

					<div class="manual">
						<input
							type="text"
							aria-label="Título correcto de {displayTitle(item)}"
							placeholder="O escribe el título correcto…"
							disabled={busyIds.has(item.id)}
							bind:value={manualTitles[item.id]}
							onkeydown={(e) => e.key === 'Enter' && confirmManual(item)}
						/>
						<button
							class="btn btn-secondary"
							disabled={busyIds.has(item.id) || !(manualTitles[item.id] ?? '').trim()}
							onclick={() => confirmManual(item)}
						>
							Usar este título
						</button>
					</div>

					<div class="foot">
						{#if confirmingDiscard === item.id}
							<span class="confirm" role="alertdialog" aria-label="Confirmar descarte">
								¿Descartar «{displayTitle(item)}»? Se quita de tu galería; el archivo no se toca.
								<button class="btn btn-quiet-danger btn-sm" disabled={busyIds.has(item.id)} onclick={() => discard(item)}>
									Sí, descartar
								</button>
								<button class="btn btn-secondary btn-sm" onclick={() => (confirmingDiscard = null)}>Cancelar</button>
							</span>
						{:else}
							<button
								class="btn btn-secondary btn-sm"
								title="Ya se puede reproducir; solo se queda sin carátula ni datos de TMDB"
								disabled={busyIds.has(item.id)}
								onclick={() => leaveUnidentified(item)}
							>
								Dejar sin identificar
							</button>
							<button
								class="btn btn-quiet-danger btn-sm"
								title="No es una película ni una serie real"
								disabled={busyIds.has(item.id)}
								onclick={() => (confirmingDiscard = item.id)}
							>
								Descartar
							</button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}

	{#if !loading && junk.length > 0}
		<section class="discarded">
			<h2>Descartados <span class="n">{junk.length}</span></h2>
			<p class="muted">
				GALA detectó que estos nombres no son títulos reales (extras, muestras, vídeos sin información útil) y
				los descartó solo. Es solo lectura.
			</p>
			<ul>
				{#each junk as item (item.id)}
					<li>{item.title}</li>
				{/each}
			</ul>
		</section>
	{/if}
</div>

<style>
	.page {
		max-width: 900px;
		margin: 0 auto;
		padding: 34px 28px 60px;
	}
	header h1 {
		font-family: var(--display);
		font-size: 28px;
		font-weight: 800;
		margin-bottom: 8px;
	}
	.muted {
		color: var(--muted);
		font-size: 14px;
	}
	.note {
		margin: 18px 0 0;
	}
	.error {
		color: var(--bad);
		font-size: 13.5px;
		margin-bottom: 10px;
	}
	.id-error {
		margin-bottom: 12px;
		color: var(--warn);
		font-size: 14px;
	}
	.list {
		display: flex;
		flex-direction: column;
		gap: 24px;
		margin-top: 24px;
	}
	.item-skel {
		height: 260px;
	}
	.item {
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.2);
		padding: 18px 20px;
		scroll-margin-top: 90px;
	}
	.item h2 {
		font-family: var(--display);
		font-size: 18px;
		font-weight: 600;
		margin-bottom: 12px;
		overflow-wrap: anywhere;
	}
	.candidates {
		display: flex;
		gap: 12px;
		flex-wrap: wrap;
	}
	.manual {
		display: flex;
		gap: 10px;
		margin-top: 16px;
	}
	.manual input {
		flex: 1;
		min-width: 0;
		min-height: 40px;
		background: var(--bg);
		border: 1px solid var(--line);
		border-radius: 10px;
		padding: 9px 12px;
		color: var(--text);
		font-size: 14px;
	}
	.manual input:focus {
		outline: none;
		border-color: var(--accent);
	}
	.foot {
		margin-top: 14px;
		padding-top: 14px;
		border-top: 1px dashed var(--line);
		display: flex;
		gap: 10px;
		align-items: center;
		flex-wrap: wrap;
	}
	.confirm {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
		font-size: 13.5px;
		color: var(--text);
	}
	.discarded {
		margin-top: 44px;
		padding-top: 24px;
		border-top: 1px solid var(--line);
	}
	.discarded h2 {
		font-family: var(--display);
		font-size: 17px;
		font-weight: 600;
		margin-bottom: 6px;
	}
	.discarded .n {
		font-family: var(--mono);
		font-size: 12px;
		color: var(--muted);
		font-weight: 400;
		margin-left: 6px;
	}
	.discarded ul {
		list-style: none;
		margin-top: 12px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.discarded li {
		font-size: 13.5px;
		color: var(--muted);
		overflow-wrap: anywhere;
	}

	@media (max-width: 900px) {
		.page {
			padding: 20px 16px 60px;
		}
		.candidates :global(.candidate) {
			width: calc(33.33% - 8px);
			min-width: 104px;
		}
	}
	@media (max-width: 480px) {
		.manual {
			flex-direction: column;
		}
		.manual :global(.btn) {
			width: 100%;
		}
	}
</style>
