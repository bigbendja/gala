package identify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/store"
)

// fixture lee un JSON de testdata/tmdb/ (Brief M3 §6).
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "tmdb", name))
	if err != nil {
		t.Fatalf("leer fixture %s: %v", name, err)
	}
	return data
}

// tmdbRoutes crea un cliente TMDB de prueba que sirve fixtures fijas según
// la ruta pedida (search/movie, movie/{id}, search/tv, tv/{id}).
func tmdbRoutes(t *testing.T, routes map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for prefix, file := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				w.Write(fixture(t, file))
				return
			}
		}
		w.Write(fixture(t, "search_movie_empty.json"))
	}))
	t.Cleanup(srv.Close)

	c := NewClient("test-key", t.TempDir(), newMemCache())
	c.BaseURL = srv.URL
	c.ImageBase = srv.URL
	return c
}

func TestMatchAltaConfidence(t *testing.T) {
	client := tmdbRoutes(t, map[string]string{
		"/search/movie": "search_movie_inception.json",
	})
	parsed := Parse("Inception.2010.1080p.BluRay.x264-SPARKS.mkv", "")

	res, err := Match(context.Background(), client, parsed, "Inception.2010.1080p.BluRay.x264-SPARKS.mkv")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if res.Confidence != ConfidenceAlta {
		t.Fatalf("Confidence = %v, esperado ConfidenceAlta", res.Confidence)
	}
	if res.Best == nil || res.Best.ID != 27205 {
		t.Fatalf("Best = %+v, esperado tmdb_id 27205", res.Best)
	}
}

func TestMatchMultipleCandidatesMedia(t *testing.T) {
	client := tmdbRoutes(t, map[string]string{
		"/search/movie": "search_movie_ambiguous.json",
	})
	// Consulta deliberadamente distinta de los 3 candidatos del fixture:
	// ninguno debe ganar con confianza ALTA, así que los tres deben
	// devolverse como candidatos para "Por confirmar" (Brief §3).
	parsed := Parsed{Title: "Aventura Desconocida Sin Relacion", Kind: KindMovie}

	res, err := Match(context.Background(), client, parsed, "padrino.mkv")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if res.Confidence == ConfidenceAlta {
		t.Fatalf("Confidence = Alta, esperado Media o Baja (candidatos ambiguos, no debe auto-confirmar)")
	}
	if len(res.Candidates) < 2 {
		t.Fatalf("Candidates = %d, esperado varios (Brief §3: hasta 5)", len(res.Candidates))
	}
	if len(res.Candidates) > maxCandidates {
		t.Fatalf("Candidates = %d, esperado como máximo %d", len(res.Candidates), maxCandidates)
	}
}

func TestMatchNoResults(t *testing.T) {
	client := tmdbRoutes(t, map[string]string{
		"/search/movie": "search_movie_empty.json",
	})
	parsed := Parsed{Title: "Pelicula Que No Existe En TMDB", Kind: KindMovie}

	res, err := Match(context.Background(), client, parsed, "pelicula.mkv")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if res.Confidence != ConfidenceBaja {
		t.Fatalf("Confidence = %v, esperado ConfidenceBaja", res.Confidence)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("Candidates = %d, esperado 0 (sin resultados → Por confirmar sin candidatos)", len(res.Candidates))
	}
	if res.Best != nil {
		t.Fatal("Best no debería estar poblado sin resultados")
	}
}

// TestScoreCandidateYearBonus prueba la regla de año ±1 del Brief §3
// directamente sobre scoreCandidate (más preciso que pasar por Match+HTTP).
func TestScoreCandidateYearBonus(t *testing.T) {
	parsed := Parsed{Title: "Interstellar", Year: 2014}
	exact := SearchResult{Title: "Interstellar", ReleaseDate: "2014-11-05"}
	near := SearchResult{Title: "Interstellar", ReleaseDate: "2013-11-05"}
	far := SearchResult{Title: "Interstellar", ReleaseDate: "2005-11-05"}

	sExact, sNear, sFar := scoreCandidate(parsed, exact), scoreCandidate(parsed, near), scoreCandidate(parsed, far)
	if !(sExact > sNear && sNear > sFar) {
		t.Errorf("bonus de año: exact=%v near=%v far=%v; esperado exact > near > far", sExact, sNear, sFar)
	}
}

func TestMatchYearOffByOneStillHighConfidence(t *testing.T) {
	client := tmdbRoutes(t, map[string]string{
		"/search/movie": "search_movie_yearoff.json",
	})
	parsed := Parsed{Title: "Interstellar", Year: 2014, Kind: KindMovie}

	res, err := Match(context.Background(), client, parsed, "Interstellar.2014.mkv")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	// Título idéntico + año ±1: debe seguir siendo una confianza alta,
	// aunque el bonus sea menor que con año exacto (Brief §3).
	if res.Confidence != ConfidenceAlta {
		t.Fatalf("Confidence = %v, esperado ConfidenceAlta (título exacto, año ±1)", res.Confidence)
	}
}

func TestIsPackHint(t *testing.T) {
	cases := map[string]bool{
		"El Padrino (1972) [Trilogia][Español Latino-Ingles][BluRay 1080p].mp4": true,
		"Money.Heist.S01.Complete.720p.NF.WEBRip.x264-Shadow.mkv":               true,
		"Inception.2010.1080p.BluRay.x264-SPARKS.mkv":                           false,
		"Breaking.Bad.S01E01.1080p.BluRay.x264-DEMAND.mkv":                      false,
	}
	for name, want := range cases {
		if got := isPackHint(name); got != want {
			t.Errorf("isPackHint(%q) = %v, esperado %v", name, got, want)
		}
	}
}

func TestMatchPackNeverAutoConfirms(t *testing.T) {
	client := tmdbRoutes(t, map[string]string{
		"/search/movie": "search_movie_inception.json",
	})
	// Mismo candidato de altísima confianza que TestMatchAltaConfidence,
	// pero el nombre de archivo original delata un pack: debe capar en
	// MEDIA como mucho, nunca auto-confirmar (precisión acordada con el
	// titular del proyecto).
	parsed := Parse("Inception.2010.1080p.BluRay.x264-SPARKS.mkv", "")
	rawPack := "Inception Trilogia (2010) [BluRay 1080p].mkv"

	res, err := Match(context.Background(), client, parsed, rawPack)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if res.Confidence == ConfidenceAlta {
		t.Fatal("Confidence = Alta para un pack; nunca debe auto-confirmarse")
	}
}

// TestMatchNumericGuessNeverAutoConfirms cubre la corrección post-entrega
// de M3: un archivo puramente numérico rescatado por la temporada de la
// carpeta (Episode=0 desconocido) no debe auto-confirmarse aunque el
// título de la carpeta encaje perfectamente con un candidato TMDB.
func TestMatchNumericGuessNeverAutoConfirms(t *testing.T) {
	client := tmdbRoutes(t, map[string]string{
		"/search/tv": "search_tv_breakingbad.json",
	})
	parsed := Parse("1017.mkv", "Breaking.Bad.Temporada.1.COMPLETA")
	if !parsed.LowConfidenceGuess() {
		t.Fatal("LowConfidenceGuess = false; el fixture de este test asume el rescate por carpeta")
	}

	res, err := Match(context.Background(), client, parsed, "1017.mkv")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if res.Confidence == ConfidenceAlta {
		t.Fatal("Confidence = Alta para un episodio adivinado (Episode=0); nunca debe auto-confirmarse")
	}
}

func TestApplyIdentificationDedupe(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{
		"/movie/27205": "movie_27205.json",
	})

	first := insertProvisionalItem(t, ctx, repo, "inception.2010.1080p")
	if err := ApplyIdentification(ctx, client, repo, first, 27205, "movie", Parsed{}); err != nil {
		t.Fatalf("ApplyIdentification (1º): %v", err)
	}
	second := insertProvisionalItem(t, ctx, repo, "inception.2010.bluray-group2")
	if err := ApplyIdentification(ctx, client, repo, second, 27205, "movie", Parsed{}); err != nil {
		t.Fatalf("ApplyIdentification (2º, dedupe): %v", err)
	}

	if _, err := repo.Get(ctx, second); err != catalog.ErrNotFound {
		t.Fatalf("Get(second) = %v, esperado ErrNotFound", err)
	}
	it, err := repo.Get(ctx, first)
	if err != nil {
		t.Fatalf("Get(first): %v", err)
	}
	if it.PosterPath == nil || *it.PosterPath == "" {
		t.Error("PosterPath vacío tras identificar (se esperaba imagen descargada y cacheada)")
	}
	if len(it.Sources) != 2 {
		t.Errorf("Sources = %d, esperado 2 (fusión por dedupe)", len(it.Sources))
	}
}

func TestApplyIdentificationSeriesHierarchy(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{
		"/tv/1396": "tv_1396.json",
	})

	item := insertProvisionalItem(t, ctx, repo, "breaking.bad.s01e01")
	parsed := Parsed{Season: 1, Episode: 1}
	if err := ApplyIdentification(ctx, client, repo, item, 1396, "tv", parsed); err != nil {
		t.Fatalf("ApplyIdentification: %v", err)
	}

	ep, err := repo.Get(ctx, item)
	if err != nil {
		t.Fatalf("Get(episodio): %v", err)
	}
	if ep.Kind != catalog.KindEpisode || ep.EpisodeNumber == nil || *ep.EpisodeNumber != 1 {
		t.Fatalf("episodio = %+v", ep)
	}
	season, err := repo.Get(ctx, *ep.ParentID)
	if err != nil {
		t.Fatalf("Get(temporada): %v", err)
	}
	if season.Title != "Temporada 1" {
		t.Errorf("Title de temporada = %q, esperado 'Temporada 1' (del detalle TMDB)", season.Title)
	}
	series, err := repo.Get(ctx, *season.ParentID)
	if err != nil {
		t.Fatalf("Get(serie): %v", err)
	}
	if series.Title != "Breaking Bad" || series.Overview == nil || *series.Overview == "" {
		t.Errorf("serie = %+v", series)
	}
}

// --- helpers compartidos con repo_identify_test.go (paquete distinto:
// catalog_test usa newTestRepo; aquí, package identify, se define uno propio) ---

func newRepoForTest(t *testing.T) *catalog.Repo {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return catalog.NewRepo(st.DB)
}

func insertProvisionalItem(t *testing.T, ctx context.Context, r *catalog.Repo, title string) int64 {
	t.Helper()
	item := catalog.Item{Kind: catalog.KindOther, Title: title, Genres: []string{}, Confirmed: false}
	id, err := r.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: id, Provider: "localfs", Locator: title + ".mkv"}
	if _, err := r.InsertSource(ctx, &src, ""); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	return id
}
