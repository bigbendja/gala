import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import SecurityPanel from './SecurityPanel.svelte';
import type { AuthSession, AuthStatus } from '$lib/api/types';

const m = vi.hoisted(() => ({
	getAuthStatus: vi.fn(),
	listAuthSessions: vi.fn(),
	changeAdminPassword: vi.fn(),
	adminLogoutAll: vi.fn(),
	adminLogout: vi.fn(),
	setNetworkAdmin: vi.fn()
}));

vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return { ...actual, ...m };
});

const status = (o: Partial<AuthStatus> = {}): AuthStatus => ({
	configured: true,
	authenticated: true,
	is_localhost: true,
	network_admin: true,
	...o
});
const sess = (o: Partial<AuthSession> = {}): AuthSession => ({
	id: 'abc',
	device: 'Chrome en Windows',
	ip: '192.168.1.40',
	created_at: new Date().toISOString(),
	last_seen: new Date().toISOString(),
	current: false,
	...o
});

beforeEach(() => {
	Object.values(m).forEach((f) => f.mockReset());
	m.listAuthSessions.mockResolvedValue([]);
});

describe('Ajustes › Seguridad', () => {
	it('sin contraseña, desde el PC: deja ponerla (sin pedir la actual)', async () => {
		m.getAuthStatus.mockResolvedValue(status({ configured: false }));
		m.changeAdminPassword.mockResolvedValue({ ok: true });
		const { findByLabelText, getByText, queryByLabelText } = render(SecurityPanel);
		const nueva = await findByLabelText('Nueva contraseña');
		expect(queryByLabelText('Contraseña actual')).toBeNull();
		await fireEvent.input(nueva, { target: { value: 'una-clave-larga' } });
		await fireEvent.input(await findByLabelText('Repetir contraseña'), { target: { value: 'una-clave-larga' } });
		await fireEvent.click(getByText('Poner contraseña'));
		await waitFor(() => expect(m.changeAdminPassword).toHaveBeenCalledWith('', 'una-clave-larga'));
	});

	it('valida longitud y coincidencia antes de enviar', async () => {
		m.getAuthStatus.mockResolvedValue(status());
		const { findByLabelText, getByText } = render(SecurityPanel);
		await fireEvent.input(await findByLabelText('Contraseña actual'), { target: { value: 'vieja-clave-1' } });
		await fireEvent.input(await findByLabelText('Nueva contraseña'), { target: { value: 'corta' } });
		expect(getByText('Mínimo 8 caracteres.')).toBeInTheDocument();
		await fireEvent.input(await findByLabelText('Nueva contraseña'), { target: { value: 'nueva-clave-123' } });
		await fireEvent.input(await findByLabelText('Repetir contraseña'), { target: { value: 'otra-distinta-1' } });
		expect(getByText('Las dos contraseñas no coinciden.')).toBeInTheDocument();
		expect((getByText('Cambiar contraseña') as HTMLButtonElement).disabled).toBe(true);
	});

	it('lista las sesiones y marca la de este dispositivo', async () => {
		m.getAuthStatus.mockResolvedValue(status({ is_localhost: false }));
		m.listAuthSessions.mockResolvedValue([
			sess({ current: true }),
			sess({ id: 'x', device: 'Firefox en Linux', ip: '192.168.1.22' })
		]);
		const { findByText, getByText } = render(SecurityPanel);
		expect(await findByText('Este dispositivo')).toBeInTheDocument();
		expect(getByText('activa')).toBeInTheDocument();
		expect(getByText('Firefox en Linux')).toBeInTheDocument();
	});

	it('cerrar todas las sesiones pide confirmación', async () => {
		m.getAuthStatus.mockResolvedValue(status());
		m.listAuthSessions.mockResolvedValue([sess()]);
		m.adminLogoutAll.mockResolvedValue({ ok: true });
		const { findByText, getByText } = render(SecurityPanel);
		await fireEvent.click(await findByText('Cerrar todas las sesiones'));
		expect(m.adminLogoutAll).not.toHaveBeenCalled();
		await fireEvent.click(getByText('Sí, cerrar'));
		await waitFor(() => expect(m.adminLogoutAll).toHaveBeenCalled());
	});

	it('el interruptor de red solo funciona desde el PC', async () => {
		m.getAuthStatus.mockResolvedValue(status({ is_localhost: false }));
		const { findByRole } = render(SecurityPanel);
		const sw = (await findByRole('switch')) as HTMLButtonElement;
		expect(sw.disabled).toBe(true);
	});

	it('desde el PC el interruptor llama al backend', async () => {
		m.getAuthStatus.mockResolvedValue(status());
		m.setNetworkAdmin.mockResolvedValue({ network_admin: false });
		const { findByRole } = render(SecurityPanel);
		await fireEvent.click(await findByRole('switch'));
		await waitFor(() => expect(m.setNetworkAdmin).toHaveBeenCalledWith(false));
	});

	it('desde la red y sin contraseña configurada, explica qué hacer', async () => {
		m.getAuthStatus.mockResolvedValue(status({ configured: false, is_localhost: false, authenticated: false }));
		const { findByText } = render(SecurityPanel);
		expect(await findByText(/Pon una contraseña desde el equipo o por SSH/)).toBeInTheDocument();
	});
});
