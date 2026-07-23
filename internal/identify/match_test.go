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

// tmdbRoutesLang crea un cliente de prueba que sirve una fixture distinta
// según el parámetro language de la petición (es-ES vs en-US): necesario
// para probar la búsqueda en dos idiomas del matcher (corrección
// post-entrega M3: TMDB en es-ES trae el título traducido, y solo en en-US
// coincide con el nombre de archivo real cuando la obra no es anglosajona).
func tmdbRoutesLang(t *testing.T, esFixture, enFixture string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("language") == "en-US" {
			w.Write(fixture(t, enFixture))
			return
		}
		w.Write(fixture(t, esFixture))
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

	sExact, sNear, sFar := scoreCandidate(parsed, exact, ""), scoreCandidate(parsed, near, ""), scoreCandidate(parsed, far, "")
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

// TestMatchTranslatedTitleVsHomonymPopularityTiebreak reproduce el bug
// detectado en la prueba real contra TMDB (datos reales de /search/movie
// verificados con curl el 2026-07-23): "Parasite.2019.KOREAN..." se
// auto-confirmaba como la película oscura "Parasites" (id 1493942, popularity
// 0.1874) en vez de "Parásitos"/Parasite 2019 (id 496243, popularity 35.4478),
// porque con language=es-ES el título viene traducido ("Parásitos") y el
// original_title en coreano ("기생충") no se parece en nada al nombre de
// archivo en inglés, mientras que el homónimo oscuro coincide casi letra por
// letra. La búsqueda en en-US (title="Parasite" exacto para 496243) y la
// popularidad (189x mayor en el candidato correcto) deben resolverlo.
func TestMatchTranslatedTitleVsHomonymPopularityTiebreak(t *testing.T) {
	client := tmdbRoutesLang(t, "search_movie_parasite_es.json", "search_movie_parasite_en.json")
	parsed := Parse("Parasite.2019.KOREAN.1080p.BluRay.mkv", "")

	res, err := Match(context.Background(), client, parsed, "Parasite.2019.KOREAN.1080p.BluRay.mkv")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if res.Confidence == ConfidenceAlta && res.Best.ID != 496243 {
		t.Fatalf("auto-confirmado tmdb_id=%d (%q); esperado 496243 (Parásitos/Parasite 2019) o degradar a Media, nunca el homónimo oscuro",
			res.Best.ID, res.Best.EffectiveTitle())
	}
	if res.Confidence != ConfidenceAlta {
		t.Fatalf("Confidence = %v; con búsqueda en dos idiomas y popularidad tan dispar (35.4 vs 0.19) se esperaba ConfidenceAlta sobre el candidato correcto", res.Confidence)
	}
	if res.Best.ID != 496243 {
		t.Fatalf("Best.ID = %d, esperado 496243 (Parásitos)", res.Best.ID)
	}
}

// TestMatchNonAnglophoneAnalogs cubre, con datos reales de TMDB, dos casos
// más de cine no anglosajón con original_title en otro alfabeto (japonés,
// chino), análogos a Parasite: el nombre de archivo está en inglés, TMDB en
// es-ES trae el título traducido y el original en su propio alfabeto (que no
// acerca nada a un nombre de archivo en inglés), y hay un candidato oscuro
// homónimo/relacionado en el propio conjunto de resultados.
func TestMatchNonAnglophoneAnalogs(t *testing.T) {
	cases := []struct {
		name     string
		esFile   string
		enFile   string
		filename string
		wantID   int64
	}{
		// "Spirited Away"/"El viaje de Chihiro" (japonés, 千と千尋の神隠し):
		// sin la búsqueda en inglés, NI el título ni el original_title (ambos
		// en es-ES) se acercan al nombre de archivo real (score < siquiera
		// ConfidenceMedia); ver TestScoreCandidateEnglishTitleFixesUnmatchable.
		{"Spirited Away/El viaje de Chihiro", "search_movie_spiritedaway_es.json", "search_movie_spiritedaway_en.json",
			"Spirited.Away.2001.1080p.BluRay.x264-GROUP.mkv", 129},
		// "Crouching Tiger, Hidden Dragon"/"Tigre y dragón" (chino, 卧虎藏龍):
		// el "making of" homónimo (nunca traducido, título en inglés en
		// es-ES también) puntúa más que "Tigre y dragón" sin la búsqueda en
		// inglés.
		{"Crouching Tiger/Tigre y dragón", "search_movie_crouchingtiger_es.json", "search_movie_crouchingtiger_en.json",
			"Crouching.Tiger.Hidden.Dragon.2000.1080p.BluRay.x264-GROUP.mkv", 146},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := tmdbRoutesLang(t, tc.esFile, tc.enFile)
			parsed := Parse(tc.filename, "")
			res, err := Match(context.Background(), client, parsed, tc.filename)
			if err != nil {
				t.Fatalf("Match: %v", err)
			}
			if res.Confidence != ConfidenceAlta {
				t.Fatalf("Confidence = %v, esperado ConfidenceAlta (título en-US coincide con el nombre de archivo)", res.Confidence)
			}
			if res.Best == nil || res.Best.ID != tc.wantID {
				t.Fatalf("Best = %+v, esperado tmdb_id %d", res.Best, tc.wantID)
			}
		})
	}
}

// TestScoreCandidateEnglishTitleFixesUnmatchable prueba scoreCandidate
// directamente (sin HTTP) con los datos reales de Spirited Away: sin el
// título en inglés, NI el título ES ("El viaje de Chihiro") ni el original
// japonés ("千と千尋の神隠し") se parecen al nombre de archivo real
// ("Spirited Away") lo suficiente como para superar ni siquiera
// ThresholdMedia. El título en-US lo resuelve de sobra.
func TestScoreCandidateEnglishTitleFixesUnmatchable(t *testing.T) {
	parsed := Parsed{Title: "Spirited Away", Year: 2001}
	candidate := SearchResult{Title: "El viaje de Chihiro", OriginalTitle: "千と千尋の神隠し", ReleaseDate: "2001-07-20"}

	withoutEN := scoreCandidate(parsed, candidate, "")
	if withoutEN >= ThresholdMedia {
		t.Fatalf("withoutEN = %.4f, esperado < ThresholdMedia (%.2f): sin título en inglés, este caso real ni siquiera debería llegar a MEDIA", withoutEN, ThresholdMedia)
	}

	withEN := scoreCandidate(parsed, candidate, "Spirited Away")
	if withEN < ThresholdAlta {
		t.Fatalf("withEN = %.4f, esperado >= ThresholdAlta (%.2f) con coincidencia exacta de título en inglés", withEN, ThresholdAlta)
	}
}

// TestIsPopularityRiskIndependentOfScoreGap prueba isPopularityRisk
// directamente: debe degradar aunque el ganador tenga el score muy por
// delante del resto (gap >= ambiguousScoreGap, así que resolveByPopularity
// ni entraría) si su popularidad es marginal frente a otro candidato del
// mismo conjunto — la red de seguridad pedida en la corrección post-entrega
// M3 es independiente del desempate por score reñido.
func TestIsPopularityRiskIndependentOfScoreGap(t *testing.T) {
	obscureWinner := scoredCandidate{result: SearchResult{ID: 1, Title: "Homónimo Oscuro", Popularity: 0.2}, score: 1.5}
	popularLoser := scoredCandidate{result: SearchResult{ID: 2, Title: "Otra Cosa", Popularity: 40.0}, score: 0.9}
	gap := obscureWinner.score - popularLoser.score
	if gap < ambiguousScoreGap {
		t.Fatalf("gap = %.2f, este test necesita un gap >= ambiguousScoreGap (%.2f) para probar que la red es independiente", gap, ambiguousScoreGap)
	}
	if !isPopularityRisk([]scoredCandidate{obscureWinner, popularLoser}, obscureWinner) {
		t.Fatal("isPopularityRisk = false; esperado true: el ganador tiene una popularidad ínfima frente a otro candidato del conjunto")
	}

	// Caso sano: el ganador ES el más popular del conjunto (ratio 1) -> sin riesgo.
	healthyWinner := scoredCandidate{result: SearchResult{ID: 3, Title: "La Real", Popularity: 40.0}, score: 1.5}
	other := scoredCandidate{result: SearchResult{ID: 4, Title: "Otra", Popularity: 5.0}, score: 0.9}
	if isPopularityRisk([]scoredCandidate{healthyWinner, other}, healthyWinner) {
		t.Fatal("isPopularityRisk = true para un ganador que ya es el más popular del conjunto; no debería haber riesgo")
	}
}

// TestMatchTranslatedTitleAnalogs cubre 2-3 análogos de título traducido
// pedidos en la corrección post-entrega de M3: el nombre de archivo está en
// inglés (original_title), pero TMDB en es-ES solo trae el título traducido
// en el campo "title"; el matcher debe seguir comparando también contra
// original_title y llegar a ConfidenceAlta.
func TestMatchTranslatedTitleAnalogs(t *testing.T) {
	cases := []struct {
		name     string
		fixture  string
		filename string
		wantID   int64
	}{
		{"Inception/Origen", "search_movie_inception.json", "Inception.2010.1080p.BluRay.x264-SPARKS.mkv", 27205},
		{"Godfather II/El Padrino II", "search_movie_godfather2.json", "The.Godfather.Part.II.1974.1080p.BluRay.mkv", 240},
		{"Avengers Endgame/Vengadores", "search_movie_endgame.json", "Avengers.Endgame.2019.2160p.UHD.BluRay.x265-GROUP.mkv", 299534},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := tmdbRoutes(t, map[string]string{
				"/search/movie": tc.fixture,
			})
			parsed := Parse(tc.filename, "")
			res, err := Match(context.Background(), client, parsed, tc.filename)
			if err != nil {
				t.Fatalf("Match: %v", err)
			}
			if res.Confidence != ConfidenceAlta {
				t.Fatalf("Confidence = %v, esperado ConfidenceAlta (título original coincide vía original_title)", res.Confidence)
			}
			if res.Best == nil || res.Best.ID != tc.wantID {
				t.Fatalf("Best = %+v, esperado tmdb_id %d", res.Best, tc.wantID)
			}
		})
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
