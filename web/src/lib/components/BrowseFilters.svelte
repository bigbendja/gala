<script lang="ts">
	// Barra de filtros y orden de las vistas Películas y Series (mockup Vistas2).
	// Género, Año y Ordenar; contador «N de M» y «Limpiar». El estado vive en la
	// URL (lo gestiona la vista): esta barra solo pinta y avisa de los cambios.
	//
	// Hueco para «Idioma»: GALA aún no sabe el idioma de cada fuente (fase
	// futura), así que NO se muestra. Cuando exista, va aquí, entre Año y
	// Ordenar, con el mismo `fgroup`.
	import type { Facets, SortOrder } from '$lib/api/types';

	interface Props {
		facets: Facets;
		genre: string;
		year: number;
		sort: SortOrder;
		/** «3 de 22» cuando hay filtros; vacío si no. */
		counter: string;
		onChange: (next: { genre?: string; year?: number; sort?: SortOrder }) => void;
		onClear: () => void;
	}
	let { facets, genre, year, sort, counter, onChange, onClear }: Props = $props();

	const SORTS: { id: SortOrder; label: string }[] = [
		{ id: 'recent', label: 'Añadidas' },
		{ id: 'year', label: 'Año ↓' },
		{ id: 'title', label: 'Título A-Z' },
		{ id: 'rating', label: 'Mejor valoradas' }
	];
	const genres = $derived([...facets.genres].sort((a, b) => a.name.localeCompare(b.name, 'es')));
	const dirty = $derived(!!genre || !!year || sort !== 'recent');
</script>

<div class="filters" role="search" aria-label="Filtros y orden">
	<div class="fgroup">
		<label for="f-genre">Género</label>
		<select id="f-genre" value={genre} onchange={(e) => onChange({ genre: e.currentTarget.value })}>
			<option value="">Todos</option>
			{#each genres as g (g.name)}
				<option value={g.name}>{g.name}</option>
			{/each}
		</select>
	</div>
	<div class="fgroup">
		<label for="f-year">Año</label>
		<select
			id="f-year"
			value={year ? String(year) : ''}
			onchange={(e) => onChange({ year: Number(e.currentTarget.value) || 0 })}
		>
			<option value="">Todos</option>
			{#each facets.years as y (y)}
				<option value={String(y)}>{y}</option>
			{/each}
		</select>
	</div>
	<div class="fgroup">
		<label for="f-sort">Ordenar</label>
		<select id="f-sort" value={sort} onchange={(e) => onChange({ sort: e.currentTarget.value as SortOrder })}>
			{#each SORTS as s (s.id)}
				<option value={s.id}>{s.label}</option>
			{/each}
		</select>
	</div>
	<button class="fclear" disabled={!dirty} title={dirty ? '' : 'No hay filtros que limpiar'} onclick={onClear}>
		Limpiar
	</button>
	{#if counter}<span class="fcount">{counter}</span>{/if}
</div>

<style>
	.filters {
		display: flex;
		align-items: center;
		gap: 10px;
		margin-bottom: 26px;
		flex-wrap: wrap;
		padding: 12px 14px;
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 12px;
	}
	.fgroup {
		display: flex;
		align-items: center;
		gap: 7px;
	}
	.fgroup label {
		font-size: 12px;
		color: var(--muted);
		font-family: var(--mono);
		letter-spacing: 0.04em;
	}
	select {
		background: var(--surface-2);
		border: 1px solid var(--line);
		border-radius: 9px;
		padding: 8px 12px;
		font-size: 13.5px;
		color: var(--text);
		min-width: 120px;
		min-height: 38px;
		font-family: inherit;
		cursor: pointer;
	}
	select:hover {
		border-color: #4d3a22;
	}
	.fclear {
		font-size: 12.5px;
		color: var(--accent);
		background: none;
		border: none;
		font-weight: 500;
		min-height: 38px;
		padding: 0 6px;
		cursor: pointer;
	}
	.fclear:disabled {
		color: var(--muted);
		opacity: 0.55;
		cursor: default;
	}
	.fcount {
		margin-left: auto;
		font-family: var(--mono);
		font-size: 12px;
		color: var(--muted);
	}
	@media (max-width: 760px) {
		.fgroup {
			flex: 1 1 140px;
		}
		select {
			flex: 1;
			min-width: 0;
		}
	}
</style>
