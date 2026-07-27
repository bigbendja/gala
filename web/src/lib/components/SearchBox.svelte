<script lang="ts">
	import Card from './Card.svelte';
	import { listItems } from '$lib/api/client';
	import type { Item } from '$lib/api/types';

	interface Props {
		/** Se llama tras navegar a un resultado (el menú móvil lo usa para
		 *  cerrarse solo). */
		onNavigate?: () => void;
	}
	let { onNavigate }: Props = $props();

	const DEBOUNCE_MS = 300;
	const MAX_RESULTS = 8;

	let query = $state('');
	let results = $state<Item[]>([]);
	let total = $state(0);
	let loading = $state(false);
	let searched = $state(false);
	let focused = $state(false);
	let debounceHandle: ReturnType<typeof setTimeout> | undefined;
	let requestSeq = 0;

	function scheduleSearch(q: string) {
		clearTimeout(debounceHandle);
		const trimmed = q.trim();
		if (trimmed.length < 2) {
			results = [];
			total = 0;
			searched = false;
			loading = false;
			return;
		}
		debounceHandle = setTimeout(() => runSearch(trimmed), DEBOUNCE_MS);
	}

	async function runSearch(q: string) {
		loading = true;
		const seq = ++requestSeq;
		try {
			const res = await listItems({ q, limit: MAX_RESULTS, offset: 0 });
			if (seq !== requestSeq) return; // respuesta obsoleta (el usuario ya siguió escribiendo)
			results = res.items;
			total = res.total;
			searched = true;
		} catch {
			if (seq !== requestSeq) return;
			results = [];
			total = 0;
			searched = true;
		} finally {
			if (seq === requestSeq) loading = false;
		}
	}

	function onInput(e: Event) {
		query = (e.target as HTMLInputElement).value;
		scheduleSearch(query);
	}

	function clear() {
		query = '';
		results = [];
		total = 0;
		searched = false;
		requestSeq++; // descarta cualquier búsqueda en vuelo
	}

	function onBlur() {
		// Retraso para que un click sobre un resultado (un <a>) navegue antes
		// de que el desplegable desaparezca.
		setTimeout(() => (focused = false), 150);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			clear();
			(e.target as HTMLElement).blur();
		}
	}

	function onResultClick() {
		onNavigate?.();
	}

	const showDropdown = $derived(focused && query.trim().length >= 2);
</script>

<div class="search-box">
	<div class="search-input">
		<span class="icon" aria-hidden="true">🔍</span>
		<input
			type="search"
			placeholder="Buscar en tu galería…"
			value={query}
			oninput={onInput}
			onfocus={() => (focused = true)}
			onblur={onBlur}
			onkeydown={onKeydown}
			aria-label="Buscar en tu galería"
		/>
		{#if query}
			<button class="clear" onclick={clear} aria-label="Limpiar búsqueda">✕</button>
		{/if}
	</div>

	{#if showDropdown}
		<div class="dropdown">
			{#if loading && results.length === 0}
				<p class="hint">Buscando…</p>
			{:else if searched && results.length === 0}
				<p class="hint">Sin resultados para "{query.trim()}".</p>
			{:else if results.length > 0}
				<!-- Delegación de click: los hijos reales son <a> de <Card>, ya
				     interactivos y accesibles por teclado por sí mismos; este div
				     solo escucha para cerrar el desplegable tras navegar. -->
				<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
				<!-- svelte-ignore a11y_click_events_have_key_events -->
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div class="results" onclick={onResultClick}>
					{#each results as item (item.id)}
						<Card {item} />
					{/each}
				</div>
				{#if total > results.length}
					<p class="more">y {total - results.length} resultado{total - results.length === 1 ? '' : 's'} más…</p>
				{/if}
			{/if}
		</div>
	{/if}
</div>

<style>
	.search-box {
		position: relative;
		min-width: 220px;
	}
	.search-input {
		display: flex;
		align-items: center;
		gap: 8px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 10px;
		padding: 8px 12px;
	}
	.search-input:focus-within {
		border-color: var(--accent);
	}
	.icon {
		font-size: 13px;
		opacity: 0.8;
	}
	input[type='search'] {
		flex: 1;
		background: none;
		border: none;
		color: var(--text);
		font-size: 14px;
		outline: none;
		min-width: 0;
	}
	input[type='search']::placeholder {
		color: var(--muted);
	}
	input[type='search']::-webkit-search-cancel-button {
		display: none;
	}
	.clear {
		color: var(--muted);
		font-size: 12px;
		padding: 2px 4px;
	}
	.clear:hover {
		color: var(--text);
	}
	.dropdown {
		position: absolute;
		top: calc(100% + 8px);
		right: 0;
		width: min(560px, 88vw);
		max-height: 70vh;
		overflow-y: auto;
		background: var(--surface-2);
		border: 1px solid var(--line);
		border-radius: 12px;
		padding: 14px;
		box-shadow: 0 18px 40px rgba(0, 0, 0, 0.5);
		z-index: 60;
	}
	.hint {
		color: var(--muted);
		font-size: 13.5px;
		padding: 8px 4px;
		text-align: center;
	}
	.results {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
		gap: 12px;
	}
	.more {
		color: var(--muted);
		font-size: 12px;
		text-align: center;
		padding-top: 10px;
	}

	@media (max-width: 900px) {
		.search-box {
			min-width: 0;
			width: 100%;
		}
		.dropdown {
			position: fixed;
			left: 12px;
			right: 12px;
			top: auto;
			width: auto;
		}
	}
</style>
