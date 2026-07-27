<script lang="ts">
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const typeLabels: Record<string, string> = {
		m3u: 'Lista M3U',
		m3u_remoto: 'M3U remota',
		csv: 'CSV',
		text: 'Texto pegado'
	};
	function typeLabel(type: string): string {
		return typeLabels[type] ?? type;
	}
</script>

<svelte:head>
	<title>GALA — Listas</title>
</svelte:head>

<section class="section">
	<h1>Listas</h1>
	<p class="hint">Los lotes que has ingerido con "Pega lo que sea" — M3U, CSV o texto suelto.</p>

	{#if data.error}
		<p class="muted">No se pudieron cargar las listas: {data.error}</p>
	{:else if data.lists.length === 0}
		<p class="empty">Todavía no has ingerido ninguna lista.</p>
	{:else}
		<div class="lists">
			{#each data.lists as list (list.id)}
				<div class="list-row">
					<div class="main">
						<span class="name">{list.name}</span>
						<span class="type">{typeLabel(list.type)}</span>
					</div>
					<div class="meta">
						<span>{list.item_count} {list.item_count === 1 ? 'item' : 'items'}</span>
						{#if list.last_sync}<span>· sincronizada {list.last_sync}</span>{/if}
					</div>
					{#if list.source_url}
						<div class="url">{list.source_url}</div>
					{/if}
				</div>
			{/each}
		</div>
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
	.muted,
	.empty {
		color: var(--muted);
	}
	.lists {
		display: flex;
		flex-direction: column;
		gap: 10px;
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
	}
	.name {
		font-weight: 600;
		font-size: 14.5px;
		overflow-wrap: anywhere;
	}
	.type {
		font-family: var(--mono);
		font-size: 11px;
		color: var(--accent);
		background: rgba(242, 169, 59, 0.1);
		border: 1px solid rgba(242, 169, 59, 0.3);
		border-radius: 6px;
		padding: 2px 8px;
		white-space: nowrap;
	}
	.meta {
		margin-top: 6px;
		font-size: 12.5px;
		color: var(--muted);
		display: flex;
		gap: 6px;
	}
	.url {
		margin-top: 4px;
		font-family: var(--mono);
		font-size: 11.5px;
		color: var(--muted);
		overflow-wrap: anywhere;
	}
</style>
