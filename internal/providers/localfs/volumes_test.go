package localfs

import (
	"path/filepath"
	"testing"
)

func TestMatchVolume(t *testing.T) {
	root := filepath.Join("E:", "Peliculas")
	vols := []Volume{{ID: "v1", Roots: []string{root}}}

	file := filepath.Join(root, "pelicula.mkv")
	v, ok := MatchVolume(file, vols)
	if !ok || v.ID != "v1" {
		t.Fatalf("esperado match con v1, got %+v ok=%v", v, ok)
	}

	// La propia raíz también coincide (carpeta registrada == raíz del volumen).
	if _, ok := MatchVolume(root, vols); !ok {
		t.Fatal("esperado match para la propia raíz")
	}

	other := filepath.Join("F:", "Otros", "x.mkv")
	if _, ok := MatchVolume(other, vols); ok {
		t.Fatal("no debería coincidir con ningún volumen")
	}

	// Prefijo parcial de nombre (p. ej. "E:\Peliculas2") no debe confundirse
	// con "E:\Peliculas".
	partial := filepath.Join("E:", "Peliculas2", "x.mkv")
	if _, ok := MatchVolume(partial, vols); ok {
		t.Fatal("un prefijo de nombre parcial no debería coincidir")
	}
}

func TestNormalizePathTrailingSeparator(t *testing.T) {
	a := NormalizePath(filepath.Join("E:", "Peliculas") + string(filepath.Separator))
	b := NormalizePath(filepath.Join("E:", "Peliculas"))
	if a != b {
		t.Fatalf("normalización inconsistente: %q != %q", a, b)
	}
}
