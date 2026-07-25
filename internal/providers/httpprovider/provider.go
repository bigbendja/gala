package httpprovider

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/providers"
)

// ProviderID identifica este provider en catalog.Source.Provider.
const ProviderID = "http"

// Provider implementa providers.Provider para enlaces directos (Brief M5
// §5). A diferencia de localfs, no "descubre" nada por sí solo: sus fuentes
// las crea la ingesta (internal/ingest, M5); Discover devuelve vacío.
type Provider struct {
	HTTPClient *http.Client
}

// New crea el provider http con un cliente con timeout razonable.
func New() *Provider {
	return &Provider{HTTPClient: &http.Client{Timeout: 10 * time.Second}}
}

func (p *Provider) ID() string { return ProviderID }

// Discover no hace nada: las fuentes http nacen de una ingesta explícita
// (M5), no de un escaneo periódico como localfs.
func (p *Provider) Discover(ctx context.Context) ([]providers.RawItem, error) {
	return nil, nil
}

// Resolve documenta el contenedor por extensión; el streaming real
// (redirect 302) ya lo sirve directamente internal/api/stream.go desde M4.
func (p *Provider) Resolve(ctx context.Context, src catalog.Source) (providers.PlayableStream, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(src.Locator)), ".")
	return providers.PlayableStream{URL: src.Locator, Container: ext}, nil
}

// Check verifica que el enlace responda, con HEAD y GET parcial de respaldo
// (algunos servidores no implementan HEAD), y con protección SSRF básica
// (Brief M5 §6): nunca sigue un locator que resuelva a una IP privada,
// loopback o link-local.
func (p *Provider) Check(ctx context.Context, src catalog.Source) (providers.HealthStatus, error) {
	if err := ValidateExternalURL(ctx, src.Locator); err != nil {
		return providers.HealthStatus(catalog.HealthDead), nil
	}
	if p.probe(ctx, http.MethodHead, src.Locator) {
		return providers.HealthStatus(catalog.HealthAlive), nil
	}
	if p.probe(ctx, http.MethodGet, src.Locator) {
		return providers.HealthStatus(catalog.HealthAlive), nil
	}
	return providers.HealthStatus(catalog.HealthDead), nil
}

func (p *Provider) probe(ctx context.Context, method, url string) bool {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return false
	}
	if method == http.MethodGet {
		// GET parcial: no descargar el archivo entero solo para comprobar salud.
		req.Header.Set("Range", "bytes=0-0")
	}
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 400
}
