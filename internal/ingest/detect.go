package ingest

import (
	"os"
	"regexp"
	"strings"
)

// DetectInputType clasifica un blob de entrada completo (Brief M5 §1).
// filename es el nombre del archivo subido, si lo hay (vacío para el texto
// pegado en el textarea).
func DetectInputType(blob, filename string) InputType {
	trimmed := strings.TrimSpace(blob)
	lowerName := strings.ToLower(strings.TrimSpace(filename))

	if strings.HasPrefix(trimmed, "#EXTM3U") ||
		strings.HasSuffix(lowerName, ".m3u") || strings.HasSuffix(lowerName, ".m3u8") {
		return InputM3U
	}
	if looksLikeCSV(trimmed, lowerName) {
		return InputCSV
	}
	return InputText
}

// looksLikeCSV es la heurística del Brief M5 §1: "varias líneas con
// separadores consistentes y cabecera plausible (columnas tipo
// title,url,year)... si dudoso, tratar como texto suelto". Soporte mínimo:
// exige una columna de URL reconocible; sin ella, nunca es CSV (evita que un
// texto suelto con comas se confunda con CSV).
func looksLikeCSV(blob, lowerFilename string) bool {
	if strings.HasSuffix(lowerFilename, ".csv") {
		return true
	}
	lines := nonEmptyLines(blob)
	if len(lines) < 2 {
		return false
	}
	delim := detectDelimiter(lines[0])
	if delim == 0 {
		return false
	}
	header := strings.Split(lines[0], string(delim))
	if !hasURLColumn(header) {
		return false
	}
	consistent := 0
	for _, l := range lines[1:] {
		if len(strings.Split(l, string(delim))) == len(header) {
			consistent++
		}
	}
	// Al menos el 70% de las líneas con el mismo nº de campos que la
	// cabecera: dudoso por debajo de eso, se trata como texto suelto.
	return consistent*10 >= len(lines[1:])*7
}

func hasURLColumn(header []string) bool {
	for _, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "url", "link", "enlace":
			return true
		}
	}
	return false
}

func detectDelimiter(headerLine string) rune {
	best, bestCount := rune(0), 0
	for _, c := range []rune{',', ';', '\t'} {
		if n := strings.Count(headerLine, string(c)); n > bestCount {
			bestCount, best = n, c
		}
	}
	return best
}

func nonEmptyLines(blob string) []string {
	raw := strings.Split(strings.ReplaceAll(blob, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// reURL extrae URLs http(s) de texto suelto (Brief §1: "se le extraen URLs
// con regex y se ignora el resto: emojis, comentarios"). No es RFC 3986
// estricto a propósito: basta con capturar lo que un usuario real pega
// desde WhatsApp/Telegram, incluida puntuación de cierre que hay que recortar.
var reURL = regexp.MustCompile(`https?://[^\s<>"'\x60]+`)

// trailingPunctuation son caracteres que casi nunca son parte de una URL
// real pero sí de la frase que la rodea ("mira este enlace: https://x.com/y!").
const trailingPunctuation = ".,;:!?)]}\"'"

// ExtractURLs extrae todas las URLs http(s) de un texto suelto, recortando
// puntuación de cierre que claramente pertenece a la frase, no a la URL.
func ExtractURLs(text string) []string {
	matches := reURL.FindAllString(text, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, strings.TrimRight(m, trailingPunctuation))
	}
	return out
}

// ExtractLocalPaths recorre las líneas de texto suelto buscando rutas de
// archivo/carpeta que EXISTEN en este equipo (Brief §1). Cada línea se
// comprueba como mucho una vez con os.Stat: barato incluso con miles de
// líneas de un pegado de WhatsApp.
func ExtractLocalPaths(text string) []LocalPathCandidate {
	var out []LocalPathCandidate
	for _, line := range nonEmptyLines(text) {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "://") || !looksLikePath(line) {
			continue
		}
		info, err := os.Stat(line)
		if err != nil {
			continue
		}
		out = append(out, LocalPathCandidate{Path: line, IsDir: info.IsDir()})
	}
	return out
}

// looksLikePath filtra candidatas antes del os.Stat (barato): unidad de
// Windows ("C:\" o "C:/"), ruta UNC ("\\servidor\recurso"), o ruta absoluta
// de Unix ("/mnt/...").
func looksLikePath(s string) bool {
	if len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
		return true
	}
	if strings.HasPrefix(s, `\\`) {
		return true
	}
	if strings.HasPrefix(s, "/") {
		return true
	}
	return false
}
