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
