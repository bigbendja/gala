import { describe, expect, it } from 'vitest';
import { galleryVisible } from './gallery';
import type { Item } from './api/types';

function item(overrides: Partial<Item> = {}): Item {
	return {
		id: 1,
		kind: 'movie',
		title: 'x',
		genres: [],
		confirmed: true,
		wished: false,
		added_at: new Date(0).toISOString(),
		updated_at: new Date(0).toISOString(),
		source_count: 1,
		...overrides
	};
}

describe('galleryVisible (Fase 0 tarea 0.4)', () => {
	it('un enlace directo sin identificar (kind=other, confirmed=false) con fuente SÍ se ve', () => {
		const it1 = item({ id: 28, kind: 'other', confirmed: false, identify_status: 'review', source_count: 1 });
		expect(galleryVisible([it1])).toEqual([it1]);
	});

	it('un item kind=other en pending/error también se ve (no hace falta esperar a que M3 termine)', () => {
		const pending = item({ id: 2, kind: 'other', confirmed: false, identify_status: 'pending', source_count: 1 });
		const errored = item({ id: 3, kind: 'other', confirmed: false, identify_status: 'error', source_count: 1 });
		expect(galleryVisible([pending, errored])).toEqual([pending, errored]);
	});

	it('un kind=other SIN fuente todavía no se ve (nada que reproducir)', () => {
		const noSource = item({ id: 4, kind: 'other', confirmed: false, identify_status: 'pending', source_count: 0 });
		expect(galleryVisible([noSource])).toEqual([]);
	});

	it('un item marcado junk NO se ve (sigue solo en Por confirmar → Descartados)', () => {
		const junk = item({ id: 5, kind: 'other', confirmed: false, identify_status: 'junk', source_count: 1 });
		expect(galleryVisible([junk])).toEqual([]);
	});

	it('season/episode sueltos NO se ven (cuelgan de su serie en la ficha)', () => {
		const season = item({ id: 6, kind: 'season', source_count: 0 });
		const episode = item({ id: 7, kind: 'episode', source_count: 1 });
		expect(galleryVisible([season, episode])).toEqual([]);
	});

	it('movie/series confirmados se ven igual que siempre (sin cambio de comportamiento)', () => {
		const movie = item({ id: 8, kind: 'movie', confirmed: true, source_count: 2 });
		// una serie nunca tiene fuente propia (la tienen sus episodios); debe
		// seguir viéndose aunque source_count sea 0, exactamente como antes.
		const series = item({ id: 9, kind: 'series', confirmed: true, source_count: 0 });
		expect(galleryVisible([movie, series])).toEqual([movie, series]);
	});
});
