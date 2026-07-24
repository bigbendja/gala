package stream

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newManagerForTest(t *testing.T, ctx context.Context, ffmpegPath string) *Manager {
	t.Helper()
	return NewManager(ctx, ffmpegPath, t.TempDir(), discardLogger())
}

// TestStartSessionCreatesFilesAndTracksSession cubre la creación básica de
// una sesión (Brief M4 §8: "Sesiones HLS: creación").
func TestStartSessionCreatesFilesAndTracksSession(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 0)
	ctx := context.Background()
	m := newManagerForTest(t, ctx, ffmpeg)

	sess, err := m.StartSession(context.Background(), 1, "video.mkv", 0, false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if sess.SourceID != 1 {
		t.Errorf("SourceID = %d, esperado 1", sess.SourceID)
	}
	if _, err := os.Stat(filepath.Join(sess.Dir, "index.m3u8")); err != nil {
		t.Errorf("no se creó index.m3u8: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sess.Dir, "seg00000.ts")); err != nil {
		t.Errorf("no se creó el segmento: %v", err)
	}

	got, ok := m.Session(sess.ID)
	if !ok || got.ID != sess.ID {
		t.Fatalf("Session(%q) = %+v, %v", sess.ID, got, ok)
	}
}

// TestStartSessionSeekPassesStartSeconds comprueba que el -ss llega a
// ffmpeg como argumento (nuestro ffmpeg falso no lo interpreta, pero el
// propio StartSession lo expone en Session.StartAt).
func TestStartSessionSeekPassesStartSeconds(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 0)
	m := newManagerForTest(t, context.Background(), ffmpeg)

	sess, err := m.StartSession(context.Background(), 1, "video.mkv", 123.5, false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if sess.StartAt != 123.5 {
		t.Errorf("StartAt = %v, esperado 123.5", sess.StartAt)
	}
}

// TestStartSessionPerSourceLimitClosesOldest: al superar
// maxSessionsPerSource para la MISMA fuente (p. ej. seeks sucesivos), la
// sesión más antigua se cierra (Brief §4: "mantener como máximo 2 sesiones
// por source; limpiar la anterior").
func TestStartSessionPerSourceLimitClosesOldest(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 200)
	m := newManagerForTest(t, context.Background(), ffmpeg)

	var sessions []*Session
	for i := 0; i < maxSessionsPerSource; i++ {
		sess, err := m.StartSession(context.Background(), 42, "video.mkv", float64(i*10), false)
		if err != nil {
			t.Fatalf("StartSession #%d: %v", i, err)
		}
		sessions = append(sessions, sess)
		time.Sleep(5 * time.Millisecond) // asegurar orden de creación distinguible
	}

	// Una sesión más para la misma fuente: debe cerrar sessions[0] (la más vieja).
	newest, err := m.StartSession(context.Background(), 42, "video.mkv", 999, false)
	if err != nil {
		t.Fatalf("StartSession (extra): %v", err)
	}

	if _, ok := m.Session(sessions[0].ID); ok {
		t.Error("la sesión más antigua sigue registrada; debía haberse cerrado")
	}
	waitNoDir(t, sessions[0].Dir)

	count := 0
	for _, s := range sessions[1:] {
		if _, ok := m.Session(s.ID); ok {
			count++
		}
	}
	if _, ok := m.Session(newest.ID); ok {
		count++
	}
	if count != maxSessionsPerSource {
		t.Errorf("sesiones vivas de la fuente = %d, esperado %d (maxSessionsPerSource)", count, maxSessionsPerSource)
	}
}

// TestStartSessionGlobalLimitReturns503Equivalent cubre el límite GLOBAL
// (Brief §4/§8: "límite concurrente (503)"); aquí se prueba a nivel de
// Manager (el handler HTTP traduce ErrTooManySessions a 503, ver
// internal/api/stream_test.go).
func TestStartSessionGlobalLimitReturns503Equivalent(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 300)
	m := newManagerForTest(t, context.Background(), ffmpeg)

	// maxGlobalSessions sesiones en fuentes DISTINTAS: el límite por fuente
	// no debería interferir.
	for i := int64(1); i <= maxGlobalSessions; i++ {
		if _, err := m.StartSession(context.Background(), i, "video.mkv", 0, false); err != nil {
			t.Fatalf("StartSession fuente %d: %v", i, err)
		}
	}

	_, err := m.StartSession(context.Background(), maxGlobalSessions+1, "video.mkv", 0, false)
	if !errors.Is(err, ErrTooManySessions) {
		t.Fatalf("err = %v, esperado ErrTooManySessions", err)
	}
}

// TestStartSessionCancelledByRequestContext cubre "cancelación... contexto
// del request" (Brief §4/§8): si quien pidió arrancar la sesión corta antes
// de que la playlist esté lista, la sesión recién creada se aborta y se
// limpia, sin quedar huérfana.
func TestStartSessionCancelledByRequestContext(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 2000, 0) // tarda 2s en escribir la playlist
	m := newManagerForTest(t, context.Background(), ffmpeg)

	reqCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := m.StartSession(reqCtx, 1, "video.mkv", 0, false)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("err = nil, esperado error por contexto cancelado")
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("StartSession tardó %v; esperado cortarse mucho antes de los 2s del proceso falso (contexto cancelado a los 250ms)", elapsed)
	}

	m.mu.Lock()
	n := len(m.sessions)
	m.mu.Unlock()
	if n != 0 {
		t.Errorf("sesiones registradas tras cancelar = %d, esperado 0 (limpieza tras cancelación)", n)
	}
}

// TestCleanupOrphansRemovesInactiveSessions cubre el barrido de huérfanas
// EN MEMORIA por inactividad (Brief §4/§8).
func TestCleanupOrphansRemovesInactiveSessions(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 0)
	m := newManagerForTest(t, context.Background(), ffmpeg)

	sess, err := m.StartSession(context.Background(), 1, "video.mkv", 0, false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	// Forzar que parezca vieja sin esperar de verdad orphanMaxAge.
	m.mu.Lock()
	m.sessions[sess.ID].lastAccess = time.Now().Add(-1 * time.Hour)
	m.mu.Unlock()

	m.CleanupOrphans(30 * time.Minute)

	if _, ok := m.Session(sess.ID); ok {
		t.Error("la sesión inactiva sigue registrada tras CleanupOrphans")
	}
	waitNoDir(t, sess.Dir)
}

// TestCleanupOrphansKeepsActiveSessions: una sesión con actividad reciente
// no debe tocarse.
func TestCleanupOrphansKeepsActiveSessions(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 0)
	m := newManagerForTest(t, context.Background(), ffmpeg)

	sess, err := m.StartSession(context.Background(), 1, "video.mkv", 0, false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	m.CleanupOrphans(30 * time.Minute)
	if _, ok := m.Session(sess.ID); !ok {
		t.Error("una sesión activa fue eliminada por CleanupOrphans")
	}
}

// TestCleanupOrphanDirsRemovesUntrackedOldDirs cubre el barrido de
// directorios huérfanos EN DISCO (restos de una sesión de GALA anterior que
// no se cerró limpiamente; Brief §4).
func TestCleanupOrphanDirsRemovesUntrackedOldDirs(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 0)
	m := newManagerForTest(t, context.Background(), ffmpeg)

	oldDir := filepath.Join(m.cacheDir, "restos-de-antes")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(oldDir, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	freshDir := filepath.Join(m.cacheDir, "recien-creado")
	if err := os.MkdirAll(freshDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	m.CleanupOrphanDirs(30 * time.Minute)

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("el directorio huérfano viejo no se borró")
	}
	if _, err := os.Stat(freshDir); err != nil {
		t.Error("un directorio reciente (no huérfano de verdad) se borró por error")
	}
}

// TestServeFileServesPlaylistAndRejectsUnknownSession.
func TestServeFileServesPlaylistAndRejectsUnknownSession(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 0)
	m := newManagerForTest(t, context.Background(), ffmpeg)
	sess, err := m.StartSession(context.Background(), 1, "video.mkv", 0, false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	if err := m.ServeFile(w, req, sess.ID, "index.m3u8"); err != nil {
		t.Fatalf("ServeFile: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/vnd.apple.mpegurl" {
		t.Errorf("Content-Type = %q", w.Header().Get("Content-Type"))
	}

	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	w2 := httptest.NewRecorder()
	err2 := m.ServeFile(w2, req2, "sesion-que-no-existe", "index.m3u8")
	if !errors.Is(err2, ErrSessionNotFound) {
		t.Fatalf("err = %v, esperado ErrSessionNotFound", err2)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/x", nil)
	w3 := httptest.NewRecorder()
	if err := m.ServeFile(w3, req3, sess.ID, "../../etc/passwd"); err == nil {
		t.Fatal("ServeFile con nombre fuera del patrón permitido debería fallar")
	}
}

// TestStopSessionRemovesDirAndTracking.
func TestStopSessionRemovesDirAndTracking(t *testing.T) {
	ffmpeg := buildFakeFFmpeg(t, 0, 0)
	m := newManagerForTest(t, context.Background(), ffmpeg)
	sess, err := m.StartSession(context.Background(), 1, "video.mkv", 0, false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	m.StopSession(sess.ID)

	if _, ok := m.Session(sess.ID); ok {
		t.Error("la sesión sigue registrada tras StopSession")
	}
	waitNoDir(t, sess.Dir)

	// Segunda llamada (sesión ya limpiada): no-op, sin pánico.
	m.StopSession(sess.ID)
}

// waitNoDir espera (con un margen corto) a que dir deje de existir: el
// borrado ocurre en una goroutine (removeLocked) y no debe bloquear al
// llamante de StartSession.
func waitNoDir(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s no se borró a tiempo", dir)
}
