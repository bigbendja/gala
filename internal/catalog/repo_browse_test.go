package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func addBrowseMovie(t *testing.T, ctx context.Context, r *catalog.Repo, title string, year int, genres []string, vote *float64, withSource bool) int64 {
	t.Helper()
	tmdb := int64(1000 + year)
	y := year
	it := catalog.Item{Kind: catalog.KindMovie, Title: title, TMDBID: &tmdb, Year: &y, Genres: genres, Confirmed: true, IdentifyStatus: catalog.IdentifyOK}
	id, err := r.InsertItem(ctx, &it, "")
	if err != nil {
		t.Fatal(err)
	}
	if withSource {
		if _, err := r.InsertSource(ctx, &catalog.Source{ItemID: id, Provider: "localfs", Locator: "C:/x/" + title + ".mkv"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	if vote != nil {
		if err := r.UpsertDetails(ctx, &catalog.Details{ItemID: id, TMDBKind: "movie", VoteAverage: vote}); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func titles(res *catalog.ListResult) []string {
	out := []string{}
	for _, it := range res.Items {
		out = append(out, it.Title)
	}
	return out
}

func TestListSortAndVotes(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	v82, v61 := 8.2, 6.1
	addBrowseMovie(t, ctx, r, "Beta", 2019, []string{"Drama"}, &v61, true)
	addBrowseMovie(t, ctx, r, "alfa", 2008, []string{"Drama"}, &v82, true)
	addBrowseMovie(t, ctx, r, "Gamma", 2021, []string{"Drama"}, nil, true)

	cases := map[string][]string{
		"year":   {"Gamma", "Beta", "alfa"},
		"title":  {"alfa", "Beta", "Gamma"},
		"rating": {"alfa", "Beta", "Gamma"}, // la que aún no tiene nota, al final
	}
	for sort, want := range cases {
		res, err := r.List(ctx, catalog.ListFilter{Kind: "movie", Sort: sort, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		got := titles(res)
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("sort=%s: %v, esperado %v", sort, got, want)
				break
			}
		}
	}

	res, _ := r.List(ctx, catalog.ListFilter{Kind: "movie", Sort: "rating", Limit: 50})
	if res.Items[0].VoteAverage == nil || *res.Items[0].VoteAverage != 8.2 {
		t.Errorf("la nota debe salir en el listado: %+v", res.Items[0].VoteAverage)
	}
	if res.Items[2].VoteAverage != nil {
		t.Errorf("sin detalle no hay nota: %v", *res.Items[2].VoteAverage)
	}
}

func TestListPlayableAndFacets(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	addBrowseMovie(t, ctx, r, "Con fuente", 2019, []string{"Drama", "Acción"}, nil, true)
	addBrowseMovie(t, ctx, r, "Sin fuente", 2010, []string{"Drama"}, nil, false)
	tmdb := int64(9)
	y := 2008
	series := catalog.Item{Kind: catalog.KindSeries, Title: "Serie", TMDBID: &tmdb, Year: &y, Genres: []string{"Drama"}, Confirmed: true, IdentifyStatus: catalog.IdentifyOK}
	if _, err := r.InsertItem(ctx, &series, ""); err != nil {
		t.Fatal(err)
	}

	res, _ := r.List(ctx, catalog.ListFilter{Kind: "movie", Playable: true, Limit: 50})
	if res.Total != 1 || res.Items[0].Title != "Con fuente" {
		t.Errorf("playable (películas) = %v, total %d", titles(res), res.Total)
	}
	res, _ = r.List(ctx, catalog.ListFilter{Kind: "series", Playable: true, Limit: 50})
	if res.Total != 1 {
		t.Errorf("una serie no necesita fuente propia: total %d", res.Total)
	}

	f, err := r.Facets(ctx, catalog.KindMovie)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Years) != 1 || f.Years[0] != 2019 {
		t.Errorf("años de películas = %v (solo lo que tiene fuente)", f.Years)
	}
	if len(f.Genres) != 2 || f.Genres[0].Name != "Acción" && f.Genres[0].Name != "Drama" {
		t.Errorf("géneros = %+v", f.Genres)
	}
	fs, _ := r.Facets(ctx, catalog.KindSeries)
	if len(fs.Genres) != 1 || fs.Genres[0].Name != "Drama" || len(fs.Years) != 1 || fs.Years[0] != 2008 {
		t.Errorf("facetas de series = %+v", fs)
	}
	if _, err := r.Facets(ctx, catalog.KindEpisode); err == nil {
		t.Error("kind no válido debe dar error")
	}
	empty, _ := newTestRepo(t).Facets(ctx, catalog.KindMovie)
	if empty.Genres == nil || empty.Years == nil {
		t.Error("las listas nunca son null")
	}
}
