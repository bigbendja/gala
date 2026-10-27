import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';
import type { PendingItem } from '$lib/api/types';

const { deleteItemMock, confirmItemMock, invalidateAllMock } = vi.hoisted(() => ({
	deleteItemMock: vi.fn(),
	confirmItemMock: vi.fn(),
	invalidateAllMock: vi.fn()
}));

vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return { ...actual, deleteItem: deleteItemMock, confirmItem: confirmItemMock };
});

vi.mock('$app/navigation', () => ({ invalidateAll: invalidateAllMock }));

// Datos del layout raíz (Modo LAN, M7): estos tests corren como el PC.
const layoutLan = { session: { local: true, pin_required: false, authenticated: true }, lanEnabled: false };

function makeItem(overrides: Partial<PendingItem> = {}): PendingItem {
	return {
		id: 42,
		kind: 'other',
		title: 'VID-20230101-WA0001',
		genres: [],
		confirmed: false,
		wished: false,
		added_at: new Date(0).toISOString(),
		updated_at: new Date(0).toISOString(),
		source_count: 1,
		identify_status: 'review',
		candidates: [],
		...overrides
	};
}

describe('Por confirmar — descartar (Brief M10c §3)', () => {
	beforeEach(() => {
		deleteItemMock.mockReset();
		confirmItemMock.mockReset();
		invalidateAllMock.mockReset();
	});

	it('pide confirmación antes de descartar, y al confirmar llama a DELETE y quita el item de la lista', async () => {
		deleteItemMock.mockResolvedValueOnce({ status: 'eliminado' });
		const { getByText, queryByText } = render(Page, {
			data: { items: [makeItem()], error: null, pendingCount: 1, ...layoutLan }
		});

		expect(getByText('VID-20230101-WA0001')).toBeInTheDocument();
		await fireEvent.click(getByText('No es un título real — descartar'));
		expect(getByText(/¿Descartar/)).toBeInTheDocument();
		expect(deleteItemMock).not.toHaveBeenCalled();

		await fireEvent.click(getByText('Sí, descartar'));

		await waitFor(() => expect(deleteItemMock).toHaveBeenCalledWith(42));
		await waitFor(() => expect(queryByText('VID-20230101-WA0001')).toBeNull());
		expect(invalidateAllMock).toHaveBeenCalled();
	});

	it('cancelar la confirmación no llama a DELETE y conserva el item', async () => {
		const { getByText, queryByText } = render(Page, {
			data: { items: [makeItem()], error: null, pendingCount: 1, ...layoutLan }
		});

		await fireEvent.click(getByText('No es un título real — descartar'));
		await fireEvent.click(getByText('Cancelar'));

		expect(deleteItemMock).not.toHaveBeenCalled();
		expect(queryByText(/¿Descartar/)).toBeNull();
		expect(getByText('VID-20230101-WA0001')).toBeInTheDocument();
	});

	it('un fallo al descartar muestra un error y conserva el item', async () => {
		const { ApiError } = await import('$lib/api/client');
		deleteItemMock.mockRejectedValueOnce(new ApiError('no encontrado', 404));
		const { getByText } = render(Page, {
			data: { items: [makeItem()], error: null, pendingCount: 1, ...layoutLan }
		});

		await fireEvent.click(getByText('No es un título real — descartar'));
		await fireEvent.click(getByText('Sí, descartar'));

		await waitFor(() => expect(getByText('no encontrado')).toBeInTheDocument());
		expect(getByText('VID-20230101-WA0001')).toBeInTheDocument();
	});
});

describe('Por confirmar — dejar sin identificar (Fase 0 tarea 0.4)', () => {
	beforeEach(() => {
		deleteItemMock.mockReset();
		confirmItemMock.mockReset();
		invalidateAllMock.mockReset();
	});

	it('el campo de título manual viene precargado con el nombre limpio del item', () => {
		const { getByPlaceholderText } = render(Page, {
			data: {
				items: [makeItem({ title: 'Big_Buck_Bunny_360_10s_1MB.mp4' })],
				error: null,
				pendingCount: 1, ...layoutLan
			}
		});
		const input = getByPlaceholderText('O escribe el título correcto…') as HTMLInputElement;
		expect(input.value).toBe('Big Buck Bunny 360 10s 1MB mp4');
	});

	it('"Dejar sin identificar" confirma con el título limpio, ignorando lo que haya en el input', async () => {
		confirmItemMock.mockResolvedValueOnce({ id: 42 });
		const { getByText, getByPlaceholderText, queryByText } = render(Page, {
			data: { items: [makeItem({ title: 'pelicula.rara.2024.mkv' })], error: null, pendingCount: 1, ...layoutLan }
		});

		const input = getByPlaceholderText('O escribe el título correcto…') as HTMLInputElement;
		await fireEvent.input(input, { target: { value: 'un título a medio escribir' } });

		await fireEvent.click(getByText(/Dejar sin identificar/));

		await waitFor(() =>
			expect(confirmItemMock).toHaveBeenCalledWith(42, { title: 'pelicula rara 2024 mkv' })
		);
		await waitFor(() => expect(queryByText('pelicula rara 2024 mkv')).toBeNull());
		expect(invalidateAllMock).toHaveBeenCalled();
	});

	it('un fallo al dejarlo sin identificar muestra error y conserva el item', async () => {
		const { ApiError } = await import('$lib/api/client');
		confirmItemMock.mockRejectedValueOnce(new ApiError('error interno', 500));
		const { getByText } = render(Page, {
			data: { items: [makeItem()], error: null, pendingCount: 1, ...layoutLan }
		});

		await fireEvent.click(getByText(/Dejar sin identificar/));

		await waitFor(() => expect(getByText('error interno')).toBeInTheDocument());
		expect(getByText('VID-20230101-WA0001')).toBeInTheDocument();
	});
});

describe('Por confirmar — items en identify_status=error son accionables (diagnóstico Fase 0)', () => {
	beforeEach(() => {
		deleteItemMock.mockReset();
		confirmItemMock.mockReset();
		invalidateAllMock.mockReset();
	});

	it('un item en error (p. ej. clave TMDB inválida) aparece como accionable, no como "procesándose"', () => {
		const errored = makeItem({
			id: 4,
			title: 'Big_Buck_Bunny_360_10s_1MB',
			identify_status: 'error',
			identify_note: 'tmdb /movie/1: 401 {"status_message":"Invalid API key: You must be granted a valid key."}'
		});
		const { getByText, queryByText } = render(Page, {
			data: { items: [errored], error: null, pendingCount: 1, ...layoutLan }
		});

		expect(getByText('Big Buck Bunny 360 10s 1MB')).toBeInTheDocument();
		expect(getByText(/La clave de TMDB no es válida/)).toBeInTheDocument();
		expect(queryByText('No hay nada por confirmar ahora mismo.')).toBeNull();
		expect(getByText(/Dejar sin identificar/)).toBeInTheDocument();
	});

	it('"Dejar sin identificar" también funciona sobre un item en error', async () => {
		confirmItemMock.mockResolvedValueOnce({ id: 4 });
		const errored = makeItem({ id: 4, title: 'Big_Buck_Bunny_360_10s_1MB', identify_status: 'error' });
		const { getByText, queryByText } = render(Page, {
			data: { items: [errored], error: null, pendingCount: 1, ...layoutLan }
		});

		await fireEvent.click(getByText(/Dejar sin identificar/));

		await waitFor(() =>
			expect(confirmItemMock).toHaveBeenCalledWith(4, { title: 'Big Buck Bunny 360 10s 1MB' })
		);
		await waitFor(() => expect(queryByText('Big Buck Bunny 360 10s 1MB')).toBeNull());
	});
});
