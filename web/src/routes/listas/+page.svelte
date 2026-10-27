<script lang="ts">
	import { onMount } from 'svelte';
	import { listIngestLists } from '$lib/api/client';
	import type { IngestList } from '$lib/api/types';
	import { errorMessage, formatDate, listDisplayName } from '$lib/format';
	import StateBlock from '$lib/components/StateBlock.svelte';

	const typeLabels: Record<string, string> = {
		m3u: 'Lista M3U',
		m3u_remoto: 'M3U remota',
		csv: 'CSV',
		text: 'Texto'
	};
	function typeLabel(type: string): string {
		return typeLabels[type] ?? 'Lista';
	}

	let lists = $state<IngestList[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			lists = await listIngestLists();
		} catch (err) {
			error = errorMessage(err, 'No se pudieron cargar las listas');
		} finally {
			loading = false;
		}
	}
	onMount(load);

	function openIngest() {
		window.dispatchEvent(new CustomEvent('gala:open-ingest'));
	}
</script>

<svelte:head>
	<title>GALA — Listas</title>
</svelte:head>

<section class="section">
	<h1>Listas</h1>
	<p class="hint">Cada vez que usas «Pega lo que sea» (enlaces, M3U, CSV o texto suelto) queda aquí como un lote.</p>

	{#if loading}
		<div class="lists" aria-busy="true" aria-label="Cargando listas">
			{#each { length: 3 } as _, i (i)}
				<div class="skeleton row-skel"></div>
			{/each}
		</div>
	{:else if error}
		<StateBlock compact tone="error" title="No se pudo cargar" text={error} actionLabel="Reintentar" onAction={load} />
	{:else if lists.length === 0}
		<StateBlock
			compact
			title="Aún no has añadido ninguna lista"
			text="Pega enlaces, una lista M3U o un texto con enlaces y quedará registrado aquí."
			actionLabel="＋ Añadir"
			onAction={openIngest}
		/>
	{:else}
		<ul class="lists">
			{#each lists as list (list.id)}
				<li class="list-row">
					<div class="main">
						<span class="name">{listDisplayName(list.name, list.type)}</span>
						<span class="type">{typeLabel(list.type)}</span>
					</div>
					<div class="meta">
						<span>{list.item_count} {list.item_count === 1 ? 'título' : 'títulos'}</span>
						{#if list.last_sync}<span>· añadida {formatDate(list.last_sync)}</span>{/if}
					</div>
					{#if list.source_url}
						<div class="url" title={list.source_url}>{list.source_url}</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.section {
		max-width: 900px;
		margin: 0 auto;
		padding: 34px 28px 60px;
	}
	h1 {
		font-family: var(--display);
		font-size: 26px;
		font-weight: 700;
	}
	.hint {
		color: var(--muted);
		margin: 8px 0 26px;
		font-size: 14px;
	}
	.lists {
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.row-skel {
		height: 66px;
		border-radius: 10px;
	}
	.list-row {
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 10px;
		padding: 14px 16px;
	}
	.main {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
	}
	.name {
		font-weight: 600;
		font-size: 14.5px;
		overflow-wrap: anywhere;
	}
	.type {
		font-size: 11.5px;
		font-weight: 600;
		color: var(--muted);
		background: var(--surface-2);
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 2px 8px;
		white-space: nowrap;
	}
	.meta {
		margin-top: 6px;
		font-size: 13px;
		color: var(--muted);
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
	}
	.url {
		margin-top: 4px;
		font-family: var(--mono);
		font-size: 11.5px;
		color: var(--muted);
		overflow-wrap: anywhere;
	}
	@media (max-width: 900px) {
		.section {
			padding-left: 16px;
			padding-right: 16px;
		}
	}
</style>
