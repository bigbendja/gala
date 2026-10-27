import { describe, expect, it } from 'vitest';
import { nextEpisode, seriesStatusText, seriesSummary } from './series';
import type { Item } from './api/types';

let n = 1;
function ep(over: Partial<Item> = {}): Item {
	const id = n++;
	return {
		id,
		kind: 'episode',
		title: `Ep ${id}`,
		genres: [],
		confirmed: true,
		wished: false,
		added_at: '',
		updated_at: '',
		source_count: 1,
		health_summary: { viva: 1 },
		...over
	};
}

describe('seriesSummary / seriesStatusText', () => {
	it('deriva el estado de los episodios, no de la serie', () => {
		const s = seriesSummary({
			1: [ep(), ep({ source_count: 0, health_summary: undefined })],
			2: [ep({ health_summary: { caída: 1 } })]
		});
		expect(s).toEqual({ episodes: 3, available: 2, healthy: 1 });
		expect(seriesStatusText(s)).toBe('2 de 3 episodios con fuente.');
	});

	it('textos de los casos límite', () => {
		expect(seriesStatusText(seriesSummary({}))).toContain('Todavía no hay episodios');
		expect(seriesStatusText(seriesSummary({ 1: [ep({ source_count: 0 })] }))).toContain('Ninguno');
		expect(seriesStatusText(seriesSummary({ 1: [ep(), ep()] }))).toBe('2 episodios disponibles.');
		expect(seriesStatusText(seriesSummary({ 1: [ep()] }))).toBe('1 episodio disponible.');
	});
});

describe('nextEpisode', () => {
	it('prioriza el episodio a medias más reciente', () => {
		const a = ep({ progress: { position_s: 10, watched: false, updated_at: '2026-10-01T00:00:00Z' } });
		const b = ep({ progress: { position_s: 5, watched: false, updated_at: '2026-10-05T00:00:00Z' } });
		const c = ep();
		expect(nextEpisode([a, b, c])?.id).toBe(b.id);
	});

	it('si no hay a medias, el primero sin ver con fuente', () => {
		const visto = ep({ progress: { position_s: 100, watched: true, updated_at: '' } });
		const sinFuente = ep({ source_count: 0 });
		const toca = ep();
		expect(nextEpisode([visto, sinFuente, toca])?.id).toBe(toca.id);
	});

	it('si todo está visto, el primero; sin fuentes, null', () => {
		const a = ep({ progress: { position_s: 1, watched: true, updated_at: '' } });
		const b = ep({ progress: { position_s: 1, watched: true, updated_at: '' } });
		expect(nextEpisode([a, b])?.id).toBe(a.id);
		expect(nextEpisode([ep({ source_count: 0 })])).toBeNull();
		expect(nextEpisode([])).toBeNull();
	});
});
