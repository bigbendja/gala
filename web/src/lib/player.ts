// Lógica de reproducción SIN DOM (Brief M10b §1/§5: testable sin montar el
// componente). El estado del <video>/hls.js real vive en Player.svelte; aquí
// solo la decisión de "qué fuente toca ahora".
import type { PlaySourceView } from './api/types';

/** direct/remux son reproducibles de verdad; reject/analyzing no (Brief M4). */
export function isPlayable(mode: PlaySourceView['mode']): boolean {
	return mode === 'direct' || mode === 'remux';
}

/**
 * Siguiente fuente a intentar tras un fallo (Brief M10b §1: "saltar
 * automáticamente a la siguiente fuente"), respetando el orden que ya
 * decidió el backend (salud → provider → prioridad → calidad). Nunca
 * reintenta una fuente ya excluida en esta sesión de reproducción, y nunca
 * elige automáticamente una fuente reject/analyzing (esas solo se prueban
 * si el usuario las elige a mano en el selector).
 */
export function pickNextSource(
	sources: PlaySourceView[],
	excluded: ReadonlySet<number>
): PlaySourceView | null {
	return sources.find((s) => isPlayable(s.mode) && !excluded.has(s.source_id)) ?? null;
}
