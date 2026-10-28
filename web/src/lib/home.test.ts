import { describe, expect, it } from 'vitest';
import {
	HERO_MAX,
	genreRows,
	heroCandidates,
	pickHero,
	playHref,
	playLabel,
	playableMovies
} from './home';
import type { Item } from './api/types';

let nextId = 1;
function item(over: Partial<Item> = {}): Item {
	const id = nextId++;
	return {
		id,
		kind: 'movie',
		title: `Título ${id}`,
		genres: [],
		confirmed: true,
		wished: false,
		added_at: '2026-10-01T10:00:00Z',
		updated_at: '2026-10-01T10:00:00Z',
		source_count: 1,
		backdrop_path: `bd${id}.jpg`,
		overview: 'Sinopsis',
		tmdb_id: id,
		...over
	};
}

const NOW = Date.parse('2026-10-27T12:00:00Z');

describe('heroCandidates', () => {
	it('exige fondo, y fuente salvo en series', () => {
		const ok = item();
		const sinFondo = item({ backdrop_path: undefined });
		const sinFuente = item({ source_count: 0 });
		const serie = item({ kind: 'series', source_count: 0 });
		const basura = item({ identify_status: 'junk' });
		const episodio = item({ kind: 'episode' });
		const ids = heroCandidates([ok, sinFondo, sinFuente, serie, basura, episodio]).map((i) => i.id);
		expect(ids).toEqual([ok.id, serie.id]);
	});
});

describe('pickHero', () => {
	it('"nuevos" ordena por fecha de alta, la más reciente primero, con tope', () => {
		const items = Array.from({ length: 12 }, (_, i) =>
			item({ added_at: `2026-10-${String(i + 1).padStart(2, '0')}T10:00:00Z` })
		);
		const got = pickHero(items, 'nuevos', NOW);
		expect(got).toHaveLength(HERO_MAX);
		expect(got[0].added_at.startsWith('2026-10-12')).toBe(true);
		expect(got[1].added_at.startsWith('2026-10-11')).toBe(true);
	});

	it('"sin ver" descarta lo que este dispositivo ya ha empezado o visto', () => {
		const visto = item({ progress: { position_s: 10, watched: true, updated_at: '' } });
		const empezado = item({ progress: { position_s: 10, watched: false, updated_at: '' } });
		const nuevo = item();
		expect(pickHero([visto, empezado, nuevo], 'sin_ver', NOW).map((i) => i.id)).toEqual([nuevo.id]);
	});

	it('"destacados" solo toma lo identificado y con sinopsis, y es estable en el mismo día', () => {
		const a = item();
		const b = item();
		const sinSinopsis = item({ overview: undefined });
		const sinIdentificar = item({ confirmed: false });
		const first = pickHero([a, b, sinSinopsis, sinIdentificar], 'destacados', NOW);
		expect(first.map((i) => i.id).sort()).toEqual([a.id, b.id].sort());
		const again = pickHero([a, b, sinSinopsis, sinIdentificar], 'destacados', NOW + 3_600_000);
		expect(again.map((i) => i.id)).toEqual(first.map((i) => i.id));
	});

	it('el hero se acota a 5 destacados aunque haya más títulos con fondo', () => {
		expect(HERO_MAX).toBe(5);
		const items = Array.from({ length: 9 }, () => item());
		expect(pickHero(items, 'destacados', NOW)).toHaveLength(5);
		expect(pickHero(items, 'nuevos', NOW)).toHaveLength(5);
	});

	it('"destacados" rota entre días (no devuelve siempre el mismo orden)', () => {
		const items = Array.from({ length: 8 }, () => item());
		const orders = new Set<string>();
		for (let d = 0; d < 6; d++) {
			orders.add(
				pickHero(items, 'destacados', NOW + d * 86_400_000)
					.map((i) => i.id)
					.join(',')
			);
		}
		expect(orders.size).toBeGreaterThan(1);
	});

	it('sin candidatos devuelve vacío en cualquier modo', () => {
		for (const mode of ['destacados', 'nuevos', 'sin_ver'] as const) {
			expect(pickHero([], mode, NOW)).toEqual([]);
		}
	});
});

describe('playHref / playLabel', () => {
	it('una serie lleva a su ficha; lo demás, al reproductor', () => {
		expect(playHref({ id: 5, kind: 'series' })).toBe('/item/5');
		expect(playHref({ id: 6, kind: 'movie' })).toBe('/ver/6');
		expect(playHref({ id: 7, kind: 'other' })).toBe('/ver/7');
		expect(playLabel({ kind: 'series' })).toContain('episodios');
		expect(playLabel({ kind: 'movie' })).toContain('Reproducir');
	});
});

describe('genreRows', () => {
	it('quita Documental y limita el número de filas', () => {
		const genres = [
			{ name: 'Drama', count: 9 },
			{ name: 'Documental', count: 8 },
			...['A', 'B', 'C', 'D', 'E', 'F', 'G'].map((name) => ({ name, count: 1 }))
		];
		const rows = genreRows(genres);
		expect(rows.map((g) => g.name)).not.toContain('Documental');
		expect(rows).toHaveLength(6);
		expect(rows[0].name).toBe('Drama');
	});
});

describe('playableMovies', () => {
	it('oculta películas sin fuente y basura, deja series y otros', () => {
		const sinFuente = item({ source_count: 0 });
		const ok = item();
		const serie = item({ kind: 'series', source_count: 0 });
		const basura = item({ identify_status: 'junk' });
		expect(playableMovies([sinFuente, ok, serie, basura]).map((i) => i.id)).toEqual([ok.id, serie.id]);
	});
});
