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

	it('muestra el badge NUEVO si se añadió hace menos de 24h', () => {
		const { getByText } = render(Card, {
			item: makeItem({ added_at: new Date().toISOString() })
		});
		expect(getByText('NUEVO')).toBeInTheDocument();
	});

	it('no muestra NUEVO si se añadió hace más de 24h', () => {
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

	it('variante wish muestra "Sin fuente" y "buscando…", sin puntos de salud', () => {
		const { getByText, container } = render(Card, {
			item: makeItem({ wished: true, health_summary: {} }),
			variant: 'wish'
		});
		expect(getByText('Sin fuente')).toBeInTheDocument();
		expect(container.querySelector('.m')?.textContent).toContain('buscando');
	});
});
