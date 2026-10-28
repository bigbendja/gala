// Utilidades mínimas de formato para vistas (Brief M10a §3: "utilidades
// mínimas si hacen falta, justificadas"). Nada de librerías de fechas/i18n:
// el rango de casos es pequeño y controlado.
import { ApiError } from './api/client';
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
export function isUnidentified(item: Pick<Item, 'confirmed' | 'tmdb_id'> & { kind?: Item['kind'] }): boolean {
	// Temporadas y episodios cuelgan de una serie ya identificada y por diseño
	// no llevan tmdb_id propio: nunca son "sin identificar".
	if (item.kind === 'season' || item.kind === 'episode') return false;
	return !item.confirmed || !item.tmdb_id;
}

// Brief M10c §2: "avisar claramente 'API key de TMDB inválida' cuando el
// error sea 401 — esto le habría ahorrado horas al usuario". LastError es
// el error de Go crudo (p. ej. `tmdb /movie/1: 401 {"status_message":
// "Invalid API key..."}`); esto lo traduce a un aviso legible sin tocar lo
// que guarda el backend.
export function friendlyIdentifyError(lastError: string | undefined | null): string | null {
	if (!lastError) return null;
	if (/\b401\b/.test(lastError) || /invalid api key/i.test(lastError)) {
		return 'La clave de TMDB no es válida. Cámbiala más abajo, en "Clave TMDB".';
	}
	if (/sin conexión con tmdb/i.test(lastError)) {
		return 'Sin conexión con TMDB en este momento. Se reintenta solo cada pocos minutos.';
	}
	return lastError;
}

/** Título de presentación: limpia separadores de nombre de archivo SOLO
 * cuando el item no está identificado; un item ya identificado por TMDB
 * conserva su título real tal cual. */
export function displayTitle(item: Pick<Item, 'title' | 'confirmed' | 'tmdb_id'> & { kind?: Item['kind'] }): string {
	if (!isUnidentified(item)) return item.title;
	return item.title
		.replace(/[._]+/g, ' ')
		.replace(/\s+/g, ' ')
		.trim();
}

// --- Sistema de Diseño GALA §2.1: nada de texto crudo del sistema ----------

/** Mensaje de error para el usuario: en español, qué pasó y qué hacer. Nunca
 *  `ApiError: …`, ni el texto de un error 5xx del servidor (jerga de Go). */
export function errorMessage(err: unknown, action = 'No se pudo completar la operación'): string {
	if (err instanceof ApiError) {
		if (err.status === undefined) return err.message; // sin conexión: ya viene redactado
		if (err.status === 404) return 'No encontrado.';
		if (err.status >= 500) return `${action}. Reintenta; si sigue, reinicia GALA.`;
		return err.message;
	}
	return `${action}. Reintenta o revisa que GALA siga abierto.`;
}

/** Título y texto de las páginas de error (SvelteKit +error), en español. */
export function httpStatusText(status: number): { title: string; hint: string } {
	switch (status) {
		case 404:
			return { title: 'No encontrado', hint: 'Esta página o título no existe, o ya no está en tu galería.' };
		case 403:
			return { title: 'Sin permiso', hint: 'Esto solo se puede hacer desde el PC donde corre GALA.' };
		case 401:
			return { title: 'Hace falta el PIN', hint: 'Vuelve a la galería para escribirlo.' };
		default:
			return { title: 'Algo ha ido mal', hint: 'Reintenta o revisa que GALA siga abierto.' };
	}
}

const MONTHS = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'];

/** Fecha humana en español: "hace 5 min", "hoy", "ayer", "hace 3 días",
 *  y a partir de una semana "23 jul 2026". Nunca ISO. */
export function formatDate(iso: string | undefined | null, now: number = Date.now()): string {
	if (!iso) return '';
	const d = new Date(iso);
	const t = d.getTime();
	if (Number.isNaN(t)) return '';
	const diffMin = Math.floor((now - t) / 60_000);
	if (diffMin < 1) return 'ahora mismo';
	if (diffMin < 60) return `hace ${diffMin} min`;
	const diffH = Math.floor(diffMin / 60);
	if (diffH < 24) return diffH === 1 ? 'hace 1 hora' : `hace ${diffH} horas`;
	const days = Math.floor(diffH / 24);
	if (days === 1) return 'ayer';
	if (days < 7) return `hace ${days} días`;
	return `${d.getDate()} ${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
}

/** Detalle de salud sin jerga ("HEAD → http 404", "sin respuesta (GET): …",
 *  rutas Go): una frase corta de qué le pasa al enlace o archivo. */
export function friendlyHealthDetail(detail: string | undefined | null): string | null {
	const d = cleanHealthDetail(detail);
	if (!d) return null;
	const low = d.toLowerCase();
	const http = /http (\d{3})/.exec(d);
	if (http) {
		const code = Number(http[1]);
		if (code === 404 || code === 410) return 'Enlace caído: ya no existe';
		if (code === 401 || code === 403) return 'Enlace caído: no deja entrar';
		if (code >= 500) return 'El servidor del enlace falla';
		return `Enlace caído (error ${code})`;
	}
	if (low.startsWith('sin respuesta')) return 'Sin respuesta del servidor';
	if (low.includes('página web')) return 'El enlace lleva a una web, no a un vídeo';
	if (low.includes('red privada') || low.includes('bloqueada por seguridad')) {
		return 'El enlace apunta a una red privada: bloqueado';
	}
	if (low.includes('respuesta lenta')) return 'Responde despacio';
	if (low.includes('pendiente de conector')) return 'Falta el conector (próximamente)';
	if (low.includes('volumen no está conectado')) return 'Disco desconectado';
	if (low.includes('archivo no encontrado')) return 'No se encuentra el archivo';
	if (low.includes('es una carpeta')) return 'La ruta es una carpeta, no un archivo';
	if (low.includes('disco lento')) return 'Disco lento';
	return 'No se pudo verificar';
}

/** Nombre de archivo (o último tramo de una URL) de un locator. */
export function locatorName(locator: string): string {
	const clean = locator.split(/[?#]/)[0].replace(/[\\/]+$/, '');
	const i = Math.max(clean.lastIndexOf('/'), clean.lastIndexOf('\\'));
	return i >= 0 ? clean.slice(i + 1) || clean : clean;
}

/** Carpeta contenedora (o host) de un locator, para mostrar en pequeño. */
export function locatorWhere(locator: string): string {
	if (/^https?:\/\//i.test(locator)) {
		try {
			return new URL(locator).host;
		} catch {
			return '';
		}
	}
	const clean = locator.replace(/[\\/]+$/, '');
	const i = Math.max(clean.lastIndexOf('/'), clean.lastIndexOf('\\'));
	return i > 0 ? clean.slice(0, i) : '';
}

/** "T1 · E3" para episodios, "Temporada 2" para temporadas. */
export function episodeCode(item: Pick<Item, 'kind' | 'season_number' | 'episode_number'>, seasonNumber?: number): string {
	if (item.kind === 'season') return item.season_number ? `Temporada ${item.season_number}` : 'Temporada';
	const s = seasonNumber ?? item.season_number;
	const e = item.episode_number;
	if (s && e) return `T${s} · E${e}`;
	if (e) return `E${e}`;
	return '';
}

/** Nombre legible de una lista ingerida: el servidor nombra el texto pegado
 *  "pegado <fecha ISO>" y las M3U remotas con su URL; ninguna de las dos cosas
 *  se enseña tal cual. */
export function listDisplayName(name: string, type: string): string {
	if (/^pegado\s+\d{4}-\d{2}-\d{2}T/.test(name)) {
		if (type === 'm3u') return 'Lista M3U pegada';
		if (type === 'csv') return 'CSV pegado';
		return 'Texto pegado';
	}
	if (name.startsWith('http://') || name.startsWith('https://')) {
		try {
			const u = new URL(name);
			const last = locatorName(u.pathname);
			return last ? `${u.host} · ${last}` : u.host;
		} catch {
			return name;
		}
	}
	return name;
}


// --- Ficha enriquecida (Fase 1, Parte B) -------------------------------------

/** 132 → "2 h 12 min"; 45 → "45 min". */
export function formatRuntime(min: number | undefined | null): string {
	if (!min || min <= 0) return '';
	const h = Math.floor(min / 60);
	const m = Math.round(min % 60);
	if (h === 0) return `${m} min`;
	return m === 0 ? `${h} h` : `${h} h ${String(m).padStart(2, '0')} min`;
}

/** Nota de TMDB con coma decimal: 8.4 → "8,4". */
export function formatRating(v: number | undefined | null): string {
	if (!v || v <= 0) return '';
	return v.toFixed(1).replace('.', ',');
}

/** 2254857830 → "2,1 GB". */
export function formatBytes(n: number | undefined | null): string {
	if (!n || n <= 0) return '';
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	let v = n;
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	const text = i === 0 ? String(Math.round(v)) : v.toFixed(v >= 100 ? 0 : 1).replace('.', ',');
	return `${text} ${units[i]}`;
}

const VIDEO_CODECS: Record<string, string> = {
	h264: 'H.264',
	hevc: 'HEVC (H.265)',
	h265: 'HEVC (H.265)',
	vp9: 'VP9',
	vp8: 'VP8',
	av1: 'AV1',
	mpeg4: 'MPEG-4',
	mpeg2video: 'MPEG-2'
};
const AUDIO_CODECS: Record<string, string> = {
	aac: 'AAC',
	ac3: 'Dolby Digital (AC3)',
	eac3: 'Dolby Digital Plus',
	dts: 'DTS',
	mp3: 'MP3',
	opus: 'Opus',
	vorbis: 'Vorbis',
	flac: 'FLAC',
	truehd: 'Dolby TrueHD'
};
const CONTAINERS: Record<string, string> = {
	matroska: 'MKV',
	'matroska,webm': 'MKV',
	mp4: 'MP4',
	'mov,mp4,m4a,3gp,3g2,mj2': 'MP4',
	avi: 'AVI',
	webm: 'WebM',
	mpegts: 'MPEG-TS'
};

export function videoCodecLabel(c: string | undefined): string {
	return c ? (VIDEO_CODECS[c.toLowerCase()] ?? c.toUpperCase()) : '';
}
export function audioCodecLabel(c: string | undefined): string {
	return c ? (AUDIO_CODECS[c.toLowerCase()] ?? c.toUpperCase()) : '';
}
export function containerLabel(c: string | undefined): string {
	return c ? (CONTAINERS[c.toLowerCase()] ?? c.split(',')[0].toUpperCase()) : '';
}

/** Iniciales de una persona para el respaldo de su foto ("Song Kang-ho" → "SK"). */
export function initials(name: string): string {
	const parts = name
		.trim()
		.split(/\s+/)
		.filter((p) => /[\p{L}\p{N}]/u.test(p));
	if (parts.length === 0) return '?';
	const first = Array.from(parts[0])[0];
	const last = parts.length > 1 ? Array.from(parts[parts.length - 1])[0] : '';
	return (first + last).toUpperCase();
}

/** Tono estable (0–359) por nombre: el mismo actor siempre tiene el mismo color. */
export function nameHue(name: string): number {
	let h = 0;
	for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) >>> 0;
	return h % 360;
}
