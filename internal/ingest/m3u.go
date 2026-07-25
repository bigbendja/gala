package ingest

import (
	"regexp"
	"strconv"
	"strings"
)

// M3UEntry es una entrada de una lista M3U (Brief M5 §3): el título del
// EXTINF alimenta el normalizador de M3 igual que un nombre de archivo.
type M3UEntry struct {
	Title      string // texto tras la coma de EXTINF
	DurationS  int    // duración declarada; -1 si desconocida (habitual en IPTV)
	GroupTitle string // atributo group-title, si está
	TvgName    string // atributo tvg-name, si está
	URL        string
}

var reExtInfAttr = regexp.MustCompile(`(\w[\w-]*)="([^"]*)"`)

// ParseM3U interpreta un documento M3U/M3U8 (Brief M5 §3): #EXTM3U,
// entradas "#EXTINF:<dur>[ attrs],<título>" seguidas de su URL, atributos
// comunes (tvg-name, group-title), robusto ante líneas en blanco,
// comentarios/directivas no soportadas y CRLF. Una línea de URL sin EXTINF
// previo se admite igual, como M3U "a pelo".
func ParseM3U(content string) []M3UEntry {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	var out []M3UEntry
	var pending *M3UEntry

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXTM3U"):
			continue
		case strings.HasPrefix(line, "#EXTINF:"):
			pending = parseExtInf(line)
		case strings.HasPrefix(line, "#"):
			continue // otra directiva (#EXTGRP, #EXTVLCOPT...): se ignora, no corta el resto del parseo
		default:
			if pending == nil {
				pending = &M3UEntry{DurationS: -1}
			}
			pending.URL = line
			out = append(out, *pending)
			pending = nil
		}
	}
	return out
}

func parseExtInf(line string) *M3UEntry {
	e := &M3UEntry{DurationS: -1}
	rest := strings.TrimPrefix(line, "#EXTINF:")

	commaIdx := findTitleComma(rest)
	meta, title := rest, ""
	if commaIdx >= 0 {
		meta, title = rest[:commaIdx], rest[commaIdx+1:]
	}
	e.Title = strings.TrimSpace(title)

	fields := strings.SplitN(strings.TrimSpace(meta), " ", 2)
	if len(fields) > 0 {
		if d, err := strconv.Atoi(strings.TrimSpace(fields[0])); err == nil {
			e.DurationS = d
		}
	}
	if len(fields) > 1 {
		for _, m := range reExtInfAttr.FindAllStringSubmatch(fields[1], -1) {
			switch strings.ToLower(m[1]) {
			case "group-title":
				e.GroupTitle = m[2]
			case "tvg-name":
				e.TvgName = m[2]
			}
		}
	}
	return e
}

// findTitleComma busca la coma que separa metadatos de título en una línea
// EXTINF, ignorando comas dentro de valores entrecomillados (p. ej.
// group-title="Acción, Drama").
func findTitleComma(s string) int {
	inQuotes := false
	for i, r := range s {
		switch r {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				return i
			}
		}
	}
	return -1
}

// classifyM3UContent decide, tras descargar un .m3u/.m3u8, si es una LISTA
// de canales/entradas distintas o el playlist de medios HLS de un único
// vídeo (Brief M5 §2/§3: heurística). Un playlist de medios trae sus
// propias etiquetas de segmentación (#EXT-X-TARGETDURATION/-MEDIA-SEQUENCE),
// que una lista de canales IPTV nunca usa; si no aparecen, se cae al
// criterio de "varios títulos EXTINF distintos → lista".
func classifyM3UContent(content string) (isChannelList bool) {
	if strings.Contains(content, "#EXT-X-TARGETDURATION") || strings.Contains(content, "#EXT-X-MEDIA-SEQUENCE") {
		return false
	}
	entries := ParseM3U(content)
	if len(entries) <= 1 {
		return false
	}
	titles := make(map[string]bool, len(entries))
	for _, e := range entries {
		titles[strings.TrimSpace(e.Title)] = true
	}
	return len(titles) > 1
}
