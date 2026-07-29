import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Card from './Card.svelte';
import type { Item } from '$lib/api/types';

function makeItem(overrides: Partial<Item> = {}): Item {
	return {
		id: 1,
		kind: 'movie',
		title: 'Cielo de Harmattan',
		genres: ['Drama'],
		confirmed: true,
		tmdb_id: 27205,
		wished: false,
		added_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
		source_count: 1,
		...overrides
	};
}

describe('Card', () => {
	it('enlaza a /item/{id} y muestra el título y el año', () => {
		const { getByRole, getAllByText, getByText } = render(Card, {
			item: makeItem({ id: 42, year: 2023 })
		});
		const link = getByRole('link');
		expect(link).toHaveAttribute('href', '/item/42');
		// El título aparece dos veces sin carátula real: como arte de
		// respaldo (como el mockup) y como texto de la tarjeta.
		expect(getAllByText('Cielo de Harmattan')).toHaveLength(2);
		expect(getByText('2023')).toBeInTheDocument();
	});

	it('muestra el badge NUEVO recién añadido', () => {
		const { getByText } = render(Card, {
			item: makeItem({ added_at: new Date().toISOString() })
		});
		expect(getByText('NUEVO')).toBeInTheDocument();
	});

	it('muestra NUEVO dentro de la ventana de 48h (Brief M10a-fix A1)', () => {
		const { getByText } = render(Card, {
			item: makeItem({ added_at: new Date(Date.now() - 30 * 3600_000).toISOString() })
		});
		expect(getByText('NUEVO')).toBeInTheDocument();
	});

	it('no muestra NUEVO pasadas las 48h', () => {
		const { queryByText } = render(Card, {
			item: makeItem({ added_at: new Date(Date.now() - 3 * 86_400_000).toISOString() })
		});
		expect(queryByText('NUEVO')).not.toBeInTheDocument();
	});

	it('reconstruye un HealthDot por fuente desde health_summary', () => {
		const { container } = render(Card, {
			item: makeItem({ health_summary: { viva: 2, caída: 1 } })
		});
		expect(container.querySelectorAll('[role="img"]')).toHaveLength(3);
	});

	it('variante wish SIN fuente (deseo aspiracional) muestra "Sin fuente" y "buscando…"', () => {
		const { getByText, container } = render(Card, {
			item: makeItem({ wished: true, source_count: 0, health_summary: {} }),
			variant: 'wish'
		});
		expect(getByText('Sin fuente')).toBeInTheDocument();
		expect(container.querySelector('.m')?.textContent).toContain('buscando');
	});

	// Fase 0 — diagnóstico integral: M10a-fix permite desear cualquier item
	// YA existente (con fuente real), no solo títulos aspiracionales; la
	// tarjeta seguía mostrando "Sin fuente · buscando…" siempre en variant
	// wish, aunque el item fuera perfectamente reproducible — mentía sobre
	// el estado real del item.
	it('variante wish CON fuente NO dice "Sin fuente"/"buscando" — se ve como una tarjeta normal', () => {
		const { queryByText, getByText } = render(Card, {
			item: makeItem({ wished: true, source_count: 1, year: 2023 }),
			variant: 'wish'
		});
		expect(queryByText('Sin fuente')).not.toBeInTheDocument();
		expect(queryByText(/buscando/)).not.toBeInTheDocument();
		expect(getByText('2023')).toBeInTheDocument();
	});

	it('variante wish CON fuente pero sin identificar muestra "Sin identificar", no el año', () => {
		const { getByText, queryByText } = render(Card, {
			item: makeItem({
				wished: true,
				source_count: 1,
				confirmed: false,
				tmdb_id: undefined,
				year: 2023
			}),
			variant: 'wish'
		});
		expect(getByText('Sin identificar')).toBeInTheDocument();
		expect(queryByText('2023')).not.toBeInTheDocument();
		expect(queryByText('Sin fuente')).not.toBeInTheDocument();
	});

	it('limpia el título crudo y marca "Sin identificar" para items sin confirmar (Brief M10a-fix A4)', () => {
		const { getAllByText, getByText, queryByText } = render(Card, {
			item: makeItem({
				title: 'Pelicula.2019.1080p.x264-GRUPO',
				confirmed: false,
				tmdb_id: undefined,
				year: 2019
			})
		});
		expect(getAllByText('Pelicula 2019 1080p x264-GRUPO')).toHaveLength(2);
		expect(getByText('Sin identificar')).toBeInTheDocument();
		// El año no se muestra a la vez que "Sin identificar" (no hay
		// espacio ni sentido en mostrar los dos).
		expect(queryByText('2019')).not.toBeInTheDocument();
	});

	it('un item confirmado y con tmdb_id NO se marca sin identificar', () => {
		const { queryByText } = render(Card, {
			item: makeItem({ confirmed: true, tmdb_id: 27205 })
		});
		expect(queryByText('Sin identificar')).not.toBeInTheDocument();
	});
});
