import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';
import type { FolderInfo, IdentifyStatus, ScanStatus } from '$lib/api/types';

const { addFolderMock, removeFolderMock, listFoldersMock, putTMDBSettingsMock, identifyStatusMock, startScanMock, scanStatusMock, startHealthCheckMock, healthSummaryMock } =
	vi.hoisted(() => ({
		addFolderMock: vi.fn(),
		removeFolderMock: vi.fn(),
		listFoldersMock: vi.fn(),
		putTMDBSettingsMock: vi.fn(),
		identifyStatusMock: vi.fn(),
		startScanMock: vi.fn(),
		scanStatusMock: vi.fn(),
		startHealthCheckMock: vi.fn(),
		healthSummaryMock: vi.fn()
	}));

vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return {
		...actual,
		addFolder: addFolderMock,
		removeFolder: removeFolderMock,
		listFolders: listFoldersMock,
		putTMDBSettings: putTMDBSettingsMock,
		identifyStatus: identifyStatusMock,
		startScan: startScanMock,
		scanStatus: scanStatusMock,
		startHealthCheck: startHealthCheckMock,
		healthSummary: healthSummaryMock
	};
});

// Datos del layout raíz (Modo LAN, M7): estos tests corren como el PC.
const layoutLan = {
	session: { local: true, pin_required: false, authenticated: true },
	chrome: Promise.resolve({ pendingCount: 0, lanEnabled: false })
};

function baseData(overrides: Partial<Record<string, unknown>> = {}) {
	const scan: ScanStatus = { running: false, files_seen: 0, items_new: 0 };
	const identify: IdentifyStatus = { pending: 0, processing: false, online: true, processed: 3 };
	return {
		...layoutLan,
		folders: [] as FolderInfo[],
		foldersError: null,
		scan,
		scanError: null,
		identify,
		identifyError: null,
		tmdb: { configured: false },
		tmdbError: null,
		health: { total: 0, viva: 0, lenta: 0, caida: 0, desconectada: 0, sin_verificar: 0, running: false },
		healthError: null,
		sys: { status: 'ok', app: 'gala', version: '0.1.0-test', ffmpeg_available: true },
		sysError: null,
		build: { version: 'v0.9.0', commit: 'abc1234def', date: '2026-10-30T10:00:00Z' },
		...overrides
	};
}

describe('Ajustes', () => {
	beforeEach(() => {
		addFolderMock.mockReset();
		removeFolderMock.mockReset();
		listFoldersMock.mockReset();
		putTMDBSettingsMock.mockReset();
		identifyStatusMock.mockReset();
	});

	it('añadir carpeta llama a POST /folders y refresca la lista', async () => {
		addFolderMock.mockResolvedValueOnce({ path: 'E:\\Peliculas', connected: true });
		listFoldersMock.mockResolvedValueOnce([{ path: 'E:\\Peliculas', connected: true }]);
		const { getByPlaceholderText, getByText } = render(Page, { data: baseData() });

		const input = getByPlaceholderText(/Ruta de la carpeta/);
		await fireEvent.input(input, { target: { value: 'E:\\Peliculas' } });
		await fireEvent.click(getByText('Añadir carpeta'));

		await waitFor(() => expect(addFolderMock).toHaveBeenCalledWith('E:\\Peliculas'));
		await waitFor(() => expect(getByText('E:\\Peliculas')).toBeInTheDocument());
	});

	it('un error al añadir carpeta (409 ya registrada) se muestra sin romper la página', async () => {
		const { ApiError } = await import('$lib/api/client');
		addFolderMock.mockRejectedValueOnce(new ApiError('la carpeta ya está registrada', 409));
		const { getByPlaceholderText, getByText } = render(Page, { data: baseData() });

		const input = getByPlaceholderText(/Ruta de la carpeta/);
		await fireEvent.input(input, { target: { value: 'E:\\Peliculas' } });
		await fireEvent.click(getByText('Añadir carpeta'));

		await waitFor(() => expect(getByText('la carpeta ya está registrada')).toBeInTheDocument());
	});

	it('quitar una carpeta pide confirmación inline antes de llamar a DELETE', async () => {
		removeFolderMock.mockResolvedValueOnce({ status: 'eliminada' });
		listFoldersMock.mockResolvedValueOnce([]);
		const { getByText, queryByText } = render(Page, {
			data: baseData({ folders: [{ path: 'E:\\Peliculas', connected: true }] })
		});

		await fireEvent.click(getByText('Quitar'));
		expect(removeFolderMock).not.toHaveBeenCalled();
		expect(getByText('¿Quitar?')).toBeInTheDocument();

		await fireEvent.click(getByText('Sí'));
		await waitFor(() => expect(removeFolderMock).toHaveBeenCalledWith('E:\\Peliculas'));
		await waitFor(() => expect(queryByText('E:\\Peliculas')).toBeNull());
	});

	it('guardar una clave TMDB válida la persiste y muestra éxito', async () => {
		putTMDBSettingsMock.mockResolvedValueOnce({ status: 'ok' });
		identifyStatusMock.mockResolvedValueOnce({ pending: 0, processing: false, online: true, processed: 3 });
		const { getByPlaceholderText, getByText } = render(Page, { data: baseData() });

		const input = getByPlaceholderText('Nueva clave de TMDB');
		await fireEvent.input(input, { target: { value: 'clave-real-de-tmdb' } });
		await fireEvent.click(getByText('Guardar clave'));

		await waitFor(() => expect(putTMDBSettingsMock).toHaveBeenCalledWith('clave-real-de-tmdb'));
		await waitFor(() => expect(getByText('Clave guardada y verificada contra TMDB.')).toBeInTheDocument());
	});

	it('una clave TMDB inválida (422) muestra el error sin guardarla', async () => {
		const { ApiError } = await import('$lib/api/client');
		putTMDBSettingsMock.mockRejectedValueOnce(new ApiError('la clave de TMDB no es válida', 422));
		const { getByPlaceholderText, getByText } = render(Page, { data: baseData() });

		const input = getByPlaceholderText('Nueva clave de TMDB');
		await fireEvent.input(input, { target: { value: 'clave-mala' } });
		await fireEvent.click(getByText('Guardar clave'));

		await waitFor(() => expect(getByText('la clave de TMDB no es válida')).toBeInTheDocument());
	});

	it('muestra versión, commit y fecha de compilación en Sistema', () => {
		const { getByText } = render(Page, { data: baseData() });
		expect(getByText('Versión v0.9.0')).toBeInTheDocument();
		expect(getByText('commit abc1234')).toBeInTheDocument();
		expect(getByText(/compilada el .*2026/)).toBeInTheDocument();
	});

	it('muestra el error de identificación 401 en cristiano', () => {
		const { getByText } = render(Page, {
			data: baseData({
				identify: {
					pending: 2,
					processing: false,
					online: true,
					processed: 1,
					last_error: 'tmdb /movie/1: 401 {"status_message":"Invalid API key: You must be granted a valid key."}'
				}
			})
		});
		expect(getByText(/La clave de TMDB no es válida/)).toBeInTheDocument();
	});
});
