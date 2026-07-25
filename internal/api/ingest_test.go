package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/bigbendja/gala/internal/api"
	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/ingest"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
)

type ingestTestServer struct {
	*httptest.Server
	repo *catalog.Repo
}

func newIngestTestServer(t *testing.T) *ingestTestServer {
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

	pipeline := ingest.NewPipeline(repo, nil, log)

	srv := api.New(st, repo, cfg, sc, fakeEnum{}, nil, nil, dir, log, nil, nil, pipeline)
	mux := http.NewServeMux()
	srv.Routes(mux)
	hts := httptest.NewServer(mux)
	t.Cleanup(func() { hts.Close(); cancel(); st.Close() })
	return &ingestTestServer{Server: hts, repo: repo}
}

type resumenDTO struct {
	Detectados           int            `json:"detectados"`
	Total                int            `json:"total"`
	Anadidos             int            `json:"anadidos"`
	Duplicados           int            `json:"duplicados"`
	PendientesConector   int            `json:"pendientes_conector"`
	OmitidosNoSoportados int            `json:"omitidos_no_soportados"`
	SinClasificar        int            `json:"sin_clasificar"`
	Basura               int            `json:"basura"`
	PorTipo              map[string]int `json:"por_tipo"`
	Detalles             []string       `json:"detalles"`
	ListID               int64          `json:"list_id"`
}

func TestHandleIngestPlainText(t *testing.T) {
	its := newIngestTestServer(t)
	body := "https://cdn.ejemplo.net/v/pelicula.mp4\nhttps://streamtape.com/e/abc\n"

	resp, err := http.Post(its.URL+"/api/v1/ingest", "text/plain", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("POST /ingest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, data)
	}
	var res resumenDTO
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Anadidos != 1 || res.OmitidosNoSoportados != 1 {
		t.Fatalf("Anadidos=%d OmitidosNoSoportados=%d, esperado 1/1: %+v", res.Anadidos, res.OmitidosNoSoportados, res)
	}
	if res.ListID == 0 {
		t.Error("ListID = 0, esperado un id de ingest_list")
	}
}

func TestHandleIngestMultipartFile(t *testing.T) {
	its := newIngestTestServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "lista.m3u")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = fw.Write([]byte("#EXTM3U\n#EXTINF:-1,Uno\nhttps://cdn.ejemplo.net/v/uno.mp4\n#EXTINF:-1,Dos\nhttps://cdn.ejemplo.net/v/dos.mp4\n"))
	if err := mw.Close(); err != nil {
		t.Fatalf("mw.Close: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, its.URL+"/api/v1/ingest", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest (multipart): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, data)
	}
	var res resumenDTO
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Anadidos != 2 {
		t.Fatalf("Anadidos = %d, esperado 2: %+v", res.Anadidos, res)
	}
}

func TestHandleIngestTooLarge413(t *testing.T) {
	its := newIngestTestServer(t)
	big := bytes.Repeat([]byte("a"), 6<<20) // 6 MB > límite de 5 MB

	resp, err := http.Post(its.URL+"/api/v1/ingest", "text/plain", bytes.NewReader(big))
	if err != nil {
		t.Fatalf("POST /ingest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, esperado 413", resp.StatusCode)
	}
}

func TestHandleIngestEmptyBody(t *testing.T) {
	its := newIngestTestServer(t)
	resp, err := http.Post(its.URL+"/api/v1/ingest", "text/plain", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST /ingest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", resp.StatusCode)
	}
}

func TestHandleIngestListsTraceability(t *testing.T) {
	its := newIngestTestServer(t)
	body := "https://cdn.ejemplo.net/v/pelicula.mp4\n"
	resp, err := http.Post(its.URL+"/api/v1/ingest", "text/plain", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("POST /ingest: %v", err)
	}
	resp.Body.Close()

	listResp, err := http.Get(its.URL + "/api/v1/ingest/lists")
	if err != nil {
		t.Fatalf("GET /ingest/lists: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", listResp.StatusCode)
	}
	var lists []catalog.IngestList
	if err := json.NewDecoder(listResp.Body).Decode(&lists); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(lists) != 1 || lists[0].Type != "text" {
		t.Fatalf("lists = %+v, esperado 1 de tipo text", lists)
	}
}
