import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import IngestModal from './IngestModal.svelte';
import type { ResumenIngesta } from '$lib/api/types';

const { ingestMock } = vi.hoisted(() => ({ ingestMock: vi.fn() }));

vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return { ...actual, ingest: ingestMock };
});

function makeResumen(overrides: Partial<ResumenIngesta> = {}): ResumenIngesta {
	return {
		detectados: 3,
		total: 5,
		anadidos: 4,
		duplicados: 0,
		pendientes_conector: 1,
		omitidos_no_soportados: 2,
		sin_clasificar: 0,
		basura: 0,
		por_tipo: { url: 2, m3u: 3 },
		detalles: [],
		list_id: 1,
		...overrides
	};
}

describe('IngestModal', () => {
	it('envía el texto pegado y muestra el ResumenIngesta REAL devuelto', async () => {
		ingestMock.mockResolvedValueOnce(makeResumen());
		const onIngested = vi.fn();
		const { getByPlaceholderText, getByText } = render(IngestModal, {
			open: true,
			onClose: vi.fn(),
			onIngested
		});

		const textarea = getByPlaceholderText(/Pega aquí/);
		await fireEvent.input(textarea, { target: { value: 'https://cdn.ejemplo.net/v/pelicula.mp4' } });
		await fireEvent.click(getByText('Añadir a mi galería'));

		await waitFor(() => expect(getByText('3')).toBeInTheDocument()); // detectados
		expect(ingestMock).toHaveBeenCalledWith('https://cdn.ejemplo.net/v/pelicula.mp4');
		expect(onIngested).toHaveBeenCalled();
		expect(getByText('4')).toBeInTheDocument(); // añadidos
		expect(getByText('1')).toBeInTheDocument(); // pendientes_conector
		expect(getByText('2')).toBeInTheDocument(); // omitidos_no_soportados
	});

	it('no deja enviar con el textarea vacío', async () => {
		const { getByText } = render(IngestModal, { open: true, onClose: vi.fn(), onIngested: vi.fn() });
		const submit = getByText('Añadir a mi galería');
		expect(submit).toBeDisabled();
	});

	it('un fallo de ingest muestra el mensaje de error y no rompe la modal', async () => {
		const { ApiError } = await import('$lib/api/client');
		ingestMock.mockRejectedValueOnce(new ApiError('el contenido supera el límite de 5 MB', 413));
		const { getByPlaceholderText, getByText } = render(IngestModal, {
			open: true,
			onClose: vi.fn(),
			onIngested: vi.fn()
		});

		const textarea = getByPlaceholderText(/Pega aquí/);
		await fireEvent.input(textarea, { target: { value: 'algo' } });
		await fireEvent.click(getByText('Añadir a mi galería'));

		await waitFor(() =>
			expect(getByText('el contenido supera el límite de 5 MB')).toBeInTheDocument()
		);
	});
});
