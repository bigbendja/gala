import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, getItem, imgUrl, listItems } from './client';

function mockFetchOnce(body: unknown, init: { ok?: boolean; status?: number } = {}) {
	const { ok = true, status = 200 } = init;
	vi.stubGlobal(
		'fetch',
		vi.fn().mockResolvedValue({
			ok,
			status,
			json: () => Promise.resolve(body)
		})
	);
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('listItems', () => {
	it('construye la query string con los parámetros dados', async () => {
		mockFetchOnce({ items: [], total: 0, limit: 60, offset: 0 });
		await listItems({ kind: 'movie', confirmed: true, limit: 10, offset: 20 });

		const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
		expect(calledUrl).toBe('/api/v1/items?kind=movie&confirmed=true&limit=10&offset=20');
	});

	it('usa límite/offset por defecto si no se dan', async () => {
		mockFetchOnce({ items: [], total: 0, limit: 60, offset: 0 });
		await listItems();
		const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
		expect(calledUrl).toBe('/api/v1/items?limit=60&offset=0');
	});
});

describe('getItem', () => {
	it('devuelve el item decodificado', async () => {
		mockFetchOnce({ id: 5, title: 'Peli' });
		const item = await getItem(5);
		expect(item).toEqual({ id: 5, title: 'Peli' });
	});

	it('lanza ApiError con el mensaje del backend en un error HTTP', async () => {
		mockFetchOnce({ error: 'no encontrado' }, { ok: false, status: 404 });
		await expect(getItem(999)).rejects.toMatchObject(
			new ApiError('no encontrado', 404)
		);
	});

	it('lanza ApiError con mensaje de "sin conexión" si fetch falla', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn().mockRejectedValue(new TypeError('network error'))
		);
		await expect(getItem(1)).rejects.toThrow('No se pudo conectar con GALA');
	});
});

describe('imgUrl', () => {
	it('construye la URL de /api/v1/img/ para un nombre de archivo', () => {
		expect(imgUrl('poster_w342_abc.jpg')).toBe('/api/v1/img/poster_w342_abc.jpg');
	});

	it('devuelve null sin nombre de archivo (placeholder degradado)', () => {
		expect(imgUrl(undefined)).toBeNull();
		expect(imgUrl(null)).toBeNull();
		expect(imgUrl('')).toBeNull();
	});
});
