import { fireEvent, render } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import BrowseFilters from './BrowseFilters.svelte';

const facets = {
	genres: [
		{ name: 'Drama', count: 6 },
		{ name: 'Acción', count: 4 }
	],
	years: [2021, 2019]
};

function setup(over: Partial<{ genre: string; year: number; sort: 'recent' | 'year' | 'title' | 'rating'; counter: string }> = {}) {
	const onChange = vi.fn();
	const onClear = vi.fn();
	const r = render(BrowseFilters, { facets, genre: '', year: 0, sort: 'recent', counter: '', onChange, onClear, ...over });
	return { ...r, onChange, onClear };
}

describe('BrowseFilters', () => {
	it('ofrece Género, Año y Ordenar; NO ofrece Idioma (aún no hay dato)', () => {
		const { getByLabelText, queryByLabelText, getByText } = setup();
		expect(getByLabelText('Género')).toBeInTheDocument();
		expect(getByLabelText('Año')).toBeInTheDocument();
		expect(getByLabelText('Ordenar')).toBeInTheDocument();
		expect(queryByLabelText('Idioma')).toBeNull();
		for (const t of ['Añadidas', 'Año ↓', 'Título A-Z', 'Mejor valoradas']) expect(getByText(t)).toBeInTheDocument();
	});

	it('los géneros salen en orden alfabético y avisan al cambiar', async () => {
		const { getByLabelText, onChange } = setup();
		const sel = getByLabelText('Género') as HTMLSelectElement;
		expect([...sel.options].map((o) => o.text)).toEqual(['Todos', 'Acción', 'Drama']);
		await fireEvent.change(sel, { target: { value: 'Drama' } });
		expect(onChange).toHaveBeenCalledWith({ genre: 'Drama' });
		await fireEvent.change(getByLabelText('Año'), { target: { value: '2019' } });
		expect(onChange).toHaveBeenCalledWith({ year: 2019 });
		await fireEvent.change(getByLabelText('Ordenar'), { target: { value: 'rating' } });
		expect(onChange).toHaveBeenCalledWith({ sort: 'rating' });
	});

	it('Limpiar solo está activo con filtros y enseña el contador «N de M»', async () => {
		const clean = setup();
		expect(clean.getByText('Limpiar')).toBeDisabled();
		clean.unmount();

		const dirty = setup({ genre: 'Drama', counter: '3 de 22' });
		expect(dirty.getByText('3 de 22')).toBeInTheDocument();
		await fireEvent.click(dirty.getByText('Limpiar'));
		expect(dirty.onClear).toHaveBeenCalled();
	});
});
