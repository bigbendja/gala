<script lang="ts">
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { ApiError, confirmItem, deleteItem } from '$lib/api/client';
	import CandidateCard from '$lib/components/CandidateCard.svelte';
	import type { Candidate, PendingItem } from '$lib/api/types';
	import { displayTitle } from '$lib/format';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Semilla intencional (untrack): a partir de aquí la lista vive de las
	// confirmaciones del usuario (se quitan items al resolverlos), no de
	// recargas de `data` — mismo patrón que el resto de la app.
	let items = $state<PendingItem[]>(untrack(() => data.items));
	let manualTitles = $state<Record<number, string>>({});
	let busyIds = $state(new Set<number>());
	let errors = $state<Record<number, string>>({});
	// Brief M10c §3: descartar un "Por confirmar" que no es un título real
	// (cierra el hueco anotado en M10b/ISSUES-MVP2.md — ahora sí hay
	// endpoint: DELETE /api/v1/items/{id}).
	let confirmingDiscard = $state<number | null>(null);

	const review = $derived(items.filter((it) => it.identify_status === 'review'));
	const junk = $derived(items.filter((it) => it.identify_status === 'junk'));
	// pending/error son transitorios: la propia cola de M3 los reintenta
	// sola (Brief M10b §3 no pide mostrarlos como algo a resolver a mano;
	// se anota su número para no esconder que existen, sin ofrecer
	// acciones sobre ellos).
	const inProgressCount = $derived(
		items.length - review.length - junk.length
	);

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
