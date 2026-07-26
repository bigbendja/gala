package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

// TestListParentIDFiltersHierarchy cubre el filtro parent_id/TopLevelOnly
// añadido para que la ficha de una serie pueda listar sus temporadas y
// episodios directos (Brief M10a-fix B1).
func TestListParentIDFiltersHierarchy(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	num := func(i int) *int { return &i }

	seriesID, err := r.InsertItem(ctx, &catalog.Item{
		Kind: catalog.KindSeries, Title: "Ruta 3", Genres: []string{}, Confirmed: true,
	}, "")
	if err != nil {
		t.Fatalf("InsertItem serie: %v", err)
	}
	seasonID, err := r.InsertItem(ctx, &catalog.Item{
		Kind: catalog.KindSeason, ParentID: &seriesID, SeasonNumber: num(1),
		Title: "Temporada 1", Genres: []string{}, Confirmed: true,
	}, "")
	if err != nil {
		t.Fatalf("InsertItem temporada: %v", err)
	}
	episodeID, err := r.InsertItem(ctx, &catalog.Item{
		Kind: catalog.KindEpisode, ParentID: &seasonID, SeasonNumber: num(1), EpisodeNumber: num(1),
		Title: "T1E1", Genres: []string{}, Confirmed: true,
	}, "")
	if err != nil {
		t.Fatalf("InsertItem episodio: %v", err)
	}
	movieID, err := r.InsertItem(ctx, &catalog.Item{
		Kind: catalog.KindMovie, Title: "Película Suelta", Genres: []string{}, Confirmed: true,
	}, "")
	if err != nil {
		t.Fatalf("InsertItem película: %v", err)
	}

	// Sin filtro: todo mezclado (comportamiento previo, compatibilidad).
	all, err := r.List(ctx, catalog.ListFilter{})
	if err != nil {
		t.Fatalf("List sin filtro: %v", err)
	}
	if all.Total != 4 {
		t.Fatalf("List sin filtro = %d, esperado 4", all.Total)
	}

	// Hijos directos de la serie: solo la temporada.
	seasons, err := r.List(ctx, catalog.ListFilter{ParentID: &seriesID})
	if err != nil {
		t.Fatalf("List(parent_id=serie): %v", err)
	}
	if seasons.Total != 1 || seasons.Items[0].ID != seasonID {
		t.Fatalf("temporadas = %+v, esperado solo %d", seasons.Items, seasonID)
	}

	// Hijos directos de la temporada: solo el episodio (no el episodio Y la
	// temporada mezclados; parent_id es de UN nivel, no recursivo).
	episodes, err := r.List(ctx, catalog.ListFilter{ParentID: &seasonID})
	if err != nil {
		t.Fatalf("List(parent_id=temporada): %v", err)
	}
	if episodes.Total != 1 || episodes.Items[0].ID != episodeID {
		t.Fatalf("episodios = %+v, esperado solo %d", episodes.Items, episodeID)
	}

	// Un id sin hijos: vacío, no error.
	none, err := r.List(ctx, catalog.ListFilter{ParentID: &episodeID})
	if err != nil {
		t.Fatalf("List(parent_id=episodio): %v", err)
	}
	if none.Total != 0 {
		t.Fatalf("hijos de un episodio = %d, esperado 0", none.Total)
	}

	// Solo nivel superior: la serie y la película, NUNCA temporada/episodio.
	topLevel, err := r.List(ctx, catalog.ListFilter{TopLevelOnly: true})
	if err != nil {
		t.Fatalf("List(TopLevelOnly): %v", err)
	}
	if topLevel.Total != 2 {
		t.Fatalf("nivel superior = %d, esperado 2 (serie + película)", topLevel.Total)
	}
	for _, it := range topLevel.Items {
		if it.ID == seasonID || it.ID == episodeID {
			t.Fatalf("TopLevelOnly incluyó un hijo: %+v", it)
		}
	}
	if movieID == 0 {
		t.Fatal("movieID no asignado")
	}
}
