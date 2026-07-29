<script lang="ts">
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { ApiError, confirmItem, deleteItem } from '$lib/api/client';
	import CandidateCard from '$lib/components/CandidateCard.svelte';
	import type { Candidate, PendingItem } from '$lib/api/types';
	import { displayTitle, friendlyIdentifyError } from '$lib/format';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Semilla intencional (untrack): a partir de aquí la lista vive de las
	// confirmaciones del usuario (se quitan items al resolverlos), no de
	// recargas de `data` — mismo patrón que el resto de la app.
	let items = $state<PendingItem[]>(untrack(() => data.items));
	// Precargado con el título limpio de cada item (Fase 0 tarea 0.4: el
	// dato ya se conoce y se muestra en el <h2> de arriba; obligar a
	// reescribirlo a mano era fricción sin motivo). El usuario puede editarlo
	// antes de pulsar "Usar este título", o dejarlo tal cual.
	let manualTitles = $state<Record<number, string>>(
		untrack(() => Object.fromEntries(data.items.map((it) => [it.id, displayTitle(it)])))
	);
	let busyIds = $state(new Set<number>());
	let errors = $state<Record<number, string>>({});
	// Brief M10c §3: descartar un "Por confirmar" que no es un título real
	// (cierra el hueco anotado en M10b/ISSUES-MVP2.md — ahora sí hay
	// endpoint: DELETE /api/v1/items/{id}).
	let confirmingDiscard = $state<number | null>(null);

	// Fase 0 — diagnóstico integral: `error` (M3 lo intentó y TMDB falló —
	// típicamente una clave inválida, Brief M10c §2) se trataba igual que
	// `pending` (transitorio, "se reintenta solo"), pero un error de clave
	// inválida NUNCA se resuelve solo reintentando cada 5 min: el usuario
	// quedaba con el mismo item atascado para siempre, sin ninguna acción
	// disponible salvo arreglar la clave en Ajustes primero. Se trata igual
	// que `review` (candidatos vacíos, pero con las mismas acciones:
	// título manual, dejar sin identificar, descartar) — la cola de M3
	// sigue reintentándolo sola en segundo plano igualmente si la causa era
	// transitoria; esto solo añade una salida manual para cuando no lo es.
	const review = $derived(items.filter((it) => it.identify_status === 'review' || it.identify_status === 'error'));
	const junk = $derived(items.filter((it) => it.identify_status === 'junk'));
	// pending es lo único puramente transitorio que queda: todavía no le ha
	// tocado turno a la cola de M3 (Brief M10b §3 no pide mostrarlo como
	// algo a resolver a mano; se anota su número para no esconder que
	// existe, sin ofrecer acciones sobre él).
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
			errors[item.id] = err instanceof ApiError ? err.message : 'No se pudo confirmar. Inténtalo de nuevo.';
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

	// Fase 0 tarea 0.4: "dejarlo como está" tiene que ser una acción real, no
	// un limbo. Usa el mismo camino que el título manual (catalog.
	// ApplyManualConfirm ya existe y hace justo esto: confirmed=true,
	// identify_status=ok, sin tocar tmdb_id) pero con el nombre de archivo
	// limpio de siempre, ignorando lo que haya escrito el usuario en el
	// campo de título manual. El item sigue mostrando "Sin identificar" en
	// la galería (isUnidentified mira tmdb_id, no confirmed) y ya es
	// reproducible — de hecho ya lo era antes de confirmar, /play nunca
	// exigió `confirmed`.
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
			errors[item.id] = err instanceof ApiError ? err.message : 'No se pudo descartar. Inténtalo de nuevo.';
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
			Títulos que GALA no pudo identificar solo con el nombre de archivo. Elige el candidato
			correcto, o escribe el título a mano.
		</p>
	</header>

	{#if data.error}
		<p class="error">No se pudo cargar: {data.error}</p>
	{:else if review.length === 0}
		<div class="empty">
			<p>No hay nada por confirmar ahora mismo.</p>
			{#if inProgressCount > 0}
				<p class="muted">
					{inProgressCount}
					{inProgressCount === 1 ? 'item' : 'items'} más se están procesando en segundo plano.
				</p>
			{/if}
		</div>
	{:else}
		{#if inProgressCount > 0}
			<p class="muted note">
				Además, {inProgressCount}
				{inProgressCount === 1 ? 'item sigue procesándose' : 'items siguen procesándose'} en segundo
				plano (se reintentan solos).
			</p>
		{/if}
		<div class="list">
			{#each review as item (item.id)}
				<div class="item">
					<h2>{displayTitle(item)}</h2>
					{#if item.identify_status === 'error'}
						<p class="muted id-error">
							GALA no pudo consultar TMDB para este item: {friendlyIdentifyError(item.identify_note)}
						</p>
					{/if}
					{#if errors[item.id]}<p class="error">{errors[item.id]}</p>{/if}

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
					{:else}
						<p class="muted">TMDB no propuso ningún candidato para este nombre.</p>
					{/if}

					<div class="manual">
						<input
							type="text"
							placeholder="O escribe el título correcto…"
							disabled={busyIds.has(item.id)}
							bind:value={manualTitles[item.id]}
							onkeydown={(e) => e.key === 'Enter' && confirmManual(item)}
						/>
						<button
							class="btn-ghost"
							disabled={busyIds.has(item.id) || !(manualTitles[item.id] ?? '').trim()}
							onclick={() => confirmManual(item)}
						>
							Usar este título
						</button>
					</div>

					<div class="leave-row">
						<button class="btn-link" disabled={busyIds.has(item.id)} onclick={() => leaveUnidentified(item)}>
							Dejar sin identificar — ya es reproducible, solo sin datos de TMDB
						</button>
					</div>

					<div class="discard-row">
						{#if confirmingDiscard === item.id}
							<span class="confirm">
								¿Descartar "{displayTitle(item)}"? No es una película/serie real.
								<button class="btn-ghost small" disabled={busyIds.has(item.id)} onclick={() => discard(item)}>
									Sí, descartar
								</button>
								<button class="btn-ghost small" onclick={() => (confirmingDiscard = null)}>Cancelar</button>
							</span>
						{:else}
							<button
								class="btn-link-discard"
								disabled={busyIds.has(item.id)}
								onclick={() => (confirmingDiscard = item.id)}
							>
								No es un título real — descartar
							</button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}

	{#if junk.length > 0}
		<section class="discarded">
			<h2>Descartados ({junk.length})</h2>
			<p class="muted">
				GALA detectó que estos nombres no son títulos reales (extras, muestras, vídeos sin
				información útil) y los descartó automáticamente. Solo lectura.
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
		margin-bottom: 18px;
	}
	.error {
		color: var(--bad);
		font-size: 13.5px;
	}
	.id-error {
		margin-bottom: 12px;
		color: var(--warn);
	}
	.empty {
		margin-top: 40px;
		text-align: center;
	}
	.list {
		display: flex;
		flex-direction: column;
		gap: 28px;
		margin-top: 24px;
	}
	.item {
		border: 1px solid var(--line);
		border-radius: var(--r);
		background: rgba(0, 0, 0, 0.2);
		padding: 18px 20px;
	}
	.item h2 {
		font-family: var(--display);
		font-size: 17px;
		font-weight: 600;
		margin-bottom: 12px;
	}
	.candidates {
		display: flex;
		gap: 12px;
		flex-wrap: wrap;
	}
	.leave-row {
		margin-top: 10px;
	}
	.btn-link {
		color: var(--accent);
		font-size: 12.5px;
		opacity: 0.85;
		transition: 0.15s;
	}
	.btn-link:hover:not(:disabled) {
		opacity: 1;
		text-decoration: underline;
	}
	.btn-link:disabled {
		opacity: 0.4;
		cursor: default;
	}
	.discard-row {
		margin-top: 14px;
		padding-top: 14px;
		border-top: 1px dashed var(--line);
	}
	.btn-link-discard {
		color: var(--bad);
		font-size: 12.5px;
		opacity: 0.75;
		transition: 0.15s;
	}
	.btn-link-discard:hover:not(:disabled) {
		opacity: 1;
		text-decoration: underline;
	}
	.btn-link-discard:disabled {
		opacity: 0.4;
		cursor: default;
	}
	.discard-row .confirm {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		font-size: 13px;
		color: var(--warn);
	}
	.btn-ghost.small {
		padding: 5px 12px;
		font-size: 12.5px;
	}
	.manual {
		display: flex;
		gap: 10px;
		margin-top: 16px;
	}
	.manual input {
		flex: 1;
		background: var(--bg);
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 9px 12px;
		color: var(--text);
		font-size: 13.5px;
	}
	.manual input:focus {
		outline: none;
		border-color: var(--accent);
	}
	.btn-ghost {
		padding: 9px 16px;
		border-radius: 8px;
		border: 1px solid var(--line);
		color: var(--text);
		background: rgba(255, 255, 255, 0.03);
		font-weight: 500;
		font-size: 13.5px;
		white-space: nowrap;
	}
	.btn-ghost:disabled {
		opacity: 0.5;
		cursor: default;
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
	.discarded ul {
		list-style: none;
		margin-top: 12px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.discarded li {
		font-family: var(--mono);
		font-size: 12.5px;
		color: var(--muted);
	}

	@media (max-width: 480px) {
		.page {
			padding: 20px 16px 60px;
		}
		.manual {
			flex-direction: column;
		}
		.manual .btn-ghost {
			width: 100%;
			justify-content: center;
		}
	}
</style>
