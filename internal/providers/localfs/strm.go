package localfs

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

// StrmExt es la extensión de los archivos .strm: un formato abierto (Kodi,
// Jellyfin) que es un puntero de texto a un enlace, no un vídeo.
const StrmExt = ".strm"

// maxStrmBytes acota la lectura: un .strm legítimo es una línea; algo mayor
// no es un puntero y no se lee entero.
const maxStrmBytes = 64 * 1024

// ReadStrmURL lee un .strm y devuelve su URL (ver ParseStrm). El contenido es
// un dato: solo se interpreta como texto, jamás se ejecuta.
func ReadStrmURL(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxStrmBytes))
	if err != nil {
		return "", err
	}
	return ParseStrm(string(data))
}

// ParseStrm extrae la primera URL http/https válida del contenido de un
// .strm. Ignora líneas vacías y comentarios (#), como en M3U, y también
// líneas que no son una URL http(s) con host. Error si no hay ninguna.
func ParseStrm(content string) (string, error) {
	content = strings.TrimPrefix(content, string(rune(0xFEFF))) // BOM UTF-8
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := url.Parse(line)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("el .strm no contiene ninguna URL http/https")
}
