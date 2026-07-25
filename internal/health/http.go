package health

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/providers/httpprovider"
)

const (
	// httpCheckTimeout acota toda la comprobación (Brief M6 §2: "timeout
	// corto, p. ej. 8 s"). Valor por defecto de Client.Timeout; ver
	// NewHTTPChecker.
	httpCheckTimeout = 8 * time.Second
	// httpSlowThreshold: por encima de esto la fuente responde, pero se
	// marca lenta en vez de viva (Brief M6 §2). Valor por defecto; ver
	// HTTPChecker.SlowThreshold para sustituirlo (tests deterministas).
	httpSlowThreshold = 3 * time.Second
)

// HTTPChecker comprueba enlaces directos: HEAD y, si el servidor no lo
// soporta, GET con Range parcial (Brief M6 §2). Aplica el mismo bloqueo SSRF
// de M5 antes de cualquier petición saliente.
//
// Implementación propia en vez de reutilizar httpprovider.Provider.Check:
// ese método solo distingue vivo/caído para decidir el orden de /play; el
// checker de salud necesita además medir latencia (para "lenta") y mirar el
// Content-Type (para detectar una página de error disfrazada de 200 en vez
// de un vídeo real), señales que Provider.Check no expone ni necesita.
type HTTPChecker struct {
	Client *http.Client

	// SlowThreshold sustituye httpSlowThreshold cuando es > 0 (tests con
	// umbrales pequeños y deterministas).
	SlowThreshold time.Duration
}

// NewHTTPChecker crea el checker con el timeout del Brief M6 §2.
func NewHTTPChecker() *HTTPChecker {
	return &HTTPChecker{Client: &http.Client{Timeout: httpCheckTimeout}}
}

func (c *HTTPChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	if err := httpprovider.ValidateExternalURL(ctx, s.Locator); err != nil {
		return catalog.HealthDead, err.Error()
	}
	return c.checkLocator(ctx, s.Locator)
}

// checkLocator hace la comprobación real (HEAD/GET + interpretación de la
// respuesta), sin el paso de SSRF de Check: separado para poder probarlo en
// tests contra un httptest.Server real, que por definición escucha en
// 127.0.0.1 — una IP que ValidateExternalURL bloquearía siempre si el test
// pasara por Check (Brief M6 §6, SSRF se prueba aparte con una IP privada
// literal en TestHTTPCheckerSSRFBlocked).
func (c *HTTPChecker) checkLocator(ctx context.Context, locatorURL string) (string, string) {
	timeout := c.Client.Timeout
	if timeout <= 0 {
		timeout = httpCheckTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	slow := c.SlowThreshold
	if slow <= 0 {
		slow = httpSlowThreshold
	}

	start := time.Now()
	resp, method, err := c.probe(ctx, locatorURL)
	elapsed := time.Since(start)
	if err != nil {
		return catalog.HealthDead, fmt.Sprintf("sin respuesta (%s): %v", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return catalog.HealthDead, fmt.Sprintf("%s → http %d", method, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return catalog.HealthDead, "el servidor devolvió una página web (text/html), no un vídeo"
	}
	if elapsed > slow {
		return catalog.HealthSlow, "respuesta lenta"
	}
	return catalog.HealthAlive, ""
}

// probe intenta HEAD primero; si el servidor no lo soporta (405/501, o
// falla directamente), recurre a un GET con Range mínimo (Brief M6 §2).
func (c *HTTPChecker) probe(ctx context.Context, url string) (*http.Response, string, error) {
	if resp, err := c.do(ctx, http.MethodHead, url, false); err == nil {
		if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotImplemented {
			return resp, "HEAD", nil
		}
		resp.Body.Close()
	}
	resp, err := c.do(ctx, http.MethodGet, url, true)
	return resp, "GET", err
}

func (c *HTTPChecker) do(ctx context.Context, method, url string, partial bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	if partial {
		// GET parcial: no descargar el archivo entero solo para comprobar
		// salud (Brief M6 §2: "GET con Range 0-1").
		req.Header.Set("Range", "bytes=0-1")
	}
	return c.Client.Do(req)
}
