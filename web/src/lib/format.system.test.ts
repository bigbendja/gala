// Sistema de Diseño GALA §2.1: nada de texto crudo del sistema.
import { describe, expect, it } from 'vitest';
import { ApiError } from './api/client';
import {
	episodeCode,
	errorMessage,
	formatDate,
	friendlyHealthDetail,
	httpStatusText,
	isUnidentified,
	listDisplayName,
	locatorName,
	locatorWhere
} from './format';

describe('errorMessage', () => {
	it('un fallo de red ya viene redactado: se respeta', () => {
		const e = new ApiError('No se pudo conectar con GALA. Comprueba que el programa sigue en marcha.');
		expect(errorMessage(e)).toBe(e.message);
	});

	it('un 5xx no enseña el texto crudo del servidor, dice qué hacer', () => {
		const text = errorMessage(new ApiError('sql: database is locked', 500), 'No se pudo añadir');
		expect(text).toBe('No se pudo añadir. Reintenta; si sigue, reinicia GALA.');
		expect(text).not.toContain('sql');
	});

	it('un 4xx conserva el mensaje en español del servidor; un 404 se traduce', () => {
		expect(errorMessage(new ApiError('Esto solo se puede hacer desde el PC.', 403))).toContain('desde el PC');
		expect(errorMessage(new ApiError('not found', 404))).toBe('No encontrado.');
	});

	it('un error cualquiera nunca enseña "ApiError:" ni el texto de la excepción', () => {
		const text = errorMessage(new TypeError('x is undefined'), 'No se pudo cargar');
		expect(text).toBe('No se pudo cargar. Reintenta o revisa que GALA siga abierto.');
		expect(errorMessage('boom')).not.toContain('boom');
	});
});

describe('httpStatusText', () => {
	it('traduce los códigos, sin "Not Found" en inglés', () => {
		expect(httpStatusText(404).title).toBe('No encontrado');
		expect(httpStatusText(403).title).toBe('Sin permiso');
		expect(httpStatusText(500).title).toBe('Algo ha ido mal');
	});
});

describe('formatDate', () => {
	const now = Date.parse('2026-10-27T12:00:00Z');
	it('humano y en español, nunca ISO', () => {
		expect(formatDate('2026-10-27T11:59:50Z', now)).toBe('ahora mismo');
		expect(formatDate('2026-10-27T11:55:00Z', now)).toBe('hace 5 min');
		expect(formatDate('2026-10-27T10:00:00Z', now)).toBe('hace 2 horas');
		expect(formatDate('2026-10-26T10:00:00Z', now)).toBe('ayer');
		expect(formatDate('2026-10-24T12:00:00Z', now)).toBe('hace 3 días');
		const old = formatDate('2026-07-23T14:02:23Z', now);
		expect(old).toBe('23 jul 2026');
		expect(old).not.toMatch(/T\d\d:/);
	});
	it('vacío o inválido da cadena vacía', () => {
		expect(formatDate(undefined)).toBe('');
		expect(formatDate('no es una fecha')).toBe('');
	});
});

describe('friendlyHealthDetail', () => {
	it('traduce los motivos técnicos de M6', () => {
		expect(friendlyHealthDetail('HEAD → http 404')).toBe('Enlace caído: ya no existe');
		expect(friendlyHealthDetail('GET → http 503')).toBe('El servidor del enlace falla');
		expect(friendlyHealthDetail('sin respuesta (HEAD): dial tcp: timeout')).toBe('Sin respuesta del servidor');
		expect(friendlyHealthDetail('el servidor devolvió una página web (text/html), no un vídeo')).toContain('web');
		expect(friendlyHealthDetail('respuesta lenta')).toBe('Responde despacio');
		expect(friendlyHealthDetail('pendiente de conector')).toContain('conector');
		expect(friendlyHealthDetail('archivo no encontrado o no accesible: open x')).toBe('No se encuentra el archivo');
		expect(friendlyHealthDetail('httpprovider: la URL apunta a una red privada o local, bloqueada por seguridad')).toContain(
			'red privada'
		);
	});
	it('nunca devuelve jerga ni flechas', () => {
		for (const d of ['HEAD → http 404', 'sin respuesta (GET): EOF', 'algo raro de Go: &{}']) {
			const out = friendlyHealthDetail(d) ?? '';
			expect(out).not.toMatch(/→|HEAD|GET|&\{/);
		}
		expect(friendlyHealthDetail(null)).toBeNull();
	});
});

describe('locatorName / locatorWhere', () => {
	it('separa el nombre del archivo de su carpeta (Windows y POSIX)', () => {
		expect(locatorName('C:\\Pelis\\Serie\\S01E01.mkv')).toBe('S01E01.mkv');
		expect(locatorWhere('C:\\Pelis\\Serie\\S01E01.mkv')).toBe('C:\\Pelis\\Serie');
		expect(locatorName('/media/pelis/Dune.mp4')).toBe('Dune.mp4');
		expect(locatorWhere('/media/pelis/Dune.mp4')).toBe('/media/pelis');
	});
	it('en una URL, el nombre es el último tramo y "dónde" el servidor', () => {
		expect(locatorName('https://cdn.example.com/v/peli.mp4?token=abc')).toBe('peli.mp4');
		expect(locatorWhere('https://cdn.example.com/v/peli.mp4')).toBe('cdn.example.com');
	});
});

describe('listDisplayName', () => {
	it('no enseña "pegado <ISO>" ni la URL cruda', () => {
		expect(listDisplayName('pegado 2026-10-27T16:15:04Z', 'text')).toBe('Texto pegado');
		expect(listDisplayName('pegado 2026-10-27T16:15:04Z', 'm3u')).toBe('Lista M3U pegada');
		expect(listDisplayName('https://lista.example.com/canales/tv.m3u', 'm3u_remoto')).toBe('lista.example.com · tv.m3u');
		expect(listDisplayName('mis-pelis.csv', 'csv')).toBe('mis-pelis.csv');
	});
});

describe('episodios', () => {
	it('un episodio o temporada nunca es "sin identificar"', () => {
		expect(isUnidentified({ kind: 'episode', confirmed: true, tmdb_id: undefined })).toBe(false);
		expect(isUnidentified({ kind: 'season', confirmed: true, tmdb_id: undefined })).toBe(false);
		expect(isUnidentified({ kind: 'other', confirmed: true, tmdb_id: undefined })).toBe(true);
	});
	it('código "T1 · E3"', () => {
		expect(episodeCode({ kind: 'episode', season_number: 1, episode_number: 3 })).toBe('T1 · E3');
		expect(episodeCode({ kind: 'episode', episode_number: 3 }, 2)).toBe('T2 · E3');
		expect(episodeCode({ kind: 'season', season_number: 2 })).toBe('Temporada 2');
	});
});
