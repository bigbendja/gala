package media

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func touchExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("fake"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// TestLocatePrefersFfmpegSubfolder cubre el orden de búsqueda del Brief M4
// §1: primero la carpeta "ffmpeg/" junto al ejecutable.
func TestLocatePrefersFfmpegSubfolder(t *testing.T) {
	dir := t.TempDir()
	inSub := filepath.Join(dir, "ffmpeg", exeName("ffmpeg"))
	inRoot := filepath.Join(dir, exeName("ffmpeg"))
	touchExecutable(t, inSub)
	touchExecutable(t, inRoot)
	// ffprobe solo junto al ejecutable, para comprobar que cada binario se
	// busca de forma independiente.
	touchExecutable(t, filepath.Join(dir, exeName("ffprobe")))

	f := Locate(dir, discardLogger())
	if f.FFmpegPath != inSub {
		t.Errorf("FFmpegPath = %q, esperado la carpeta ffmpeg/ (%q)", f.FFmpegPath, inSub)
	}
	if !f.Available() {
		t.Error("Available() = false con ambos binarios localizados")
	}
}

// TestLocateFallsBackToExeDir: sin carpeta ffmpeg/, se usa el binario junto
// al ejecutable (Brief M4 §1, paso 2).
func TestLocateFallsBackToExeDir(t *testing.T) {
	dir := t.TempDir()
	inRoot := filepath.Join(dir, exeName("ffmpeg"))
	touchExecutable(t, inRoot)
	touchExecutable(t, filepath.Join(dir, exeName("ffprobe")))

	f := Locate(dir, discardLogger())
	if f.FFmpegPath != inRoot {
		t.Errorf("FFmpegPath = %q, esperado %q", f.FFmpegPath, inRoot)
	}
}

// TestLocateNotFoundNeverAborts cubre "nunca abortar el arranque": sin
// binarios en ningún sitio (y asumiendo que el PATH de prueba no los trae),
// Locate devuelve un FFmpeg no disponible, sin pánico ni error.
func TestLocateNotFoundNeverAborts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir) // PATH controlado: no debe colarse un ffmpeg real del sistema

	f := Locate(dir, discardLogger())
	if f == nil {
		t.Fatal("Locate devolvió nil")
	}
	if f.Available() {
		t.Errorf("Available() = true sin binarios en ningún sitio: FFmpeg=%q FFprobe=%q", f.FFmpegPath, f.FFprobePath)
	}
}

// TestFFmpegAvailableNilSafe: Available() sobre un *FFmpeg nil no debe
// entrar en pánico (los handlers de la API lo llaman incluso cuando el
// campo Server.FFmpeg es nil en tests que no ejercitan M4).
func TestFFmpegAvailableNilSafe(t *testing.T) {
	var f *FFmpeg
	if f.Available() {
		t.Error("Available() sobre nil = true, esperado false")
	}
}
