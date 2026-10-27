// Lógica pura de la ficha de serie (Sistema de Diseño GALA §3.1): el estado de
// fuentes de una serie se DERIVA de sus episodios — una serie nunca tiene
// fuente propia — y "Reproducir" lleva al episodio que toca.
import type { Item } from './api/types';

export interface SeriesSummary {
	/** Episodios catalogados. */
	episodes: number;
	/** Episodios con al menos una fuente. */
	available: number;
	/** Episodios con alguna fuente viva o lenta (reproducibles ahora). */
	healthy: number;
}

export function seriesSummary(episodesBySeason: Record<number, Item[]>): SeriesSummary {
	let episodes = 0;
	let available = 0;
	let healthy = 0;
	for (const list of Object.values(episodesBySeason)) {
		for (const ep of list) {
			episodes++;
			if (ep.source_count > 0) available++;
			const hs = ep.health_summary ?? {};
			if ((hs.viva ?? 0) + (hs.lenta ?? 0) > 0) healthy++;
		}
	}
	return { episodes, available, healthy };
}

/** Texto de estado de la cabecera de una serie, derivado de sus episodios. */
export function seriesStatusText(s: SeriesSummary): string {
	if (s.episodes === 0) return 'Todavía no hay episodios catalogados de esta serie.';
	if (s.available === 0) return 'Ninguno de sus episodios tiene fuente todavía.';
	const eps = (n: number) => `${n} ${n === 1 ? 'episodio' : 'episodios'}`;
	if (s.available === s.episodes) {
		return s.episodes === 1 ? '1 episodio disponible.' : `${s.episodes} episodios disponibles.`;
	}
	return `${s.available} de ${eps(s.episodes)} con fuente.`;
}

/**
 * Episodio al que lleva "Reproducir" en una serie: el que está a medias
 * (el más reciente), si no el primero sin ver con fuente, y si todos están
 * vistos, el primero. Recibe los episodios en orden temporada→episodio.
 */
export function nextEpisode(ordered: Item[]): Item | null {
	const playable = ordered.filter((e) => e.source_count > 0);
	if (playable.length === 0) return null;
	const inProgress = playable
		.filter((e) => e.progress && !e.progress.watched && e.progress.position_s > 0)
		.sort((a, b) => (b.progress?.updated_at ?? '').localeCompare(a.progress?.updated_at ?? ''));
	if (inProgress.length > 0) return inProgress[0];
	return playable.find((e) => !e.progress?.watched) ?? playable[0];
}
