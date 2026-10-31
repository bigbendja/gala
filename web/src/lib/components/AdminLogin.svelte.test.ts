import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import AdminLogin from './AdminLogin.svelte';
import { ApiError } from '$lib/api/client';

const { adminLoginMock } = vi.hoisted(() => ({ adminLoginMock: vi.fn() }));
vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return { ...actual, adminLogin: adminLoginMock };
});

const ok = { configured: true, authenticated: false, is_localhost: false, network_admin: true };

beforeEach(() => {
	adminLoginMock.mockReset();
});

describe('AdminLogin', () => {
	it('inicia sesión y avisa', async () => {
		adminLoginMock.mockResolvedValue(undefined);
		const onDone = vi.fn();
		const { getByLabelText, getByText } = render(AdminLogin, { status: ok, onDone });
		await fireEvent.input(getByLabelText('Contraseña'), { target: { value: 'una-clave-larga' } });
		await fireEvent.click(getByText('Entrar'));
		await waitFor(() => expect(onDone).toHaveBeenCalled());
		expect(adminLoginMock).toHaveBeenCalledWith('una-clave-larga');
	});

	it('enseña el error del servidor y vacía el campo', async () => {
		adminLoginMock.mockImplementation(() => Promise.reject(new ApiError('Contraseña incorrecta. Inténtalo de nuevo.', 401, 'bad_password')));
		const { getByLabelText, getByText, findByRole } = render(AdminLogin, { status: ok, onDone: vi.fn() });
		const input = getByLabelText('Contraseña') as HTMLInputElement;
		await fireEvent.input(input, { target: { value: 'mala-clave-x' } });
		await fireEvent.click(getByText('Entrar'));
		expect((await findByRole('alert')).textContent).toContain('Contraseña incorrecta');
		expect(input.value).toBe('');
	});

	it('sin contraseña configurada no enseña el formulario', () => {
		const { queryByLabelText, getByText } = render(AdminLogin, {
			status: { ...ok, configured: false },
			onDone: vi.fn()
		});
		expect(queryByLabelText('Contraseña')).toBeNull();
		expect(getByText(/no está configurada/)).toBeInTheDocument();
	});

	it('con la administración de red desactivada lo explica', () => {
		const { queryByLabelText, getByText } = render(AdminLogin, {
			status: { ...ok, network_admin: false },
			onDone: vi.fn()
		});
		expect(queryByLabelText('Contraseña')).toBeNull();
		expect(getByText(/desactivada/)).toBeInTheDocument();
	});
});
