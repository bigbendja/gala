package update

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// recentWindow: una reproducción cuenta como «en curso» si hubo actividad de
// streaming en los últimos segundos (un vídeo en pausa no pide nada, pero uno
// que se está viendo pide trozos continuamente).
const recentWindow = 30 * time.Second

// Activity cuenta las peticiones de streaming para no actualizar en mitad de
// una reproducción. Envuelve al mux; solo mira /api/v1/stream/.
type Activity struct {
	inflight atomic.Int64
	last     atomic.Int64 // unix nano de la última petición de streaming
}

// Wrap devuelve el handler con el conteo activado.
func (a *Activity) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/stream/") {
			a.inflight.Add(1)
			a.last.Store(time.Now().UnixNano())
			defer func() {
				a.last.Store(time.Now().UnixNano())
				a.inflight.Add(-1)
			}()
		}
		next.ServeHTTP(w, r)
	})
}

// Active devuelve cuántas reproducciones parece haber: las peticiones de
// streaming en vuelo o, si no hay ninguna, 1 si hubo actividad reciente.
func (a *Activity) Active() int {
	if n := a.inflight.Load(); n > 0 {
		return int(n)
	}
	if t := a.last.Load(); t > 0 && time.Since(time.Unix(0, t)) < recentWindow {
		return 1
	}
	return 0
}
