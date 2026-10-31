// Cliente de API tipado (Brief M10a §3): wrapper mínimo sobre fetch, con
// mensajes de error en español para "sin conexión con el backend" y errores
// HTTP. Nada de librerías de fetch/HTTP: es un helper mínimo justificado
// (CLAUDE.md: "sí se permiten utilidades mínimas si hacen falta").
import type {
	UpdateCheck,
	UpdateStatus,
	AuthSession,
	AuthStatus,
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
	ItemDetails,
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

/** Evento de window: una petición de administración recibió 401 «login_required»
 *  (la sesión de administrador caducó o se cerró con la app abierta). */
export const ADMIN_LOGIN_EVENT = 'gala:admin-login-required';

export class ApiError extends Error {
	status?: number;
	/** Código estable del backend (`login_required`, `bad_password`...). */
	code?: string;
	constructor(message: string, status?: number, code?: string) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.code = code;
	}
}

let lastAdminLoginEvent = 0;

// Token CSRF de la sesión de administrador (cliente de red). El backend lo
// exige en las escrituras hechas por sesión; desde el PC no hace falta y no
// se manda. Vive solo en memoria: se recupera con GET /auth/status.
let csrfToken = '';
export function setCsrfToken(token: string | undefined | null) {
	csrfToken = token ?? '';
}

function withCsrf(init?: RequestInit): RequestInit | undefined {
	const method = (init?.method ?? 'GET').toUpperCase();
	if (!csrfToken || method === 'GET' || method === 'HEAD') return init;
	const headers = new Headers(init?.headers);
	headers.set('X-GALA-CSRF', csrfToken);
	return { ...init, headers };
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	let res: Response;
	try {
		res = await fetch(path, withCsrf(init));
	} catch {
		throw new ApiError('No se pudo conectar con GALA. Comprueba que el programa sigue en marcha.');
	}
	if (!res.ok) {
		let detail = '';
		let code: string | undefined;
		try {
			const body = (await res.json()) as { error?: string; code?: string };
			detail = body?.error ?? '';
			code = body?.code;
			if (res.status === 401 && body?.code === 'login_required' && typeof window !== 'undefined') {
				// Como mucho una vez cada 2 s: las cargas de la página que se
				// relanzan al invalidar también darían 401 y no deben encadenar
				// invalidaciones sin fin.
				const now = Date.now();
				if (now - lastAdminLoginEvent > 2000) {
					lastAdminLoginEvent = now;
					window.dispatchEvent(new CustomEvent(ADMIN_LOGIN_EVENT));
				}
			}
			// Modo LAN: la sesión del PIN caducó (o se cambió el PIN) con la
			// app abierta; el layout escucha esto y vuelve a pedirlo.
			if (res.status === 401 && body?.code === 'pin_required' && typeof window !== 'undefined') {
				window.dispatchEvent(new CustomEvent(PIN_REQUIRED_EVENT));
			}
		} catch {
			// el cuerpo de error no era JSON; se usa el mensaje genérico de abajo
		}
		throw new ApiError(detail || `El servidor respondió con un error (${res.status}).`, res.status, code);
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

/** GET /api/v1/version — versión, commit y fecha de compilación del binario. */
export function getVersion(): Promise<{ version: string; commit: string; date: string }> {
	return request('/api/v1/version');
}

// --- Administración con contraseña (M-Servidor) ------------------------------

/** GET /api/v1/auth/status — qué pintar: login, contraseña sin configurar... */
export function getAuthStatus(): Promise<AuthStatus> {
	return request<AuthStatus>('/api/v1/auth/status');
}

/** POST /api/v1/auth/login — inicia sesión de administrador (cookie httpOnly). */
export async function adminLogin(password: string): Promise<void> {
	const r = await request<{ ok: boolean; csrf?: string }>('/api/v1/auth/login', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ password })
	});
	setCsrfToken(r.csrf);
}

/** POST /api/v1/auth/logout — cierra la sesión de este dispositivo. */
export async function adminLogout(): Promise<void> {
	await request('/api/v1/auth/logout', { method: 'POST' });
	setCsrfToken('');
}

/** POST /api/v1/auth/logout-all — cierra todas las sesiones. */
export function adminLogoutAll(): Promise<{ ok: boolean }> {
	return request('/api/v1/auth/logout-all', { method: 'POST' });
}

/** POST /api/v1/auth/password — pone o cambia la contraseña de administrador. */
export function changeAdminPassword(current: string, next: string): Promise<{ ok: boolean }> {
	return request('/api/v1/auth/password', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ current, new: next })
	});
}

/** GET /api/v1/auth/sessions — sesiones de administración abiertas. */
export function listAuthSessions(): Promise<AuthSession[]> {
	return orEmpty(request<AuthSession[] | null>('/api/v1/auth/sessions'));
}

/** PUT /api/v1/auth/network-admin — permitir/impedir administrar desde la red (solo PC). */
export function setNetworkAdmin(enabled: boolean): Promise<{ network_admin: boolean }> {
	return request('/api/v1/auth/network-admin', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ enabled })
	});
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

/** GET /api/v1/items/{id}/details — reparto, equipo, similares, duración,
 *  nota y ficha técnica (listas vacías si TMDB aún no se ha consultado). */
export async function getItemDetails(id: number): Promise<ItemDetails> {
	const d = await request<ItemDetails>(`/api/v1/items/${id}/details`);
	return {
		...d,
		countries: d.countries ?? [],
		cast: d.cast ?? [],
		crew: d.crew ?? [],
		similar: d.similar ?? [],
		files: d.files ?? []
	};
}

// --- Actualización (M-Servidor) ----------------------------------------------

/** GET /api/v1/update/check — consulta GitHub (con force, ignora la caché de 24 h). */
export function checkUpdate(force = false): Promise<UpdateCheck> {
	return request<UpdateCheck>(`/api/v1/update/check${force ? '?force=1' : ''}`);
}

/** POST /api/v1/update/apply — lanza la actualización. Con 409 `playback_active`
 *  hay reproducciones en curso: se puede repetir con force. */
export function applyUpdate(force = false): Promise<{ ok: boolean }> {
	return request('/api/v1/update/apply', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ force })
	});
}

/** GET /api/v1/update/status — progreso, resultado de la última actualización y
 *  versión disponible conocida (sin tocar la red). */
export function getUpdateStatus(): Promise<UpdateStatus> {
	return request<UpdateStatus>('/api/v1/update/status');
}

/** DELETE /api/v1/update/result — descarta el aviso del resultado. */
export function dismissUpdateResult(): Promise<{ ok: boolean }> {
	return request('/api/v1/update/result', { method: 'DELETE' });
}

/** ¿Responde GALA? (GET /healthz, sin pasar por el manejo de errores de la app:
 *  durante el reinicio es normal que falle.) */
export async function isUp(): Promise<boolean> {
	try {
		const res = await fetch('/healthz', { cache: 'no-store' });
		return res.ok;
	} catch {
		return false;
	}
}

/** GET /api/v1/version — versión que está corriendo ahora mismo. */
export async function runningVersion(): Promise<string | null> {
	try {
		const res = await fetch('/api/v1/version', { cache: 'no-store' });
		if (!res.ok) return null;
		return ((await res.json()) as { version?: string }).version ?? null;
	} catch {
		return null;
	}
}
