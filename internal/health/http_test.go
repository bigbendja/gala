package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
)

// Nota: estos tests llaman a checkLocator directamente (no a Check) porque
// httptest.NewServer escucha en 127.0.0.1 — una IP que ValidateExternalURL
// bloquearía siempre si el test pasara por Check. La propia protección SSRF
// se prueba aparte en TestHTTPCheckerSSRFBlocked, con Check de verdad.

func TestHTTPCheckerAlive200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPChecker()
	got, detail := c.checkLocator(context.Background(), srv.URL)
	if got != catalog.HealthAlive {
		t.Fatalf("checkLocator = %q (%q), esperado viva", got, detail)
	}
}

func TestHTTPCheckerDead404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewHTTPChecker()
	got, detail := c.checkLocator(context.Background(), srv.URL)
	if got != catalog.HealthDead {
		t.Fatalf("checkLocator = %q (%q), esperado caída", got, detail)
	}
}

func TestHTTPCheckerDeadTextHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPChecker()
	got, detail := c.checkLocator(context.Background(), srv.URL)
	if got != catalog.HealthDead {
		t.Fatalf("checkLocator = %q (%q), esperado caída (página web, no vídeo)", got, detail)
	}
}

func TestHTTPCheckerSlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &HTTPChecker{Client: &http.Client{Timeout: 2 * time.Second}, SlowThreshold: 5 * time.Millisecond}
	got, detail := c.checkLocator(context.Background(), srv.URL)
	if got != catalog.HealthSlow {
		t.Fatalf("checkLocator = %q (%q), esperado lenta", got, detail)
	}
}

func TestHTTPCheckerTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &HTTPChecker{Client: &http.Client{Timeout: 10 * time.Millisecond}}
	got, detail := c.checkLocator(context.Background(), srv.URL)
	if got != catalog.HealthDead {
		t.Fatalf("checkLocator = %q (%q), esperado caída (timeout)", got, detail)
	}
}

func TestHTTPCheckerHeadNotAllowedFallsBackToGet(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gotMethod = r.Method
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPChecker()
	got, _ := c.checkLocator(context.Background(), srv.URL)
	if got != catalog.HealthAlive {
		t.Fatalf("checkLocator = %q, esperado viva tras respaldo GET", got)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("método usado tras 405 = %q, esperado GET", gotMethod)
	}
}

// TestHTTPCheckerSSRFBlocked SÍ pasa por Check (no checkLocator): es la
// única prueba de que el bloqueo SSRF de M5 se aplica de verdad antes de
// cualquier petición saliente (Brief M6 §2/§6).
func TestHTTPCheckerSSRFBlocked(t *testing.T) {
	c := NewHTTPChecker()
	got, detail := c.Check(context.Background(), catalog.Source{Locator: "http://127.0.0.1:9/lista.mp4"})
	if got != catalog.HealthDead {
		t.Fatalf("Check = %q (%q), esperado caída (bloqueado por SSRF)", got, detail)
	}
	if detail == "" {
		t.Error("detail vacío, esperado el motivo del bloqueo SSRF")
	}
}
