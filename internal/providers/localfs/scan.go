// Package localfs implementa el Provider de carpetas y discos locales
// (Brief M2). Escanea carpetas registradas, identifica volúmenes y detecta
// su conexión/desconexión.
package localfs

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
)

// ProviderID es el identificador de este provider en la tabla sources.
const ProviderID = "localfs"

// Extensions son las extensiones de vídeo admitidas por el escaneo (Brief M2).
var Extensions = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".wmv": true,
	".flv": true, ".webm": true, ".m4v": true, ".mpg": true, ".mpeg": true,
	".ts": true, ".m2ts": true, ".3gp": true, ".ogv": true,
}

// MinFileSize descarta samples/basura. Constante de código, no de config de
// usuario (Brief M2).
const MinFileSize int64 = 10 * 1024 * 1024 // 10 MB

// ignoredDirs son carpetas de sistema que el escaneo nunca recorre.
var ignoredDirs = map[string]bool{
	"$RECYCLE.BIN":              true,
	"System Volume Information": true,
	"node_modules":              true,
}

// FoundFile es un vídeo válido hallado durante el escaneo de una carpeta.
type FoundFile struct {
	Path string // ruta absoluta
	Size int64
	Strm bool // es un .strm (puntero a una URL), no un vídeo
}

// WalkFolder recorre root y devuelve los vídeos que superan los filtros de
// M2 (extensión admitida, no oculto, no en carpeta de sistema, tamaño
// mínimo). Un error de permiso en una subcarpeta se registra en debug y el
// recorrido continúa; solo se devuelve error si root mismo es inaccesible.
func WalkFolder(ctx context.Context, root string, log *slog.Logger) ([]FoundFile, error) {
	var found []FoundFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if path == root {
				return err
			}
			if log != nil {
				log.Debug("acceso denegado durante escaneo", "path", path, "err", err)
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || ignoredDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasPrefix(name, ".") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == StrmExt {
			// Un .strm es un puntero de pocos bytes: no pasa por el filtro de
			// tamaño mínimo de los vídeos.
			found = append(found, FoundFile{Path: path, Strm: true})
			return nil
		}
		if !Extensions[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if log != nil {
				log.Debug("no se pudo leer info del fichero", "path", path, "err", err)
			}
			return nil
		}
		if info.Size() < MinFileSize {
			return nil
		}
		found = append(found, FoundFile{Path: path, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
