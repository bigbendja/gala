// Cliente de API tipado (Brief M10a §3): wrapper mínimo sobre fetch, con
// mensajes de error en español para "sin conexión con el backend" y errores
// HTTP. Nada de librerías de fetch/HTTP: es un helper mínimo justificado
// (CLAUDE.md: "sí se permiten utilidades mínimas si hacen falta").
import type {
	ConfirmRequest,
	FolderInfo,
	HealthSummary,
	IdentifyStatus,
	IngestList,
	Item,
	LanSession,
	LanStatus,
	ListItemsParams,
	ListResult,
	GenreCount,
	HomeSettings,
	PendingItem,
	PlayResponse,
	Progress,
	ResumenIngesta,
	ScanStatus,
	TMDBSettings,
	Volume
} from './types';

/** Evento de window que se lanza cuando el backend pide el PIN del Modo LAN. */
export const PIN_REQUIRED_EVENT = 'gala:pin-required';

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
			const body = (await res.json()) as { error?: string; code?: string };
			detail = body?.error ?? '';
			// Modo LAN: la sesión del PIN caducó (o se cambió el PIN) con la
			// app abierta; el layout escucha esto y vuelve a pedirlo.
			if (res.status === 401 && body?.code === 'pin_required' && typeof window !== 'undefined') {
				window.dispatchEvent(new CustomEvent(PIN_REQUIRED_EVENT));
			}
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

/** Una lista que el servidor devuelva como `null` (o sin cuerpo) se trata como
 *  vacía: el frontend recorre las listas sin comprobar, y un `null` de más
 *  tumbaba toda la app con una BD vacía. */
async function orEmpty<T>(p: Promise<T[] | null | undefined>): Promise<T[]> {
	return (await p) ?? [];
}

/** GET /api/v1/volumes — volúmenes conocidos (M2); usado solo para mostrar
 *  la etiqueta real del disco de origen en el eyebrow del hero. */
export function listVolumes(): Promise<Volume[]> {
	return orEmpty(request<Volume[] | null>('/api/v1/volumes'));
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

// --- Reproducción (Brief M10b §1) -------------------------------------------

/** GET /api/v1/play/{item_id} — fuentes ordenadas, modo de cada una y la
 *  elegida por el backend. */
export function getPlay(itemId: number): Promise<PlayResponse> {
	return request<PlayResponse>(`/api/v1/play/${itemId}`);
}

/** URL de streaming directo de una fuente (GET /api/v1/stream/{source_id}):
 *  se usa tal cual como `src` de `<video>`, no hace falta pasar por fetch(). */
export function streamUrl(sourceId: number): string {
	return `/api/v1/stream/${sourceId}`;
}

/** GET /api/v1/stream/{source_id}/hls/start — crea una sesión de remux y
 *  devuelve dónde está su playlist. t = segundo desde el que arrancar (seek). */
export function startHLS(sourceId: number, t?: number): Promise<{ session_id: string; playlist_url: string }> {
	const q = t ? `?t=${encodeURIComponent(String(t))}` : '';
	return request(`/api/v1/stream/${sourceId}/hls/start${q}`);
}

/** DELETE /api/v1/stream/{source}/hls/{session} — libera la sesión de remux
 *  (al salir del reproductor, cambiar de fuente o rearrancar). `keepalive`
 *  la deja salir aunque la pestaña se esté cerrando. Nunca falla hacia fuera:
 *  si no llega, el servidor igualmente barre las sesiones huérfanas. */
export function stopHLS(sourceId: number, sessionId: string): void {
	fetch(`/api/v1/stream/${sourceId}/hls/${encodeURIComponent(sessionId)}`, {
		method: 'DELETE',
		keepalive: true
	}).catch(() => {});
}

/** POST /api/v1/sources/{id}/failed — el reproductor la llama cuando el
 *  <video> dispara "error" (Brief M10b §1: fallback visible a la siguiente
 *  fuente). */
export function reportSourceFailed(sourceId: number): Promise<{ status: string; health: string }> {
	return request(`/api/v1/sources/${sourceId}/failed`, { method: 'POST' });
}

/** GET /api/v1/playback/{item_id} — progreso guardado, o null si el
 *  dispositivo nunca reprodujo este item. */
export function getPlayback(itemId: number): Promise<Progress | null> {
	return request(`/api/v1/playback/${itemId}`);
}

/** PUT /api/v1/playback/{item_id} — guarda posición (Brief M10b §1: cada
 *  ~10s y al pausar; el backend decide "watched" según el umbral de M4). */
export function putPlayback(
	itemId: number,
	positionS: number,
	durationS?: number
): Promise<{ status: string; watched: boolean }> {
	return request(`/api/v1/playback/${itemId}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ position_s: positionS, duration_s: durationS })
	});
}

// --- Ingesta (Brief M10b §2) -------------------------------------------------

/** POST /api/v1/ingest — texto plano (el textarea) o un File arrastrado
 *  (multipart), Brief M5 §5 / M10b §2. */
export function ingest(input: string | File): Promise<ResumenIngesta> {
	if (typeof input === 'string') {
		return request('/api/v1/ingest', {
			method: 'POST',
			headers: { 'Content-Type': 'text/plain' },
			body: input
		});
	}
	// FormData: NO fijar Content-Type a mano — el navegador añade el
	// boundary correcto solo si se deja que lo calcule él.
	const form = new FormData();
	form.append('file', input);
	return request('/api/v1/ingest', { method: 'POST', body: form });
}

// --- Por confirmar (Brief M10b §3) ------------------------------------------

/** GET /api/v1/pending — items "Por confirmar" (junk incluidos; el
 *  frontend decide cómo agruparlos). */
export function listPending(): Promise<PendingItem[]> {
	return orEmpty(request<PendingItem[] | null>('/api/v1/pending'));
}

/** POST /api/v1/items/{id}/confirm — elige un candidato TMDB o confirma un
 *  título manual. */
export function confirmItem(id: number, body: ConfirmRequest): Promise<Item> {
	return request(`/api/v1/items/${id}/confirm`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
}

/**
 * URL de la carátula de un candidato SIN CONFIRMAR todavía: a diferencia de
 * imgUrl() (nuestra propia caché), este candidato aún no se descargó —
 * mostrar su vista previa exige pedirla directo al CDN de TMDB. Excepción
 * deliberada a "todo funciona offline" (CLAUDE.md regla 2): la búsqueda que
 * produjo este candidato ya fue online; esto es solo una vista previa de
 * ESE MISMO paso, no de contenido ya catalogado. Sin red, el <img>
 * simplemente no carga (la card de candidato lo trata como si no hubiera
 * carátula).
 */
export function candidatePosterUrl(posterPath: string | undefined): string | null {
	if (!posterPath) return null;
	return `https://image.tmdb.org/t/p/w154${posterPath}`;
}

/** DELETE /api/v1/items/{id} — descarta un item de "Por confirmar" que no
 *  es un título real (Brief M10c §3). Borra el item y sus fuentes (y, si
 *  tiene hijos, también los hijos). Sin confirmación aquí: la pide la UI. */
export function deleteItem(id: number): Promise<{ status: string }> {
	return request(`/api/v1/items/${id}`, { method: 'DELETE' });
}

// --- Ajustes (Brief M10c §2) -------------------------------------------------

/** GET /api/v1/folders — carpetas registradas con su estado de conexión. */
export function listFolders(): Promise<FolderInfo[]> {
	return orEmpty(request<FolderInfo[] | null>('/api/v1/folders'));
}

/** POST /api/v1/folders — añade una carpeta; 400 no existe, 409 duplicada. */
export function addFolder(path: string): Promise<FolderInfo> {
	return request('/api/v1/folders', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ path })
	});
}

/** DELETE /api/v1/folders?path=... — quita una carpeta (los items se
 *  conservan; solo se quitan sus fuentes locales). */
export function removeFolder(path: string): Promise<{ status: string }> {
	return request(`/api/v1/folders?${new URLSearchParams({ path }).toString()}`, { method: 'DELETE' });
}

/** POST /api/v1/scan — rescaneo de todas las carpetas registradas. */
export function startScan(): Promise<{ status: string }> {
	return request('/api/v1/scan', { method: 'POST' });
}

/** GET /api/v1/scan/status. */
export function scanStatus(): Promise<ScanStatus> {
	return request('/api/v1/scan/status');
}

/** GET /api/v1/identify/status — estado de la cola de identificación
 *  (Brief M10c §2: mostrar last_error en cristiano, p. ej. clave inválida). */
export function identifyStatus(): Promise<IdentifyStatus> {
	return request('/api/v1/identify/status');
}

/** POST /api/v1/health/check — verificación de salud bajo demanda. */
export function startHealthCheck(): Promise<{ status: string }> {
	return request('/api/v1/health/check', { method: 'POST' });
}

/** GET /api/v1/settings/tmdb — nunca devuelve la clave, solo si existe. */
export function getTMDBSettings(): Promise<TMDBSettings> {
	return request('/api/v1/settings/tmdb');
}

/** PUT /api/v1/settings/tmdb — valida la clave contra TMDB antes de
 *  guardarla (422 si es inválida, 503 si no hay conexión para probarla). */
export function putTMDBSettings(apiKey: string): Promise<{ status: string }> {
	return request('/api/v1/settings/tmdb', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ api_key: apiKey })
	});
}

/** GET /api/v1/ingest/lists — lotes/M3U ingeridos ("Listas" de la topbar). */
export function listIngestLists(): Promise<IngestList[]> {
	return orEmpty(request<IngestList[] | null>('/api/v1/ingest/lists'));
}

/** GET /api/v1/ping — versión y disponibilidad de ffmpeg, para Ajustes →
 *  Sistema (Brief M10c §2, sin inventar un endpoint nuevo solo para esto). */
export function ping(): Promise<{ status: string; app: string; version: string; ffmpeg_available: boolean }> {
	return request('/api/v1/ping');
}

// --- Modo LAN (M7) -----------------------------------------------------------

/** GET /api/v1/lan/session — ¿es el PC o un dispositivo de la red? ¿Falta el PIN? */
export function getLanSession(): Promise<LanSession> {
	return request('/api/v1/lan/session');
}

/** POST /api/v1/lan/auth — teclea el PIN desde un dispositivo de la red. */
export function lanAuth(pin: string): Promise<{ ok: boolean }> {
	return request('/api/v1/lan/auth', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ pin })
	});
}

/** GET /api/v1/lan/status — solo desde el PC. */
export function getLanStatus(): Promise<LanStatus> {
	return request('/api/v1/lan/status');
}

/** POST /api/v1/lan/toggle — activa o desactiva el Modo LAN (solo desde el PC). */
export function setLanEnabled(enabled: boolean): Promise<LanStatus> {
	return request('/api/v1/lan/toggle', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ enabled })
	});
}

/** PUT /api/v1/lan/pin — pone o cambia el PIN ("1234"), o lo quita (""). */
export function setLanPin(pin: string): Promise<LanStatus> {
	return request('/api/v1/lan/pin', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ pin })
	});
}

/** GET /api/v1/genres — géneros con contenido, de más a menos títulos. */
export function listGenres(): Promise<GenreCount[]> {
	return orEmpty(request<GenreCount[] | null>('/api/v1/genres'));
}

/** GET /api/v1/settings/home — ¿la Home enseña el hero? */
export function getHomeSettings(): Promise<HomeSettings> {
	return request<HomeSettings>('/api/v1/settings/home');
}

/** PUT /api/v1/settings/home — Ajustes › Inicio (solo desde el PC). */
export function putHomeSettings(settings: HomeSettings): Promise<HomeSettings> {
	return request<HomeSettings>('/api/v1/settings/home', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(settings)
	});
}
