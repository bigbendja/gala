// Lógica pura de la Home nueva (hero-slider y filas), separada de los
// componentes para poder testearla sin montar nada (mismo patrón que
// gallery.ts y player.ts).
import type { Item } from './api/types';
import { galleryVisible } from './gallery';

/** Filtros del hero: qué alimenta el slider. */
export type HeroMode = 'destacados' | 'nuevos' | 'sin_ver';

export const HERO_MODES: { id: HeroMode; label: string }[] = [
	{ id: 'destacados', label: 'Destacados' },
	{ id: 'nuevos', label: 'Nuevos' },
	{ id: 'sin_ver', label: 'Sin ver' }
];

/** Máximo de diapositivas del hero. */
export const HERO_MAX = 5;

/** Género de TMDB (en español) que alimenta la fila "Documentales". */
export const DOCUMENTARY_GENRE = 'Documental';

/** Cuántos géneros dinámicos se muestran como filas, como máximo. */
export const MAX_GENRE_ROWS = 6;

/**
 * Candidatos a hero: títulos de nivel superior con fondo (backdrop) y algo que
 * ver. Lo que no tiene fondo no sale en el hero (nunca se inventa una imagen);
 * una película o enlace sin fuente tampoco (no se podría reproducir). Una
 * serie nunca tiene fuente propia: la tienen sus episodios.
 */
export function heroCandidates(items: Item[]): Item[] {
	return galleryVisible(items).filter((it) => {
		if (!it.backdrop_path) return false;
		if (it.kind === 'series') return true;
		return it.source_count > 0;
	});
}

/** Día (número de días desde 1970, UTC) para la selección diaria. */
export function dayNumber(now: number): number {
	return Math.floor(now / 86_400_000);
}

// Hash determinista y barato (mezcla tipo Knuth) de (id, día): "Destacados"
// cambia de un día a otro, pero no baraja en cada recarga.
function dailyScore(id: number, day: number): number {
	let h = (id * 2654435761 + day * 40503) >>> 0;
	h = ((h ^ (h >>> 15)) * 2246822519) >>> 0;
	h = ((h ^ (h >>> 13)) * 3266489917) >>> 0;
	return (h ^ (h >>> 16)) >>> 0;
}

/**
 * Diapositivas del hero según el filtro.
 * - destacados: la selección del día entre lo identificado (con sinopsis);
 *   no hay una puntuación de TMDB guardada, así que "destacado" es una
 *   rotación diaria estable, no un ranking.
 * - nuevos: lo añadido más recientemente.
 * - sin_ver: lo que este dispositivo aún no ha empezado, lo más nuevo primero.
 */
export function pickHero(items: Item[], mode: HeroMode, now: number = Date.now()): Item[] {
	const pool = heroCandidates(items);
	let picked: Item[];
	switch (mode) {
		case 'nuevos':
			picked = [...pool].sort((a, b) => b.added_at.localeCompare(a.added_at));
			break;
		case 'sin_ver':
			picked = pool
				.filter((it) => !it.progress)
				.sort((a, b) => b.added_at.localeCompare(a.added_at));
			break;
		default: {
			const day = dayNumber(now);
			picked = pool
				.filter((it) => it.confirmed && !!it.overview)
				.sort((a, b) => dailyScore(a.id, day) - dailyScore(b.id, day));
		}
	}
	return picked.slice(0, HERO_MAX);
}

/** A dónde lleva "Reproducir": una serie se elige desde su ficha (episodios). */
export function playHref(item: Pick<Item, 'id' | 'kind'>): string {
	return item.kind === 'series' ? `/item/${item.id}` : `/ver/${item.id}`;
}

/** Etiqueta del botón principal del hero. */
export function playLabel(item: Pick<Item, 'kind'>): string {
	return item.kind === 'series' ? '▶ Ver episodios' : '▶ Reproducir';
}

/** Géneros que merecen fila propia: los más poblados, sin el de Documentales
 *  (que ya tiene su sección). */
export function genreRows<T extends { name: string; count: number }>(genres: T[]): T[] {
	return genres.filter((g) => g.name !== DOCUMENTARY_GENRE && g.count > 0).slice(0, MAX_GENRE_ROWS);
}

/** Lo que se enseña en una fila de películas: con fuente (un fantasma sin
 *  fuente no se puede reproducir). */
export function playableMovies(items: Item[]): Item[] {
	return items.filter((it) => it.identify_status !== 'junk' && (it.kind !== 'movie' || it.source_count > 0));
}
