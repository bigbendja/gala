package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func insertProvisional(t *testing.T, ctx context.Context, r *catalog.Repo, title string) int64 {
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

func TestApplyMovieIdentification(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	id := insertProvisional(t, ctx, r, "inception.2010.1080p")
	overview := "Un ladrón que roba secretos corporativos..."
	if err := r.ApplyMovieIdentification(ctx, id, 27205, "Origen", nil, &overview, nil, nil, nil,
		nil, []string{"Ciencia ficción"}, ""); err != nil {
		t.Fatalf("ApplyMovieIdentification: %v", err)
	}

	it, err := r.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.Kind != catalog.KindMovie || !it.Confirmed || it.TMDBID == nil || *it.TMDBID != 27205 {
		t.Fatalf("item tras identificar = %+v", it)
	}
	if it.IdentifyStatus != catalog.IdentifyOK {
		t.Errorf("IdentifyStatus = %q, esperado %q", it.IdentifyStatus, catalog.IdentifyOK)
	}
	if len(it.Sources) != 1 {
		t.Errorf("Sources = %d, esperado 1 (se conserva la fuente original)", len(it.Sources))
	}
}

func TestApplyMovieIdentificationDedupe(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	first := insertProvisional(t, ctx, r, "inception.2010.1080p")
	if err := r.ApplyMovieIdentification(ctx, first, 27205, "Origen", nil, nil, nil, nil, nil,
		nil, []string{}, ""); err != nil {
		t.Fatalf("ApplyMovieIdentification (1º): %v", err)
	}

	second := insertProvisional(t, ctx, r, "inception.2010.bluray-group2")
	if err := r.ApplyMovieIdentification(ctx, second, 27205, "Origen", nil, nil, nil, nil, nil,
		nil, []string{}, ""); err != nil {
		t.Fatalf("ApplyMovieIdentification (2º, dedupe): %v", err)
	}

	// El segundo provisional debe haber desaparecido: sus fuentes se
	// movieron al primero (Brief §3, dedupe por tmdb_id).
	if _, err := r.Get(ctx, second); err != catalog.ErrNotFound {
		t.Fatalf("Get(second) = %v, esperado ErrNotFound (dedupe debe borrar el provisional)", err)
	}
	it, err := r.Get(ctx, first)
	if err != nil {
		t.Fatalf("Get(first): %v", err)
	}
	if len(it.Sources) != 2 {
		t.Errorf("Sources = %d, esperado 2 (fuentes de ambos provisionales fusionadas)", len(it.Sources))
	}
}

func TestApplyEpisodeIdentificationHierarchy(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	e1 := insertProvisional(t, ctx, r, "breaking.bad.s01e01")
	e2 := insertProvisional(t, ctx, r, "breaking.bad.s01e02")

	apply := func(itemID int64, season, episode int) {
		t.Helper()
		if err := r.ApplyEpisodeIdentification(ctx, itemID, 1396, "Breaking Bad", nil, nil, nil, nil,
			[]string{"Drama"}, season, episode, "", "", ""); err != nil {
			t.Fatalf("ApplyEpisodeIdentification: %v", err)
		}
	}
	apply(e1, 1, 1)
	apply(e2, 1, 2)

	it1, err := r.Get(ctx, e1)
	if err != nil {
		t.Fatalf("Get(e1): %v", err)
	}
	if it1.Kind != catalog.KindEpisode || it1.EpisodeNumber == nil || *it1.EpisodeNumber != 1 {
		t.Fatalf("e1 = %+v", it1)
	}
	if it1.ParentID == nil {
		t.Fatal("e1.ParentID vacío: debería colgar de la temporada")
	}

	season, err := r.Get(ctx, *it1.ParentID)
	if err != nil {
		t.Fatalf("Get(temporada): %v", err)
	}
	if season.Kind != catalog.KindSeason || season.SeasonNumber == nil || *season.SeasonNumber != 1 {
		t.Fatalf("temporada = %+v", season)
	}
	if season.ParentID == nil {
		t.Fatal("temporada.ParentID vacío: debería colgar de la serie")
	}

	series, err := r.Get(ctx, *season.ParentID)
	if err != nil {
		t.Fatalf("Get(serie): %v", err)
	}
	if series.Kind != catalog.KindSeries || series.TMDBID == nil || *series.TMDBID != 1396 {
		t.Fatalf("serie = %+v", series)
	}

	it2, err := r.Get(ctx, e2)
	if err != nil {
		t.Fatalf("Get(e2): %v", err)
	}
	if it2.ParentID == nil || *it2.ParentID != *it1.ParentID {
		t.Error("e1 y e2 deberían compartir la misma temporada (reutilizada, no duplicada)")
	}
}

func TestApplyEpisodeIdentificationDedupe(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	first := insertProvisional(t, ctx, r, "breaking.bad.s01e01.720p")
	if err := r.ApplyEpisodeIdentification(ctx, first, 1396, "Breaking Bad", nil, nil, nil, nil,
		nil, 1, 1, "", "", ""); err != nil {
		t.Fatalf("ApplyEpisodeIdentification (1º): %v", err)
	}

	second := insertProvisional(t, ctx, r, "breaking.bad.s01e01.1080p")
	if err := r.ApplyEpisodeIdentification(ctx, second, 1396, "Breaking Bad", nil, nil, nil, nil,
		nil, 1, 1, "", "", ""); err != nil {
		t.Fatalf("ApplyEpisodeIdentification (2º, dedupe): %v", err)
	}

	if _, err := r.Get(ctx, second); err != catalog.ErrNotFound {
		t.Fatalf("Get(second) = %v, esperado ErrNotFound", err)
	}
	it, err := r.Get(ctx, first)
	if err != nil {
		t.Fatalf("Get(first): %v", err)
	}
	if len(it.Sources) != 2 {
		t.Errorf("Sources = %d, esperado 2 (720p + 1080p fusionadas en el mismo episodio)", len(it.Sources))
	}
}

func TestApplyManualConfirm(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	id := insertProvisional(t, ctx, r, "video_raro.mp4")
	if err := r.ApplyManualConfirm(ctx, id, "Título elegido a mano", ""); err != nil {
		t.Fatalf("ApplyManualConfirm: %v", err)
	}
	it, err := r.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.Title != "Título elegido a mano" || !it.Confirmed || it.TMDBID != nil {
		t.Fatalf("item tras confirmación manual = %+v", it)
	}
}

func TestSetIdentifyCandidatesAndPendingList(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	id := insertProvisional(t, ctx, r, "peli_dudosa.mkv")
	cands := []catalog.Candidate{
		{TMDBID: 1, Kind: "movie", Title: "Peli Dudosa", Year: 2020, Score: 0.7},
		{TMDBID: 2, Kind: "movie", Title: "Peli Dudosa 2", Year: 2021, Score: 0.65},
	}
	if err := r.SetIdentifyCandidates(ctx, id, cands, 0, 0, nil); err != nil {
		t.Fatalf("SetIdentifyCandidates: %v", err)
	}

	pending, err := r.ListPendingConfirmation(ctx)
	if err != nil {
		t.Fatalf("ListPendingConfirmation: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, esperado 1", len(pending))
	}
	if pending[0].IdentifyStatus != catalog.IdentifyReview {
		t.Errorf("IdentifyStatus = %q, esperado %q", pending[0].IdentifyStatus, catalog.IdentifyReview)
	}
	if len(pending[0].Candidates) != 2 {
		t.Fatalf("Candidates = %d, esperado 2", len(pending[0].Candidates))
	}

	// PendingIdentify (la cola) NO debe incluir items en estado "review":
	// es un estado terminal hasta que el usuario confirme a mano.
	queueItems, err := r.PendingIdentify(ctx)
	if err != nil {
		t.Fatalf("PendingIdentify: %v", err)
	}
	for _, it := range queueItems {
		if it.ID == id {
			t.Error("PendingIdentify incluyó un item en estado 'review' (debería ser terminal)")
		}
	}
}

func TestPendingIdentifyIncludesPendingAndError(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	pendingID := insertProvisional(t, ctx, r, "nuevo.mkv")

	errID := insertProvisional(t, ctx, r, "fallo_red.mkv")
	if err := r.SetIdentifyStatus(ctx, errID, catalog.IdentifyError, nil); err != nil {
		t.Fatalf("SetIdentifyStatus: %v", err)
	}

	junkID := insertProvisional(t, ctx, r, "video_sin_titulo.mkv")
	if err := r.SetIdentifyStatus(ctx, junkID, catalog.IdentifyJunk, nil); err != nil {
		t.Fatalf("SetIdentifyStatus: %v", err)
	}

	items, err := r.PendingIdentify(ctx)
	if err != nil {
		t.Fatalf("PendingIdentify: %v", err)
	}
	got := map[int64]bool{}
	for _, it := range items {
		got[it.ID] = true
	}
	if !got[pendingID] || !got[errID] {
		t.Errorf("PendingIdentify = %v, esperaba incluir pending=%d y error=%d", got, pendingID, errID)
	}
	if got[junkID] {
		t.Error("PendingIdentify incluyó un item Junk (nunca se reintenta TMDB)")
	}
}

func TestTMDBAndSearchCache(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	if _, ok, err := r.GetTMDBDetail(ctx, 1, "movie", "es-ES"); err != nil || ok {
		t.Fatalf("GetTMDBDetail sobre caché vacía: ok=%v err=%v", ok, err)
	}
	if err := r.PutTMDBDetail(ctx, 1, "movie", "es-ES", `{"id":1,"title":"X"}`); err != nil {
		t.Fatalf("PutTMDBDetail: %v", err)
	}
	payload, ok, err := r.GetTMDBDetail(ctx, 1, "movie", "es-ES")
	if err != nil || !ok || payload != `{"id":1,"title":"X"}` {
		t.Fatalf("GetTMDBDetail = %q, %v, %v", payload, ok, err)
	}

	if err := r.PutSearch(ctx, "hash1", "movie", "es-ES", `{"results":[]}`); err != nil {
		t.Fatalf("PutSearch: %v", err)
	}
	sPayload, ok, err := r.GetSearch(ctx, "hash1", "movie", "es-ES")
	if err != nil || !ok || sPayload != `{"results":[]}` {
		t.Fatalf("GetSearch = %q, %v, %v", sPayload, ok, err)
	}
}

func strp(s string) *string { return &s }

// TestItemsMissingImagesAndUpdateItemImages cubre el backfill de imágenes
// (corrección post-entrega M3, caso real "Coco": item confirmado con
// backdrop_path pero poster_path NULL, porque el CDN de imágenes falló para
// esa única descarga). ItemsMissingImages debe encontrarlo; UpdateItemImages
// debe rellenar solo lo que falta sin pisar lo que ya había.
func TestItemsMissingImagesAndUpdateItemImages(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	id := insertProvisional(t, ctx, r, "coco.2017.1080p")
	if err := r.ApplyMovieIdentification(ctx, id, 354912, "Coco", nil, nil, nil, strp("backdrop354912.jpg"), nil,
		nil, []string{}, ""); err != nil {
		t.Fatalf("ApplyMovieIdentification: %v", err)
	}

	// Un item confirmado normal (con las dos imágenes) no debe aparecer.
	completeID := insertProvisional(t, ctx, r, "inception.2010.1080p")
	if err := r.ApplyMovieIdentification(ctx, completeID, 27205, "Origen", nil, nil, strp("poster27205.jpg"),
		strp("backdrop27205.jpg"), nil, nil, []string{}, ""); err != nil {
		t.Fatalf("ApplyMovieIdentification (completo): %v", err)
	}
	// Un provisional sin confirmar tampoco (aún no tiene tmdb_id).
	_ = insertProvisional(t, ctx, r, "sin.identificar")

	missing, err := r.ItemsMissingImages(ctx)
	if err != nil {
		t.Fatalf("ItemsMissingImages: %v", err)
	}
	if len(missing) != 1 || missing[0].ID != id {
		t.Fatalf("ItemsMissingImages = %+v, esperado solo el item de Coco (id=%d)", missing, id)
	}
	if missing[0].PosterPath != nil {
		t.Errorf("PosterPath = %v, esperado nil", missing[0].PosterPath)
	}
	if missing[0].BackdropPath == nil {
		t.Error("BackdropPath = nil, esperado ya guardado")
	}

	if err := r.UpdateItemImages(ctx, id, strp("poster354912.jpg"), strp("backdrop-que-no-deberia-pisar.jpg")); err != nil {
		t.Fatalf("UpdateItemImages: %v", err)
	}
	it, err := r.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.PosterPath == nil || *it.PosterPath != "poster354912.jpg" {
		t.Fatalf("PosterPath = %v, esperado 'poster354912.jpg' (backfill)", it.PosterPath)
	}
	if it.BackdropPath == nil || *it.BackdropPath != "backdrop354912.jpg" {
		t.Fatalf("BackdropPath = %v, esperado sin cambios (COALESCE no debe pisar un valor ya guardado)", it.BackdropPath)
	}

	// Tras rellenar el póster, el item ya no debe aparecer en la lista.
	missing2, err := r.ItemsMissingImages(ctx)
	if err != nil {
		t.Fatalf("ItemsMissingImages (tras backfill): %v", err)
	}
	if len(missing2) != 0 {
		t.Fatalf("ItemsMissingImages tras backfill = %+v, esperado vacío", missing2)
	}
}
