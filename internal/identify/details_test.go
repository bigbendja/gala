package identify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildDetailsFromFullDetail(t *testing.T) {
	client := tmdbRoutes(t, map[string]string{"/movie/27205": "movie_27205_full.json"})
	fd, err := client.FullDetail(context.Background(), 27205, "movie")
	if err != nil {
		t.Fatal(err)
	}
	d, cert := BuildDetails(7, "movie", fd)

	if d.RuntimeMin == nil || *d.RuntimeMin != 148 {
		t.Errorf("duración = %v", d.RuntimeMin)
	}
	if d.VoteAverage == nil || *d.VoteAverage != 8.4 {
		t.Errorf("nota = %v, esperado 8.4 (redondeada a 1 decimal)", d.VoteAverage)
	}
	if cert != "12" {
		t.Errorf("clasificación = %q, esperado la numérica de España (12)", cert)
	}
	if len(d.Countries) != 2 || d.Countries[0] != "Reino Unido" || d.Countries[1] != "Estados Unidos" {
		t.Errorf("países = %v (en español)", d.Countries)
	}
	// Reparto: por orden de TMDB, sin personas sin nombre.
	if len(d.Cast) != 4 || d.Cast[0].Name != "Leonardo DiCaprio" || d.Cast[3].Name != "Tom Hardy" {
		t.Fatalf("reparto = %+v", d.Cast)
	}
	if d.Cast[0].Character != "Dom Cobb" || d.Cast[0].ProfilePath != "/leo.jpg" {
		t.Errorf("DiCaprio = %+v", d.Cast[0])
	}
	// Equipo: roles en español, sin duplicar nombres y sin oficios irrelevantes.
	roles := map[string][]string{}
	for _, g := range d.Crew {
		roles[g.Role] = g.Names
	}
	if len(d.Crew) != 4 || roles["Dirección"][0] != "Christopher Nolan" || len(roles["Guion"]) != 1 ||
		roles["Música"][0] != "Hans Zimmer" || roles["Fotografía"][0] != "Wally Pfister" {
		t.Errorf("equipo = %+v", d.Crew)
	}
	if d.Crew[0].Role != "Dirección" {
		t.Errorf("el orden empieza por Dirección: %+v", d.Crew)
	}
	// Similares: sin id no entra.
	if len(d.Similar) != 2 || d.Similar[0].Title != "Interstellar" || d.Similar[0].Year != 2014 || d.Similar[0].Kind != "movie" {
		t.Errorf("similares = %+v", d.Similar)
	}
}

func TestEnrichDetailsStoresEverythingAndPhotos(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{"/movie/27205": "movie_27205_full.json"})
	id := insertProvisionalItem(t, ctx, repo, "Inception.2010")
	tmdb := int64(27205)
	if err := repo.ApplyMovieIdentification(ctx, id, tmdb, "Origen", nil, nil, nil, nil, nil, nil, []string{"Acción"}, ""); err != nil {
		t.Fatal(err)
	}

	q := NewQueue(ctx, repo, client, discardLogger())
	q.enrichDetails()

	d, err := repo.GetDetails(ctx, id)
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}
	if len(d.Cast) != 4 || d.Cast[0].Photo == "" {
		t.Errorf("la foto de DiCaprio debe estar descargada: %+v", d.Cast[0])
	}
	if d.Cast[3].Photo != "" || d.Cast[3].ProfilePath != "" {
		t.Errorf("Tom Hardy no tiene foto en TMDB: %+v", d.Cast[2])
	}
	if d.Similar[0].Poster == "" || d.Similar[1].Poster != "" {
		t.Errorf("carátulas de similares: %+v", d.Similar)
	}
	if d.ImagesPending {
		t.Error("sin fallos de descarga no debe quedar nada pendiente")
	}
	it, _ := repo.Get(ctx, id)
	if it.RatingCert == nil || *it.RatingCert != "12" {
		t.Errorf("rating_cert = %v", it.RatingCert)
	}
	if it.Year == nil || *it.Year != 2010 {
		t.Errorf("el año que faltaba se rellena desde TMDB: %v", it.Year)
	}
	// Auto-terminante: ya no está pendiente.
	if left, _ := repo.ItemsNeedingDetails(ctx); len(left) != 0 {
		t.Errorf("siguen pendientes %d items", len(left))
	}
}

// Sin conexión con TMDB no se rompe nada ni se guarda una fila a medias: se
// reintenta en la siguiente pasada.
func TestEnrichDetailsOfflineKeepsItemPending(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "caído", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	client := NewClient("k", t.TempDir(), newMemCache())
	client.BaseURL, client.ImageBase = srv.URL, srv.URL
	id := insertProvisionalItem(t, ctx, repo, "Inception.2010")
	if err := repo.ApplyMovieIdentification(ctx, id, 27205, "Origen", nil, nil, nil, nil, nil, intPtr(2010), nil, ""); err != nil {
		t.Fatal(err)
	}

	NewQueue(ctx, repo, client, discardLogger()).enrichDetails()

	if _, err := repo.GetDetails(ctx, id); err == nil {
		t.Fatal("no debe haber detalles guardados sin conexión")
	}
	if left, _ := repo.ItemsNeedingDetails(ctx); len(left) != 1 {
		t.Errorf("el item debe seguir pendiente: %d", len(left))
	}
}

// Una foto que falla se marca pendiente y se reintenta; tras recuperarse la
// red queda descargada. Mientras tanto la web enseña iniciales.
func TestDetailImagesRetryAfterFailure(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	var imagesUp atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/movie/") {
			w.Write(fixture(t, "movie_27205_full.json"))
			return
		}
		if !imagesUp.Load() {
			http.Error(w, "cdn caído", http.StatusBadGateway)
			return
		}
		w.Write([]byte("jpegbytes"))
	}))
	defer srv.Close()
	client := NewClient("k", t.TempDir(), newMemCache())
	client.BaseURL, client.ImageBase = srv.URL, srv.URL
	client.BackoffUnit = time.Millisecond
	id := insertProvisionalItem(t, ctx, repo, "Inception.2010")
	if err := repo.ApplyMovieIdentification(ctx, id, 27205, "Origen", nil, nil, nil, nil, nil, intPtr(2010), nil, ""); err != nil {
		t.Fatal(err)
	}

	q := NewQueue(ctx, repo, client, discardLogger())
	q.enrichDetails()
	d, err := repo.GetDetails(ctx, id)
	if err != nil {
		t.Fatalf("los datos de texto se guardan aunque fallen las fotos: %v", err)
	}
	if !d.ImagesPending || d.Cast[0].Photo != "" || len(d.Cast) != 4 {
		t.Fatalf("tras el fallo: pending=%v cast0=%+v", d.ImagesPending, d.Cast[0])
	}

	imagesUp.Store(true)
	q.retryDetailImages()
	d, _ = repo.GetDetails(ctx, id)
	if d.ImagesPending || d.Cast[0].Photo == "" || d.Similar[0].Poster == "" {
		t.Errorf("tras reintentar: pending=%v cast0=%+v similar0=%+v", d.ImagesPending, d.Cast[0], d.Similar[0])
	}
	if d.ImageAttempts != 1 {
		t.Errorf("intentos = %d", d.ImageAttempts)
	}
}
