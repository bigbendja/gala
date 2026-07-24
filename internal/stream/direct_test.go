package stream

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("escribir %s: %v", full, err)
	}
	return full
}

func TestValidatePathInsideFolder(t *testing.T) {
	dir := t.TempDir()
	full := writeTestFile(t, dir, "peli.mp4", "contenido")

	if err := ValidatePath(full, []string{dir}); err != nil {
		t.Fatalf("ValidatePath: %v, esperado nil (dentro de la carpeta registrada)", err)
	}
}

func TestValidatePathOutsideFolderRejected(t *testing.T) {
	registered := t.TempDir()
	outside := t.TempDir()
	full := writeTestFile(t, outside, "peli.mp4", "contenido")

	err := ValidatePath(full, []string{registered})
	if err != ErrOutsideRegisteredFolders {
		t.Fatalf("ValidatePath = %v, esperado ErrOutsideRegisteredFolders", err)
	}
}

// TestValidatePathTraversalRejected cubre el caso explícito de path
// traversal con "..": debe quedar fuera aunque el prefijo textual coincida
// (CLAUDE.md: validar siempre contra las carpetas registradas).
func TestValidatePathTraversalRejected(t *testing.T) {
	registered := t.TempDir()
	sub := filepath.Join(registered, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Fuera de "sub" (que es lo registrado) pero dentro de "registered": el
	// ".." debe resolverse antes de comparar, no compararse como texto.
	secretFile := writeTestFile(t, registered, "secreto.txt", "no debería servirse")
	traversal := filepath.Join(sub, "..", "secreto.txt")

	err := ValidatePath(traversal, []string{sub})
	if err != ErrOutsideRegisteredFolders {
		t.Fatalf("ValidatePath(%q) = %v, esperado ErrOutsideRegisteredFolders", traversal, err)
	}
	_ = secretFile
}

func TestServeDirectRangeRequests(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("0123456789", 1000) // 10000 bytes
	full := writeTestFile(t, dir, "video.mp4", content)

	// Sin Range: 200 con todo el contenido.
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	if err := ServeDirect(w, req, full, []string{dir}); err != nil {
		t.Fatalf("ServeDirect: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", w.Code)
	}
	if w.Body.Len() != len(content) {
		t.Fatalf("body = %d bytes, esperado %d", w.Body.Len(), len(content))
	}
	if w.Header().Get("Content-Type") != "video/mp4" {
		t.Errorf("Content-Type = %q, esperado video/mp4", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("ETag") == "" {
		t.Error("ETag vacío")
	}
	if w.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("Accept-Ranges = %q, esperado bytes", w.Header().Get("Accept-Ranges"))
	}

	// Con Range parcial: 206 y el rango exacto pedido.
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req2.Header.Set("Range", "bytes=100-199")
	w2 := httptest.NewRecorder()
	if err := ServeDirect(w2, req2, full, []string{dir}); err != nil {
		t.Fatalf("ServeDirect (range): %v", err)
	}
	if w2.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, esperado 206", w2.Code)
	}
	if w2.Body.String() != content[100:200] {
		t.Fatalf("body = %q, esperado los bytes 100-199", w2.Body.String())
	}
	if cr := w2.Header().Get("Content-Range"); !strings.HasPrefix(cr, "bytes 100-199/") {
		t.Errorf("Content-Range = %q, esperado empezar por 'bytes 100-199/'", cr)
	}

	// Rango inválido (más allá del tamaño del archivo): 416.
	req3 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req3.Header.Set("Range", "bytes=999999-1000010")
	w3 := httptest.NewRecorder()
	if err := ServeDirect(w3, req3, full, []string{dir}); err != nil {
		t.Fatalf("ServeDirect (range inválido): %v", err)
	}
	if w3.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, esperado 416", w3.Code)
	}
}

func TestServeDirectRejectsOutsideFolder(t *testing.T) {
	registered := t.TempDir()
	outside := t.TempDir()
	full := writeTestFile(t, outside, "video.mp4", "x")

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	err := ServeDirect(w, req, full, []string{registered})
	if err != ErrOutsideRegisteredFolders {
		t.Fatalf("ServeDirect = %v, esperado ErrOutsideRegisteredFolders", err)
	}
}

func TestContentTypeByExtension(t *testing.T) {
	cases := map[string]string{
		"a.mp4": "video/mp4", "a.MKV": "video/x-matroska", "a.webm": "video/webm",
		"a.ts": "video/mp2t", "a.m3u8": "application/vnd.apple.mpegurl",
		"a.unknownext": "application/octet-stream",
	}
	for name, want := range cases {
		if got := ContentType(name); got != want {
			t.Errorf("ContentType(%q) = %q, esperado %q", name, got, want)
		}
	}
}
