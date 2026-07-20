package localfs

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/store"
)

type fakePollerEnum struct{ vols []Volume }

func (f *fakePollerEnum) Volumes(ctx context.Context, folders []string) ([]Volume, error) {
	return f.vols, nil
}

func newPollerTestRepo(t *testing.T) *catalog.Repo {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return catalog.NewRepo(st.DB)
}

// TestPollerDisconnectAndReconnect simula, sin depender del SO, un volumen
// que desaparece y vuelve a aparecer (Brief M2 §Poller): las fuentes deben
// pasar a "desconectada" y de vuelta a "viva", con rescaneo en la reconexión.
func TestPollerDisconnectAndReconnect(t *testing.T) {
	ctx := context.Background()
	repo := newPollerTestRepo(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	folder := filepath.Join("E:", "Peliculas")
	vol := Volume{ID: "vol-1", Label: "USB", Roots: []string{folder}}
	enum := &fakePollerEnum{vols: []Volume{vol}}

	var rescanned [][]string
	p := &Poller{
		Enum:    enum,
		Folders: func() []string { return []string{folder} },
		Repo:    repo,
		Rescan:  func(fs []string) { rescanned = append(rescanned, fs) },
		Log:     log,
	}

	// Primer tick: volumen nuevo -> se registra y se dispara el rescaneo de
	// la carpeta registrada que vive en él (nunca el disco entero).
	if err := p.tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if len(rescanned) != 1 || len(rescanned[0]) != 1 || rescanned[0][0] != folder {
		t.Fatalf("rescaneo inicial inesperado: %v", rescanned)
	}

	itemID, err := repo.InsertItem(ctx, &catalog.Item{Kind: catalog.KindOther, Title: "x", Genres: []string{}}, "")
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}
	volID := vol.ID
	if _, err := repo.InsertSource(ctx, &catalog.Source{
		ItemID: itemID, Provider: ProviderID, Locator: filepath.Join(folder, "peli.mkv"),
		VolumeID: &volID, Health: catalog.HealthAlive,
	}, ""); err != nil {
		t.Fatalf("insert source: %v", err)
	}

	// Desconexión: el volumen deja de estar presente.
	enum.vols = nil
	if err := p.tick(ctx); err != nil {
		t.Fatalf("tick 2 (desconexión): %v", err)
	}

	it, err := repo.Get(ctx, itemID)
	if err != nil {
		t.Fatalf("get tras desconexión: %v", err)
	}
	if len(it.Sources) != 1 || it.Sources[0].Health != catalog.HealthDisconnected {
		t.Fatalf("esperado 'desconectada', got %+v", it.Sources)
	}
	vols, err := repo.ListVolumes(ctx)
	if err != nil {
		t.Fatalf("list volumes: %v", err)
	}
	if len(vols) != 1 || vols[0].Connected {
		t.Fatalf("esperado volumen desconectado: %+v", vols)
	}

	// Reconexión: reaparece -> las fuentes vuelven a "viva" y se dispara
	// un nuevo rescaneo.
	enum.vols = []Volume{vol}
	rescanned = nil
	if err := p.tick(ctx); err != nil {
		t.Fatalf("tick 3 (reconexión): %v", err)
	}

	it, err = repo.Get(ctx, itemID)
	if err != nil {
		t.Fatalf("get tras reconexión: %v", err)
	}
	if len(it.Sources) != 1 || it.Sources[0].Health != catalog.HealthAlive {
		t.Fatalf("esperado 'viva' tras reconexión, got %+v", it.Sources)
	}
	if len(rescanned) != 1 || rescanned[0][0] != folder {
		t.Fatalf("esperado rescaneo tras reconexión, got %v", rescanned)
	}

	vols, err = repo.ListVolumes(ctx)
	if err != nil {
		t.Fatalf("list volumes tras reconexión: %v", err)
	}
	if len(vols) != 1 || !vols[0].Connected {
		t.Fatalf("esperado volumen conectado: %+v", vols)
	}
}

// TestPollerVolumeSinCarpetaRegistradaNoEscanea confirma que un volumen
// nuevo sin ninguna carpeta registrada en él NO dispara ningún rescaneo
// (Brief M2: "el usuario decide qué carpetas registra").
func TestPollerVolumeSinCarpetaRegistradaNoEscanea(t *testing.T) {
	ctx := context.Background()
	repo := newPollerTestRepo(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	vol := Volume{ID: "vol-2", Label: "Otro", Roots: []string{filepath.Join("F:", "")}}
	enum := &fakePollerEnum{vols: []Volume{vol}}

	var rescanned [][]string
	p := &Poller{
		Enum:    enum,
		Folders: func() []string { return nil }, // ninguna carpeta registrada
		Repo:    repo,
		Rescan:  func(fs []string) { rescanned = append(rescanned, fs) },
		Log:     log,
	}

	if err := p.tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(rescanned) != 0 {
		t.Fatalf("no se esperaba rescaneo: %v", rescanned)
	}
	vols, err := repo.ListVolumes(ctx)
	if err != nil {
		t.Fatalf("list volumes: %v", err)
	}
	if len(vols) != 1 || vols[0].ID != vol.ID {
		t.Fatalf("el volumen debería registrarse igualmente: %+v", vols)
	}
}
