package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bigbendja/gala/internal/api"
	"github.com/bigbendja/gala/internal/auth"
	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/lan"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
)

const (
	remoteAddr = "192.168.1.50:4444"
	localAddr  = "127.0.0.1:5555"
	goodPass   = "una-clave-larga"
)

// authHarness monta la API con el guardián y la autenticación reales, y
// permite lanzar peticiones «desde el PC» o «desde la red».
type authHarness struct {
	t     *testing.T
	h     http.Handler
	svc   *auth.Service
	cfg   *config.Config
	guard *lan.Guard
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()
	t.Cleanup(auth.SetFastParamsForTests())
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	repo := catalog.NewRepo(st.DB)
	cfg := config.NewAt(dir)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sc := scan.New(ctx, cfg, repo, fakeEnum{}, log)
	srv := api.New(st, repo, cfg, sc, fakeEnum{}, nil, nil, dir, log, nil, nil, nil, nil)
	svc, err := auth.New(st.DB, cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	srv.Auth = svc
	guard := lan.NewGuard(cfg)
	guard.Admin = svc
	srv.LanGuard = guard
	mux := http.NewServeMux()
	srv.Routes(mux)
	return &authHarness{t: t, h: guard.Middleware(mux), svc: svc, cfg: cfg, guard: guard}
}

type reqOpt func(*http.Request)

func fromRemote() reqOpt {
	return func(r *http.Request) { r.RemoteAddr = remoteAddr; r.Host = "192.168.1.5:7420" }
}
func withCookies(cs []*http.Cookie) reqOpt {
	return func(r *http.Request) {
		for _, c := range cs {
			r.AddCookie(c)
		}
	}
}
func withHeader(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }

func (a *authHarness) do(method, path, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	a.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = localAddr
	r.Host = "127.0.0.1:7420"
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, r)
	return w
}

func code(w *httptest.ResponseRecorder) string {
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	s, _ := v["code"].(string)
	return s
}

// login inicia sesión desde la red y devuelve cookies y token CSRF.
func (a *authHarness) login(extra ...reqOpt) ([]*http.Cookie, string) {
	a.t.Helper()
	w := a.do("POST", "/api/v1/auth/login", `{"password":"`+goodPass+`"}`, append([]reqOpt{fromRemote()}, extra...)...)
	if w.Code != 200 {
		a.t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	var v struct{ CSRF string }
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	return w.Result().Cookies(), v.CSRF
}

func TestAuthSinClaveLocalhostSigueSiendoAdmin(t *testing.T) {
	a := newAuthHarness(t)
	for _, p := range []string{"/api/v1/folders", "/api/v1/settings/tmdb", "/api/v1/health/summary"} {
		if w := a.do("GET", p, ""); w.Code != 200 {
			t.Errorf("localhost sin clave: %s → %d (debe seguir siendo admin)", p, w.Code)
		}
	}
	w := a.do("GET", "/api/v1/auth/status", "")
	if !strings.Contains(w.Body.String(), `"authenticated":true`) || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Errorf("status: %s", w.Body)
	}
}

func TestAuthRutaAdminSinClaveDesdeLaRed(t *testing.T) {
	a := newAuthHarness(t)
	w := a.do("GET", "/api/v1/folders", "", fromRemote())
	if w.Code != 401 || code(w) != "auth_not_configured" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "La administración desde la red no está configurada") {
		t.Errorf("mensaje: %s", w.Body)
	}
	// Lo abierto sigue abierto sin clave.
	if w := a.do("GET", "/api/v1/items", "", fromRemote()); w.Code != 200 {
		t.Errorf("items abierto: %d", w.Code)
	}
}

func TestAuthLoginSesionCSRFYLogoutAll(t *testing.T) {
	a := newAuthHarness(t)
	if err := a.svc.SetPassword(context.Background(), goodPass); err != nil {
		t.Fatal(err)
	}
	if w := a.do("GET", "/api/v1/folders", "", fromRemote()); w.Code != 401 || code(w) != "login_required" {
		t.Fatalf("sin sesión: %d %s", w.Code, w.Body)
	}
	if w := a.do("POST", "/api/v1/auth/login", `{"password":"mala-clave-x"}`, fromRemote()); w.Code != 401 || code(w) != "bad_password" {
		t.Fatalf("clave mala: %d %s", w.Code, w.Body)
	}
	cookies, csrf := a.login()
	var sc *http.Cookie
	for _, c := range cookies {
		if c.Name == auth.SessionCookie {
			sc = c
		}
	}
	if sc == nil || !sc.HttpOnly || sc.SameSite != http.SameSiteLaxMode || sc.Path != "/" || sc.Secure {
		t.Fatalf("cookie de sesión mal formada: %+v", sc)
	}
	if w := a.do("GET", "/api/v1/folders", "", fromRemote(), withCookies(cookies)); w.Code != 200 {
		t.Fatalf("con sesión: %d %s", w.Code, w.Body)
	}
	// Escritura sin CSRF → 403; con CSRF pasa.
	if w := a.do("POST", "/api/v1/auth/logout-all", "", fromRemote(), withCookies(cookies)); w.Code != 403 || code(w) != "csrf_invalid" {
		t.Fatalf("sin CSRF: %d %s", w.Code, w.Body)
	}
	if w := a.do("POST", "/api/v1/auth/logout-all", "", fromRemote(), withCookies(cookies), withHeader("X-GALA-CSRF", "falso")); w.Code != 403 {
		t.Fatalf("CSRF falso: %d", w.Code)
	}
	if w := a.do("POST", "/api/v1/auth/logout-all", "", fromRemote(), withCookies(cookies), withHeader("X-GALA-CSRF", csrf)); w.Code != 200 {
		t.Fatalf("con CSRF: %d %s", w.Code, w.Body)
	}
	if w := a.do("GET", "/api/v1/folders", "", fromRemote(), withCookies(cookies)); w.Code != 401 {
		t.Fatalf("tras logout-all la sesión debe estar muerta: %d", w.Code)
	}
}

func TestAuthXForwardedForSeIgnora(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	w := a.do("GET", "/api/v1/folders", "", fromRemote(),
		withHeader("X-Forwarded-For", "127.0.0.1"), withHeader("X-Real-IP", "127.0.0.1"))
	if w.Code != 401 {
		t.Fatalf("una cabecera no puede convertir a un cliente de red en localhost: %d", w.Code)
	}
}

func TestAuthSetPasswordCortaLaSesionViva(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	cookies, _ := a.login()
	if w := a.do("GET", "/api/v1/folders", "", fromRemote(), withCookies(cookies)); w.Code != 200 {
		t.Fatal(w.Code)
	}
	// Lo que hace `gala admin set-password` desde otro proceso.
	a.svc.SetPassword(context.Background(), "otra-clave-nueva")
	if w := a.do("GET", "/api/v1/folders", "", fromRemote(), withCookies(cookies)); w.Code != 401 {
		t.Fatalf("la sesión debe morir al instante: %d", w.Code)
	}
}

func TestAuthNetworkAdminSoloDesdeElPC(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	cookies, csrf := a.login()
	remote := []reqOpt{fromRemote(), withCookies(cookies), withHeader("X-GALA-CSRF", csrf)}
	if w := a.do("PUT", "/api/v1/auth/network-admin", `{"enabled":false}`, remote...); w.Code != 403 {
		t.Fatalf("desde la red no se puede tocar: %d", w.Code)
	}
	if w := a.do("PUT", "/api/v1/auth/network-admin", `{"enabled":false}`); w.Code != 200 {
		t.Fatalf("desde el PC: %d %s", w.Code, w.Body)
	}
	if w := a.do("GET", "/api/v1/folders", "", remote...); w.Code != 403 || code(w) != "network_admin_disabled" {
		t.Fatalf("administración de red desactivada: %d %s", w.Code, w.Body)
	}
	// El PC sigue siendo admin y lo abierto sigue abierto.
	if w := a.do("GET", "/api/v1/folders", ""); w.Code != 200 {
		t.Fatalf("localhost: %d", w.Code)
	}
	if w := a.do("GET", "/api/v1/items", "", remote...); w.Code != 200 {
		t.Fatalf("items: %d", w.Code)
	}
}

func TestAuthLanToggleSoloPCyPinPermitidoAlAdmin(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	cookies, csrf := a.login()
	remote := []reqOpt{fromRemote(), withCookies(cookies), withHeader("X-GALA-CSRF", csrf)}
	if w := a.do("POST", "/api/v1/lan/toggle", `{"enabled":false}`, remote...); w.Code != 403 {
		t.Fatalf("lan/toggle desde la red: %d", w.Code)
	}
	if w := a.do("GET", "/api/v1/lan/status", "", remote...); w.Code != 403 {
		t.Fatalf("lan/status desde la red: %d", w.Code)
	}
	if w := a.do("PUT", "/api/v1/lan/pin", `{"pin":"1234"}`, fromRemote()); w.Code != 401 {
		t.Fatalf("lan/pin sin sesión: %d", w.Code)
	}
	w := a.do("PUT", "/api/v1/lan/pin", `{"pin":"1234"}`, remote...)
	if w.Code != 200 {
		t.Fatalf("lan/pin con sesión: %d %s", w.Code, w.Body)
	}
	got := false
	for _, c := range w.Result().Cookies() {
		got = got || c.Name == lan.SessionCookie
	}
	if !got {
		t.Error("el admin que cambia el PIN debe recibir sesión LAN nueva (no se expulsa a sí mismo)")
	}
	if _, pin := a.cfg.LanState(); pin != "1234" {
		t.Errorf("pin = %q", pin)
	}
}

func TestAuthConPINPrimeroElPIN(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	a.cfg.SetLanPIN("1234")
	if w := a.do("GET", "/api/v1/folders", "", fromRemote()); w.Code != 401 || code(w) != "pin_required" {
		t.Fatalf("el PIN va antes que la clave: %d %s", w.Code, w.Body)
	}
}

func TestAuthLoginRateLimit(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	for i := 0; i < 5; i++ {
		a.do("POST", "/api/v1/auth/login", `{"password":"mala-clave-x"}`, fromRemote())
	}
	w := a.do("POST", "/api/v1/auth/login", `{"password":"`+goodPass+`"}`, fromRemote())
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || code(w) != "rate_limited" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// Desde el PC nunca se bloquea.
	if w := a.do("POST", "/api/v1/auth/login", `{"password":"`+goodPass+`"}`); w.Code != 200 {
		t.Fatalf("localhost: %d", w.Code)
	}
}

func TestAuthLoginExigeJSON(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("password="+goodPass))
	r.RemoteAddr, r.Host = remoteAddr, "192.168.1.5:7420"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatalf("un formulario cruzado no debe poder iniciar sesión: %d", w.Code)
	}
}

func TestAuthCambiarClave(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	c1, csrf1 := a.login()
	c2, _ := a.login()
	op := []reqOpt{fromRemote(), withCookies(c1), withHeader("X-GALA-CSRF", csrf1)}
	if w := a.do("POST", "/api/v1/auth/password", `{"current":"no-es-esta-1","new":"clave-nueva-123"}`, op...); w.Code != 401 {
		t.Fatalf("clave actual errónea: %d", w.Code)
	}
	if w := a.do("POST", "/api/v1/auth/password", `{"current":"`+goodPass+`","new":"corta"}`, op...); w.Code != 400 {
		t.Fatalf("clave nueva corta: %d", w.Code)
	}
	if w := a.do("POST", "/api/v1/auth/password", `{"current":"`+goodPass+`","new":"clave-nueva-123"}`, op...); w.Code != 200 {
		t.Fatalf("cambio: %d %s", w.Code, w.Body)
	}
	if w := a.do("GET", "/api/v1/folders", "", fromRemote(), withCookies(c1)); w.Code != 200 {
		t.Errorf("la sesión actual se conserva: %d", w.Code)
	}
	if w := a.do("GET", "/api/v1/folders", "", fromRemote(), withCookies(c2)); w.Code != 401 {
		t.Errorf("las demás sesiones se cierran: %d", w.Code)
	}
	// Sin clave previa, desde la red no se puede poner la primera.
	b := newAuthHarness(t)
	if w := b.do("POST", "/api/v1/auth/password", `{"new":"clave-nueva-123"}`, fromRemote()); w.Code != 401 {
		t.Errorf("primera clave desde la red: %d", w.Code)
	}
	// Desde el PC sí.
	if w := b.do("POST", "/api/v1/auth/password", `{"new":"clave-nueva-123"}`); w.Code != 200 {
		t.Errorf("primera clave desde el PC: %d %s", w.Code, w.Body)
	}
}

func TestAuthLocalhostSinBypassExigeSesionSoloConClave(t *testing.T) {
	a := newAuthHarness(t)
	off := false
	a.cfg.AdminBypassSetting = &off
	// Sin clave: localhost nunca se queda fuera.
	if w := a.do("GET", "/api/v1/folders", ""); w.Code != 200 {
		t.Fatalf("sin clave: %d", w.Code)
	}
	a.svc.SetPassword(context.Background(), goodPass)
	if w := a.do("GET", "/api/v1/folders", ""); w.Code != 401 {
		t.Fatalf("con clave y bypass desactivado: %d", w.Code)
	}
	w := a.do("POST", "/api/v1/auth/login", `{"password":"`+goodPass+`"}`)
	if w.Code != 200 {
		t.Fatalf("login local: %d", w.Code)
	}
	if w := a.do("GET", "/api/v1/folders", "", withCookies(w.Result().Cookies())); w.Code != 200 {
		t.Fatalf("local con sesión: %d", w.Code)
	}
}

func TestAuthSesionesYLogout(t *testing.T) {
	a := newAuthHarness(t)
	a.svc.SetPassword(context.Background(), goodPass)
	cookies, csrf := a.login(withHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0) Chrome/120 Safari/537"))
	w := a.do("GET", "/api/v1/auth/sessions", "", fromRemote(), withCookies(cookies))
	var rows []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rows)
	if len(rows) != 1 || rows[0]["device"] != "Chrome en Windows" || rows[0]["current"] != true {
		t.Fatalf("sesiones: %s", w.Body)
	}
	if strings.Contains(w.Body.String(), "id_hash") {
		t.Error("no se exponen identificadores completos")
	}
	if w := a.do("POST", "/api/v1/auth/logout", "", fromRemote(), withCookies(cookies), withHeader("X-GALA-CSRF", csrf)); w.Code != 200 {
		t.Fatalf("logout: %d", w.Code)
	}
	if w := a.do("GET", "/api/v1/folders", "", fromRemote(), withCookies(cookies)); w.Code != 401 {
		t.Fatalf("tras logout: %d", w.Code)
	}
}
