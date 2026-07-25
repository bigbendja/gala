package health

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
)

// fakeFileInfo implementa os.FileInfo mínimamente, para inyectar un stat
// falso y lento sin depender de I/O real (Brief M6 §6: "inyectar stat lento").
type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "x" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }

func TestLocalfsCheckerAliveNoVolume(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "peli.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("escribir archivo: %v", err)
	}
	c := &LocalfsChecker{Repo: newTestRepo(t)}
	got, detail := c.Check(context.Background(), catalog.Source{Locator: file})
	if got != catalog.HealthAlive {
		t.Fatalf("Check = %q (%q), esperado viva", got, detail)
	}
}

func TestLocalfsCheckerDeadFileDeleted(t *testing.T) {
	file := filepath.Join(t.TempDir(), "no-existe.mp4")
	c := &LocalfsChecker{Repo: newTestRepo(t)}
	got, detail := c.Check(context.Background(), catalog.Source{Locator: file})
	if got != catalog.HealthDead {
		t.Fatalf("Check = %q (%q), esperado caída", got, detail)
	}
	if detail == "" {
		t.Error("detail vacío, esperado un motivo legible")
	}
}

func TestLocalfsCheckerDeadIsDirectory(t *testing.T) {
	dir := t.TempDir()
	c := &LocalfsChecker{Repo: newTestRepo(t)}
	got, _ := c.Check(context.Background(), catalog.Source{Locator: dir})
	if got != catalog.HealthDead {
		t.Fatalf("Check(carpeta) = %q, esperado caída", got)
	}
}

func TestLocalfsCheckerDisconnectedVolumeNeverBecomesDead(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if err := repo.UpsertVolume(ctx, catalog.Volume{ID: "vol-1", Connected: false}, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("UpsertVolume: %v", err)
	}
	vol := "vol-1"
	c := &LocalfsChecker{Repo: repo}
	// Ruta que ni siquiera existe: si el checker mirase el disco antes que
	// el volumen, esto daría caída. Debe dar desconectada SIEMPRE que el
	// volumen esté ausente, sin importar el archivo (Brief M6 §1/§2).
	got, detail := c.Check(ctx, catalog.Source{Locator: "/no/existe.mp4", VolumeID: &vol})
	if got != catalog.HealthDisconnected {
		t.Fatalf("Check = %q (%q), esperado desconectada", got, detail)
	}
}

func TestLocalfsCheckerConnectedVolumeFallsBackToStat(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if err := repo.UpsertVolume(ctx, catalog.Volume{ID: "vol-1", Connected: true}, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("UpsertVolume: %v", err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "peli.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("escribir archivo: %v", err)
	}
	vol := "vol-1"
	c := &LocalfsChecker{Repo: repo}
	got, _ := c.Check(ctx, catalog.Source{Locator: file, VolumeID: &vol})
	if got != catalog.HealthAlive {
		t.Fatalf("Check (volumen conectado) = %q, esperado viva", got)
	}
}

func TestLocalfsCheckerUnknownVolumeAssumedConnected(t *testing.T) {
	repo := newTestRepo(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "peli.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("escribir archivo: %v", err)
	}
	vol := "nunca-visto"
	c := &LocalfsChecker{Repo: repo}
	got, _ := c.Check(context.Background(), catalog.Source{Locator: file, VolumeID: &vol})
	if got != catalog.HealthAlive {
		t.Fatalf("Check (volumen desconocido) = %q, esperado viva (sin evidencia de desconexión)", got)
	}
}

func TestLocalfsCheckerSlow(t *testing.T) {
	c := &LocalfsChecker{
		Repo: newTestRepo(t),
		Stat: func(name string) (os.FileInfo, error) {
			time.Sleep(20 * time.Millisecond)
			return fakeFileInfo{}, nil
		},
		SlowThreshold: 5 * time.Millisecond,
	}
	got, detail := c.Check(context.Background(), catalog.Source{Locator: "/x/peli.mp4"})
	if got != catalog.HealthSlow {
		t.Fatalf("Check = %q (%q), esperado lenta", got, detail)
	}
}

// TestLocalfsCheckerAliveDeadAliveTransition cubre las transiciones
// viva→caída (borrado) y caída→viva (reparación/reaparición) sobre la
// MISMA fuente, con el mismo checker (Brief M6 §6).
func TestLocalfsCheckerAliveDeadAliveTransition(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "peli.mp4")
	c := &LocalfsChecker{Repo: newTestRepo(t)}
	src := catalog.Source{Locator: file}
	ctx := context.Background()

	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("escribir archivo: %v", err)
	}
	if got, _ := c.Check(ctx, src); got != catalog.HealthAlive {
		t.Fatalf("primer chequeo = %q, esperado viva", got)
	}

	if err := os.Remove(file); err != nil {
		t.Fatalf("borrar archivo: %v", err)
	}
	if got, _ := c.Check(ctx, src); got != catalog.HealthDead {
		t.Fatalf("tras borrar = %q, esperado caída", got)
	}

	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("recrear archivo: %v", err)
	}
	if got, _ := c.Check(ctx, src); got != catalog.HealthAlive {
		t.Fatalf("tras recrear = %q, esperado viva de nuevo", got)
	}
}
