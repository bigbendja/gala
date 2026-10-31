import { afterEach, describe, expect, it, vi } from 'vitest';
import { adminLogin, listFolders, removeFolder, setCsrfToken } from './client';

afterEach(() => {
	setCsrfToken('');
	vi.unstubAllGlobals();
});

function stubFetch(body: unknown = {}) {
	const f = vi.fn(() => Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) }));
	vi.stubGlobal('fetch', f);
	return f;
}

function headerOf(call: unknown[], name: string): string | null {
	const init = call[1] as RequestInit | undefined;
	return new Headers(init?.headers).get(name);
}

describe('token CSRF de administración', () => {
	it('el login lo guarda y las escrituras lo mandan', async () => {
		const f = stubFetch({ ok: true, csrf: 'tok123' });
		await adminLogin('una-clave-larga');
		f.mockClear();
		await removeFolder('/media');
		expect(headerOf(f.mock.calls[0] as unknown[], 'X-GALA-CSRF')).toBe('tok123');
	});

	it('las lecturas no lo mandan', async () => {
		setCsrfToken('tok123');
		const f = stubFetch([]);
		await listFolders();
		expect(headerOf(f.mock.calls[0] as unknown[], 'X-GALA-CSRF')).toBeNull();
	});

	it('sin sesión (desde el PC) no se manda nada', async () => {
		const f = stubFetch({});
		await removeFolder('/media');
		expect(headerOf(f.mock.calls[0] as unknown[], 'X-GALA-CSRF')).toBeNull();
	});

	it('el código del backend llega en el ApiError', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(() =>
				Promise.resolve({
					ok: false,
					status: 401,
					json: () => Promise.resolve({ error: 'Contraseña incorrecta.', code: 'bad_password' })
				})
			)
		);
		await expect(adminLogin('x')).rejects.toMatchObject({ status: 401, code: 'bad_password' });
	});
});

describe('evento de sesión caducada', () => {
	it('se lanza una sola vez aunque lleguen varios 401 seguidos', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(() =>
				Promise.resolve({
					ok: false,
					status: 401,
					json: () => Promise.resolve({ error: 'Inicia sesión', code: 'login_required' })
				})
			)
		);
		const { ADMIN_LOGIN_EVENT } = await import('./client');
		const handler = vi.fn();
		window.addEventListener(ADMIN_LOGIN_EVENT, handler);
		for (let i = 0; i < 5; i++) await listFolders().catch(() => {});
		window.removeEventListener(ADMIN_LOGIN_EVENT, handler);
		expect(handler.mock.calls.length).toBeLessThanOrEqual(1);
	});
});
