// Package stream sirve el contenido catalogado: direct play con range
// requests, y remux al vuelo a HLS cuando el contenedor no es compatible con
// el navegador pero los códecs sí (Brief M4 §4).
package stream

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ErrOutsideRegisteredFolders señala un intento de servir una ruta fuera de
// las carpetas registradas: anti path-traversal obligatorio (CLAUDE.md,
// Brief M4 §4).
var ErrOutsideRegisteredFolders = errors.New("stream: ruta fuera de las carpetas registradas")

// ValidatePath comprueba que absPath cae dentro de alguna de folders. Ambas
// partes se normalizan (limpieza + resolución de símlinks/„..“) antes de
// comparar, para que un ".." en el locator o una carpeta registrada sin
// limpiar no burlen la comprobación.
func ValidatePath(absPath string, folders []string) error {
	cleanPath, err := resolvePath(absPath)
	if err != nil {
		return err
	}
	for _, folder := range folders {
		cleanFolder, err := resolvePath(folder)
		if err != nil {
			continue // carpeta registrada ilegible (p. ej. volumen desconectado): no bloquea a las demás
		}
		if cleanPath == cleanFolder || strings.HasPrefix(cleanPath, cleanFolder+string(filepath.Separator)) {
			return nil
		}
	}
	return ErrOutsideRegisteredFolders
}

// resolvePath limpia y, si es posible, resuelve símlinks de path (para que
// una ruta trucada con símlinks no escape de la carpeta registrada). Si el
// fichero aún no existe (EvalSymlinks falla), se usa la ruta limpia sin más:
// ValidatePath se llama también antes de que el archivo exista en algunos
// contextos de test, y el 404 posterior ya cubre ese caso.
func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(abs), nil
}

// videoContentTypes son los Content-Type por extensión que de verdad
// aparecen en un catálogo de vídeo; el resto cae a application/octet-stream
// en vez de adivinar (mejor un descarga genérica que un tipo incorrecto).
var videoContentTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/x-m4v",
	".webm": "video/webm",
	".mkv":  "video/x-matroska",
	".avi":  "video/x-msvideo",
	".mov":  "video/quicktime",
	".ts":   "video/mp2t",
	".m3u8": "application/vnd.apple.mpegurl",
}

// ContentType devuelve el Content-Type por extensión de path.
func ContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := videoContentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

// ServeDirect valida absPath contra folders y lo sirve con
// http.ServeContent: range requests, ETag y 206 gratis (Brief M4 §4). El
// llamante ya comprobó que la fuente existe en catálogo y es provider
// localfs; aquí solo se valida el sistema de archivos.
func ServeDirect(w http.ResponseWriter, r *http.Request, absPath string, folders []string) error {
	if err := ValidatePath(absPath, folders); err != nil {
		return err
	}
	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", ContentType(absPath))
	// http.ServeContent no genera ETag por sí solo (solo lo USA si ya está en
	// la respuesta); tamaño+mtime es suficiente como validador débil sin
	// tener que hashear el archivo entero en cada petición (Brief M4 §4).
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size()))
	http.ServeContent(w, r, filepath.Base(absPath), info.ModTime(), f)
	return nil
}
