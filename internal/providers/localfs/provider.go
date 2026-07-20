package localfs

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/providers"
)

// Provider implementa providers.Provider para carpetas y discos locales.
// En M2 es el único provider con implementación completa (Brief M2 §1).
type Provider struct {
	// Folders devuelve las carpetas registradas actualmente (normalmente
	// config.Config.MediaFolders); se invoca en cada Discover para reflejar
	// altas/bajas sin reconstruir el Provider.
	Folders func() []string
	Log     *slog.Logger
}

// New crea un Provider localfs.
func New(folders func() []string, log *slog.Logger) *Provider {
	return &Provider{Folders: folders, Log: log}
}

func (p *Provider) ID() string { return ProviderID }

// Discover recorre todas las carpetas registradas. Una carpeta inaccesible
// (p. ej. volumen desconectado) se registra y se omite; no aborta el resto.
// Nota: la sincronización fina con el catálogo (altas/bajas, transacciones
// por lotes, volumen de cada fichero) la hace internal/scan, que llama a
// WalkFolder directamente porque necesita más granularidad por carpeta de la
// que expone este contrato genérico.
func (p *Provider) Discover(ctx context.Context) ([]providers.RawItem, error) {
	var out []providers.RawItem
	for _, folder := range p.Folders() {
		found, err := WalkFolder(ctx, folder, p.Log)
		if err != nil {
			if p.Log != nil {
				p.Log.Warn("no se pudo escanear carpeta", "folder", folder, "err", err)
			}
			continue
		}
		for _, f := range found {
			out = append(out, providers.RawItem{
				ProviderID: ProviderID,
				Locator:    f.Path,
				Hints:      map[string]string{"size_bytes": strconv.FormatInt(f.Size, 10)},
			})
		}
	}
	return out, nil
}

// Resolve documenta explícitamente que en M2 no hay streaming aún: el
// endpoint real (/api/v1/stream/{source_id}) llega en M4. Devuelve el
// contenedor deducido de la extensión y URL vacía.
func (p *Provider) Resolve(ctx context.Context, src catalog.Source) (providers.PlayableStream, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(src.Locator)), ".")
	return providers.PlayableStream{URL: "", Container: ext}, nil
}

// Check comprueba existencia y legibilidad del fichero. No distingue
// "disco no conectado" (eso lo gestiona el poller de volúmenes sobre todas
// las fuentes de un volumen a la vez, con más contexto del que tiene una
// fuente aislada); aquí solo se informa vivo/caído.
func (p *Provider) Check(ctx context.Context, src catalog.Source) (providers.HealthStatus, error) {
	info, err := os.Stat(src.Locator)
	if err != nil || info.IsDir() {
		return providers.HealthStatus(catalog.HealthDead), nil
	}
	return providers.HealthStatus(catalog.HealthAlive), nil
}
