// Utilidades mínimas de formato para vistas (Brief M10a §3: "utilidades
// mínimas si hacen falta, justificadas"). Nada de librerías de fechas/i18n:
// el rango de casos es pequeño y controlado.
import type { Item } from './api/types';

/** "Añadida hoy" / "Añadida ayer" / "Añadida hace N días", a partir de
 * added_at (RFC3339, siempre en UTC desde el backend). */
export function addedEyebrow(addedAt: string): string {
	const added = new Date(addedAt);
	const days = Math.floor((Date.now() - added.getTime()) / 86_400_000);
	if (days <= 0) return 'Añadida hoy';
	if (days === 1) return 'Añadida ayer';
	return `Añadida hace ${days} días`;
}

// catalog.Source.Provider -> etiqueta corta del mockup (.tag). No se
// distingue "M3U" de un enlace directo: ambos llegan como provider="http"
// (la procedencia M3U solo consta en ingest_lists.type, no en la propia
// fuente; ver informe de la entrega — cruzarlo exigiría una petición extra
// por fuente).
const PROVIDER_LABEL: Record<string, string> = {
	localfs: 'LOCAL',
	http: 'ENLACE',
	drive: 'DRIVE',
	mega: 'MEGA'
};

export function providerLabel(provider: string): string {
	return PROVIDER_LABEL[provider] ?? provider.toUpperCase();
}

/** Abrevia una ruta/URL larga para el src-stack, tipo "drive.google.com/…/9fQ". */
export function shortLocator(locator: string, maxLen = 46): string {
	if (locator.length <= maxLen) return locator;
	const head = Math.ceil((maxLen - 1) * 0.6);
	const tail = maxLen - 1 - head;
	return `${locator.slice(0, head)}…${locator.slice(-tail)}`;
}

// Brief M10a-fix A3: el detalle de salud (internal/health/*.go) a veces
// incluye el nombre del paquete Go que originó el error (p. ej.
// "httpprovider: no se pudo resolver…"), un detalle de implementación que
// el usuario no debe ver. Se limpia SOLO para mostrar; el dato real que
// guarda la API no cambia.
const INTERNAL_DETAIL_PREFIX = /^(httpprovider|catalog|identify|ingest|media|stream|health):\s*/i;

/** Detalle de salud legible, sin nombres de paquete internos. null si no
 * hay detalle que mostrar. */
export function cleanHealthDetail(detail: string | undefined | null): string | null {
	if (!detail) return null;
	return detail.replace(INTERNAL_DETAIL_PREFIX, '');
}

// Brief M10a-fix A4: un item sin identificar (M3 aún no lo confirmó, o no
// tiene tmdb_id) guarda el nombre de archivo CRUDO tal cual llegó del
// escaneo/ingesta (puntos y guiones bajos de un nombre de fichero típico).
// No se toca ese dato ni la lógica de identificación: esto es solo
// presentación.
export function isUnidentified(item: Pick<Item, 'confirmed' | 'tmdb_id'>): boolean {
	return !item.confirmed || !item.tmdb_id;
}

/** Título de presentación: limpia separadores de nombre de archivo SOLO
 * cuando el item no está identificado; un item ya identificado por TMDB
 * conserva su título real tal cual. */
export function displayTitle(item: Pick<Item, 'title' | 'confirmed' | 'tmdb_id'>): string {
	if (!isUnidentified(item)) return item.title;
	return item.title
		.replace(/[._]+/g, ' ')
		.replace(/\s+/g, ' ')
		.trim();
}
