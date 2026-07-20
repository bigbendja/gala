package localfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalkFolderFiltersCorrectly(t *testing.T) {
	root := t.TempDir()

	valid := filepath.Join(root, "Pelicula.2024.1080p.mkv")
	writeTestFile(t, valid, int(MinFileSize)+1)

	// Exclusiones: extensión no admitida, tamaño < 10 MB, fichero oculto,
	// carpeta oculta, carpeta de sistema.
	writeTestFile(t, filepath.Join(root, "notas.txt"), int(MinFileSize)+1)
	writeTestFile(t, filepath.Join(root, "sample.mkv"), int(MinFileSize)-1)
	writeTestFile(t, filepath.Join(root, ".oculto.mkv"), int(MinFileSize)+1)
	writeTestFile(t, filepath.Join(root, ".config", "peli.mkv"), int(MinFileSize)+1)
	writeTestFile(t, filepath.Join(root, "$RECYCLE.BIN", "peli.mkv"), int(MinFileSize)+1)

	found, err := WalkFolder(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("esperado 1 fichero válido, got %d: %+v", len(found), found)
	}
	if found[0].Path != valid {
		t.Fatalf("path=%q, esperado %q", found[0].Path, valid)
	}
}

func TestWalkFolderRootInaccesibleDevuelveError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "no-existe")
	if _, err := WalkFolder(context.Background(), root, nil); err == nil {
		t.Fatal("esperado error para una carpeta raíz inexistente")
	}
}
