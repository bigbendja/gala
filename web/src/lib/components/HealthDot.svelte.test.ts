import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import HealthDot from './HealthDot.svelte';

describe('HealthDot', () => {
	it('traduce "viva" al color ok y a la etiqueta en español', () => {
		const { getByRole } = render(HealthDot, { health: 'viva' });
		const dot = getByRole('img');
		expect(dot).toHaveAttribute('aria-label', 'Viva');
		expect(dot.style.background).toBe('var(--health-alive)');
	});

	it('traduce "caída" al color bad', () => {
		const { getByRole } = render(HealthDot, { health: 'caída' });
		expect(getByRole('img').style.background).toBe('var(--health-dead)');
	});

	it('un estado desconocido no rompe: cae a --muted', () => {
		const { getByRole } = render(HealthDot, { health: 'lo-que-sea' });
		expect(getByRole('img').style.background).toBe('var(--muted)');
	});
});
