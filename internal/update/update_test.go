package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/store"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// --- marcador y resultado ---------------------------------------------------

func TestMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := ReadMarker(dir); ok || err != nil {
		t.Fatalf("sin marcador: ok=%v err=%v", ok, err)
	}
	want := Marker{State: StateSwapped, From: "v0.9.0", New: "v0.10.0", Attempts: 2, MigracionesNuevas: true}
	if err := WriteMarker(dir, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadMarker(dir)
	if err != nil || !ok || got != want {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
	// Sin restos de ficheros temporales.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("debe quedar solo el marcador: %v", entries)
	}
	// Un valor con saltos de línea no rompe el formato.
	if err := WriteMarker(dir, Marker{State: "swapped\nattempts=99", From: "a", New: "b"}); err != nil {
		t.Fatal(err)
	}
	if m, _, _ := ReadMarker(dir); m.Attempts != 0 {
		t.Fatalf("inyección en el marcador: %+v", m)
	}
}

func TestResultRoundTripYDescartar(t *testing.T) {
	dir := t.TempDir()
	if _, ok, _ := ReadResult(dir); ok {
		t.Fatal("sin resultado")
	}
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	WriteResult(dir, Result{Status: ResultRolledBk, From: "v0.9.0", To: "v0.10.0", Reason: "falló", At: at})
	r, ok, _ := ReadResult(dir)
	if !ok || r.Status != ResultRolledBk || r.From != "v0.9.0" || r.To != "v0.10.0" || r.Reason != "falló" || !r.At.Equal(at) {
		t.Fatalf("%+v", r)
	}
	if err := ClearResult(dir); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := ReadResult(dir); ok {
		t.Fatal("descartado")
	}
	if err := ClearResult(dir); err != nil {
		t.Fatalf("descartar dos veces no es un error: %v", err)
	}
}

// --- versiones --------------------------------------------------------------

func TestVersiones(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.9.0", "v0.10.0", -1},
		{"v0.10.0", "v0.9.0", 1},
		{"v1.0.0", "1.0.0", 0},
		{"v0.9.0-3-gabc1234-dirty", "v0.9.0", 0}, // compilación intermedia = versión base
		{"0.1.0-dev", "v0.9.0", -1},
		{"desconocida", "v0.0.1", -1},
		{"v0.9.10", "v0.9.9", 1},
	}
	for _, c := range cases {
		if got := ParseVersion(c.a).Compare(ParseVersion(c.b)); got != c.want {
			t.Errorf("%s vs %s = %d, quería %d", c.a, c.b, got, c.want)
		}
	}
	if ParseVersion("v1.2").Valid || ParseVersion("x.y.z").Valid || ParseVersion("").Valid {
		t.Error("versiones mal formadas no deben ser válidas")
	}
}

// --- comprobar --------------------------------------------------------------

type fakeGitHub struct {
	srv      *httptest.Server
	binary   []byte
	sums     string
	hits     atomic.Int64
	release  ghRelease
	failAPI  atomic.Bool
	noSums   bool
	asset    string
	bodyOver func() []byte // sustituye el binario servido (para probar corrupción)
}

func newFakeGitHub(t *testing.T, tag, goos, goarch string, binary []byte) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{binary: binary, asset: AssetName(goos, goarch)}
	sum := sha256.Sum256(binary)
	f.sums = hex.EncodeToString(sum[:]) + "  " + f.asset + "\n" +
		strings.Repeat("0", 64) + "  gala-otra-plataforma\n"
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/bigbendja/gala/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if f.failAPI.Load() {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		rel := f.release
		rel.TagName = tag
		rel.Body = "- Cuenta de administrador\n- Mejoras"
		rel.HTMLURL = f.srv.URL + "/release"
		rel.Assets = []ghAsset{{Name: f.asset, URL: f.srv.URL + "/dl/" + f.asset, Size: int64(len(binary))}}
		if !f.noSums {
			rel.Assets = append(rel.Assets, ghAsset{Name: sumsAsset, URL: f.srv.URL + "/dl/" + sumsAsset})
		}
		_ = json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/dl/"+sumsAsset, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, f.sums) })
	mux.HandleFunc("/dl/"+f.asset, func(w http.ResponseWriter, r *http.Request) {
		b := f.binary
		if f.bodyOver != nil {
			b = f.bodyOver()
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(b)))
		w.Write(b)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) checker(current, cacheFile string) *Checker {
	c := NewChecker("linux", "amd64", current, cacheFile)
	c.APIBase = f.srv.URL
	c.AllowHost = func(string) bool { return true } // httptest es http://127.0.0.1
	return c
}

func TestCheckOfreceVersionNuevaConChecksum(t *testing.T) {
	f := newFakeGitHub(t, "v0.10.0", "linux", "amd64", []byte("binario-nuevo"))
	c := f.checker("v0.9.0", "")
	info, err := c.Check(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !info.UpdateAvailable || info.Latest != "v0.10.0" || info.Current != "v0.9.0" {
		t.Fatalf("%+v", info)
	}
	sum := sha256.Sum256([]byte("binario-nuevo"))
	if info.Checksum != hex.EncodeToString(sum[:]) || info.AssetName != "gala-linux-amd64" || info.AssetURL == "" {
		t.Fatalf("asset/checksum: %+v", info)
	}
	if !strings.Contains(info.Notes, "Cuenta de administrador") {
		t.Errorf("notas: %q", info.Notes)
	}
}

func TestCheckVersionIgualNoOfreceActualizar(t *testing.T) {
	f := newFakeGitHub(t, "v0.9.0", "linux", "amd64", []byte("x"))
	info, err := f.checker("v0.9.0", "").Check(context.Background(), false)
	if err != nil || info.UpdateAvailable {
		t.Fatalf("misma versión: %+v err=%v", info, err)
	}
	info, _ = f.checker("v0.10.0", "").Check(context.Background(), false)
	if info.UpdateAvailable {
		t.Fatal("nunca ofrece una versión más vieja (sin downgrade)")
	}
}

func TestCheckCacheYForzado(t *testing.T) {
	f := newFakeGitHub(t, "v0.10.0", "linux", "amd64", []byte("x"))
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	c := f.checker("v0.9.0", "")
	c.Now = func() time.Time { return now }
	c.Check(context.Background(), false)
	c.Check(context.Background(), false)
	if f.hits.Load() != 1 {
		t.Fatalf("la caché de 24 h debe evitar consultas: %d", f.hits.Load())
	}
	c.Check(context.Background(), true) // forzada pero <1 min: no machaca GitHub
	if f.hits.Load() != 1 {
		t.Fatalf("forzar dos veces seguidas no debe consultar: %d", f.hits.Load())
	}
	now = now.Add(2 * time.Minute)
	c.Check(context.Background(), true)
	if f.hits.Load() != 2 {
		t.Fatalf("forzada pasado el minuto sí consulta: %d", f.hits.Load())
	}
	now = now.Add(25 * time.Hour)
	c.Check(context.Background(), false)
	if f.hits.Load() != 3 {
		t.Fatalf("caducada la caché de 24 h se vuelve a consultar: %d", f.hits.Load())
	}
}

func TestCheckSinRedNoRompeYConservaLaCache(t *testing.T) {
	f := newFakeGitHub(t, "v0.10.0", "linux", "amd64", []byte("x"))
	cache := filepath.Join(t.TempDir(), "update-check.json")
	c := f.checker("v0.9.0", cache)
	if _, err := c.Check(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	// Reinicio: la caché se recupera de disco.
	c2 := f.checker("v0.9.0", cache)
	if info, ok := c2.Cached(); !ok || !info.UpdateAvailable {
		t.Fatalf("la caché debe sobrevivir al reinicio: %+v ok=%v", info, ok)
	}
	f.failAPI.Store(true)
	now := time.Now().Add(48 * time.Hour)
	c2.Now = func() time.Time { return now }
	_, err := c2.Check(context.Background(), false)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("GitHub caído debe ser ErrOffline: %v", err)
	}
	f.srv.Close() // sin red de verdad
	c3 := f.checker("v0.9.0", "")
	if _, err := c3.Check(context.Background(), false); !errors.Is(err, ErrOffline) {
		t.Fatalf("sin red: %v", err)
	}
}

func TestCheckIgnoraBorradoresYPrereleases(t *testing.T) {
	f := newFakeGitHub(t, "v0.10.0", "linux", "amd64", []byte("x"))
	f.release.Prerelease = true
	if _, err := f.checker("v0.9.0", "").Check(context.Background(), false); !errors.Is(err, ErrOffline) {
		t.Fatalf("prerelease: %v", err)
	}
}

func TestCheckSinChecksumsNoOfreceAplicar(t *testing.T) {
	f := newFakeGitHub(t, "v0.10.0", "linux", "amd64", []byte("x"))
	f.noSums = true
	info, err := f.checker("v0.9.0", "").Check(context.Background(), false)
	if err != nil || !info.UpdateAvailable || info.AssetURL != "" || info.Checksum != "" {
		t.Fatalf("se anuncia pero no se puede aplicar: %+v err=%v", info, err)
	}
}

func TestCheckRechazaHostsQueNoSonGitHub(t *testing.T) {
	f := newFakeGitHub(t, "v0.10.0", "linux", "amd64", []byte("x"))
	c := f.checker("v0.9.0", "")
	c.AllowHost = nil // política real: https + hosts de GitHub
	if _, err := c.Check(context.Background(), false); err == nil || errors.Is(err, ErrOffline) {
		t.Fatalf("un asset en http://127.0.0.1 debe rechazarse: %v", err)
	}
	for host, want := range map[string]bool{
		"github.com": true, "objects.githubusercontent.com": true,
		"release-assets.githubusercontent.com": true, "evil.com": false,
		"githubusercontent.com.evil.com": false, "notgithub.com": false,
	} {
		if HostAllowed(host) != want {
			t.Errorf("HostAllowed(%q) = %v", host, !want)
		}
	}
}

func TestParseChecksum(t *testing.T) {
	h := strings.Repeat("ab", 32)
	sums := h + "  gala-linux-amd64\n" + strings.Repeat("0", 64) + " *gala-windows-amd64.exe\nbasura\n"
	if ParseChecksum(sums, "gala-linux-amd64") != h {
		t.Error("linux")
	}
	if ParseChecksum(sums, "gala-windows-amd64.exe") != strings.Repeat("0", 64) {
		t.Error("windows con asterisco")
	}
	if ParseChecksum(sums, "gala-darwin-arm64") != "" {
		t.Error("ausente")
	}
}

// --- instalar ----------------------------------------------------------------

type installFixture struct {
	dir     string
	exe     string
	gh      *fakeGitHub
	inst    *Installer
	exited  atomic.Int64
	playing atomic.Int64
}

func newInstallFixture(t *testing.T) *installFixture {
	t.Helper()
	fx := &installFixture{dir: t.TempDir()}
	fx.exe = filepath.Join(fx.dir, "gala")
	if err := os.WriteFile(fx.exe, []byte("binario-viejo"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(fx.dir, "gala.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fx.gh = newFakeGitHub(t, "v0.10.0", "linux", "amd64", []byte("binario-nuevo"))
	checker := fx.gh.checker("v0.9.0", "")
	if _, err := checker.Check(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	fx.inst = &Installer{
		Checker: checker, Client: &http.Client{}, ExePath: fx.exe, DB: st.DB, Log: discardLog(),
		Supervised:     true,
		PlaybackActive: func() int { return int(fx.playing.Load()) },
		RequestExit:    func() { fx.exited.Add(1) },
		RunVersion: func(_ context.Context, bin string) (string, error) {
			return "GALA v0.10.0 (commit abc, compilado hoy)", nil
		},
	}
	return fx
}

func (fx *installFixture) wait(t *testing.T, phases ...Phase) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := fx.inst.Status()
		for _, p := range phases {
			if s.Phase == p {
				return s
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout esperando %v; estado %+v", phases, fx.inst.Status())
	return Status{}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyCaminoFeliz(t *testing.T) {
	old := exitDelayForTests(t)
	defer old()
	fx := newInstallFixture(t)
	if err := fx.inst.Start(false); err != nil {
		t.Fatal(err)
	}
	fx.wait(t, PhaseRestarting)

	if read(t, fx.exe) != "binario-nuevo" {
		t.Error("el binario nuevo debe estar en su sitio")
	}
	if read(t, fx.exe+PrevSuffix) != "binario-viejo" {
		t.Error("el viejo debe quedar como .prev")
	}
	if _, err := os.Stat(fx.exe + NewSuffix); !os.IsNotExist(err) {
		t.Error("no debe quedar .new")
	}
	m, ok, _ := ReadMarker(fx.dir)
	if !ok || m.State != StateSwapped || m.From != "v0.9.0" || m.New != "v0.10.0" || m.Attempts != 0 {
		t.Fatalf("marcador: %+v ok=%v", m, ok)
	}
	if fi, err := os.Stat(filepath.Join(fx.dir, DBBackupNew)); err != nil || fi.Size() == 0 {
		t.Errorf("copia de la BD: %v", err)
	}
	waitFor(t, func() bool { return fx.exited.Load() == 1 }, "salida ordenada")
}

// exitDelayForTests no cambia nada hoy (el retraso es una constante de 1 s y
// los tests esperan); existe para dejar claro dónde se ajustaría.
func exitDelayForTests(t *testing.T) func() { return func() {} }

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", what)
}

func TestApplyChecksumIncorrectoNoTocaNada(t *testing.T) {
	fx := newInstallFixture(t)
	fx.gh.bodyOver = func() []byte { return []byte("binario-CORRUPTO") }
	if err := fx.inst.Start(false); err != nil {
		t.Fatal(err)
	}
	s := fx.wait(t, PhaseError)
	if !strings.Contains(s.Error, "checksum") {
		t.Errorf("mensaje: %q", s.Error)
	}
	assertUntouched(t, fx)
}

func TestApplyBinarioQueNoEsLaVersionEsperada(t *testing.T) {
	fx := newInstallFixture(t)
	fx.inst.RunVersion = func(context.Context, string) (string, error) { return "GALA v0.8.0", nil }
	fx.inst.Start(false)
	fx.wait(t, PhaseError)
	assertUntouched(t, fx)
}

func TestApplyBinarioQueNoEjecuta(t *testing.T) {
	fx := newInstallFixture(t)
	fx.inst.RunVersion = func(context.Context, string) (string, error) { return "", errors.New("exec format error") }
	fx.inst.Start(false)
	s := fx.wait(t, PhaseError)
	if !strings.Contains(s.Error, "no se puede ejecutar") {
		t.Errorf("%q", s.Error)
	}
	assertUntouched(t, fx)
}

func assertUntouched(t *testing.T, fx *installFixture) {
	t.Helper()
	if read(t, fx.exe) != "binario-viejo" {
		t.Error("el binario actual no debe cambiar")
	}
	for _, p := range []string{fx.exe + NewSuffix, fx.exe + PrevSuffix, filepath.Join(fx.dir, PendingFile)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("no debe existir %s", filepath.Base(p))
		}
	}
	if fx.exited.Load() != 0 {
		t.Error("no debe salir")
	}
}

func TestApplyNoActualizaEnMitadDeUnaReproduccion(t *testing.T) {
	fx := newInstallFixture(t)
	fx.playing.Store(2)
	err := fx.inst.Start(false)
	var pb *PlaybackActiveError
	if !errors.As(err, &pb) || pb.Count != 2 {
		t.Fatalf("debe avisar: %v", err)
	}
	assertUntouched(t, fx)
	if err := fx.inst.Start(true); err != nil { // «Actualizar igualmente»
		t.Fatalf("con force sí: %v", err)
	}
	fx.wait(t, PhaseRestarting)
}

func TestApplyExigeSupervisorYActualizacionDisponible(t *testing.T) {
	fx := newInstallFixture(t)
	fx.inst.Supervised = false
	if err := fx.inst.Start(false); !errors.Is(err, ErrNotSupervised) {
		t.Fatalf("sin systemd: %v", err)
	}
	if ok, reason := fx.inst.CanApply(); ok || reason == "" {
		t.Fatal("CanApply debe explicar por qué no")
	}
	fx.inst.Supervised = true
	fx.inst.Checker.Current = "v0.10.0"
	if err := fx.inst.Start(false); !errors.Is(err, ErrNoUpdate) {
		t.Fatalf("sin versión nueva: %v", err)
	}
}

func TestApplyNoSeEjecutaDosVeces(t *testing.T) {
	fx := newInstallFixture(t)
	release := make(chan struct{})
	fx.inst.RunVersion = func(context.Context, string) (string, error) {
		<-release
		return "GALA v0.10.0", nil
	}
	if err := fx.inst.Start(false); err != nil {
		t.Fatal(err)
	}
	fx.wait(t, PhaseVerifying)
	if err := fx.inst.Start(false); !errors.Is(err, ErrBusy) {
		t.Fatalf("segunda a la vez: %v", err)
	}
	close(release)
	fx.wait(t, PhaseRestarting)
}

// --- confirmar el arranque -----------------------------------------------------

func healthServer(t *testing.T, ok *atomic.Bool) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok.Load() {
			io.WriteString(w, `{"status":"ok"}`)
			return
		}
		http.Error(w, "no", http.StatusServiceUnavailable)
	}))
	t.Cleanup(s.Close)
	return s
}

var fastBoot = BootTiming{Health: 300 * time.Millisecond, Stable: 100 * time.Millisecond, Poll: 20 * time.Millisecond}

func TestConfirmBootConfirmaYPromueveLaCopia(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "gala")
	os.WriteFile(exe, []byte("nuevo"), 0o755)
	os.WriteFile(exe+PrevSuffix, []byte("viejo"), 0o755)
	os.WriteFile(filepath.Join(dir, DBBackupFile), []byte("copia de la actualización anterior"), 0o644)
	os.WriteFile(filepath.Join(dir, DBBackupNew), []byte("copia de esta"), 0o644)
	WriteMarker(dir, Marker{State: StateSwapped, From: "v0.9.0", New: "v0.10.0", Attempts: 1})
	var ok atomic.Bool
	ok.Store(true)
	srv := healthServer(t, &ok)

	var failed atomic.Bool
	ConfirmBoot(context.Background(), exe, srv.URL, "v0.10.0", discardLog(), func() { failed.Store(true) }, fastBoot)

	if failed.Load() {
		t.Fatal("no debe fallar")
	}
	if _, present, _ := ReadMarker(dir); present {
		t.Error("el marcador debe borrarse")
	}
	r, present, _ := ReadResult(dir)
	if !present || r.Status != ResultOK || r.From != "v0.9.0" || r.To != "v0.10.0" {
		t.Errorf("resultado: %+v", r)
	}
	if read(t, filepath.Join(dir, DBBackupFile)) != "copia de esta" {
		t.Error("la copia de esta actualización sustituye a la anterior solo ahora, tras el éxito")
	}
	if _, err := os.Stat(filepath.Join(dir, DBBackupNew)); !os.IsNotExist(err) {
		t.Error("el .new se promueve (renombra)")
	}
	if read(t, exe+PrevSuffix) != "viejo" {
		t.Error("gala.prev se conserva tras el éxito (rollback manual)")
	}
}

func TestConfirmBootSinSaludSaleConError(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "gala")
	os.WriteFile(exe, []byte("nuevo"), 0o755)
	os.WriteFile(exe+PrevSuffix, []byte("viejo"), 0o755)
	os.WriteFile(filepath.Join(dir, DBBackupNew), []byte("copia"), 0o644)
	WriteMarker(dir, Marker{State: StateSwapped, From: "v0.9.0", New: "v0.10.0", Attempts: 1})
	var ok atomic.Bool // nunca sano
	srv := healthServer(t, &ok)
	var failed atomic.Bool
	ConfirmBoot(context.Background(), exe, srv.URL, "v0.10.0", discardLog(), func() { failed.Store(true) }, fastBoot)
	if !failed.Load() {
		t.Fatal("debe pedir salir con error")
	}
	if _, present, _ := ReadMarker(dir); !present {
		t.Error("el marcador sigue: lo resuelve el preflight")
	}
	if _, present, _ := ReadResult(dir); present {
		t.Error("no se anuncia éxito")
	}
	if _, err := os.Stat(filepath.Join(dir, DBBackupNew)); err != nil {
		t.Error("la copia sigue en .new para un posible rollback")
	}
}

func TestConfirmBootSinMarcadorNoHaceNada(t *testing.T) {
	dir := t.TempDir()
	var failed atomic.Bool
	ConfirmBoot(context.Background(), filepath.Join(dir, "gala"), "http://127.0.0.1:1/healthz", "v1", discardLog(),
		func() { failed.Store(true) }, fastBoot)
	if failed.Load() {
		t.Fatal("sin actualización pendiente no se toca nada")
	}
}

func TestConfirmBootLimpiaUnMarcadorHuerfano(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "gala")
	os.WriteFile(exe, []byte("viejo"), 0o755)
	os.WriteFile(exe+NewSuffix, []byte("descargado"), 0o755)
	WriteMarker(dir, Marker{State: StatePreparing, From: "v0.9.0", New: "v0.10.0"})
	var failed atomic.Bool
	ConfirmBoot(context.Background(), exe, "http://127.0.0.1:1/healthz", "v0.9.0", discardLog(), func() { failed.Store(true) }, fastBoot)
	if failed.Load() {
		t.Fatal("un intercambio que no ocurrió no es un fallo")
	}
	if _, present, _ := ReadMarker(dir); present {
		t.Error("marcador huérfano sin limpiar")
	}
	if _, err := os.Stat(exe + NewSuffix); !os.IsNotExist(err) {
		t.Error(".new sin limpiar")
	}
}

func TestConfirmBootPreparingConIntercambioHechoSeTrataComoSwapped(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "gala")
	os.WriteFile(exe, []byte("nuevo"), 0o755)
	os.WriteFile(exe+PrevSuffix, []byte("viejo"), 0o755) // sin .new: ya se intercambió
	WriteMarker(dir, Marker{State: StatePreparing, From: "v0.9.0", New: "v0.10.0"})
	var ok atomic.Bool
	ok.Store(true)
	srv := healthServer(t, &ok)
	ConfirmBoot(context.Background(), exe, srv.URL, "v0.10.0", discardLog(), func() { t.Error("no debe fallar") }, fastBoot)
	if r, present, _ := ReadResult(dir); !present || r.Status != ResultOK {
		t.Fatalf("debe confirmarse: %+v", r)
	}
}

func TestActividadDeStreaming(t *testing.T) {
	var a Activity
	release := make(chan struct{})
	started := make(chan struct{})
	h := a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/stream/1" {
			close(started)
			<-release
		}
	}))
	if a.Active() != 0 {
		t.Fatal("sin actividad")
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/items", nil))
	if a.Active() != 0 {
		t.Fatal("solo cuenta el streaming")
	}
	go h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/stream/1", nil))
	<-started
	if a.Active() != 1 {
		t.Fatalf("petición en vuelo: %d", a.Active())
	}
	close(release)
	waitFor(t, func() bool { return a.inflight.Load() == 0 }, "fin de la petición")
	if a.Active() != 1 {
		t.Fatal("recién terminada cuenta como actividad reciente")
	}
	a.last.Store(time.Now().Add(-time.Minute).UnixNano())
	if a.Active() != 0 {
		t.Fatal("pasados 30 s ya no")
	}
}

func TestApplySinEspacioLibreNoTocaNadaYLoDiceEnEspanol(t *testing.T) {
	fx := newInstallFixture(t)
	fx.inst.FreeSpace = func(string) (uint64, error) { return 5 << 20, nil } // 5 MB libres
	// El binario de la release es 13 bytes; fingimos que pesa 20 MB.
	info, _ := fx.inst.Checker.Cached()
	info.AssetSize = 20 << 20
	fx.inst.Checker.cached = &info
	if err := fx.inst.Start(false); err != nil {
		t.Fatal(err)
	}
	s := fx.wait(t, PhaseError)
	if !strings.Contains(s.Error, "no hay espacio suficiente") || !strings.Contains(s.Error, "no se ha cambiado nada") {
		t.Errorf("mensaje: %q", s.Error)
	}
	assertUntouched(t, fx)
}

func TestApplyEspacioSuficienteContinua(t *testing.T) {
	fx := newInstallFixture(t)
	fx.inst.FreeSpace = func(string) (uint64, error) { return 1 << 30, nil }
	if err := fx.inst.Start(false); err != nil {
		t.Fatal(err)
	}
	fx.wait(t, PhaseRestarting)
}

func TestDiscoLlenoSeReconoce(t *testing.T) {
	if !isDiskFull(&os.PathError{Op: "write", Path: "x", Err: syscall.ENOSPC}) {
		t.Error("ENOSPC es disco lleno")
	}
	if isDiskFull(errors.New("otra cosa")) || isDiskFull(nil) {
		t.Error("lo demás no")
	}
	if freeBytesOK, err := freeBytes(t.TempDir()); err != nil || freeBytesOK == 0 {
		t.Errorf("la consulta real de espacio libre debe funcionar: %d %v", freeBytesOK, err)
	}
}
