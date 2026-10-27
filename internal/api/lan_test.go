package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bigbendja/gala/internal/api"
	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/lan"
	"github.com/bigbendja/gala/internal/store"
)

type lanEnv struct {
	srv     *api.Server
	handler http.Handler // mux detrás del guardián, como en producción
	cfg     *config.Config
	repo    *catalog.Repo
	dir     string
	binds   []string // binds pedidos a LanListen, en orden
}

func newLanEnv(t *testing.T) *lanEnv {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	repo := catalog.NewRepo(st.DB)
	cfg := config.NewAt(dir)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	env := &lanEnv{cfg: cfg, repo: repo, dir: dir}
	env.srv = api.New(st, repo, cfg, nil, fakeEnum{}, nil, nil, dir, log, nil, nil, nil, nil)
	env.srv.LanGuard = lan.NewGuard(cfg)
	env.srv.LanListen = func(bind string) error {
		env.binds = append(env.binds, bind)
		return nil
	}
	env.srv.LanCandidates = func() []lan.Candidate {
		return []lan.Candidate{{IP: "192.168.1.20", Interface: "Wi-Fi"}, {IP: "172.20.0.1", Interface: "vEthernet", Virtual: true}}
	}
	mux := http.NewServeMux()
	env.srv.Routes(mux)
	env.handler = env.srv.LanGuard.Middleware(mux)
	return env
}

// call simula una petición desde remote ("127.0.0.1:1" = el PC,
// "192.168.1.50:1" = un móvil).
func (e *lanEnv) call(method, target, remote, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = remote
	req.Host = "localhost:7420"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

const pc, phone = "127.0.0.1:50000", "192.168.1.50:50000"

func TestLanStatusDefaultsToLocalOnly(t *testing.T) {
	e := newLanEnv(t)
	rec := e.call("GET", "/api/v1/lan/status", pc, "", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var st struct {
		Enabled   bool   `json:"enabled"`
		Bind      string `json:"bind"`
		LanIP     string `json:"lan_ip"`
		URL       string `json:"url"`
		QRPayload string `json:"qr_payload"`
		PinSet    bool   `json:"pin_set"`
		Cands     []struct {
			IP string `json:"ip"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.Bind != "127.0.0.1" {
		t.Errorf("por defecto debe ser solo-localhost: %+v", st)
	}
	if st.LanIP != "192.168.1.20" || st.URL != "http://192.168.1.20:7420" || st.QRPayload != st.URL {
		t.Errorf("IP/URL/QR = %q %q %q (debe elegir la física, no la virtual)", st.LanIP, st.URL, st.QRPayload)
	}
	if len(st.Cands) != 2 {
		t.Errorf("candidatas = %d, esperadas 2", len(st.Cands))
	}
}

func TestLanToggleRebindsAndPersists(t *testing.T) {
	e := newLanEnv(t)
	rec := e.call("POST", "/api/v1/lan/toggle", pc, `{"enabled":true}`, nil)
	if rec.Code != 200 {
		t.Fatalf("activar: %d %s", rec.Code, rec.Body)
	}
	if len(e.binds) != 1 || e.binds[0] != "0.0.0.0" {
		t.Fatalf("binds pedidos = %v", e.binds)
	}
	raw, _ := os.ReadFile(filepath.Join(e.dir, "gala.json"))
	if !strings.Contains(string(raw), `"bind": "0.0.0.0"`) {
		t.Errorf("no se persistió el bind: %s", raw)
	}
	if !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Errorf("respuesta sin confirmar: %s", rec.Body)
	}

	// Idempotente: activar otra vez no reabre el listener.
	e.call("POST", "/api/v1/lan/toggle", pc, `{"enabled":true}`, nil)
	if len(e.binds) != 1 {
		t.Errorf("reabrió un listener que ya estaba activo: %v", e.binds)
	}

	rec = e.call("POST", "/api/v1/lan/toggle", pc, `{"enabled":false}`, nil)
	if rec.Code != 200 || len(e.binds) != 2 || e.binds[1] != "127.0.0.1" {
		t.Fatalf("desactivar: %d binds=%v", rec.Code, e.binds)
	}
	if e.cfg.LanEnabled() || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("no quedó desactivado: %s", rec.Body)
	}
}

func TestLanToggleFailureLeavesConfigUntouched(t *testing.T) {
	e := newLanEnv(t)
	e.srv.LanListen = func(string) error { return errors.New("puerto ocupado") }
	rec := e.call("POST", "/api/v1/lan/toggle", pc, `{"enabled":true}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("esperaba 409, tengo %d", rec.Code)
	}
	if e.cfg.LanEnabled() {
		t.Error("el bind cambió aunque el listener falló")
	}
}

func TestLanEndpointsAreOwnerOnly(t *testing.T) {
	e := newLanEnv(t)
	for _, c := range []struct{ m, p, b string }{
		{"GET", "/api/v1/lan/status", ""},
		{"POST", "/api/v1/lan/toggle", `{"enabled":true}`},
		{"PUT", "/api/v1/lan/pin", `{"pin":"1234"}`},
	} {
		if rec := e.call(c.m, c.p, phone, c.b, nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s desde el móvil = %d, esperaba 403", c.m, c.p, rec.Code)
		}
	}
	if e.cfg.LanEnabled() || e.cfg.LanPIN != "" {
		t.Error("un cliente remoto cambió la configuración")
	}
	// Defensa en profundidad: sin guardián, los handlers siguen negando a remotos.
	mux := http.NewServeMux()
	e.srv.Routes(mux)
	req := httptest.NewRequest("POST", "/api/v1/lan/toggle", strings.NewReader(`{"enabled":true}`))
	req.RemoteAddr, req.Host = phone, "localhost"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("sin guardián, toggle desde remoto = %d, esperaba 403", rec.Code)
	}
}

func TestLanMutationsRejectCSRFAndRebinding(t *testing.T) {
	e := newLanEnv(t)
	// Formulario de otra web (text/plain): sin preflight, no debe valer.
	rec := e.call("POST", "/api/v1/lan/toggle", pc, `{"enabled":true}`, map[string]string{"Content-Type": "text/plain"})
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain = %d, esperaba 415", rec.Code)
	}
	// DNS rebinding: la petición llega desde loopback pero con Host ajeno.
	req := httptest.NewRequest("POST", "/api/v1/lan/toggle", strings.NewReader(`{"enabled":true}`))
	req.RemoteAddr, req.Host = pc, "evil.example.com:7420"
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("Host ajeno = %d, esperaba 403", rec.Code)
	}
	if e.cfg.LanEnabled() {
		t.Error("se activó el Modo LAN desde una petición no legítima")
	}
}

func TestLanPINSetValidateAndNeverLeak(t *testing.T) {
	e := newLanEnv(t)
	for _, bad := range []string{`{"pin":"123"}`, `{"pin":"12345"}`, `{"pin":"abcd"}`, `{}`} {
		if rec := e.call("PUT", "/api/v1/lan/pin", pc, bad, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, esperaba 400", bad, rec.Code)
		}
	}
	rec := e.call("PUT", "/api/v1/lan/pin", pc, `{"pin":"2468"}`, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"pin_set":true`) {
		t.Fatalf("poner PIN: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "2468") ||
		strings.Contains(e.call("GET", "/api/v1/lan/status", pc, "", nil).Body.String(), "2468") {
		t.Error("el PIN aparece en claro en la API")
	}
	if _, pin := e.cfg.LanState(); pin != "2468" {
		t.Errorf("PIN no guardado en config: %q", pin)
	}

	// Quitar el PIN.
	rec = e.call("PUT", "/api/v1/lan/pin", pc, `{"pin":""}`, nil)
	if _, pin := e.cfg.LanState(); rec.Code != 200 || pin != "" || !strings.Contains(rec.Body.String(), `"pin_set":false`) {
		t.Errorf("quitar PIN: %d %s", rec.Code, rec.Body)
	}
}

func TestLanPINGatesPhoneButNotPC(t *testing.T) {
	e := newLanEnv(t)
	e.call("PUT", "/api/v1/lan/pin", pc, `{"pin":"2468"}`, nil)

	if rec := e.call("GET", "/api/v1/items", pc, "", nil); rec.Code != 200 {
		t.Errorf("el PC nunca pide PIN: %d", rec.Code)
	}
	if rec := e.call("GET", "/api/v1/items", phone, "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("móvil sin PIN: %d", rec.Code)
	}
	if rec := e.call("POST", "/api/v1/lan/auth", phone, `{"pin":"0000"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("PIN malo: %d", rec.Code)
	}
	rec := e.call("POST", "/api/v1/lan/auth", phone, `{"pin":"2468"}`, nil)
	if rec.Code != 200 || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("PIN bueno: %d", rec.Code)
	}
	cookie := rec.Result().Cookies()[0]
	cookieHdr := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}
	if rec := e.call("GET", "/api/v1/items", phone, "", cookieHdr); rec.Code != 200 {
		t.Errorf("móvil con sesión: %d", rec.Code)
	}
	// Cambiar el PIN echa a los dispositivos ya dentro.
	e.call("PUT", "/api/v1/lan/pin", pc, `{"pin":"1357"}`, nil)
	if rec := e.call("GET", "/api/v1/items", phone, "", cookieHdr); rec.Code != http.StatusUnauthorized {
		t.Errorf("tras cambiar el PIN la sesión vieja sigue valiendo: %d", rec.Code)
	}
}

func TestPhoneCannotTouchSettingsFoldersOrDelete(t *testing.T) {
	e := newLanEnv(t)
	id, _ := e.repo.InsertItem(context.Background(), &catalog.Item{Kind: catalog.KindOther, Title: "x", Genres: []string{}}, "")
	sid := strconv.FormatInt(id, 10)
	for _, c := range []struct{ m, p, b string }{
		{"GET", "/api/v1/settings/tmdb", ""},
		{"PUT", "/api/v1/settings/tmdb", `{"api_key":"k"}`},
		{"GET", "/api/v1/folders", ""},
		{"POST", "/api/v1/folders", `{"path":"C:/"}`},
		{"DELETE", "/api/v1/folders?path=C:/", ""},
		{"POST", "/api/v1/scan", ""},
		{"GET", "/api/v1/volumes", ""},
		{"GET", "/api/v1/pending", ""},
		{"POST", "/api/v1/ingest", "http://x"},
		{"DELETE", "/api/v1/items/" + sid, ""},
		{"POST", "/api/v1/items/" + sid + "/confirm", `{"title":"y"}`},
		{"POST", "/api/v1/health/check", ""},
	} {
		if rec := e.call(c.m, c.p, phone, c.b, nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s desde el móvil = %d, esperaba 403", c.m, c.p, rec.Code)
		}
	}
	if _, err := e.repo.Get(context.Background(), id); err != nil {
		t.Errorf("el item fue borrado por un cliente remoto: %v", err)
	}
}

func TestPhoneItemDetailHidesDiskPaths(t *testing.T) {
	e := newLanEnv(t)
	ctx := context.Background()
	id, _ := e.repo.InsertItem(ctx, &catalog.Item{Kind: catalog.KindOther, Title: "Peli", Genres: []string{}}, "")
	vol := "vol-secreto"
	detail := `no se puede leer D:\Privado\Peli.mkv`
	if _, err := e.repo.InsertSource(ctx, &catalog.Source{
		ItemID: id, Provider: "localfs", Locator: `D:\Privado\Pelis\Peli.mkv`, VolumeID: &vol, HealthDetail: &detail,
	}, ""); err != nil {
		t.Fatal(err)
	}
	sid := strconv.FormatInt(id, 10)

	phoneBody := e.call("GET", "/api/v1/items/"+sid, phone, "", nil).Body.String()
	for _, leak := range []string{"Privado", "vol-secreto", "no se puede leer"} {
		if strings.Contains(phoneBody, leak) {
			t.Errorf("el móvil ve %q: %s", leak, phoneBody)
		}
	}
	if !strings.Contains(phoneBody, "Peli.mkv") {
		t.Errorf("debería quedar el nombre del archivo: %s", phoneBody)
	}
	// El PC sí ve la ruta completa.
	if body := e.call("GET", "/api/v1/items/"+sid, pc, "", nil).Body.String(); !strings.Contains(body, "Privado") {
		t.Errorf("el PC debe ver la ruta completa: %s", body)
	}
}
