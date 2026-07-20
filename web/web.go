// Package web embebe el frontend compilado (web/dist) en el binario.
// En F0, dist/ contiene un placeholder estático; cuando exista el proyecto
// SvelteKit, su build (adapter-static) se volcará a este mismo dist/.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

// Handler sirve el frontend embebido con fallback a index.html (rutas de SPA).
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed roto = binario mal construido; fallar fuerte y pronto
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Si el fichero existe, se sirve; si no, index.html (routing del lado cliente).
		if _, err := fs.Stat(sub, trimPath(r.URL.Path)); err != nil {
			index, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "frontend no embebido", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func trimPath(p string) string {
	if p == "/" || p == "" {
		return "index.html"
	}
	return p[1:]
}
