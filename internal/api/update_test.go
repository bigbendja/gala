package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bigbendja/gala/internal/update"
)

// fakeReleases sirve /repos/bigbendja/gala/releases/latest con una versión nueva.
func fakeReleases(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v0.10.0", "body": "- Auto-update", "html_url": srv.URL + "/release",
				"assets": []map[string]any{
					{"name": "gala-linux-amd64", "browser_download_url": srv.URL + "/dl/gala-linux-amd64", "size": 5},
					{"name": "SHA256SUMS.txt", "browser_download_url": srv.URL + "/dl/SHA256SUMS.txt"},
				},
			})
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS.txt"):
			io.WriteString(w, strings.Repeat("ab", 32)+"  gala-linux-amd64\n")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withUpdater(t *testing.T, a *authHarness, apiBase string, supervised bool) string {
	t.Helper()
	dir := t.TempDir()
	c := update.NewChecker("linux", "amd64", "v0.9.0", filepath.Join(dir, "update-check.json"))
	c.APIBase = apiBase
	c.AllowHost = func(string) bool { return true }
	a.srv.UpdateChecker = c
	a.srv.UpdateDir = dir
	a.srv.Updater = &update.Installer{
		Checker: c, Client: &http.Client{}, ExePath: filepath.Join(dir, "gala"), Supervised: supervised,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return dir
}

func TestHealthzEsPublico(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	for _, opts := range [][]reqOpt{nil, {fromRemote()}} {
		w := a.do("GET", "/healthz", "", opts...)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok"`) {
			t.Fatalf("/healthz: %d %s", w.Code, w.Body)
		}
	}
}

func TestUpdateCheck(t *testing.T) {
	a := newAuthHarness(t)
	withUpdater(t, a, fakeReleases(t).URL, true)
	w := a.do("GET", "/api/v1/update/check", "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var v struct {
		Checked  bool `json:"checked"`
		CanApply bool `json:"can_apply"`
		Current  string
		Info     struct {
			Latest          string `json:"latest"`
			UpdateAvailable bool   `json:"update_available"`
			Notes           string `json:"notes"`
		} `json:"info"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if !v.Checked || !v.CanApply || v.Info.Latest != "v0.10.0" || !v.Info.UpdateAvailable || !strings.Contains(v.Info.Notes, "Auto-update") {
		t.Fatalf("%s", w.Body)
	}
}

func TestUpdateCheckSinRedNoRompeNada(t *testing.T) {
	a := newAuthHarness(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	withUpdater(t, a, url, true)
	w := a.do("GET", "/api/v1/update/check", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"checked":false`) ||
		!strings.Contains(w.Body.String(), "No se pudo comprobar las actualizaciones") {
		t.Fatalf("sin red debe ser un 200 tranquilo: %d %s", w.Code, w.Body)
	}
}

func TestUpdateEsDeAdministracion(t *testing.T) {
	a := newAuthHarness(t)
	withUpdater(t, a, fakeReleases(t).URL, true)
	a.svc.SetPassword(context.Background(), goodPass)
	for _, c := range [][2]string{{"GET", "/api/v1/update/check"}, {"GET", "/api/v1/update/status"},
		{"POST", "/api/v1/update/apply"}, {"DELETE", "/api/v1/update/result"}} {
		if w := a.do(c[0], c[1], "", fromRemote()); w.Code != 401 || code(w) != "login_required" {
			t.Errorf("%s %s sin sesión: %d %s", c[0], c[1], w.Code, w.Body)
		}
	}
	cookies, csrf := a.login()
	if w := a.do("GET", "/api/v1/update/check", "", fromRemote(), withCookies(cookies)); w.Code != 200 {
		t.Fatalf("con sesión: %d", w.Code)
	}
	// Escrituras: exigen CSRF.
	if w := a.do("POST", "/api/v1/update/apply", "", fromRemote(), withCookies(cookies)); w.Code != 403 || code(w) != "csrf_invalid" {
		t.Fatalf("apply sin CSRF: %d %s", w.Code, w.Body)
	}
	_ = csrf
}

func TestUpdateApplyExplicaPorQueNo(t *testing.T) {
	a := newAuthHarness(t)
	withUpdater(t, a, fakeReleases(t).URL, false) // sin systemd
	a.do("GET", "/api/v1/update/check", "")
	w := a.do("POST", "/api/v1/update/apply", "")
	if w.Code != 409 || code(w) != "not_supervised" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "servicio") {
		t.Errorf("mensaje: %s", w.Body)
	}
}

func TestUpdateApplySinNadaQueAplicar(t *testing.T) {
	a := newAuthHarness(t)
	withUpdater(t, a, fakeReleases(t).URL, true) // sin haber comprobado
	if w := a.do("POST", "/api/v1/update/apply", ""); w.Code != 409 || code(w) != "no_update" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestUpdateStatusYResultado(t *testing.T) {
	a := newAuthHarness(t)
	dir := withUpdater(t, a, fakeReleases(t).URL, true)
	w := a.do("GET", "/api/v1/update/status", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"phase":"idle"`) || strings.Contains(w.Body.String(), `"result"`) {
		t.Fatalf("inactivo: %d %s", w.Code, w.Body)
	}
	update.WriteResult(dir, update.Result{Status: update.ResultRolledBk, From: "v0.9.0", To: "v0.10.0", Reason: "falló"})
	w = a.do("GET", "/api/v1/update/status", "")
	if !strings.Contains(w.Body.String(), `"status":"rolled_back"`) || !strings.Contains(w.Body.String(), `"to":"v0.10.0"`) {
		t.Fatalf("resultado: %s", w.Body)
	}
	if w := a.do("DELETE", "/api/v1/update/result", ""); w.Code != 200 {
		t.Fatalf("descartar: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, update.ResultFile)); !os.IsNotExist(err) {
		t.Fatal("el resultado debe borrarse")
	}
}

func TestUpdateSinServicio503(t *testing.T) {
	a := newAuthHarness(t)
	if w := a.do("GET", "/api/v1/update/check", ""); w.Code != 503 {
		t.Fatalf("%d", w.Code)
	}
}
