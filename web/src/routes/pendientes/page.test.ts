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
			data: { items: [makeItem()], error: null, pendingCount: 1 }
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
			data: { items: [makeItem()], error: null, pendingCount: 1 }
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
			data: { items: [makeItem()], error: null, pendingCount: 1 }
		});

		await fireEvent.click(getByText('No es un título real — descartar'));
		await fireEvent.click(getByText('Sí, descartar'));

		await waitFor(() => expect(getByText('no encontrado')).toBeInTheDocument());
		expect(getByText('VID-20230101-WA0001')).toBeInTheDocument();
	});
});
