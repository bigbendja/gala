import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import LandscapeCard from './LandscapeCard.svelte';
import type { Item } from '$lib/api/types';

function makeItem(overrides: Partial<Item> = {}): Item {
	return {
		id: 7,
		kind: 'movie',
		title: 'Origen',
		genres: ['Acción'],
		confirmed: true,
		tmdb_id: 27205,
		wished: false,
		added_at: '2020-01-01T00:00:00Z',
		updated_at: '2020-01-01T00:00:00Z',
		source_count: 1,
		...overrides
	};
}

describe('LandscapeCard', () => {
	it('usa el backdrop, enseña año, nota de TMDB y salud, y enlaza a la ficha', () => {
		const { container, getByText, getByRole } = render(LandscapeCard, {
			item: makeItem({
				year: 2010,
				vote_average: 8.4,
				backdrop_path: 'backdrop_w780_x.jpg',
				poster_path: 'poster_w342_x.jpg',
				health_summary: { viva: 1 }
			})
		});
		expect(getByRole('link')).toHaveAttribute('href', '/item/7');
		expect(container.querySelector('img')?.getAttribute('src')).toContain('backdrop_w780_x.jpg');
		expect(getByText('2010')).toBeInTheDocument();
		expect(getByText('★ 8,4')).toBeInTheDocument();
		expect(container.querySelector('[role="img"]')).not.toBeNull(); // punto de salud
		expect(container.textContent).not.toMatch(/Castellano|Dual|VOSE|Latino/); // no hay idioma
	});

	it('una serie enseña «año · N temp.» y sin nota no inventa ninguna', () => {
		const { getByText, queryByText } = render(LandscapeCard, {
			item: makeItem({ kind: 'series', year: 2008, season_count: 5 })
		});
		expect(getByText('2008 · 5 temp.')).toBeInTheDocument();
		expect(queryByText(/★/)).toBeNull();
	});

	it('sin backdrop recorta el póster; sin nada, degradado (nunca un hueco)', () => {
		const withPoster = render(LandscapeCard, { item: makeItem({ poster_path: 'poster_w342_x.jpg' }) });
		expect(withPoster.container.querySelector('img')?.getAttribute('src')).toContain('poster_w342_x.jpg');
		expect(withPoster.container.querySelector('.poster-fallback')).not.toBeNull();
		withPoster.unmount();

		const bare = render(LandscapeCard, { item: makeItem() });
		expect(bare.container.querySelector('img')).toBeNull();
		expect(bare.container.querySelector('.thumb')?.className).toMatch(/\bp[1-8]\b/);
		expect(bare.getByText('Origen')).toBeInTheDocument();
	});

	it('NUEVO solo si se añadió hace poco', () => {
		const fresh = render(LandscapeCard, { item: makeItem({ added_at: new Date().toISOString() }) });
		expect(fresh.getByText('NUEVO')).toBeInTheDocument();
		fresh.unmount();
		const old = render(LandscapeCard, { item: makeItem() });
		expect(old.queryByText('NUEVO')).toBeNull();
	});
});
