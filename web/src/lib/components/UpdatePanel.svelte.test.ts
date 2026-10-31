import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import UpdatePanel from './UpdatePanel.svelte';
import { ApiError } from '$lib/api/client';
import type { UpdateInfo, UpdateStatus } from '$lib/api/types';

const m = vi.hoisted(() => ({
	getUpdateStatus: vi.fn(),
	checkUpdate: vi.fn(),
	applyUpdate: vi.fn(),
	dismissUpdateResult: vi.fn(),
	isUp: vi.fn(),
	runningVersion: vi.fn()
}));

vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return { ...actual, ...m };
});

const info = (o: Partial<UpdateInfo> = {}): UpdateInfo => ({
	current: 'v0.9.0',
	latest: 'v0.10.0',
	update_available: true,
	notes: '## Novedades\n- Cuenta de administrador\n* Mejoras de reproducción',
	page_url: 'https://github.com/bigbendja/gala/releases/tag/v0.10.0',
	asset_name: 'gala-linux-amd64',
	asset_url: 'https://github.com/x',
	asset_size: 11_000_000,
	checksum: 'ab',
	checked_at: new Date().toISOString(),
	...o
});

const status = (o: Partial<UpdateStatus> = {}): UpdateStatus => ({
	status: { phase: 'idle', done: 0, total: 0 },
	current: 'v0.9.0',
	can_apply: true,
	...o
});

beforeEach(() => {
	Object.values(m).forEach((f) => f.mockReset());
	m.getUpdateStatus.mockResolvedValue(status());
});

describe('Ajustes › Actualización', () => {
	it('enseña la versión actual y no consulta la red por su cuenta', async () => {
		const { findByText } = render(UpdatePanel);
		expect(await findByText('v0.9.0')).toBeInTheDocument();
		expect(m.checkUpdate).not.toHaveBeenCalled();
	});

	it('«Buscar actualizaciones» enseña la versión nueva con sus notas y el botón', async () => {
		m.checkUpdate.mockResolvedValue({ checked: true, current: 'v0.9.0', can_apply: true, info: info() });
		const { findByText, getByText } = render(UpdatePanel);
		await fireEvent.click(await findByText('Buscar actualizaciones'));
		expect(await findByText('v0.10.0')).toBeInTheDocument();
		expect(getByText('Cuenta de administrador')).toBeInTheDocument();
		expect(getByText('Mejoras de reproducción')).toBeInTheDocument();
		expect(getByText('Actualizar y reiniciar')).toBeInTheDocument();
		expect(getByText(/Se guardará la versión anterior/)).toBeInTheDocument();
		expect(m.checkUpdate).toHaveBeenCalledWith(true);
	});

	it('sin versión nueva dice que está al día', async () => {
		m.checkUpdate.mockResolvedValue({ checked: true, current: 'v0.9.0', can_apply: true, info: info({ update_available: false }) });
		const { findByText } = render(UpdatePanel);
		await fireEvent.click(await findByText('Buscar actualizaciones'));
		expect(await findByText(/Estás al día/)).toBeInTheDocument();
	});

	it('sin conexión lo dice sin romper nada', async () => {
		m.checkUpdate.mockResolvedValue({
			checked: false,
			current: 'v0.9.0',
			can_apply: true,
			error: 'No se pudo comprobar las actualizaciones. Revisa la conexión a Internet e inténtalo más tarde.'
		});
		const { findByText, queryByText } = render(UpdatePanel);
		await fireEvent.click(await findByText('Buscar actualizaciones'));
		expect(await findByText(/No se pudo comprobar las actualizaciones/)).toBeInTheDocument();
		expect(queryByText('Actualizar y reiniciar')).toBeNull();
	});

	it('si la instalación no puede actualizarse sola, explica por qué y enlaza la versión', async () => {
		m.getUpdateStatus.mockResolvedValue(
			status({ available: info(), can_apply: false, reason: 'Solo está disponible cuando GALA corre como servicio.' })
		);
		const { findByText, queryByText, getByText } = render(UpdatePanel);
		expect(await findByText(/corre como servicio/)).toBeInTheDocument();
		expect(queryByText('Actualizar y reiniciar')).toBeNull();
		expect(getByText(/Ver la versión/).getAttribute('href')).toContain('/releases/tag/v0.10.0');
	});

	it('al abrir enseña la versión disponible ya conocida', async () => {
		m.getUpdateStatus.mockResolvedValue(status({ available: info() }));
		const { findByText } = render(UpdatePanel);
		expect(await findByText('Actualizar y reiniciar')).toBeInTheDocument();
	});

	it('con una reproducción en curso pide confirmación y permite forzar', async () => {
		m.getUpdateStatus.mockResolvedValue(status({ available: info() }));
		m.applyUpdate.mockRejectedValueOnce(new ApiError('Hay una reproducción en curso.', 409, 'playback_active'));
		m.applyUpdate.mockResolvedValueOnce({ ok: true });
		const { findByText, getByText } = render(UpdatePanel);
		await fireEvent.click(await findByText('Actualizar y reiniciar'));
		expect(await findByText(/Hay una reproducción en curso/)).toBeInTheDocument();
		await fireEvent.click(getByText('Actualizar igualmente'));
		await waitFor(() => expect(m.applyUpdate).toHaveBeenLastCalledWith(true));
	});

	it('muestra el progreso de la descarga con porcentaje', async () => {
		m.getUpdateStatus.mockResolvedValue(
			status({
				available: info(),
				status: { phase: 'downloading', done: 5_500_000, total: 11_000_000 }
			})
		);
		const { findByRole, findByText } = render(UpdatePanel);
		expect(await findByText(/Descargando… 50%/)).toBeInTheDocument();
		const bar = await findByRole('progressbar');
		expect(bar.getAttribute('aria-valuenow')).toBe('50');
	});

	it('un error de la actualización se enseña tal cual', async () => {
		m.getUpdateStatus.mockResolvedValue(
			status({
				available: info(),
				status: { phase: 'error', done: 0, total: 0, error: 'La descarga está corrupta; no se ha instalado nada.' }
			})
		);
		const { findByText } = render(UpdatePanel);
		expect(await findByText(/descarga está corrupta/)).toBeInTheDocument();
	});

	it('tras una actualización con éxito recuerda que se puede volver a gala.prev', async () => {
		m.getUpdateStatus.mockResolvedValue(
			status({ result: { status: 'ok', from: 'v0.9.0', to: 'v0.10.0', at: new Date().toISOString() } })
		);
		m.dismissUpdateResult.mockResolvedValue({ ok: true });
		const { findByText, getByText, queryByText } = render(UpdatePanel);
		expect(await findByText('Actualizado a v0.10.0.')).toBeInTheDocument();
		expect(getByText('gala.prev')).toBeInTheDocument();
		expect(getByText(/Si algo queda mal aunque GALA haya arrancado bien/)).toBeInTheDocument();
		await fireEvent.click(getByText('Entendido'));
		await waitFor(() => expect(queryByText('Actualizado a v0.10.0.')).toBeNull());
		expect(m.dismissUpdateResult).toHaveBeenCalled();
	});

	it('si se revirtió, lo dice con las dos versiones y el motivo', async () => {
		m.getUpdateStatus.mockResolvedValue(
			status({
				result: { status: 'rolled_back', from: 'v0.9.0', to: 'v0.10.0', reason: 'La autocomprobación de la versión nueva falló.' }
			})
		);
		const { findByText, getByText } = render(UpdatePanel);
		expect(await findByText('La actualización a v0.10.0 falló y se revirtió a v0.9.0.')).toBeInTheDocument();
		expect(getByText(/autocomprobación/)).toBeInTheDocument();
	});
});
