package localfs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func TestParseStrm(t *testing.T) {
	casos := []struct {
		nombre, contenido, want string
		ok                      bool
	}{
		{"una línea", "https://a.example.com/v.mp4", "https://a.example.com/v.mp4", true},
		{"CRLF y espacios", "  http://a.example.com/v.mp4  \r\n", "http://a.example.com/v.mp4", true},
		{"comentarios primero", "#EXTM3U\n# nota\nhttps://a.example.com/v.mp4\n", "https://a.example.com/v.mp4", true},
		{"salta líneas que no son URL", "hola\nfile:///x\nhttps://b.example.com/v.mp4\nhttps://c.example.com/v.mp4", "https://b.example.com/v.mp4", true},
		{"con BOM", string(rune(0xFEFF)) + "https://a.example.com/v.mp4", "https://a.example.com/v.mp4", true},
		{"vacío", "", "", false},
		{"solo comentarios", "# uno\n# https://a.example.com/v.mp4\n", "", false},
		{"sin host", "http:///v.mp4", "", false},
		{"esquema no permitido", "ftp://a.example.com/v.mp4", "", false},
	}
	for _, c := range casos {
		got, err := ParseStrm(c.contenido)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("%s: got %q err=%v, want %q ok=%v", c.nombre, got, err, c.want, c.ok)
		}
	}
}

func TestWalkFolderFindsStrmBelowMinSize(t *testing.T) {
	root := t.TempDir()
	strm := filepath.Join(root, "Peli.strm")
	if err := os.WriteFile(strm, []byte("https://a.example.com/v.mp4"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Un vídeo pequeño sigue descartándose; un .strm oculto también.
	if err := os.WriteFile(filepath.Join(root, "pequeno.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".oculto.strm"), []byte("https://a.example.com/v.mp4"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := WalkFolder(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Path != strm || !found[0].Strm {
		t.Fatalf("found = %+v", found)
	}
}

func TestSourceOriginPathIsNeverSerialized(t *testing.T) {
	origin := `D:\Privado\Peli.strm`
	b, err := json.Marshal(catalog.Source{Provider: "http", Locator: "https://a.example.com/v.mp4", OriginPath: &origin})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "Privado") || strings.Contains(string(b), "origin_path") {
		t.Fatalf("la ruta del .strm se filtra por la API: %s", b)
	}
}
