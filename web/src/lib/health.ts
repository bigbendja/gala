// Traduce el campo `health` real de la API (Brief M10a §2: "mapa de
// salud→color") a la variable CSS que ya definió tokens.css y a una
// etiqueta legible en español. Punto único de esta traducción: <HealthDot>
// y cualquier otro sitio que pinte salud pasan por aquí, no reinventan el
// mapeo.
import type { Health } from './api/types';

const CSS_VAR: Record<Health, string> = {
	viva: 'var(--health-alive)',
	lenta: 'var(--health-slow)',
	caída: 'var(--health-dead)',
	desconectada: 'var(--health-disconnected)',
	sin_verificar: 'var(--health-unverified)'
};

const LABEL: Record<Health, string> = {
	viva: 'Viva',
	lenta: 'Lenta',
	caída: 'Caída',
	desconectada: 'Desconectada',
	sin_verificar: 'Sin verificar'
};

export function healthColor(health: Health | string): string {
	return CSS_VAR[health as Health] ?? 'var(--muted)';
}

export function healthLabel(health: Health | string): string {
	return LABEL[health as Health] ?? health;
}
