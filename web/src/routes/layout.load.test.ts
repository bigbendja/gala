import { afterEach, describe, expect, it, vi } from 'vitest';
import { load } from './+layout';

afterEach(() => {
	vi.unstubAllGlobals();
});

function stubApi(routes: Record<string, unknown>) {
	vi.stubGlobal(
		'fetch',
		vi.fn((url: string) => {
			const key = Object.keys(routes).find((k) => String(url).includes(k));
			return Promise.resolve({
				ok: key !== undefined,
				status: key !== undefined ? 200 : 404,
				json: () => Promise.resolve(key !== undefined ? routes[key] : { error: 'no' })
			});
		})
	);
}

// Biblioteca vacía: /pending responde null. El layout raíz corre en TODAS las
// rutas; si lanza, toda la app muestra "500 Internal Error".
describe('layout raíz con biblioteca vacía', () => {
	it('no lanza cuando /pending devuelve null', async () => {
		stubApi({
			'/api/v1/lan/session': { local: true, pin_required: false, authenticated: true },
			'/api/v1/pending': null,
			'/api/v1/lan/status': { enabled: false }
		});
		const data = await load({} as never);
		expect(data).toMatchObject({ pendingCount: 0, lanEnabled: false });
	});

	it('cuenta solo los pendientes en revisión', async () => {
		stubApi({
			'/api/v1/lan/session': { local: true, pin_required: false, authenticated: true },
			'/api/v1/pending': [
				{ id: 1, identify_status: 'review' },
				{ id: 2, identify_status: 'junk' }
			],
			'/api/v1/lan/status': { enabled: true }
		});
		const data = await load({} as never);
		expect(data).toMatchObject({ pendingCount: 1, lanEnabled: true });
	});
});
