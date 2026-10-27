package identify

import (
	"context"
	"fmt"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func seedEpisode(t *testing.T, ctx context.Context, repo *catalog.Repo, season, episode int) int64 {
	t.Helper()
	id := insertProvisionalItem(t, ctx, repo, fmt.Sprintf("breaking.bad.s%02de%02d", season, episode))
	if err := repo.ApplyEpisodeIdentification(ctx, id, 1396, "Breaking Bad", nil, nil, nil, nil,
		[]string{"Drama"}, season, episode, "", "", ""); err != nil {
		t.Fatalf("ApplyEpisodeIdentification: %v", err)
	}
	return id
}

// Los episodios con el título provisional ("Breaking Bad T1E01") reciben su
// título real, sinopsis y miniatura de TMDB; el que TMDB no conoce queda como
// "Episodio N" y deja de ser candidato (no se reintenta sin fin).
func TestEnrichEpisodesFillsRealTitles(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{"/tv/1396/season/1": "tv_1396_season_1.json"})
	e1 := seedEpisode(t, ctx, repo, 1, 1)
	e2 := seedEpisode(t, ctx, repo, 1, 2)
	e9 := seedEpisode(t, ctx, repo, 1, 9) // TMDB no lo conoce

	q := NewQueue(ctx, repo, client, discardLogger())
	q.enrichEpisodes()

	got1, _ := repo.Get(ctx, e1)
	if got1.Title != "Piloto" || got1.Overview == nil || got1.PosterPath == nil {
		t.Fatalf("episodio 1 = %+v", got1)
	}
	got2, _ := repo.Get(ctx, e2)
	if got2.Title != "El gato está en la bolsa..." {
		t.Errorf("episodio 2 = %q", got2.Title)
	}
	if got2.PosterPath != nil {
		t.Errorf("sin still_path no debe haber miniatura: %v", *got2.PosterPath)
	}
	got9, _ := repo.Get(ctx, e9)
	if got9.Title != "Episodio 9" {
		t.Errorf("episodio desconocido = %q, esperado \"Episodio 9\"", got9.Title)
	}

	if refs, _ := repo.EpisodesNeedingDetail(ctx); len(refs) != 0 {
		t.Errorf("siguen pendientes %d episodios", len(refs))
	}
}
