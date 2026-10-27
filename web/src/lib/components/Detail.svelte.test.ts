import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import CastRail from './CastRail.svelte';
import FileSpecs from './FileSpecs.svelte';
import SimilarRail from './SimilarRail.svelte';

describe('CastRail', () => {
	it('foto si la hay; iniciales (nunca hueco roto) si no o si falla al cargar', async () => {
		const { container } = render(CastRail, {
			cast: [
				{ id: 1, name: 'Song Kang-ho', character: 'Ki-taek', photo: 'profile_w185_a.jpg' },
				{ id: 2, name: 'Lee Sun-kyun' }
			]
		});
		const imgs = container.querySelectorAll('img');
		expect(imgs).toHaveLength(1);
		expect(imgs[0].getAttribute('src')).toContain('profile_w185_a.jpg');
		expect(screen.getByText('Ki-taek')).toBeInTheDocument();
		expect(screen.getByText('LS')).toBeInTheDocument(); // sin foto: iniciales

		// Si la imagen falla al cargar, cae a iniciales.
		await fireEvent.error(imgs[0]);
		expect(container.querySelectorAll('img')).toHaveLength(0);
		expect(screen.getByText('SK')).toBeInTheDocument();
	});
});

describe('SimilarRail', () => {
	it('el que está en la biblioteca enlaza a su ficha; el que no, se ve apagado y dice por qué', () => {
		const { container } = render(SimilarRail, {
			items: [
				{ tmdb_id: 1, kind: 'movie', title: 'Burning', year: 2018, item_id: 42 },
				{ tmdb_id: 2, kind: 'movie', title: 'Oldboy', year: 2003 }
			]
		});
		const link = container.querySelector('a.scard') as HTMLAnchorElement;
		expect(link.getAttribute('href')).toBe('/item/42');
		const out = container.querySelector('div.scard.out') as HTMLElement;
		expect(out.getAttribute('title')).toBe('Este título no está en tu galería');
		expect(out.textContent).toContain('Fuera de tu galería');
		expect(container.querySelectorAll('a')).toHaveLength(1);
	});
});

describe('FileSpecs', () => {
	it('enseña solo lo que se sabe, con códecs legibles', () => {
		const { container } = render(FileSpecs, {
			file: { source_id: 1, container: 'matroska,webm', video_codec: 'hevc', audio_codec: 'ac3', width: 1920, height: 1080, size_bytes: 2254857830 },
			addedAt: new Date(Date.now() - 3 * 86_400_000).toISOString()
		});
		const text = container.textContent ?? '';
		expect(text).toContain('1920 × 1080');
		expect(text).toContain('HEVC (H.265)');
		expect(text).toContain('Dolby Digital (AC3)');
		expect(text).toContain('MKV');
		expect(text).toContain('2,1 GB');
		expect(text).toContain('hace 3 días');
		expect(text).not.toContain('Duración'); // sin dato, sin fila
	});

	it('sin análisis del archivo, solo "Añadida"', () => {
		const { container } = render(FileSpecs, { file: null, addedAt: new Date().toISOString() });
		expect(container.querySelectorAll('.row')).toHaveLength(1);
		expect(container.textContent).toContain('Añadida');
	});
});
