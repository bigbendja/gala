package identify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// memCache es una implementación en memoria de Cache para tests.
type memCache struct {
	detail map[string]string
	search map[string]string
}

func newMemCache() *memCache {
	return &memCache{detail: map[string]string{}, search: map[string]string{}}
}

func (c *memCache) GetTMDBDetail(_ context.Context, id int64, kind, lang string) (string, bool, error) {
	p, ok := c.detail[detailKey(id, kind, lang)]
	return p, ok, nil
}
func (c *memCache) PutTMDBDetail(_ context.Context, id int64, kind, lang, payload string) error {
	c.detail[detailKey(id, kind, lang)] = payload
	return nil
}
func (c *memCache) GetSearch(_ context.Context, hash, kind, lang string) (string, bool, error) {
	p, ok := c.search[hash+"|"+kind+"|"+lang]
	return p, ok, nil
}
func (c *memCache) PutSearch(_ context.Context, hash, kind, lang, payload string) error {
	c.search[hash+"|"+kind+"|"+lang] = payload
	return nil
}
func detailKey(id int64, kind, lang string) string {
	return kind + "|" + lang + "|" + strconv.FormatInt(id, 10)
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("test-key", t.TempDir(), newMemCache())
	c.BaseURL = srv.URL
	c.ImageBase = srv.URL
	c.HTTP.Timeout = 5 * time.Second
	return c, &calls
}

func TestSearchMovieParsesAndCaches(t *testing.T) {
	resp := searchResponse{Results: []SearchResult{
		{ID: 27205, Title: "Origen", OriginalTitle: "Inception", ReleaseDate: "2010-07-15", Popularity: 90},
	}}
	c, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resp)
	})

	results, err := c.SearchMovie(context.Background(), "Inception", 2010)
	if err != nil {
		t.Fatalf("SearchMovie: %v", err)
	}
	if len(results) != 1 || results[0].EffectiveTitle() != "Origen" || results[0].Year() != 2010 {
		t.Fatalf("resultados inesperados: %+v", results)
	}

	// Segunda llamada idéntica: debe servirse de search_cache, sin tocar la red.
	if _, err := c.SearchMovie(context.Background(), "Inception", 2010); err != nil {
		t.Fatalf("SearchMovie (caché): %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("llamadas HTTP = %d, esperado 1 (segunda debe venir de caché)", got)
	}
}

func TestDetailLanguageFallback(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Query().Get("language")
		d := Detail{ID: 27205, Title: "Origen"}
		if lang == "en-US" {
			d.Overview = "A thief who steals corporate secrets..."
		}
		_ = json.NewEncoder(w).Encode(d)
	})

	d, err := c.MovieDetail(context.Background(), 27205)
	if err != nil {
		t.Fatalf("MovieDetail: %v", err)
	}
	if d.Overview == "" {
		t.Error("Overview vacío: se esperaba el fallback a en-US")
	}
}

func TestGetRetriesOn5xxThenSucceeds(t *testing.T) {
	var n int32
	c, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(Detail{ID: 1, Title: "OK"})
	})

	d, err := c.MovieDetail(context.Background(), 1)
	if err != nil {
		t.Fatalf("MovieDetail: %v", err)
	}
	if d.Title != "OK" {
		t.Errorf("Title = %q, esperado OK", d.Title)
	}
	if got := atomic.LoadInt32(calls); got < 3 {
		t.Errorf("llamadas HTTP = %d, esperado >= 3 (reintentos)", got)
	}
}

func TestGetExhaustsRetriesAndReturnsErrOffline(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.MovieDetail(context.Background(), 1)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("err = %v, esperado ErrOffline", err)
	}
}

func TestGetRateLimited(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Detail{ID: 1})
	})

	start := time.Now()
	if _, err := c.MovieDetail(context.Background(), 1); err != nil {
		t.Fatalf("MovieDetail: %v", err)
	}
	if _, err := c.TVDetail(context.Background(), 2); err != nil {
		t.Fatalf("TVDetail: %v", err)
	}
	if elapsed := time.Since(start); elapsed < rateInterval {
		t.Errorf("dos peticiones tardaron %v, esperado >= %v (límite de 4 req/s)", elapsed, rateInterval)
	}
}

func TestDownloadPosterCachesOnDisk(t *testing.T) {
	c, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake-jpeg-bytes"))
	})

	name, err := c.DownloadPoster(context.Background(), "/abc123.jpg")
	if err != nil {
		t.Fatalf("DownloadPoster: %v", err)
	}
	if name == "" {
		t.Fatal("nombre de archivo vacío")
	}
	full := filepath.Join(c.CacheDir, "img", name)
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("no se creó %s: %v", full, err)
	}

	// Segunda descarga: ya está en cache/img/, no debe tocar la red.
	if _, err := c.DownloadPoster(context.Background(), "/abc123.jpg"); err != nil {
		t.Fatalf("DownloadPoster (caché disco): %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("llamadas HTTP = %d, esperado 1 (segunda debe venir de disco)", got)
	}
}

// TestDownloadPosterRetriesTransientFailure cubre la corrección post-entrega
// de M3: en la prueba real contra TMDB, algunos pósters se guardaban y otros
// no en la misma pasada, porque downloadImage no reintentaba fallos
// transitorios (a diferencia de get(), que sí lo hace). Un 503 puntual no
// debe perder la imagen.
func TestDownloadPosterRetriesTransientFailure(t *testing.T) {
	var n int32
	c, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("fake-jpeg-bytes"))
	})

	name, err := c.DownloadPoster(context.Background(), "/flaky.jpg")
	if err != nil {
		t.Fatalf("DownloadPoster: %v", err)
	}
	if name == "" {
		t.Fatal("nombre de archivo vacío tras reintentar")
	}
	full := filepath.Join(c.CacheDir, "img", name)
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("no se creó %s pese al reintento: %v", full, err)
	}
	if got := atomic.LoadInt32(calls); got < 3 {
		t.Errorf("llamadas HTTP = %d, esperado >= 3 (reintentos)", got)
	}
}

// TestDownloadPosterExhaustsRetriesAndLogs cubre el otro lado del bug: si la
// imagen falla de forma persistente, DownloadPoster debe devolver un error
// claro (no una cadena vacía silenciosa) y dejar constancia en el logger del
// cliente.
func TestDownloadPosterExhaustsRetriesAndLogs(t *testing.T) {
	c, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	var logged bytes.Buffer
	c.Log = slog.New(slog.NewTextHandler(&logged, nil))

	name, err := c.DownloadPoster(context.Background(), "/siempre-falla.jpg")
	if err == nil {
		t.Fatal("err = nil, esperado error tras agotar reintentos")
	}
	if name != "" {
		t.Errorf("name = %q, esperado vacío", name)
	}
	if got := atomic.LoadInt32(calls); got != imageMaxAttempts {
		t.Errorf("llamadas HTTP = %d, esperado %d (imageMaxAttempts)", got, imageMaxAttempts)
	}
	if logged.Len() == 0 {
		t.Error("no se registró ningún log tras agotar los reintentos de la imagen")
	}
}

func TestDownloadPosterEmptyPath(t *testing.T) {
	c, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no debería llamar a la red con tmdbPath vacío")
	})
	name, err := c.DownloadPoster(context.Background(), "")
	if err != nil || name != "" {
		t.Fatalf("DownloadPoster('') = %q, %v; esperado '', nil", name, err)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("llamadas HTTP = %d, esperado 0", got)
	}
}
