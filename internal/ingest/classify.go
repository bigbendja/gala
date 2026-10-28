package ingest

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// URLClass es la categoría de un locator http(s) tras clasificarlo (Brief
// M5 §2).
type URLClass string

const (
	// ClassDirect: extensión de vídeo conocida → provider http directo.
	ClassDirect URLClass = "directo"
	// ClassM3URemote: apunta a un .m3u/.m3u8; ambiguo hasta descargarlo (ver
	// ResolveM3UAmbiguity en pipeline.go) — puede resultar en una LISTA de
	// canales o en un único stream HLS servible como directo.
	ClassM3URemote URLClass = "m3u_remoto"
	// ClassDrive/ClassMega: reconocidos pero NO resueltos en M5; se crea la
	// fuente con provider="drive"/"mega" y queda pendiente de conector.
	ClassDrive URLClass = "drive"
	ClassMega  URLClass = "mega"
	// ClassUnsupported: host de streaming gris; NO se crea ninguna entrada
	// (Brief: "GALA no resuelve ni facilita" estos hosts).
	ClassUnsupported URLClass = "host_no_soportado"
	// ClassUnknown: http(s) válida pero sin ninguna señal reconocible; se
	// intenta como directo con salud sin_verificar (M6 la marcará caída si
	// no es vídeo de verdad).
	ClassUnknown URLClass = "desconocido"
	// ClassWebPage: es una página web (HTML), no un vídeo: o por su
	// extensión (.html, .php…) o porque el servidor respondió text/html a
	// la comprobación de pages.go. No se cataloga: "no son vídeo directo".
	ClassWebPage URLClass = "pagina_web"
)

// directVideoExts son extensiones de archivo de vídeo inequívocas (Brief
// M5 §2). .m3u/.m3u8 NO están aquí a propósito: siempre pasan primero por
// ClassM3URemote y la heurística de contenido en pipeline.go decide si son
// una lista o un único stream HLS (Brief: "distinguir... es heurístico").
var directVideoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".webm": true, ".avi": true,
	".ts": true, ".mov": true, ".m4v": true, ".flv": true,
}

// webPageExts son extensiones de documentos web: un enlace que termina así es
// una página, no un archivo de vídeo (no hace falta preguntar al servidor).
var webPageExts = map[string]bool{
	".html": true, ".htm": true, ".xhtml": true, ".php": true,
	".asp": true, ".aspx": true, ".jsp": true,
}

// driveHosts/megaHosts: dominios reconocidos de Drive/Mega (Brief §2). El
// conector real (descarga/streaming) llega después de M5; aquí solo se
// reconoce el dominio para dejar la fuente marcada "pendiente".
var driveHosts = map[string]bool{"drive.google.com": true, "docs.google.com": true}
var megaHosts = map[string]bool{"mega.nz": true, "mega.co.nz": true}

// unsupportedHosts es una lista MANTENIBLE de hosts de streaming gris, para
// DETECTAR y omitir — nunca para resolver ni sugerir cómo obtener el vídeo
// (regla de hierro del proyecto). Ampliar aquí si aparecen más; no es (ni
// pretende ser) exhaustiva.
var unsupportedHosts = map[string]bool{
	"streamtape.com": true, "streamtape.net": true, "streamtape.to": true,
	"filemoon.sx": true, "filemoon.to": true, "filemoon.in": true,
	"vidoza.net": true, "vidoza.co": true,
	"doodstream.com": true, "dood.to": true, "dood.wf": true, "dood.la": true,
	"dood.ws": true, "dood.pm": true, "dood.yt": true, "dood.re": true,
	"streamsb.net": true, "sbembed.com": true, "sbplay.org": true, "sbplay1.com": true,
	"upstream.to": true, "mixdrop.co": true, "mixdrop.to": true, "mixdrop.sx": true,
	"vidguard.to": true, "vidcloud.co": true, "fembed.com": true, "vidembed.cc": true,
}

// ClassifyURL clasifica un locator http(s) (Brief M5 §2). Devuelve error si
// rawURL no es una URL http(s) válida (el llamante decide qué hacer con
// eso: en texto suelto, ExtractURLs ya solo captura http(s), así que esto
// solo debería fallar para URLs de un CSV con columna "url" corrupta).
func ClassifyURL(rawURL string) (URLClass, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("ingest: URL no válida %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("ingest: esquema no soportado %q (solo http/https)", u.Scheme)
	}
	host := normalizeHost(u.Host)
	if host == "" {
		return "", fmt.Errorf("ingest: URL sin host: %q", rawURL)
	}

	if unsupportedHosts[host] {
		return ClassUnsupported, nil
	}
	if driveHosts[host] {
		return ClassDrive, nil
	}
	if megaHosts[host] {
		return ClassMega, nil
	}

	ext := strings.ToLower(path.Ext(u.Path))
	switch {
	case ext == ".m3u" || ext == ".m3u8":
		return ClassM3URemote, nil
	case directVideoExts[ext]:
		return ClassDirect, nil
	case webPageExts[ext]:
		return ClassWebPage, nil
	default:
		return ClassUnknown, nil
	}
}

// normalizeHost quita el puerto y el prefijo "www." para comparar contra
// las listas de hosts conocidos de forma robusta. Para IPv6 entre corchetes
// ("[::1]:8080"), el último ":" cae después del "]" de cierre, así que
// sigue cortando justo antes del puerto.
func normalizeHost(hostport string) string {
	host := strings.ToLower(hostport)
	if idx := strings.LastIndex(host, ":"); idx > strings.LastIndex(host, "]") {
		host = host[:idx]
	}
	return strings.TrimPrefix(host, "www.")
}
