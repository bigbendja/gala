package identify

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

// Un item nacido de un .strm se identifica por el NOMBRE del .strm (no por la
// URL de su fuente http), con el mismo normalizador de M3.
func TestQueueIdentifiesStrmByFileName(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	client := tmdbRoutes(t, map[string]string{
		"/movie/27205":  "movie_27205.json",
		"/search/movie": "search_movie_inception.json",
	})

	item := catalog.Item{Kind: catalog.KindOther, Title: "Inception.2010.1080p.BluRay.x264-SPARKS", Genres: []string{}}
	id, err := repo.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatal(err)
	}
	origin := "D:/Pelis/Inception.2010.1080p.BluRay.x264-SPARKS.strm"
	src := catalog.Source{ItemID: id, Provider: "http", Locator: "https://cdn.example.com/v/9f3a1c.mp4", OriginPath: &origin}
	if _, err := repo.InsertSource(ctx, &src, ""); err != nil {
		t.Fatal(err)
	}

	NewQueue(ctx, repo, client, discardLogger()).drain()

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Confirmed || got.Kind != catalog.KindMovie || got.IdentifyStatus != catalog.IdentifyOK {
		t.Fatalf("item tras drain = %+v", got)
	}
}

// La herencia de carpeta de M3 funciona igual con la extensión .strm.
func TestParseStrmInheritsFolderLikeVideos(t *testing.T) {
	strm := Parse("S02E05.strm", "Breaking Bad")
	video := Parse("S02E05.mkv", "Breaking Bad")
	if strm.Kind != video.Kind || strm.Season != video.Season || strm.Episode != video.Episode || strm.Title != video.Title {
		t.Fatalf(".strm=%+v vídeo=%+v: deben normalizarse igual", strm, video)
	}
	if strm.Season != 2 || strm.Episode != 5 || strm.Title == "" {
		t.Fatalf("parse = %+v", strm)
	}
}
