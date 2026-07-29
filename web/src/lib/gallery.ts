// FASE 0 tarea 0.4: qué items de un listado crudo de la API se consideran
// "vistos" en una sección de galería general (Tu galería). Extraído a un
// módulo aparte (mismo patrón que player.ts en M10b) para poder testear la
// regla sin montar la ruta completa.
import type { Item } from './api/types';

/**
 * Un vídeo que el usuario añadió explícitamente (fuente propia, http o
 * local) nunca debe quedar invisible solo porque M3 no lo identificó
 * contra TMDB — la mayoría de enlaces directos (CDNs, vídeos caseros,
 * clips sueltos) nunca van a tener match en TMDB. Por eso esta función ya
 * NO exige `confirmed` para `kind=other`.
 *
 * Criterio, deliberadamente distinto según kind:
 * - `movie`: se ve igual que siempre (sin cambios de comportamiento aquí;
 *   el caso de una película confirmada que se queda sin fuente —p. ej. tras
 *   quitar su carpeta— es un problema aparte, anotado en ISSUES-MVP2.md, no
 *   parte de este fix).
 * - `series`: se ve igual que siempre. Una serie NUNCA tiene fuente propia
 *   (sus temporadas/episodios sí), así que nunca se le exige source_count.
 * - `other` (provisional, típicamente un enlace directo sin match en TMDB):
 *   ESTE es el caso que arregla la Fase 0 tarea 0.4 — se ve mientras tenga
 *   al menos una fuente reproducible (`source_count > 0`), sin esperar a
 *   `confirmed`. Con su marca "Sin identificar" (M10a-fix) hasta que el
 *   usuario lo resuelva en "Por confirmar": confirmar, dejar así, o
 *   descartar.
 * - `season`/`episode`: nunca sueltos aquí (cuelgan de su serie en la ficha).
 *
 * En todos los casos, un item marcado `junk` no se ve (M3 ya lo descartó
 * como no-título real; sigue visible, solo lectura, en
 * "Por confirmar" → Descartados).
 */
export function galleryVisible(items: Item[]): Item[] {
	return items.filter((it) => {
		if (it.identify_status === 'junk') return false;
		if (it.kind === 'movie' || it.kind === 'series') return true;
		if (it.kind === 'other') return it.source_count > 0;
		return false;
	});
}
