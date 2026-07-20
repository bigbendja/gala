package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/api"
	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
)

type fakeEnum struct{}

func (fakeEnum) Volumes(ctx context.Context, folders []string) ([]localfs.Volume, error) {
	return nil, nil
}

// testServer agrupa el httptest.Server con el Scanner subyacente: los tests
// necesitan esperar a que los escaneos en segundo plano (disparados por
// POST /folders o /scan) terminen antes de que el cleanup cierre la BD, o en
// Windows el fichero queda bloqueado y falla el RemoveAll de t.TempDir.
type testServer struct {
	*httptest.Server
	scanner *scan.Scanner
}

func (ts *testServer) waitScanIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !ts.scanner.Status().Running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout esperando a que el escaneo termine")
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}

	repo := catalog.NewRepo(st.DB)
	cfg := config.NewAt(dir)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	sc := scan.New(ctx, cfg, repo, fakeEnum{}, log)

	srv := api.New(st, repo, cfg, sc, fakeEnum{}, log)
	mux := http.NewServeMux()
	srv.Routes(mux)

	hts := httptest.NewServer(mux)
	ts := &testServer{Server: hts, scanner: sc}

	// Orden de cierre: parar el servidor, esperar a que no queden escaneos
	// en curso tocando la BD, cancelar el contexto y solo entonces cerrar
	// la BD (t.Cleanup se ejecuta en orden LIFO).
	t.Cleanup(func() { st.Close() })
	t.Cleanup(cancel)
	t.Cleanup(func() { ts.waitScanIdle(t) })
	t.Cleanup(hts.Close)
	return ts
}

func postFolder(t *testing.T, ts *testServer, path string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(ts.URL+"/api/v1/folders", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAddFolderAltaValida(t *testing.T) {
	ts := newTestServer(t)
	mediaDir := t.TempDir()

	resp := postFolder(t, ts, mediaDir)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("alta válida: status=%d", resp.StatusCode)
	}

	var got struct {
		Path      string `json:"path"`
		Connected bool   `json:"connected"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Connected {
		t.Fatal("esperado connected=true para una carpeta recién añadida")
	}
}

func TestAddFolderRutaInexistente(t *testing.T) {
	ts := newTestServer(t)
	resp := postFolder(t, ts, filepath.Join(t.TempDir(), "no-existe"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("ruta inexistente: status=%d, esperado 400", resp.StatusCode)
	}
}

func TestAddFolderDuplicada(t *testing.T) {
	ts := newTestServer(t)
	mediaDir := t.TempDir()

	resp1 := postFolder(t, ts, mediaDir)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("primera alta: status=%d", resp1.StatusCode)
	}

	resp2 := postFolder(t, ts, mediaDir)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicada: status=%d, esperado 409", resp2.StatusCode)
	}
}

func TestDeleteFolder(t *testing.T) {
	ts := newTestServer(t)
	mediaDir := t.TempDir()

	resp1 := postFolder(t, ts, mediaDir)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("alta previa: status=%d", resp1.StatusCode)
	}

	req, err := http.NewRequest(http.MethodDelete,
		ts.URL+"/api/v1/folders?"+url.Values{"path": {mediaDir}}.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("borrado: status=%d", resp2.StatusCode)
	}

	resp3, err := http.Get(ts.URL + "/api/v1/folders")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	var folders []any
	if err := json.NewDecoder(resp3.Body).Decode(&folders); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(folders) != 0 {
		t.Fatalf("esperado 0 carpetas tras el borrado, got %d", len(folders))
	}
}

func TestScanConflict(t *testing.T) {
	ts := newTestServer(t)

	// Simulamos "ya hay un escaneo en curso" registrando una carpeta (lo que
	// dispara un escaneo asíncrono) y golpeando /scan inmediatamente.
	mediaDir := t.TempDir()
	postFolder(t, ts, mediaDir).Body.Close()

	resp, err := http.Post(ts.URL+"/api/v1/scan", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, esperado 202 ó 409", resp.StatusCode)
	}
}
