package localfs

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
)

// Volume es un volumen de almacenamiento detectado por un Enumerator.
type Volume struct {
	ID    string
	Label string
	// Roots son las rutas raíz asociadas al volumen: letras de unidad en
	// Windows ("E:\"), o la propia carpeta registrada en el fallback
	// no-Windows. Se usan para decidir qué carpetas registradas viven en él.
	Roots []string
}

// Enumerator lista los volúmenes actualmente presentes en el sistema.
// folders son las carpetas registradas: el fallback no-Windows las necesita
// porque no enumera discos reales, solo comprueba si esas rutas existen.
// Interfaz inyectable para poder testear el poller sin depender del SO.
type Enumerator interface {
	Volumes(ctx context.Context, folders []string) ([]Volume, error)
}

// MatchVolume devuelve el volumen (si lo hay) cuya raíz contiene path, con
// coincidencia de prefijo más larga cuando hay varias.
func MatchVolume(path string, vols []Volume) (Volume, bool) {
	clean := NormalizePath(path)
	var best Volume
	bestLen := -1
	found := false
	for _, v := range vols {
		for _, root := range v.Roots {
			r := NormalizePath(root)
			if clean != r && !strings.HasPrefix(clean, r+string(filepath.Separator)) {
				continue
			}
			if len(r) > bestLen {
				best, bestLen, found = v, len(r), true
			}
		}
	}
	return best, found
}

// NormalizePath limpia path para comparaciones de prefijo consistentes:
// resuelve "." y "..", quita la barra final (salvo la raíz) y, en Windows,
// ignora mayúsculas/minúsculas.
func NormalizePath(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	if len(p) > 1 {
		p = strings.TrimSuffix(p, string(filepath.Separator))
	}
	return p
}
