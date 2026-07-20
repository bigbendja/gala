// Package providers define el contrato común de las fuentes de vídeo
// (localfs, m3u, http...) — Especificación MVP1 §3.2. Contrato estable: no
// se cambia sin actualizar antes la especificación en docs/.
package providers

import (
	"context"

	"github.com/bigbendja/gala/internal/catalog"
)

// HealthStatus es el estado de salud de una fuente; usa los valores
// catalog.Health* (HealthAlive, HealthDead, HealthDisconnected...).
type HealthStatus string

// RawItem es un hallazgo crudo de un provider, previo a identificación (M3):
// el normalizador y el matcher TMDB trabajan sobre esto, no el provider.
type RawItem struct {
	ProviderID string
	Locator    string            // ruta, URL, entrada M3U...
	Hints      map[string]string // título crudo, año, temporada/episodio si se conocen
}

// PlayableStream es el resultado de resolver una fuente para reproducirla.
type PlayableStream struct {
	URL        string // URL servible al reproductor
	Container  string // mp4, mkv, ts...
	NeedsRemux bool
}

// Provider es el contrato estable de las fuentes de vídeo.
type Provider interface {
	// ID identifica al provider: "localfs", "m3u", "http".
	ID() string
	// Discover escanea/sincroniza el origen y devuelve los vídeos hallados.
	Discover(ctx context.Context) ([]RawItem, error)
	// Resolve prepara src para reproducirse. En M2 (antes de M4) localfs.Resolve
	// devuelve URL vacía: el endpoint real de streaming (/api/v1/stream/{id})
	// llega en M4; aquí solo se documenta el contenedor.
	Resolve(ctx context.Context, src catalog.Source) (PlayableStream, error)
	// Check comprueba la salud actual de src.
	Check(ctx context.Context, src catalog.Source) (HealthStatus, error)
}
