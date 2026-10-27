import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import EpisodeRow from './EpisodeRow.svelte';
import HeroSlider from './HeroSlider.svelte';
import HomeSection from './HomeSection.svelte';
import StateBlock from './StateBlock.svelte';
import type { Item } from '$lib/api/types';

vi.mock('$lib/api/client', async () => {
	const actual = await vi.importActual<typeof import('$lib/api/client')>('$lib/api/client');
	return {
		...actual,
		setWished: vi.fn().mockResolvedValue({ status: 'ok', wished: true }),
		getItemDetails: vi.fn().mockResolvedValue({
			has_details: true,
			runtime_min: 148,
			vote_average: 8.4,
			countries: [],
			cast: [],
			crew: [],
			similar: [],
			files: []
		})
	};
});

// jsdom no implementa la Web Animations API que usan las transiciones de Svelte.
beforeAll(() => {
	(Element.prototype as unknown as { animate: () => unknown }).animate = () => ({
		cancel() {},
		finish() {},
		onfinish: null,
		currentTime: 0,
		finished: Promise.resolve()
	});
});

let n = 100;
function item(over: Partial<Item> = {}): Item {
	const id = n++;
	return {
		id,
		kind: 'movie',
		title: `Título ${id}`,
		genres: ['Drama'],
		confirmed: true,
		wished: false,
		added_at: `2026-10-${String((id % 27) + 1).padStart(2, '0')}T10:00:00Z`,
		updated_at: '',
		source_count: 1,
		backdrop_path: 'bd.jpg',
		poster_path: 'p.jpg',
		overview: 'Sinopsis del título',
		tmdb_id: id,
		...over
	};
}

describe('StateBlock', () => {
	it('un vacío ofrece su acción; un error es un alert', async () => {
		const onAction = vi.fn();
		const { rerender } = render(StateBlock, { title: 'Aún no hay nada aquí', text: 'Añade una carpeta.', actionLabel: '＋ Añadir', onAction });
		await fireEvent.click(screen.getByRole('button', { name: '＋ Añadir' }));
		expect(onAction).toHaveBeenCalled();
		await rerender({ title: 'No se pudo cargar', tone: 'error', actionLabel: 'Reintentar', onAction });
		expect(screen.getByRole('alert')).toBeInTheDocument();
	});
});

describe('EpisodeRow', () => {
	it('enseña número y título real, nunca el nombre crudo "Serie T1E01"', () => {
		const ep = item({ kind: 'episode', title: 'Piloto', episode_number: 1, poster_path: 'still.jpg', overview: 'Walter White…' });
		render(EpisodeRow, { episode: ep, seasonNumber: 1 });
		expect(screen.getByText('T1 · E1')).toBeInTheDocument();
		expect(screen.getByText('Piloto')).toBeInTheDocument();
		expect(screen.getByText('Walter White…')).toBeInTheDocument();
	});

	it('mientras llega el título de TMDB, "Episodio N" y no "Breaking Bad T1E01"', () => {
		const ep = item({ kind: 'episode', title: 'Breaking Bad T1E01', episode_number: 1, poster_path: undefined });
		const { container } = render(EpisodeRow, { episode: ep, seasonNumber: 1 });
		expect(container.textContent).toContain('Episodio 1');
		expect(container.textContent).not.toContain('T1E01');
		// Sin miniatura: respaldo sobrio con el número, no un hueco.
		expect(container.querySelector('.fallback')?.textContent).toBe('1');
	});

	it('sin fuente: el botón de reproducir está apagado y lo dice', () => {
		const ep = item({ kind: 'episode', title: 'X', episode_number: 2, source_count: 0 });
		const { container } = render(EpisodeRow, { episode: ep, seasonNumber: 1 });
		expect(screen.getByText('Sin fuente')).toBeInTheDocument();
		expect(container.querySelector('a.play')).toBeNull();
		expect(container.querySelector('.play.off')).toBeTruthy();
	});
});

describe('HeroSlider', () => {
	beforeEach(() => {
		try {
			localStorage.clear();
		} catch {
			// sin almacenamiento
		}
	});

	it('el filtro decide qué alimenta el hero y se puede cambiar', async () => {
		const viejo = item({ title: 'Viejo', added_at: '2026-01-01T00:00:00Z' });
		const nuevo = item({ title: 'Nuevo', added_at: '2026-10-20T00:00:00Z' });
		const visto = item({ title: 'Visto', added_at: '2026-10-25T00:00:00Z', progress: { position_s: 5, watched: true, updated_at: '' } });
		const { container } = render(HeroSlider, { items: [viejo, nuevo, visto], intervalMs: 60_000 });

		await fireEvent.click(screen.getByRole('button', { name: 'Nuevos' }));
		expect(container.querySelector('.slide.active h1')?.textContent).toBe('Visto'); // el más reciente primero

		await fireEvent.click(screen.getByRole('button', { name: 'Sin ver' }));
		expect(container.querySelector('.slide.active h1')?.textContent).toBe('Nuevo'); // "Visto" ya no sale
		expect(screen.getByRole('button', { name: 'Sin ver' }).getAttribute('aria-pressed')).toBe('true');
	});

	it('las flechas y los puntos cambian de diapositiva y dan la vuelta', async () => {
		const a = item({ title: 'A', added_at: '2026-10-03T00:00:00Z' });
		const b = item({ title: 'B', added_at: '2026-10-02T00:00:00Z' });
		const { container } = render(HeroSlider, { items: [a, b], intervalMs: 60_000 });
		await fireEvent.click(screen.getByRole('button', { name: 'Nuevos' }));
		expect(container.querySelector('.slide.active h1')?.textContent).toBe('A');
		await fireEvent.click(screen.getByRole('button', { name: 'Título siguiente' }));
		expect(container.querySelector('.slide.active h1')?.textContent).toBe('B');
		await fireEvent.click(screen.getByRole('button', { name: 'Título siguiente' }));
		expect(container.querySelector('.slide.active h1')?.textContent).toBe('A');
		await fireEvent.click(screen.getByRole('button', { name: 'Título anterior' }));
		expect(container.querySelector('.slide.active h1')?.textContent).toBe('B');
	});

	it('una serie lleva a sus episodios; una película, al reproductor', async () => {
		const peli = item({ title: 'Peli', added_at: '2026-10-05T00:00:00Z' });
		const serie = item({ title: 'Serie', kind: 'series', source_count: 0, added_at: '2026-10-04T00:00:00Z' });
		const { container } = render(HeroSlider, { items: [peli, serie], intervalMs: 60_000 });
		await fireEvent.click(screen.getByRole('button', { name: 'Nuevos' }));
		expect(container.querySelector('.slide.active a.btn-play')?.getAttribute('href')).toBe(`/ver/${peli.id}`);
		await fireEvent.click(screen.getByRole('button', { name: 'Título siguiente' }));
		expect(container.querySelector('.slide.active a.btn-play')?.getAttribute('href')).toBe(`/item/${serie.id}`);
		expect(container.querySelector('.slide.active a.btn-play')?.textContent).toContain('episodios');
	});

	it('gira solo, y deja de girar con prefers-reduced-motion', async () => {
		vi.useFakeTimers();
		try {
			const a = item({ title: 'A', added_at: '2026-10-03T00:00:00Z' });
			const b = item({ title: 'B', added_at: '2026-10-02T00:00:00Z' });
			localStorage.setItem('gala.home.heroMode', 'nuevos');
			const { container, unmount } = render(HeroSlider, { items: [a, b], intervalMs: 1000 });
			expect(container.querySelector('.slide.active h1')?.textContent).toBe('A');
			await vi.advanceTimersByTimeAsync(1100);
			expect(container.querySelector('.slide.active h1')?.textContent).toBe('B');
			unmount();

			const original = window.matchMedia;
			window.matchMedia = ((q: string) => ({
				matches: q.includes('reduce'),
				media: q,
				addEventListener: () => {},
				removeEventListener: () => {}
			})) as unknown as typeof window.matchMedia;
			const calm = render(HeroSlider, { items: [a, b], intervalMs: 1000 });
			await vi.advanceTimersByTimeAsync(3500);
			expect(calm.container.querySelector('.slide.active h1')?.textContent).toBe('A');
			window.matchMedia = original;
		} finally {
			vi.useRealTimers();
		}
	});

	it('si ningún título del filtro tiene qué enseñar, lo dice (no un hueco)', async () => {
		const visto = item({ title: 'Visto', progress: { position_s: 5, watched: true, updated_at: '' } });
		render(HeroSlider, { items: [visto], intervalMs: 60_000 });
		await fireEvent.click(screen.getByRole('button', { name: 'Sin ver' }));
		expect(screen.getByText('Nada que enseñar con este filtro')).toBeInTheDocument();
	});
});

describe('HomeSection', () => {
	const page = (items: Item[]) => ({ items, total: items.length, limit: 40, offset: 0 });

	it('con datos pinta la fila; vacía, se oculta (no deja un hueco)', async () => {
		const withItems = render(HomeSection, { title: 'Películas', lazy: false, fetchPage: async () => page([item({ title: 'Dune' })]) });
		expect(await screen.findByText('Dune')).toBeInTheDocument();
		withItems.unmount();

		const empty = render(HomeSection, { title: 'Documentales', lazy: false, fetchPage: async () => page([]) });
		await waitFor(() => expect(empty.container.querySelector('[aria-busy="true"]')).toBeNull());
		expect(empty.container.querySelector('section')).toBeNull();
	});

	it('mientras carga, skeleton; si falla, mensaje en español y reintentar', async () => {
		const { ApiError } = await import('$lib/api/client');
		let calls = 0;
		const fetchPage = vi.fn(async () => {
			calls++;
			if (calls === 1) throw new ApiError('boom', 500);
			return page([item({ title: 'Parásitos' })]);
		});
		const { container } = render(HomeSection, { title: 'Series', lazy: false, fetchPage });
		expect(container.querySelector('[aria-busy="true"]')).toBeTruthy();
		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('No se pudo cargar');
		expect(alert.textContent).not.toContain('boom');
		await fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }));
		expect(await screen.findByText('Parásitos')).toBeInTheDocument();
	});

	it('aplica el filtro (p. ej. película sin fuente no se enseña)', async () => {
		const ok = item({ title: 'Con fuente' });
		const ghost = item({ title: 'Fantasma', source_count: 0 });
		render(HomeSection, {
			title: 'Películas',
			lazy: false,
			fetchPage: async () => page([ok, ghost]),
			filter: (items) => items.filter((i) => i.source_count > 0)
		});
		expect(await screen.findByText('Con fuente')).toBeInTheDocument();
		expect(screen.queryByText('Fantasma')).toBeNull();
	});
});


describe('HeroSlider — mockup de la Home', () => {
	it('muestra duración y nota de TMDB cuando llegan, y solo la diapositiva activa lleva el h1', async () => {
		const a = item({ title: 'A', added_at: '2026-10-03T00:00:00Z' });
		const b = item({ title: 'B', added_at: '2026-10-02T00:00:00Z' });
		const { container } = render(HeroSlider, { items: [a, b], intervalMs: 60_000 });
		await fireEvent.click(screen.getByRole('button', { name: 'Nuevos' }));
		await waitFor(() => expect(container.textContent).toContain('★ 8,4'));
		expect(container.textContent).toContain('2 h 28 min');
		expect(container.querySelectorAll('h1')).toHaveLength(1);
		// Las demás diapositivas existen (fundido entre ellas) pero ocultas a lectores (y con inert para el teclado).
		const hidden = container.querySelectorAll('.slide[aria-hidden="true"]');
		expect(hidden.length).toBe(1);
	});

	it('el backdrop es la imagen de fondo de la diapositiva', () => {
		const a = item({ title: 'A', backdrop_path: 'backdrop_w1280_x.jpg' });
		const { container } = render(HeroSlider, { items: [a], intervalMs: 60_000 });
		const bg = container.querySelector('.slide.active .slide-bg') as HTMLElement;
		expect(bg.style.backgroundImage).toContain('backdrop_w1280_x.jpg');
	});
});
