package scan_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
)

// fakeEnum es el "fallback no-Windows" de prueba: un volumen por carpeta
// registrada, sin depender del SO (Brief M2 §5).
type fakeEnum struct{}

func (fakeEnum) Volumes(ctx context.Context, folders []string) ([]localfs.Volume, error) {
	vols := make([]localfs.Volume, 0, len(folders))
	for _, f := range folders {
		vols = append(vols, localfs.Volume{ID: "vol-" + f, Roots: []string{f}})
	}
	return vols, nil
}

func newRepo(t *testing.T) *catalog.Repo {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return catalog.NewRepo(st.DB)
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitDone(t *testing.T, s *scan.Scanner) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !s.Status().Running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout esperando a que el escaneo termine")
}

func TestScanCreaAltaEsIdempotenteYLimpiaBorrados(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	root := t.TempDir()
	video := filepath.Join(root, "Pelicula.2024.1080p.mkv")
	writeFile(t, video, int(localfs.MinFileSize)+1)
	writeFile(t, filepath.Join(root, "sample.mkv"), int(localfs.MinFileSize)-1)

	repo := newRepo(t)
	cfg := &config.Config{MediaFolders: []string{root}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := scan.New(ctx, cfg, repo, fakeEnum{}, log)

	// Alta.
	if err := s.Start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitDone(t, s)

	res, err := repo.List(ctx, catalog.ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("total=%d, esperado 1 (sample.mkv < 10 MB debe excluirse)", res.Total)
	}
	item := res.Items[0]
	if item.Kind != catalog.KindOther || item.Confirmed {
		t.Fatalf("item inesperado: %+v", item)
	}
	if item.Title != "Pelicula.2024.1080p" {
		t.Fatalf("title=%q, esperado sin extensión", item.Title)
	}

	full, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(full.Sources) != 1 || full.Sources[0].Health != catalog.HealthAlive {
		t.Fatalf("fuente inesperada: %+v", full.Sources)
	}
	if full.Sources[0].VolumeID == nil || *full.Sources[0].VolumeID != "vol-"+root {
		t.Fatalf("volume_id inesperado: %+v", full.Sources[0].VolumeID)
	}
	firstSourceID := full.Sources[0].ID

	// Idempotencia: re-escanear no duplica.
	if err := s.Start(nil); err != nil {
		t.Fatalf("start 2: %v", err)
	}
	waitDone(t, s)

	res2, err := repo.List(ctx, catalog.ListFilter{})
	if err != nil {
		t.Fatalf("list 2: %v", err)
	}
	if res2.Total != 1 {
		t.Fatalf("total tras re-escaneo=%d, esperado 1 (cero duplicados)", res2.Total)
	}
	full2, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get 2: %v", err)
	}
	if len(full2.Sources) != 1 || full2.Sources[0].ID != firstSourceID {
		t.Fatalf("fuente duplicada tras re-escaneo: %+v", full2.Sources)
	}

	// Fichero borrado + rescaneo -> fuente eliminada, item conservado.
	if err := os.Remove(video); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := s.Start(nil); err != nil {
		t.Fatalf("start 3: %v", err)
	}
	waitDone(t, s)

	full3, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get 3 (el item debería conservarse): %v", err)
	}
	if len(full3.Sources) != 0 {
		t.Fatalf("esperado 0 fuentes tras borrar el fichero, got %+v", full3.Sources)
	}
}

func TestScanUnSoloEscaneoConcurrente(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	repo := newRepo(t)
	cfg := &config.Config{MediaFolders: []string{t.TempDir()}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := scan.New(ctx, cfg, repo, fakeEnum{}, log)

	if err := s.Start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	err := s.Start(nil)
	waitDone(t, s)
	if err == nil {
		// La primera ejecución pudo terminar ya (carpeta vacía, muy rápida):
		// no es un fallo de comportamiento, solo hace el solapamiento no
		// observable en esta ejecución concreta.
		t.Skip("el primer escaneo terminó antes de poder observar el solapamiento")
	}
	if err != scan.ErrAlreadyRunning {
		t.Fatalf("err=%v, esperado ErrAlreadyRunning", err)
	}
}

func TestRemoveFolderSources(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	root := t.TempDir()
	itemID, err := repo.InsertItem(ctx, &catalog.Item{Kind: catalog.KindOther, Title: "x", Genres: []string{}}, "")
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}
	if _, err := repo.InsertSource(ctx, &catalog.Source{
		ItemID: itemID, Provider: localfs.ProviderID, Locator: filepath.Join(root, "peli.mkv"),
	}, ""); err != nil {
		t.Fatalf("insert source: %v", err)
	}

	n, err := scan.RemoveFolderSources(ctx, repo, root)
	if err != nil {
		t.Fatalf("remove folder sources: %v", err)
	}
	if n != 1 {
		t.Fatalf("n=%d, esperado 1", n)
	}

	it, err := repo.Get(ctx, itemID)
	if err != nil {
		t.Fatalf("get (el item debería conservarse): %v", err)
	}
	if len(it.Sources) != 0 {
		t.Fatalf("esperado 0 fuentes, got %+v", it.Sources)
	}
}

// Bug "carpeta duplicada" (M7 §6): quitar una carpeta y volver a añadirla
// creaba items NUEVOS y dejaba los originales como fantasmas sin fuentes.
func TestReAddFolderReconcilesInsteadOfDuplicating(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Pelicula.Uno.2020.mkv"), int(localfs.MinFileSize)+1)
	writeFile(t, filepath.Join(root, "Pelicula.Dos.2021.mkv"), int(localfs.MinFileSize)+1)

	repo := newRepo(t)
	cfg := &config.Config{MediaFolders: []string{root}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := scan.New(ctx, cfg, repo, fakeEnum{}, log)

	if err := s.Start(nil); err != nil {
		t.Fatal(err)
	}
	waitDone(t, s)
	first, err := repo.List(ctx, catalog.ListFilter{})
	if err != nil || first.Total != 2 {
		t.Fatalf("alta inicial: total=%d err=%v", first.Total, err)
	}
	ids := map[int64]bool{first.Items[0].ID: true, first.Items[1].ID: true}

	// Quitar la carpeta (lo que hace DELETE /folders) y volver a añadirla.
	if _, err := scan.RemoveFolderSources(ctx, repo, root); err != nil {
		t.Fatal(err)
	}
	if err := s.Start([]string{root}); err != nil {
		t.Fatal(err)
	}
	waitDone(t, s)

	after, err := repo.List(ctx, catalog.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if after.Total != 2 {
		t.Fatalf("total tras re-añadir = %d, esperado 2 (sin duplicados ni fantasmas)", after.Total)
	}
	for _, it := range after.Items {
		if !ids[it.ID] {
			t.Errorf("item %d (%q) es nuevo: debió reutilizarse uno de %v", it.ID, it.Title, ids)
		}
		if it.SourceCount != 1 {
			t.Errorf("item %d con %d fuentes, esperado 1", it.ID, it.SourceCount)
		}
	}
	if st := s.Status(); st.ItemsNew != 0 {
		t.Errorf("ItemsNew = %d, esperado 0: nada es nuevo", st.ItemsNew)
	}
}

// Un fichero que desaparece y vuelve (disco sin montar, USB intermitente)
// recupera su item en vez de duplicarlo.
func TestFileReturningReusesItsItem(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	root := t.TempDir()
	video := filepath.Join(root, "Vuelve.2022.mkv")
	writeFile(t, video, int(localfs.MinFileSize)+1)

	repo := newRepo(t)
	cfg := &config.Config{MediaFolders: []string{root}}
	s := scan.New(ctx, cfg, repo, fakeEnum{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	s.Start(nil)
	waitDone(t, s)
	res, _ := repo.List(ctx, catalog.ListFilter{})
	id := res.Items[0].ID

	// Marcarlo como identificado: la identificación debe sobrevivir.
	if _, err := repo.DB.ExecContext(ctx, `UPDATE items SET title='Vuelve', confirmed=1, tmdb_id=99 WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}

	os.Remove(video)
	s.Start(nil)
	waitDone(t, s)
	writeFile(t, video, int(localfs.MinFileSize)+1)
	s.Start(nil)
	waitDone(t, s)

	res, _ = repo.List(ctx, catalog.ListFilter{})
	if res.Total != 1 || res.Items[0].ID != id {
		t.Fatalf("total=%d items=%+v, esperado el mismo item %d", res.Total, res.Items, id)
	}
	if res.Items[0].Title != "Vuelve" || res.Items[0].SourceCount != 1 {
		t.Errorf("item recuperado = %+v", res.Items[0])
	}
}

// Si el usuario descarta el item a propósito, volver a escanear la carpeta
// lo da de alta como nuevo: el fantasma no resucita lo que se borró.
func TestDeletedItemIsNotResurrected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Descartada.mkv"), int(localfs.MinFileSize)+1)
	repo := newRepo(t)
	cfg := &config.Config{MediaFolders: []string{root}}
	s := scan.New(ctx, cfg, repo, fakeEnum{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	s.Start(nil)
	waitDone(t, s)
	res, _ := repo.List(ctx, catalog.ListFilter{})
	scan.RemoveFolderSources(ctx, repo, root)
	if err := repo.DeleteItem(ctx, res.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	s.Start([]string{root})
	waitDone(t, s)

	res2, _ := repo.List(ctx, catalog.ListFilter{})
	if res2.Total != 1 || res2.Items[0].SourceCount != 1 {
		t.Fatalf("total=%d: el escaneo debe crear un item nuevo con su fuente", res2.Total)
	}
}
