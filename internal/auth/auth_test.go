package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bigbendja/gala/internal/store"
)

type fakeCfg struct {
	days   int
	bypass bool
}

func (f fakeCfg) SessionDays() int           { return f.days }
func (f fakeCfg) AdminLocalhostBypass() bool { return f.bypass }

func newSvc(t *testing.T) (*Service, *time.Time) {
	t.Helper()
	restore := SetFastParamsForTests()
	t.Cleanup(restore)
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc, err := New(st.DB, fakeCfg{days: 30, bypass: true}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	return svc, &now
}

func TestHashVerify(t *testing.T) {
	defer SetFastParamsForTests()()
	h, err := HashPassword("clave-correcta")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=8,t=1,p=1$") {
		t.Fatalf("formato inesperado: %s", h)
	}
	ok, rehash, err := VerifyPassword("clave-correcta", h)
	if err != nil || !ok || rehash {
		t.Fatalf("ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if ok, _, _ := VerifyPassword("otra-clave-1", h); ok {
		t.Fatal("una clave distinta no debe verificar")
	}
	h2, _ := HashPassword("clave-correcta")
	if h == h2 {
		t.Fatal("dos hashes de la misma clave deben diferir (sal)")
	}
	// Un hash con parámetros distintos a los vigentes verifica y pide re-hash.
	argonTime = 2
	ok, rehash, _ = VerifyPassword("clave-correcta", h)
	argonTime = 1
	if !ok || !rehash {
		t.Fatalf("esperaba ok y rehash; ok=%v rehash=%v", ok, rehash)
	}
}

func TestParseHashRechazaBasura(t *testing.T) {
	for _, bad := range []string{"", "x", "$argon2id$v=19$m=999999999,t=2,p=1$AAAA$AAAA", "$bcrypt$v=19$m=8,t=1,p=1$AAAA$AAAA"} {
		if _, _, err := VerifyPassword("x", bad); err == nil {
			t.Errorf("%q debería dar error", bad)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	if ValidatePassword("corta") == nil || ValidatePassword(strings.Repeat("a", 129)) == nil {
		t.Fatal("longitudes fuera de rango deben fallar")
	}
	if ValidatePassword("ocho-car") != nil || ValidatePassword(strings.Repeat("ñ", 128)) != nil {
		t.Fatal("8 y 128 caracteres (no bytes) deben valer")
	}
}

func TestLoginYSesion(t *testing.T) {
	svc, now := newSvc(t)
	ctx := context.Background()
	if _, _, err := svc.Login(ctx, "x", "10.0.0.2", "ua", false); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("sin clave configurada: %v", err)
	}
	if err := svc.SetPassword(ctx, "una-clave-larga"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, "mala-clave-x", "10.0.0.2", "ua", false); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("clave mala: %v", err)
	}
	tok, csrf, err := svc.Login(ctx, "una-clave-larga", "10.0.0.2", "Mozilla Chrome/1 Windows", false)
	if err != nil || tok == "" || csrf == "" {
		t.Fatalf("login: %v", err)
	}
	// El token no está en la BD, solo su hash.
	var n int
	svc.db.QueryRow(`SELECT COUNT(*) FROM admin_sessions WHERE id_hash = ?`, tok).Scan(&n)
	if n != 0 {
		t.Fatal("el token en claro no debe estar guardado")
	}
	sess, err := svc.Lookup(ctx, tok)
	if err != nil || sess.IP != "10.0.0.2" {
		t.Fatalf("lookup: %v", err)
	}
	if svc.CSRFToken(sess.IDHash) != csrf {
		t.Fatal("el CSRF del login debe ser el derivado de la sesión")
	}

	// Caducidad deslizante: usarla la renueva; sin usar, caduca.
	*now = now.Add(20 * 24 * time.Hour)
	sess, err = svc.Lookup(ctx, tok)
	if err != nil {
		t.Fatalf("a los 20 días debe seguir viva: %v", err)
	}
	svc.touch(ctx, sess)
	*now = now.Add(20 * 24 * time.Hour) // 40 días desde el login, 20 desde el último uso
	if _, err := svc.Lookup(ctx, tok); err != nil {
		t.Fatalf("renovada, debe seguir viva a los 40 días: %v", err)
	}
	*now = now.Add(31 * 24 * time.Hour)
	if _, err := svc.Lookup(ctx, tok); !errors.Is(err, ErrNoSession) {
		t.Fatalf("sin uso 31 días debe caducar: %v", err)
	}
}

func TestSetPasswordCierraSesionesYLogoutAll(t *testing.T) {
	svc, _ := newSvc(t)
	ctx := context.Background()
	svc.SetPassword(ctx, "una-clave-larga")
	tok1, _, _ := svc.Login(ctx, "una-clave-larga", "10.0.0.2", "a", false)
	tok2, _, _ := svc.Login(ctx, "una-clave-larga", "10.0.0.3", "b", false)

	// Cambiar la clave desde la sesión 1 conserva la 1 y cierra la 2.
	s1, _ := svc.Lookup(ctx, tok1)
	if err := svc.ChangePassword(ctx, "una-clave-larga", "clave-nueva-1", s1.IDHash, "10.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Lookup(ctx, tok1); err != nil {
		t.Fatal("la sesión desde la que se cambia debe sobrevivir")
	}
	if _, err := svc.Lookup(ctx, tok2); !errors.Is(err, ErrNoSession) {
		t.Fatal("las demás deben cerrarse")
	}
	if err := svc.ChangePassword(ctx, "la-vieja-clave", "otra-clave-x", "", "10.0.0.2"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("clave actual errónea: %v", err)
	}
	// SetPassword (CLI) cierra todas, incluida la 1.
	svc.SetPassword(ctx, "clave-por-cli-1")
	if _, err := svc.Lookup(ctx, tok1); !errors.Is(err, ErrNoSession) {
		t.Fatal("set-password debe cortar todas las sesiones")
	}
	tok3, _, _ := svc.Login(ctx, "clave-por-cli-1", "10.0.0.2", "a", false)
	svc.LogoutAll(ctx)
	if _, err := svc.Lookup(ctx, tok3); !errors.Is(err, ErrNoSession) {
		t.Fatal("logout-all debe cortar todas")
	}
}

func TestRateLimitLogin(t *testing.T) {
	svc, now := newSvc(t)
	ctx := context.Background()
	svc.SetPassword(ctx, "una-clave-larga")
	for i := 0; i < freeAttempts; i++ {
		if _, _, err := svc.Login(ctx, "mala-clave-x", "10.0.0.9", "a", false); !errors.Is(err, ErrBadPassword) {
			t.Fatalf("intento %d: %v", i, err)
		}
	}
	var rl *RateLimitedError
	if _, _, err := svc.Login(ctx, "una-clave-larga", "10.0.0.9", "a", false); !errors.As(err, &rl) {
		t.Fatalf("tras %d fallos debe bloquear incluso la clave buena: %v", freeAttempts, err)
	}
	// Otra IP no se ve afectada.
	if _, _, err := svc.Login(ctx, "una-clave-larga", "10.0.0.10", "a", false); err != nil {
		t.Fatalf("otra IP: %v", err)
	}
	// Localhost nunca se bloquea (rescate).
	for i := 0; i < freeAttempts+3; i++ {
		svc.Login(ctx, "mala-clave-x", "127.0.0.1", "a", true)
	}
	if _, _, err := svc.Login(ctx, "una-clave-larga", "127.0.0.1", "a", true); err != nil {
		t.Fatalf("localhost no debe bloquearse: %v", err)
	}
	// Pasado el bloqueo vuelve a funcionar.
	*now = now.Add(baseLock + time.Second)
	if _, _, err := svc.Login(ctx, "una-clave-larga", "10.0.0.9", "a", false); err != nil {
		t.Fatalf("tras el bloqueo: %v", err)
	}
}

func TestLimitadorGlobalYProgresivo(t *testing.T) {
	now := time.Now()
	l := newLimiter(func() time.Time { return now })
	for i := 0; i < freeAttempts+2; i++ {
		l.fail("1.1.1.1")
	}
	ok, wait := l.allow("1.1.1.1")
	if ok || wait < 2*baseLock-time.Second {
		t.Fatalf("el bloqueo debe duplicarse: ok=%v wait=%v", ok, wait)
	}
	l2 := newLimiter(func() time.Time { return now })
	for i := 0; i < globalMaxFails; i++ {
		l2.fail(string(rune('a'+i%26)) + string(rune('A'+i)))
	}
	if ok, _ := l2.allow("nueva-ip"); ok {
		t.Fatal("el tope global debe bloquear a todos")
	}
}

func TestCheckSinSesionNiClave(t *testing.T) {
	svc, _ := newSvc(t)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/folders", nil)
	r.RemoteAddr = "192.168.1.5:1000"
	ok, status, code, _ := svc.Check(r)
	if ok || status != 401 || code != "auth_not_configured" {
		t.Fatalf("sin clave: %v %d %s", ok, status, code)
	}
	svc.SetPassword(context.Background(), "una-clave-larga")
	ok, status, code, _ = svc.Check(r)
	if ok || status != 401 || code != "login_required" {
		t.Fatalf("sin sesión: %v %d %s", ok, status, code)
	}
}

func TestLocalBypassNuncaDejaFueraSinClave(t *testing.T) {
	svc, _ := newSvc(t)
	svc.cfg = fakeCfg{days: 30, bypass: false}
	if !svc.LocalBypass() {
		t.Fatal("sin clave, localhost siempre es admin aunque el bypass esté desactivado")
	}
	svc.SetPassword(context.Background(), "una-clave-larga")
	if svc.LocalBypass() {
		t.Fatal("con clave y bypass desactivado, localhost necesita sesión")
	}
}

func TestDeviceName(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit Chrome/120 Safari/537": "Chrome en Windows",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17) AppleWebKit Safari/604":   "Safari en iPhone",
		"Mozilla/5.0 (Linux; Android 14) Chrome/120 Mobile Safari/537":    "Chrome en Android",
		"": "Navegador",
	}
	for ua, want := range cases {
		if got := DeviceName(ua); got != want {
			t.Errorf("%q → %q, quería %q", ua, got, want)
		}
	}
}

func TestTruncatePorRunas(t *testing.T) {
	got := truncate(strings.Repeat("ñ", 400), 300)
	if n := len([]rune(got)); n != 300 {
		t.Fatalf("runas = %d", n)
	}
	if !utf8.ValidString(got) {
		t.Fatal("no debe partir un carácter multibyte")
	}
	if truncate("corto", 300) != "corto" {
		t.Fatal("lo corto no cambia")
	}
}
