package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/bigbendja/gala/internal/api"
	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/identify"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
)

// identifyTestServer es como testServer pero con Identify/TMDB reales
// (contra un TMDB simulado), para ejercitar /pending, /confirm,
// /identify/status e /img.
type identifyTestServer struct {
	*httptest.Server
	repo *catalog.Repo
	dir  string
}

func newIdentifyTestServer(t *testing.T, tmdbHandler http.HandlerFunc) *identifyTestServer {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	repo := catalog.NewRepo(st.DB)
	cfg := config.NewAt(dir)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if tmdbHandler == nil {
		tmdbHandler = func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"results":[]}`)) }
	}
	tmdbSrv := httptest.NewServer(tmdbHandler)
	t.Cleanup(tmdbSrv.Close)

	client := identify.NewClient("test-key", dir, repo)
	client.BaseURL = tmdbSrv.URL
	client.ImageBase = tmdbSrv.URL

	ctx, cancel := context.WithCancel(context.Background())
	q := identify.NewQueue(ctx, repo, client, log)
	sc := scan.New(ctx, cfg, repo, fakeEnum{}, log)

	srv := api.New(st, repo, cfg, sc, fakeEnum{}, q, client, dir, log, nil, nil)
	mux := http.NewServeMux()
	srv.Routes(mux)
	hts := httptest.NewServer(mux)

	t.Cleanup(func() {
		hts.Close()
		cancel()
		st.Close()
	})
	return &identifyTestServer{Server: hts, repo: repo, dir: dir}
}

func (its *identifyTestServer) insertProvisional(t *testing.T, title string) int64 {
	t.Helper()
	item := catalog.Item{Kind: catalog.KindOther, Title: title, Genres: []string{}, Confirmed: false}
	id, err := its.repo.InsertItem(context.Background(), &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: id, Provider: "localfs", Locator: title + ".mkv"}
	if _, err := its.repo.InsertSource(context.Background(), &src, ""); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	return id
}

func TestHandleListPending(t *testing.T) {
	its := newIdentifyTestServer(t, nil)
	id := its.insertProvisional(t, "peli_dudosa")
	cands := []catalog.Candidate{{TMDBID: 1, Kind: "movie", Title: "Peli Dudosa", Score: 0.7}}
	if err := its.repo.SetIdentifyCandidates(context.Background(), id, cands, 0, 0, nil); err != nil {
		t.Fatalf("SetIdentifyCandidates: %v", err)
	}

	resp, err := http.Get(its.URL + "/api/v1/pending")
	if err != nil {
		t.Fatalf("GET /pending: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got []catalog.PendingItem
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || len(got[0].Candidates) != 1 || got[0].Candidates[0].TMDBID != 1 {
		t.Fatalf("pending = %+v", got)
	}
}

func TestHandleConfirmManual(t *testing.T) {
	its := newIdentifyTestServer(t, nil)
	id := its.insertProvisional(t, "video_raro")

	body, _ := json.Marshal(map[string]string{"title": "Título elegido a mano"})
	resp, err := http.Post(its.URL+"/api/v1/items/"+strconv.FormatInt(id, 10)+"/confirm", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, data)
	}

	it, err := its.repo.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.Title != "Título elegido a mano" || !it.Confirmed {
		t.Fatalf("item = %+v", it)
	}
}

func TestHandleConfirmWithTMDB(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "tmdb", "movie_27205.json")
	fixtureData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("leer fixture: %v", err)
	}
	its := newIdentifyTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixtureData)
	})
	id := its.insertProvisional(t, "inception.2010.1080p")

	body, _ := json.Marshal(map[string]any{"tmdb_id": 27205, "kind": "movie"})
	resp, err := http.Post(its.URL+"/api/v1/items/"+strconv.FormatInt(id, 10)+"/confirm", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, data)
	}

	it, err := its.repo.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !it.Confirmed || it.Kind != catalog.KindMovie || it.TMDBID == nil || *it.TMDBID != 27205 {
		t.Fatalf("item = %+v", it)
	}
}

func TestHandleConfirmRequiresTMDBOrTitle(t *testing.T) {
	its := newIdentifyTestServer(t, nil)
	id := its.insertProvisional(t, "algo")

	resp, err := http.Post(its.URL+"/api/v1/items/"+strconv.FormatInt(id, 10)+"/confirm", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST /confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", resp.StatusCode)
	}
}

func TestHandleIdentifyStatus(t *testing.T) {
	its := newIdentifyTestServer(t, nil)
	resp, err := http.Get(its.URL + "/api/v1/identify/status")
	if err != nil {
		t.Fatalf("GET /identify/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var st identify.Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestHandleImgServesFileAndRejectsTraversal(t *testing.T) {
	its := newIdentifyTestServer(t, nil)
	imgDir := filepath.Join(its.dir, "img")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(imgDir, "poster_w342_abc123.jpg"), []byte("fake-jpeg"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	resp, err := http.Get(its.URL + "/api/v1/img/poster_w342_abc123.jpg")
	if err != nil {
		t.Fatalf("GET /img: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "fake-jpeg" {
		t.Fatalf("body = %q", data)
	}

	resp2, err := http.Get(its.URL + "/api/v1/img/nonexistent.jpg")
	if err != nil {
		t.Fatalf("GET /img (no existe): %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", resp2.StatusCode)
	}

	for _, traversal := range []string{
		"/api/v1/img/..%2f..%2fmain.go",
		"/api/v1/img/..%5c..%5cmain.go",
	} {
		resp3, err := http.Get(its.URL + traversal)
		if err != nil {
			t.Fatalf("GET %s: %v", traversal, err)
		}
		resp3.Body.Close()
		if resp3.StatusCode == http.StatusOK {
			t.Fatalf("GET %s devolvió 200: posible path traversal", traversal)
		}
	}
}
