package lan

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeCfg struct{ bind, pin string }

func (f *fakeCfg) LanState() (string, string) { return f.bind, f.pin }

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func do(h http.Handler, method, target, remote string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = remote
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRemoteAllowedList(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/", true},
		{"GET", "/ajustes", true},
		{"POST", "/", false},
		{"GET", "/api/v1/items", true},
		{"GET", "/api/v1/items/7", true},
		{"POST", "/api/v1/items/7/wish", true},
		{"DELETE", "/api/v1/items/7/wish", true},
		{"DELETE", "/api/v1/items/7", false}, // borrar items: solo el PC
		{"POST", "/api/v1/items/7/confirm", false},
		{"GET", "/api/v1/img/p.jpg", true},
		{"GET", "/api/v1/genres", true},
		{"GET", "/api/v1/settings/home", true},
		{"PUT", "/api/v1/settings/home", false},
		{"GET", "/api/v1/settings/tmdb", false},
		{"GET", "/api/v1/play/7", true},
		{"GET", "/api/v1/stream/3", true},
		{"GET", "/api/v1/stream/3/hls/start", true},
		{"GET", "/api/v1/stream/3/hls/abc/index.m3u8", true},
		{"DELETE", "/api/v1/stream/3/hls/abc", true},
		{"DELETE", "/api/v1/stream/3", false},
		{"DELETE", "/api/v1/stream/3/hls/abc/index.m3u8", false},
		{"POST", "/api/v1/sources/3/failed", true},
		{"PUT", "/api/v1/playback/7", true},
		{"GET", "/api/v1/playback/7", true},
		{"GET", "/api/v1/folders", false},
		{"POST", "/api/v1/folders", false},
		{"DELETE", "/api/v1/folders", false},
		{"POST", "/api/v1/scan", false},
		{"GET", "/api/v1/volumes", false},
		{"GET", "/api/v1/pending", false},
		{"POST", "/api/v1/ingest", false},
		{"GET", "/api/v1/settings/tmdb", false},
		{"PUT", "/api/v1/settings/tmdb", false},
		{"POST", "/api/v1/health/check", false},
		{"GET", "/api/v1/identify/status", false},
		{"GET", "/api/v1/lan/status", false},
		{"POST", "/api/v1/lan/toggle", false},
		{"PUT", "/api/v1/lan/pin", false},
		{"GET", "/api/v1/lan/session", true},
		{"POST", "/api/v1/lan/auth", true},
		{"GET", "/api/v1/ruta-nueva-futura", false}, // lista cerrada
		{"GET", "/api/v2/items", false},
	}
	for _, c := range cases {
		if got := RemoteAllowed(c.method, c.path); got != c.want {
			t.Errorf("RemoteAllowed(%s %s) = %v, quería %v", c.method, c.path, got, c.want)
		}
	}
}

func TestGuardLocalPassesEverything(t *testing.T) {
	g := NewGuard(&fakeCfg{bind: "0.0.0.0", pin: "1234"})
	h := g.Middleware(okHandler())
	for _, remote := range []string{"127.0.0.1:5000", "[::1]:5000"} {
		if rec := do(h, "DELETE", "/api/v1/folders", remote, nil); rec.Code != 200 {
			t.Errorf("%s: el PC local nunca pide PIN ni se restringe, código %d", remote, rec.Code)
		}
	}
}

func TestGuardRemoteForbiddenRoutes(t *testing.T) {
	g := NewGuard(&fakeCfg{bind: "0.0.0.0"})
	h := g.Middleware(okHandler())
	for _, p := range []string{"/api/v1/folders", "/api/v1/settings/tmdb", "/api/v1/lan/toggle", "/api/v1/pending"} {
		if rec := do(h, "GET", p, "192.168.1.50:4000", nil); rec.Code != http.StatusForbidden {
			t.Errorf("GET %s desde LAN = %d, quería 403", p, rec.Code)
		}
	}
	if rec := do(h, "GET", "/api/v1/items", "192.168.1.50:4000", nil); rec.Code != 200 {
		t.Errorf("galería desde LAN sin PIN = %d, quería 200", rec.Code)
	}
}

func TestGuardRejectsPathTricks(t *testing.T) {
	g := NewGuard(&fakeCfg{bind: "0.0.0.0"})
	h := g.Middleware(okHandler())
	for _, p := range []string{
		"/api/v1/img/../settings/tmdb",
		"/api/v1/img/%2e%2e/settings/tmdb",
		"/api/v1//settings/tmdb",
		"/api/v1/items/./../folders",
	} {
		if rec := do(h, "GET", p, "192.168.1.50:4000", nil); rec.Code == 200 {
			t.Errorf("GET %s desde LAN pasó (200)", p)
		}
	}
}

func authBody(pin string) *strings.Reader { return strings.NewReader(`{"pin":"` + pin + `"}`) }

func postAuth(g *Guard, remote, pin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/v1/lan/auth", authBody(pin))
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	g.HandleAuth(rec, req)
	return rec
}

func TestPINFlow(t *testing.T) {
	g := NewGuard(&fakeCfg{bind: "0.0.0.0", pin: "4321"})
	h := g.Middleware(okHandler())
	remote := "192.168.1.50:4000"

	// Sin sesión: la API de datos pide PIN, la SPA y el ping no.
	if rec := do(h, "GET", "/api/v1/items", remote, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("items sin sesión = %d, quería 401", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/img/x.jpg", remote, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("carátula sin sesión = %d, quería 401", rec.Code)
	}
	for _, p := range []string{"/", "/ver/3", "/api/v1/ping", "/api/v1/lan/session"} {
		if rec := do(h, "GET", p, remote, nil); rec.Code != 200 {
			t.Errorf("%s sin sesión = %d, quería 200", p, rec.Code)
		}
	}

	// PIN incorrecto: 401 y sin cookie.
	rec := postAuth(g, remote, "0000")
	if rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("PIN malo: código %d, cookies %d", rec.Code, len(rec.Result().Cookies()))
	}

	// PIN correcto: cookie con token aleatorio, no con el PIN.
	rec = postAuth(g, remote, "4321")
	if rec.Code != 200 {
		t.Fatalf("PIN bueno = %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookie {
		t.Fatalf("cookies = %v", cookies)
	}
	c := cookies[0]
	if strings.Contains(c.Value, "4321") || len(c.Value) < 32 || !c.HttpOnly {
		t.Errorf("cookie insegura: %+v", c)
	}
	if rec := do(h, "GET", "/api/v1/items", remote, c); rec.Code != 200 {
		t.Errorf("con sesión = %d, quería 200", rec.Code)
	}
	// Una cookie inventada no vale.
	if rec := do(h, "GET", "/api/v1/items", remote, &http.Cookie{Name: SessionCookie, Value: "4321"}); rec.Code != 401 {
		t.Errorf("cookie falsa = %d, quería 401", rec.Code)
	}

	// Cambiar/quitar el PIN invalida las sesiones.
	g.ResetSessions()
	if rec := do(h, "GET", "/api/v1/items", remote, c); rec.Code != 401 {
		t.Errorf("tras reset = %d, quería 401", rec.Code)
	}
}

func TestNoPINNoGate(t *testing.T) {
	g := NewGuard(&fakeCfg{bind: "0.0.0.0"})
	if rec := do(g.Middleware(okHandler()), "GET", "/api/v1/items", "192.168.1.50:1", nil); rec.Code != 200 {
		t.Errorf("sin PIN configurado = %d", rec.Code)
	}
}

func TestSessionLocalAndRemote(t *testing.T) {
	g := NewGuard(&fakeCfg{bind: "0.0.0.0", pin: "1111"})
	get := func(remote string) string {
		req := httptest.NewRequest("GET", "/api/v1/lan/session", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		g.HandleSession(rec, req)
		b, _ := io.ReadAll(rec.Body)
		return string(b)
	}
	if b := get("127.0.0.1:1"); !strings.Contains(b, `"local":true`) || !strings.Contains(b, `"pin_required":false`) {
		t.Errorf("PC local: %s", b)
	}
	if b := get("10.0.0.7:1"); !strings.Contains(b, `"local":false`) || !strings.Contains(b, `"pin_required":true`) || !strings.Contains(b, `"authenticated":false`) {
		t.Errorf("remoto: %s", b)
	}
}

func TestRateLimitProgressive(t *testing.T) {
	now := time.Now()
	l := newLimiter(func() time.Time { return now })
	ip := "10.0.0.9"
	for i := 0; i < freeAttempts-1; i++ {
		if ok, _ := l.allow(ip); !ok {
			t.Fatalf("bloqueado antes de tiempo en el intento %d", i)
		}
		l.fail(ip)
	}
	l.fail(ip) // 5.º fallo: bloqueo de 30 s
	ok, wait := l.allow(ip)
	if ok || wait < 25*time.Second || wait > 31*time.Second {
		t.Fatalf("tras 5 fallos: ok=%v wait=%v", ok, wait)
	}
	now = now.Add(31 * time.Second)
	if ok, _ := l.allow(ip); !ok {
		t.Fatal("el bloqueo debía haber expirado")
	}
	l.fail(ip) // 6.º fallo: bloqueo de 60 s
	if _, wait := l.allow(ip); wait < 55*time.Second {
		t.Errorf("el bloqueo debe crecer: %v", wait)
	}
	// Otra IP no queda afectada.
	if ok, _ := l.allow("10.0.0.10"); !ok {
		t.Error("el bloqueo es por IP")
	}
}

func TestRateLimitGlobalCap(t *testing.T) {
	now := time.Now()
	l := newLimiter(func() time.Time { return now })
	// Muchas IPs distintas, 1 fallo cada una: ninguna se bloquea sola, pero
	// el tope global sí.
	for i := 0; i < globalMaxFails; i++ {
		l.fail("10.1.0." + string(rune('a'+i%26)) + string(rune('a'+i/26)))
	}
	if ok, _ := l.allow("10.9.9.9"); ok {
		t.Error("el tope global debía bloquear a todos")
	}
	now = now.Add(globalLock + time.Second)
	if ok, _ := l.allow("10.9.9.9"); !ok {
		t.Error("el bloqueo global debía expirar")
	}
}

func TestAuthRateLimited429(t *testing.T) {
	g := NewGuard(&fakeCfg{bind: "0.0.0.0", pin: "4321"})
	var last *httptest.ResponseRecorder
	for i := 0; i < freeAttempts+1; i++ {
		last = postAuth(g, "192.168.1.60:1", "9999")
	}
	if last.Code != http.StatusTooManyRequests || last.Header().Get("Retry-After") == "" {
		t.Fatalf("esperaba 429 con Retry-After, tengo %d", last.Code)
	}
	// Con el PIN bueno tampoco se entra mientras dure el bloqueo.
	if rec := postAuth(g, "192.168.1.60:1", "4321"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("bloqueado + PIN bueno = %d, quería 429", rec.Code)
	}
}

func TestSessionExpiry(t *testing.T) {
	now := time.Now()
	s := newSessions(func() time.Time { return now })
	tok := s.issue()
	if !s.valid(tok) {
		t.Fatal("sesión recién emitida inválida")
	}
	now = now.Add(sessionTTL + time.Hour)
	if s.valid(tok) {
		t.Error("la sesión debía caducar")
	}
}

func TestRankPrefersPhysicalPrivateIPs(t *testing.T) {
	up := net.FlagUp
	got := rank([]ifaceAddr{
		{"vEthernet (Default Switch)", up, net.ParseIP("172.20.0.1")},
		{"Tailscale", up, net.ParseIP("100.64.1.2")},
		{"Loopback Pseudo-Interface 1", up | net.FlagLoopback, net.ParseIP("127.0.0.1")},
		{"Ethernet", up, net.ParseIP("10.0.0.5")},
		{"Wi-Fi", up, net.ParseIP("192.168.1.20")},
		{"Wi-Fi", up, net.ParseIP("fe80::1")},
		{"Ethernet 2", up, net.ParseIP("169.254.3.4")},
		{"Wi-Fi 2", 0, net.ParseIP("192.168.9.9")}, // caída
	})
	want := []string{"192.168.1.20", "10.0.0.5", "172.20.0.1", "100.64.1.2"}
	if len(got) != len(want) {
		t.Fatalf("candidatas = %+v", got)
	}
	for i := range want {
		if got[i].IP != want[i] {
			t.Errorf("posición %d = %s, quería %s (%+v)", i, got[i].IP, want[i], got)
		}
	}
	if got[0].Virtual || !got[2].Virtual {
		t.Errorf("marcado virtual incorrecto: %+v", got)
	}
}

func TestControllerRebind(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := &http.Server{Handler: okHandler(), ReadHeaderTimeout: time.Second}
	c := NewController(srv, log)
	defer c.Close()

	if err := c.Listen("127.0.0.1", 0); err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(c.Addr())
	get := func() error {
		cl := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
		resp, err := cl.Get("http://127.0.0.1:" + port + "/")
		if err == nil {
			resp.Body.Close()
		}
		return err
	}
	if err := get(); err != nil {
		t.Fatalf("loopback: %v", err)
	}

	// Reabrir en 0.0.0.0 EN EL MISMO puerto, y volver.
	p := atoi(port)
	if err := c.Listen("0.0.0.0", p); err != nil {
		t.Fatalf("a 0.0.0.0: %v", err)
	}
	if !strings.HasPrefix(c.Addr(), "[::]:") && !strings.HasPrefix(c.Addr(), "0.0.0.0:") {
		t.Errorf("no escucha en todas las interfaces: %s", c.Addr())
	}
	if err := get(); err != nil {
		t.Fatalf("tras abrir: %v", err)
	}
	if err := c.Listen("127.0.0.1", p); err != nil {
		t.Fatalf("de vuelta a loopback: %v", err)
	}
	if err := get(); err != nil {
		t.Fatalf("tras cerrar: %v", err)
	}
}

func TestControllerListenFailureKeepsOldListener(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := &http.Server{Handler: okHandler(), ReadHeaderTimeout: time.Second}
	c := NewController(srv, log)
	defer c.Close()
	if err := c.Listen("127.0.0.1", 0); err != nil {
		t.Fatal(err)
	}
	// Ocupar un puerto y pedir ese: debe fallar y dejar el anterior vivo.
	busy, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skip(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port
	before := c.Addr()
	if err := c.Listen("0.0.0.0", busyPort); err == nil {
		t.Fatal("esperaba error por puerto ocupado")
	}
	if c.Addr() == "" {
		t.Fatalf("se perdió el listener anterior (antes %s)", before)
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
