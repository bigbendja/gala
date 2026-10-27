package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func identifyEpisode(t *testing.T, ctx context.Context, r *catalog.Repo, name string, season, episode int) int64 {
	t.Helper()
	id := insertProvisional(t, ctx, r, name)
	if err := r.ApplyEpisodeIdentification(ctx, id, 1396, "Breaking Bad", nil, nil, nil, nil,
		[]string{"Drama"}, season, episode, "", "", ""); err != nil {
		t.Fatalf("ApplyEpisodeIdentification: %v", err)
	}
	return id
}

// Un episodio con el título provisional de M3 ("Serie T1E01") está pendiente
// de su detalle real; al ponérselo deja de estarlo (auto-terminante).
func TestEpisodesNeedingDetailAndUpdate(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	e1 := identifyEpisode(t, ctx, r, "bb.s01e01", 1, 1)
	identifyEpisode(t, ctx, r, "bb.s02e10", 2, 10)

	refs, err := r.EpisodesNeedingDetail(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[0].SeriesTMDBID != 1396 || refs[0].Season != 1 || refs[0].Episode != 1 ||
		refs[1].Season != 2 || refs[1].Episode != 10 {
		t.Fatalf("pendientes = %+v", refs)
	}

	overview, still := "Un profesor de química...", "still_w300_abc.jpg"
	if err := r.UpdateEpisodeDetail(ctx, e1, "Piloto", &overview, &still); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, e1)
	if got.Title != "Piloto" || got.Overview == nil || *got.Overview != overview ||
		got.PosterPath == nil || *got.PosterPath != still {
		t.Fatalf("episodio = %+v", got)
	}
	refs, _ = r.EpisodesNeedingDetail(ctx)
	if len(refs) != 1 || refs[0].Episode != 10 {
		t.Fatalf("tras actualizar quedan = %+v, esperado solo el 2x10", refs)
	}

	// Sin sinopsis ni miniatura de TMDB: no pisa lo que ya había con NULL.
	if err := r.UpdateEpisodeDetail(ctx, e1, "Piloto (2)", nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Get(ctx, e1)
	if got.Overview == nil || got.PosterPath == nil {
		t.Errorf("UpdateEpisodeDetail con nil borró la sinopsis/miniatura: %+v", got)
	}
}

// Los episodios y temporadas dicen de qué serie son.
func TestAttachSeriesInfo(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	e1 := identifyEpisode(t, ctx, r, "bb.s01e01", 1, 1)
	movie := insertProvisional(t, ctx, r, "peli")

	ep, _ := r.Get(ctx, e1)
	season, _ := r.Get(ctx, *ep.ParentID)
	m, _ := r.Get(ctx, movie)
	items := []catalog.Item{*ep, *season, *m}
	if err := r.AttachSeriesInfo(ctx, items); err != nil {
		t.Fatal(err)
	}
	for _, it := range items[:2] {
		if it.SeriesTitle == nil || *it.SeriesTitle != "Breaking Bad" || it.SeriesID == nil {
			t.Errorf("%s sin serie: %+v", it.Kind, it)
		}
	}
	if items[0].SeasonNumber == nil || *items[0].SeasonNumber != 1 {
		t.Errorf("el episodio debe saber su temporada: %+v", items[0].SeasonNumber)
	}
	if items[2].SeriesTitle != nil {
		t.Errorf("una película no tiene serie: %+v", items[2])
	}
	if err := r.AttachSeriesInfo(ctx, nil); err != nil {
		t.Errorf("lista vacía: %v", err)
	}
}

// Genres cuenta solo lo que se puede enseñar: películas con fuente y series.
func TestGenres(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	add := func(kind catalog.Kind, title string, genres []string, withSource bool, status string) {
		t.Helper()
		it := catalog.Item{Kind: kind, Title: title, Genres: genres, Confirmed: true, IdentifyStatus: status}
		id, err := r.InsertItem(ctx, &it, "")
		if err != nil {
			t.Fatal(err)
		}
		if withSource {
			if _, err := r.InsertSource(ctx, &catalog.Source{ItemID: id, Provider: "http", Locator: "https://a.example/" + title}, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(catalog.KindMovie, "a", []string{"Drama", "Documental"}, true, catalog.IdentifyOK)
	add(catalog.KindMovie, "b", []string{"Drama"}, true, catalog.IdentifyOK)
	add(catalog.KindMovie, "sin-fuente", []string{"Terror"}, false, catalog.IdentifyOK) // fantasma: no cuenta
	add(catalog.KindSeries, "serie", []string{"Drama", "Crimen"}, false, catalog.IdentifyOK)
	add(catalog.KindMovie, "basura", []string{"Comedia"}, true, "junk")

	got, err := r.Genres(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"Drama": 3, "Documental": 1, "Crimen": 1}
	if len(got) != len(want) {
		t.Fatalf("géneros = %+v", got)
	}
	for _, g := range got {
		if want[g.Name] != g.Count {
			t.Errorf("%s = %d, esperado %d", g.Name, g.Count, want[g.Name])
		}
	}
	if got[0].Name != "Drama" {
		t.Errorf("orden: %+v (el género con más títulos va primero)", got)
	}
}

func TestGenresEmptyIsNotNil(t *testing.T) {
	r := newTestRepo(t)
	got, err := r.Genres(context.Background())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v, esperado [] no nil", got, err)
	}
}
