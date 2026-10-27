import { afterEach, describe, expect, it, vi } from 'vitest';
import { listFolders, listIngestLists, listPending, listVolumes } from './client';

function mockFetchBody(body: unknown) {
	vi.stubGlobal(
		'fetch',
		vi.fn().mockResolvedValue({ ok: true, status: 200, json: () => Promise.resolve(body) })
	);
}

afterEach(() => {
	vi.unstubAllGlobals();
});

// Un servidor que devuelva `null` en vez de [] no debe tumbar a quien recorre
// la lista (era el caso de /pending con la BD vacía).
describe('listas tolerantes a null', () => {
	it.each([
		['listPending', listPending],
		['listFolders', listFolders],
		['listVolumes', listVolumes],
		['listIngestLists', listIngestLists]
	])('%s devuelve [] si el servidor responde null', async (_n, fn) => {
		mockFetchBody(null);
		expect(await fn()).toEqual([]);
	});

	it('listPending conserva los datos cuando los hay', async () => {
		mockFetchBody([{ id: 1, identify_status: 'review' }]);
		expect(await listPending()).toHaveLength(1);
	});
});
