import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import SearchBox from './SearchBox.svelte';
import type { Item, ListResult } from '$lib/api/types';

const { listItemsMock } = vi.hoisted(() => ({ listItemsMock: vi.fn() }));

vi.mock('$lib/api/client', () => ({
	listItems: listItemsMock,
	imgUrl: (filename: string | null | undefined) => (filename ? `/api/v1/img/${filename}` : null)
}));

function makeItem(overrides: Partial<Item> = {}): Item {
	return {
		id: 1,
		kind: 'movie',
		title: 'Parásitos',
		poster_path: 'parasitos.jpg',
		genres: [],
		confirmed: true,
		wished: false,
		added_at: new Date(0).toISOString(),
		updated_at: new Date(0).toISOString(),
		source_count: 1,
		...overrides
	};
}

function makeResult(items: Item[], total?: number): ListResult {
	return { items, total: total ?? items.length, limit: 8, offset: 0 };
}

describe('SearchBox', () => {
	it('no busca con menos de 2 caracteres', async () => {
		const { getByLabelText, queryByText } = render(SearchBox, {});
		const input = getByLabelText('Buscar en tu galería');
		await fireEvent.focus(input);
		await fireEvent.input(input, { target: { value: 'p' } });
		await new Promise((r) => setTimeout(r, 350));
		expect(listItemsMock).not.toHaveBeenCalled();
		expect(queryByText(/Sin resultados/)).toBeNull();
	});

	it('busca (con debounce) y muestra resultados reales', async () => {
		listItemsMock.mockResolvedValueOnce(makeResult([makeItem()]));
		const { getByLabelText, getByText } = render(SearchBox, {});
		const input = getByLabelText('Buscar en tu galería');
		await fireEvent.focus(input);
		await fireEvent.input(input, { target: { value: 'parasitos' } });

		await waitFor(() => expect(listItemsMock).toHaveBeenCalledWith({ q: 'parasitos', limit: 8, offset: 0 }), {
			timeout: 1000
		});
		await waitFor(() => expect(getByText('Parásitos')).toBeInTheDocument());
	});

	it('sin resultados muestra el estado vacío', async () => {
		listItemsMock.mockResolvedValueOnce(makeResult([]));
		const { getByLabelText, getByText } = render(SearchBox, {});
		const input = getByLabelText('Buscar en tu galería');
		await fireEvent.focus(input);
		await fireEvent.input(input, { target: { value: 'inexistente' } });

		await waitFor(() => expect(getByText('Sin resultados para "inexistente".')).toBeInTheDocument(), {
			timeout: 1000
		});
	});

	it('limpiar la búsqueda vacía el input y cierra el desplegable', async () => {
		listItemsMock.mockResolvedValueOnce(makeResult([makeItem()]));
		const { getByLabelText, getByText, getByLabelText: getByLabel, queryByText } = render(SearchBox, {});
		const input = getByLabelText('Buscar en tu galería') as HTMLInputElement;
		await fireEvent.focus(input);
		await fireEvent.input(input, { target: { value: 'parasitos' } });
		await waitFor(() => expect(getByText('Parásitos')).toBeInTheDocument());

		await fireEvent.click(getByLabel('Limpiar búsqueda'));
		expect(input.value).toBe('');
		expect(queryByText('Parásitos')).toBeNull();
	});
});
