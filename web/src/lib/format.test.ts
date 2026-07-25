import { describe, expect, it } from 'vitest';
import { addedEyebrow, providerLabel, shortLocator } from './format';

describe('addedEyebrow', () => {
	it('dice "Añadida hoy" para un timestamp de hace unos minutos', () => {
		const now = new Date(Date.now() - 5 * 60_000).toISOString();
		expect(addedEyebrow(now)).toBe('Añadida hoy');
	});

	it('dice "Añadida ayer" para hace ~1 día', () => {
		const yesterday = new Date(Date.now() - 25 * 3600_000).toISOString();
		expect(addedEyebrow(yesterday)).toBe('Añadida ayer');
	});

	it('dice "Añadida hace N días" para más de un día', () => {
		const fiveDaysAgo = new Date(Date.now() - 5 * 86_400_000 - 3600_000).toISOString();
		expect(addedEyebrow(fiveDaysAgo)).toBe('Añadida hace 5 días');
	});
});

describe('providerLabel', () => {
	it('mapea providers conocidos a sus etiquetas del mockup', () => {
		expect(providerLabel('localfs')).toBe('LOCAL');
		expect(providerLabel('http')).toBe('ENLACE');
		expect(providerLabel('drive')).toBe('DRIVE');
		expect(providerLabel('mega')).toBe('MEGA');
	});

	it('providers futuros/desconocidos caen a su nombre en mayúsculas', () => {
		expect(providerLabel('futuro')).toBe('FUTURO');
	});
});

describe('shortLocator', () => {
	it('no toca cadenas cortas', () => {
		expect(shortLocator('http://x.com/a.mp4')).toBe('http://x.com/a.mp4');
	});

	it('abrevia con … cadenas largas, conservando inicio y final', () => {
		const long = 'https://drive.google.com/file/d/1xK2ABCDEFGHIJKLMNOPQRSTUVWXYZ9fQ/view';
		const short = shortLocator(long, 30);
		expect(short.length).toBe(30);
		expect(short).toContain('…');
		expect(short.startsWith('https://drive')).toBe(true);
		expect(short.endsWith('/view')).toBe(true);
	});
});
