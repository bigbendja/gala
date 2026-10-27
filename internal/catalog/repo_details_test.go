package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func addMovie(t *testing.T, ctx context.Context, r *catalog.Repo, title string, tmdb int64, withSource bool) int64 {
	t.Helper()
	id := tmdb
	it := catalog.Item{Kind: catalog.KindMovie, Title: title, TMDBID: &id, Genres: []string{}, Confirmed: true, IdentifyStatus: catalog.IdentifyOK}
	itemID, err := r.InsertItem(ctx, &it, "")
	if err != nil {
		t.Fatal(err)
	}
	if withSource {
		if _, err := r.InsertSource(ctx, &catalog.Source{ItemID: itemID, Provider: "localfs", Locator: "C:/x/" + title + ".mkv"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	return itemID
}

func TestDetailsRoundTripAndLifecycle(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	id := addMovie(t, ctx, r, "Origen", 27205, true)

	if _, err := r.GetDetails(ctx, id); err != catalog.ErrNotFound {
		t.Fatalf("sin detalles = %v, esperado ErrNotFound", err)
	}
	need, _ := r.ItemsNeedingDetails(ctx)
	if len(need) != 1 || need[0].ID != id {
		t.Fatalf("pendientes = %+v", need)
	}

	runtime, vote := 148, 8.4
	err := r.UpsertDetails(ctx, &catalog.Details{
		ItemID: id, TMDBKind: "movie", RuntimeMin: &runtime, VoteAverage: &vote, OriginalLanguage: "en",
		Countries: []string{"Reino Unido"}, Tagline: "Tu mente…",
		Cast:    []catalog.Person{{ID: 1, Name: "Leonardo DiCaprio", Character: "Cobb", ProfilePath: "/leo.jpg"}},
		Crew:    []catalog.CrewGroup{{Role: "Dirección", Names: []string{"Christopher Nolan"}}},
		Similar: []catalog.SimilarTitle{{TMDBID: 157336, Kind: "movie", Title: "Interstellar", Year: 2014, PosterPath: "/i.jpg"}},

		ImagesPending: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.GetDetails(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if *got.RuntimeMin != 148 || *got.VoteAverage != 8.4 || got.Cast[0].Name != "Leonardo DiCaprio" ||
		got.Crew[0].Names[0] != "Christopher Nolan" || got.Similar[0].Title != "Interstellar" || !got.ImagesPending {
		t.Fatalf("detalles = %+v", got)
	}
	if need, _ := r.ItemsNeedingDetails(ctx); len(need) != 0 {
		t.Errorf("tras guardar ya no está pendiente: %+v", need)
	}

	// Imágenes pendientes con tope de intentos.
	pend, _ := r.DetailsNeedingImages(ctx, 3)
	if len(pend) != 1 {
		t.Fatalf("pendientes de imagen = %d", len(pend))
	}
	got.ImageAttempts = 3
	if err := r.UpsertDetails(ctx, got); err != nil {
		t.Fatal(err)
	}
	if pend, _ := r.DetailsNeedingImages(ctx, 3); len(pend) != 0 {
		t.Errorf("con 3 intentos ya no se reintenta: %d", len(pend))
	}

	// Borrar el item borra sus detalles (ON DELETE CASCADE).
	if err := r.DeleteItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetDetails(ctx, id); err != catalog.ErrNotFound {
		t.Errorf("tras borrar el item: %v", err)
	}
}

func TestUpsertDetailsNeverStoresNullLists(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	id := addMovie(t, ctx, r, "Sola", 1, true)
	if err := r.UpsertDetails(ctx, &catalog.Details{ItemID: id, TMDBKind: "movie"}); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetDetails(ctx, id)
	if got.Cast == nil || got.Crew == nil || got.Similar == nil || got.Countries == nil {
		t.Errorf("listas nulas: %+v", got)
	}
}

func TestLibraryByTMDBOnlyOpenableTitles(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	conFuente := addMovie(t, ctx, r, "Con fuente", 10, true)
	addMovie(t, ctx, r, "Fantasma", 20, false) // sin fuente: no se puede abrir
	serie := tmdbSeries(t, ctx, r, "Serie", 30)

	lib, err := r.LibraryByTMDB(ctx, []int64{10, 20, 30, 99})
	if err != nil {
		t.Fatal(err)
	}
	if len(lib) != 2 || lib[10].ItemID != conFuente || lib[30].ItemID != serie || lib[30].Kind != catalog.KindSeries {
		t.Errorf("biblioteca = %+v", lib)
	}
	if empty, _ := r.LibraryByTMDB(ctx, nil); len(empty) != 0 {
		t.Errorf("sin ids: %+v", empty)
	}
}

func tmdbSeries(t *testing.T, ctx context.Context, r *catalog.Repo, title string, tmdb int64) int64 {
	t.Helper()
	id := tmdb
	it := catalog.Item{Kind: catalog.KindSeries, Title: title, TMDBID: &id, Genres: []string{}, Confirmed: true}
	itemID, err := r.InsertItem(ctx, &it, "")
	if err != nil {
		t.Fatal(err)
	}
	return itemID
}

func TestRatingCertOnlyFillsEmptyAndBackdropSwap(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	id := addMovie(t, ctx, r, "Peli", 5, true)
	if err := r.SetRatingCert(ctx, id, "12"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetRatingCert(ctx, id, "18"); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, id)
	if got.RatingCert == nil || *got.RatingCert != "12" {
		t.Errorf("rating_cert = %v (no se pisa uno ya guardado)", got.RatingCert)
	}

	if err := r.SetBackdrop(ctx, id, "backdrop_w780_abc.jpg"); err != nil {
		t.Fatal(err)
	}
	small, _ := r.ItemsWithSmallBackdrop(ctx)
	if len(small) != 1 {
		t.Fatalf("fondos pequeños = %d", len(small))
	}
	if err := r.SetBackdrop(ctx, id, "backdrop_w1280_abc.jpg"); err != nil {
		t.Fatal(err)
	}
	if small, _ := r.ItemsWithSmallBackdrop(ctx); len(small) != 0 {
		t.Errorf("tras mejorar ya no figura: %d", len(small))
	}
}

func TestMediaFilesForItem(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	id := addMovie(t, ctx, r, "Archivo", 6, true)
	it, _ := r.Get(ctx, id)
	srcID := it.Sources[0].ID
	if err := r.UpsertMediaInfo(ctx, catalog.MediaInfo{SourceID: srcID, Container: "matroska", VideoCodec: "h264",
		AudioCodec: "aac", Width: 1920, Height: 1080, Bitrate: 5_000_000, DurationS: 7920}); err != nil {
		t.Fatal(err)
	}
	files, err := r.MediaFilesForItem(ctx, id)
	if err != nil || len(files) != 1 || files[0].Width != 1920 || files[0].VideoCodec != "h264" || files[0].DurationS != 7920 {
		t.Fatalf("ficha técnica = %+v (%v)", files, err)
	}
}
