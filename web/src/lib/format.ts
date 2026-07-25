// Utilidades mínimas de formato para vistas (Brief M10a §3: "utilidades
// mínimas si hacen falta, justificadas"). Nada de librerías de fechas/i18n:
// el rango de casos es pequeño y controlado.

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
