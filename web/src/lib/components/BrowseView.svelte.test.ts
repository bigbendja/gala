import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import BrowseView from './BrowseView.svelte';
import type { Item, ListItemsParams } from '$lib/api/types';

const { listItemsMock, listFacetsMock, gotoMock, pageState } = vi.hoisted(() => ({
	listItemsMock: vi.fn(),
	listFacetsMock: vi.fn(),
	gotoMock: vi.fn(),
	pageState: { url: new URL('http://localhost/peliculas'), data: {} as Record<string, unknown> }
}));

vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return { ...actual, listItems: listItemsMock, listFacets: listFacetsMock };
});
vi.mock('$app/navigation', () => ({ goto: gotoMock }));
vi.mock('$app/state', () => ({ page: pageState }));

function movie(id: number, title: string, over: Partial<Item> = {}): Item {
	return {
		id,
		kind: 'movie',
		title,
		genres: [],
		confirmed: true,
		wished: false,
		added_at: '2020-01-01T00:00:00Z',
		updated_at: '2020-01-01T00:00:00Z',
		source_count: 1,
		...over
	};
}

const DB: Record<string, Item[]> = {
	Drama: [movie(1, 'Parásitos', { year: 2019 }), movie(2, 'Intocable', { year: 2011 })],
	Acción: [movie(3, 'Origen', { year: 2010 })],
	Comedia: [] // sin resultados con el filtro: su sección no debe pintarse
};

function setup(search = '') {
	pageState.url = new URL('http://localhost/peliculas' + search);
	return render(BrowseView, {
		kind: 'movie',
		title: 'Películas',
		emptyTitle: 'Aún no hay películas',
		emptyText: 'Añade algo.',
		errorTitle: 'No se pudieron cargar las películas'
	});
}

beforeEach(() => {
	listItemsMock.mockReset();
	listFacetsMock.mockReset();
	gotoMock.mockReset();
	listFacetsMock.mockResolvedValue({
		genres: [
			{ name: 'Drama', count: 2 },
			{ name: 'Acción', count: 1 },
			{ name: 'Comedia', count: 1 }
		],
		years: [2019, 2011, 2010]
	});
	listItemsMock.mockImplementation(async (p: ListItemsParams) => {
		const pool = p.genre ? (DB[p.genre] ?? []) : Object.values(DB).flat();
		const items = p.year ? pool.filter((m) => m.year === p.year) : pool;
		return { items: (p.limit ?? 60) === 1 ? items.slice(0, 1) : items, total: items.length, limit: p.limit ?? 60, offset: 0 };
	});
});

describe('BrowseView', () => {
	it('una sección por género con contenido, con su contador; la que queda vacía no se pinta', async () => {
		const { findByRole, queryByRole, container } = setup();
		expect(await findByRole('heading', { name: 'Drama' })).toBeInTheDocument();
		expect(await findByRole('heading', { name: 'Acción' })).toBeInTheDocument();
		await waitFor(() => expect(container.querySelector('[aria-busy="true"]')).toBeNull());
		expect(queryByRole('heading', { name: 'Comedia' })).toBeNull();
		// Orden de las secciones: el de las facetas (más títulos primero).
		const heads = [...container.querySelectorAll('h2')].map((h) => h.textContent);
		expect(heads).toEqual(['Drama', 'Acción']);
	});

	it('pide a la API solo lo reproducible, ordenado, y no usa nada de idioma', async () => {
		setup('?orden=rating&anio=2019');
		const sectionCall = () =>
			listItemsMock.mock.calls.map((c) => c[0] as ListItemsParams).find((c) => c.genre === 'Drama' && c.limit === 40);
		await waitFor(() => expect(sectionCall()).toBeDefined());
		const section = sectionCall();
		expect(section).toMatchObject({ kind: 'movie', playable: true, sort: 'rating', year: 2019, parent_id: 'null' });
	});

	it('con un género elegido en la URL solo se muestra esa sección y el contador «N de M»', async () => {
		const { findByText, container } = setup('?genero=Drama');
		expect(await findByText('2 de 3')).toBeInTheDocument();
		await waitFor(() => expect(container.querySelectorAll('h2')).toHaveLength(1));
		expect([...container.querySelectorAll('h2')].map((h) => h.textContent)).toEqual(['Drama']);
	});

	it('cambiar un filtro actualiza la URL; Limpiar la deja sin parámetros', async () => {
		const { findByLabelText, getByText } = setup('?genero=Drama');
		const year = (await findByLabelText('Año')) as HTMLSelectElement;
		await fireEvent.change(year, { target: { value: '2019' } });
		expect(gotoMock).toHaveBeenCalledWith('/peliculas?genero=Drama&anio=2019', expect.anything());
		await fireEvent.click(getByText('Limpiar'));
		expect(gotoMock).toHaveBeenLastCalledWith('/peliculas', expect.anything());
	});

	it('sin resultados con los filtros: mensaje y botón para limpiarlos', async () => {
		const { findByText } = setup('?anio=1999');
		expect(await findByText('Sin resultados con estos filtros')).toBeInTheDocument();
		expect(await findByText('Limpiar filtros')).toBeInTheDocument();
	});

	it('biblioteca sin películas: estado vacío con acción, sin barra de filtros', async () => {
		listFacetsMock.mockResolvedValue({ genres: [], years: [] });
		listItemsMock.mockResolvedValue({ items: [], total: 0, limit: 1, offset: 0 });
		const { findByText, queryByLabelText } = setup();
		expect(await findByText('Aún no hay películas')).toBeInTheDocument();
		expect(queryByLabelText('Género')).toBeNull();
	});
});
