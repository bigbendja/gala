package identify

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestQueueDrainAppliesAltaConfidence(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{
		"/movie/27205":  "movie_27205.json",
		"/search/movie": "search_movie_inception.json",
	})
	id := insertProvisionalItem(t, ctx, repo, "Inception.2010.1080p.BluRay.x264-SPARKS")

	q := NewQueue(ctx, repo, client, discardLogger())
	q.drain()

	it, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !it.Confirmed || it.IdentifyStatus != catalog.IdentifyOK || it.Kind != catalog.KindMovie {
		t.Fatalf("item tras drain = %+v", it)
	}
}

func TestQueueProcessesJunkAndExtra(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{}) // no debería llamarse nunca

	junkID := insertProvisionalItem(t, ctx, repo, "VID-20230412-WA0012")
	extraID := insertProvisionalItem(t, ctx, repo, "Inception.2010.1080p.BluRay.x264-SPARKS.sample")

	q := NewQueue(ctx, repo, client, discardLogger())
	q.drain()

	it, err := repo.Get(ctx, junkID)
	if err != nil {
		t.Fatalf("Get(junk): %v", err)
	}
	if it.IdentifyStatus != catalog.IdentifyJunk || it.Confirmed {
		t.Errorf("item junk = %+v", it)
	}

	if _, err := repo.Get(ctx, extraID); err != catalog.ErrNotFound {
		t.Errorf("Get(extra) = %v, esperado ErrNotFound (los extras no aparecen en la galería)", err)
	}
}

func TestQueueMediaGoesToPendingConfirmation(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{
		"/search/movie": "search_movie_ambiguous.json",
	})
	id := insertProvisionalItem(t, ctx, repo, "Aventura Desconocida Sin Relacion")

	q := NewQueue(ctx, repo, client, discardLogger())
	q.drain()

	it, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.Confirmed {
		t.Fatal("item quedó confirmado; candidatos ambiguos deben ir a Por confirmar")
	}
	pending, err := repo.ListPendingConfirmation(ctx)
	if err != nil {
		t.Fatalf("ListPendingConfirmation: %v", err)
	}
	if len(pending) != 1 || len(pending[0].Candidates) < 2 {
		t.Fatalf("pending = %+v, esperado 1 item con varios candidatos", pending)
	}
}

func TestQueueOfflineThenRecovers(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)

	var fail int32 = 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&fail) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/movie/") {
			w.Write(fixture(t, "movie_27205.json"))
			return
		}
		w.Write(fixture(t, "search_movie_inception.json"))
	}))
	t.Cleanup(srv.Close)
	client := NewClient("test-key", t.TempDir(), newMemCache())
	client.BaseURL = srv.URL
	client.ImageBase = srv.URL

	id := insertProvisionalItem(t, ctx, repo, "Inception.2010.1080p.BluRay.x264-SPARKS")

	q := NewQueue(ctx, repo, client, discardLogger())
	q.drain()

	it, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.IdentifyStatus != catalog.IdentifyError {
		t.Fatalf("IdentifyStatus = %q, esperado %q (sin red)", it.IdentifyStatus, catalog.IdentifyError)
	}
	if it.Confirmed {
		t.Fatal("item confirmado sin red disponible")
	}
	if q.Status().Online {
		t.Error("Status().Online = true tras fallo de red, esperado false")
	}

	// "Recupera" la red: el mismo item, en estado error, se reintenta solo
	// en el siguiente drenado y esta vez se identifica con éxito (Brief §4).
	atomic.StoreInt32(&fail, 0)
	q.drain()

	it2, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get (tras recuperar red): %v", err)
	}
	if !it2.Confirmed {
		t.Error("item no confirmado tras recuperar la red")
	}
	if !q.Status().Online {
		t.Error("Status().Online = false tras recuperar la red")
	}
}

// TestQueueDoesNotBusyLoopOnPersistentAPIError reproduce el bug real
// reportado en uso: con una API key de TMDB inválida (401, un error de API
// real, NO offline: internal/identify/tmdb.go solo envuelve en ErrOffline
// los 5xx/429/errores de red tras agotar reintentos), drain() volvía a
// PendingIdentify de inmediato una y otra vez porque PendingIdentify incluye
// items en error a propósito (para reintentarlos) — sin ninguna pasada que
// progrese, el bucle exterior nunca paraba, martillando TMDB cientos de
// veces por segundo en vez de esperar al siguiente drenado periódico.
func TestQueueDoesNotBusyLoopOnPersistentAPIError(t *testing.T) {
	repo := newRepoForTest(t)

	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status_code":7,"status_message":"Invalid API key"}`))
	}))
	t.Cleanup(srv.Close)
	client := NewClient("clave-invalida", t.TempDir(), newMemCache())
	client.BaseURL = srv.URL
	client.ImageBase = srv.URL

	id := insertProvisionalItem(t, context.Background(), repo, "Inception.2010.1080p.BluRay.x264-SPARKS")

	// Contexto acotado: si el bug de bucle sin backoff reaparece, el test
	// falla con un mensaje claro (y rápido) en vez de colgarse para siempre.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	q := NewQueue(ctx, repo, client, discardLogger())

	done := make(chan struct{})
	go func() {
		q.drain()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("drain() no volvió en 1s: parece seguir en bucle sin backoff")
	}

	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("peticiones a TMDB = %d, esperado exactamente 1 (una sola pasada, sin reintento inmediato)", got)
	}

	it, err := repo.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.IdentifyStatus != catalog.IdentifyError {
		t.Fatalf("IdentifyStatus = %q, esperado %q", it.IdentifyStatus, catalog.IdentifyError)
	}
	if q.Status().LastError == "" {
		t.Error("Status().LastError vacío, esperado el motivo del fallo persistente (para poder diagnosticarlo)")
	}
}

func TestQueueRebuildsFromDB(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{
		"/movie/27205":  "movie_27205.json",
		"/search/movie": "search_movie_inception.json",
	})
	id := insertProvisionalItem(t, ctx, repo, "Inception.2010.1080p.BluRay.x264-SPARKS")

	// Primera "sesión": la cola se crea pero se descarta sin drenar (como
	// si la app se hubiera cerrado a mitad de proceso).
	_ = NewQueue(ctx, repo, client, discardLogger())

	// Segunda "sesión": una cola nueva, sin memoria compartida con la
	// anterior, debe encontrar el pendiente igual (Brief §4: "la cola se
	// reconstruye de BD, no de memoria").
	q2 := NewQueue(ctx, repo, client, discardLogger())
	q2.drain()

	it, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !it.Confirmed {
		t.Error("la segunda cola no procesó el pendiente heredado de la primera sesión")
	}
}

// TestQueueBackfillsMissingImageAfterConfirm reproduce el bug real de
// "Coco" (tmdb_id 354912 en la prueba real: backdrop_path guardado, poster_
// path NULL, de forma reproducible). La causa: el CDN de imágenes de TMDB
// puede fallar de forma persistente para UNA imagen concreta en la misma
// pasada en que otra (del mismo item) sí se descarga; una vez que
// ApplyMovieIdentification confirma el item (identificación correcta, solo
// falta el arte), PendingIdentify ya no vuelve a mirarlo nunca — así que sin
// backfillImages el póster se pierde para siempre, aunque el CDN se
// recupere después. Por eso "con reintento ya activo" seguía siendo
// reproducible: el reintento cubre el momento, no el después.
func TestQueueBackfillsMissingImageAfterConfirm(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)

	var posterShouldFail int32 = 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/movie/"):
			w.Write(fixture(t, "movie_27205.json"))
		case strings.HasPrefix(r.URL.Path, "/search/movie"):
			w.Write(fixture(t, "search_movie_inception.json"))
		case strings.Contains(r.URL.Path, "poster27205"):
			if atomic.LoadInt32(&posterShouldFail) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Write([]byte("fake-poster-bytes"))
		case strings.Contains(r.URL.Path, "backdrop27205"):
			w.Write([]byte("fake-backdrop-bytes"))
		default:
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	client := NewClient("test-key", t.TempDir(), newMemCache())
	client.BaseURL = srv.URL
	client.ImageBase = srv.URL

	id := insertProvisionalItem(t, ctx, repo, "Inception.2010.1080p.BluRay.x264-SPARKS")

	q := NewQueue(ctx, repo, client, discardLogger())
	q.drain()

	it, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !it.Confirmed || it.IdentifyStatus != catalog.IdentifyOK {
		t.Fatalf("item tras drain = %+v, esperado confirmado/ok pese al fallo de imagen (la identificación en sí fue correcta)", it)
	}
	if it.PosterPath != nil {
		t.Fatalf("PosterPath = %v, esperado nil (el CDN de pósters seguía fallando)", it.PosterPath)
	}
	if it.BackdropPath == nil {
		t.Fatal("BackdropPath = nil, esperado descargado (esa URL nunca falló)")
	}

	pending, err := repo.PendingIdentify(ctx)
	if err != nil {
		t.Fatalf("PendingIdentify: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("PendingIdentify = %d, esperado 0: el item ya está confirmado y sin backfillImages nunca se reintentaría", len(pending))
	}

	// El CDN "se recupera": backfillImages debe rellenar el póster sin
	// tocar el backdrop ya guardado.
	atomic.StoreInt32(&posterShouldFail, 0)
	q.backfillImages()

	it2, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get (tras backfill): %v", err)
	}
	if it2.PosterPath == nil || *it2.PosterPath == "" {
		t.Fatal("PosterPath sigue vacío tras backfillImages")
	}
	if it2.BackdropPath == nil || *it2.BackdropPath != *it.BackdropPath {
		t.Errorf("BackdropPath cambió tras el backfill (debía quedar intacto): antes=%v, después=%v", it.BackdropPath, it2.BackdropPath)
	}
}

func TestQueueEnqueueDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{})
	q := NewQueue(ctx, repo, client, discardLogger())

	// Varias llamadas seguidas no deben bloquear ni acumular (canal con
	// buffer 1 y select con default).
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			q.Enqueue()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Enqueue se bloqueó")
	}
}
