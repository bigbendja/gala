// Cliente de API tipado (Brief M10a §3): wrapper mínimo sobre fetch, con
// mensajes de error en español para "sin conexión con el backend" y errores
// HTTP. Nada de librerías de fetch/HTTP: es un helper mínimo justificado
// (CLAUDE.md: "sí se permiten utilidades mínimas si hacen falta").
import type { HealthSummary, Item, ListItemsParams, ListResult, Volume } from './types';

export class ApiError extends Error {
	status?: number;
	constructor(message: string, status?: number) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
	}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	let res: Response;
	try {
		res = await fetch(path, init);
	} catch {
		throw new ApiError('No se pudo conectar con GALA. Comprueba que el programa sigue en marcha.');
	}
	if (!res.ok) {
		let detail = '';
		try {
			const body = (await res.json()) as { error?: string };
			detail = body?.error ?? '';
		} catch {
			// el cuerpo de error no era JSON; se usa el mensaje genérico de abajo
		}
		throw new ApiError(detail || `El servidor respondió con un error (${res.status}).`, res.status);
	}
	return (await res.json()) as T;
}

/** GET /api/v1/items — Brief M10a §3. */
export function listItems(params: ListItemsParams = {}): Promise<ListResult> {
	const q = new URLSearchParams();
	if (params.kind) q.set('kind', params.kind);
	if (params.q) q.set('q', params.q);
	if (params.genre) q.set('genre', params.genre);
	if (params.year) q.set('year', String(params.year));
	if (params.wished !== undefined) q.set('wished', String(params.wished));
	if (params.confirmed !== undefined) q.set('confirmed', String(params.confirmed));
	if (params.parent_id !== undefined) {
		q.set('parent_id', params.parent_id === 'null' ? '' : String(params.parent_id));
	}
	if (params.in_progress !== undefined) q.set('in_progress', String(params.in_progress));
	q.set('limit', String(params.limit ?? 60));
	q.set('offset', String(params.offset ?? 0));
	return request<ListResult>(`/api/v1/items?${q.toString()}`);
}

/** GET /api/v1/items/{id} — ficha con fuentes. */
export function getItem(id: number): Promise<Item> {
	return request<Item>(`/api/v1/items/${id}`);
}

/** POST (wished=true) o DELETE (wished=false) /api/v1/items/{id}/wish —
 *  Brief M10a-fix B2, botón "+ Mi lista" de la ficha. */
export function setWished(id: number, wished: boolean): Promise<{ status: string; wished: boolean }> {
	return request(`/api/v1/items/${id}/wish`, { method: wished ? 'POST' : 'DELETE' });
}

/** GET /api/v1/health/summary — indicador global (M6). */
export function healthSummary(): Promise<HealthSummary> {
	return request<HealthSummary>('/api/v1/health/summary');
}

/** GET /api/v1/volumes — volúmenes conocidos (M2); usado solo para mostrar
 *  la etiqueta real del disco de origen en el eyebrow del hero. */
export function listVolumes(): Promise<Volume[]> {
	return request<Volume[]>('/api/v1/volumes');
}

/**
 * URL de una carátula servida por GET /api/v1/img/{archivo}. poster_path/
 * backdrop_path de un Item son NOMBRES DE ARCHIVO, no rutas ni URLs
 * completas (internal/api/identify.go, handleImg). undefined/null cuando el
 * item no tiene esa imagen (placeholder degradado en <Card>/<Hero>).
 */
export function imgUrl(filename: string | undefined | null): string | null {
	if (!filename) return null;
	return `/api/v1/img/${encodeURIComponent(filename)}`;
}
