package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func insertSourceForStreamTest(t *testing.T, ctx context.Context, r *catalog.Repo, itemTitle, locator string) (int64, int64) {
	t.Helper()
	item := catalog.Item{Kind: catalog.KindMovie, Title: itemTitle, Genres: []string{}, Confirmed: true}
	itemID, err := r.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: itemID, Provider: "localfs", Locator: locator}
	srcID, err := r.InsertSource(ctx, &src, "")
	if err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	return itemID, srcID
}

func TestSourceByID(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	_, srcID := insertSourceForStreamTest(t, ctx, r, "peli", "/x/peli.mp4")

	src, err := r.SourceByID(ctx, srcID)
	if err != nil {
		t.Fatalf("SourceByID: %v", err)
	}
	if src.Locator != "/x/peli.mp4" || src.Provider != "localfs" {
		t.Fatalf("src = %+v", src)
	}

	if _, err := r.SourceByID(ctx, 999999); err != catalog.ErrNotFound {
		t.Fatalf("SourceByID(inexistente) = %v, esperado ErrNotFound", err)
	}
}

func TestMediaInfoUpsertGetAndPending(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	_, srcID := insertSourceForStreamTest(t, ctx, r, "peli", "/x/peli.mkv")

	// Sin sondear: aparece en SourcesWithoutMediaInfo.
	pending, err := r.SourcesWithoutMediaInfo(ctx, "localfs")
	if err != nil {
		t.Fatalf("SourcesWithoutMediaInfo: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != srcID {
		t.Fatalf("pending = %+v, esperado solo la fuente recién insertada", pending)
	}
	if _, ok, err := r.GetMediaInfo(ctx, srcID); err != nil || ok {
		t.Fatalf("GetMediaInfo antes de sondear: ok=%v err=%v", ok, err)
	}

	mi := catalog.MediaInfo{SourceID: srcID, Container: "matroska", DurationS: 120.5,
		VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080, Bitrate: 5_000_000}
	if err := r.UpsertMediaInfo(ctx, mi); err != nil {
		t.Fatalf("UpsertMediaInfo: %v", err)
	}

	got, ok, err := r.GetMediaInfo(ctx, srcID)
	if err != nil || !ok {
		t.Fatalf("GetMediaInfo tras upsert: ok=%v err=%v", ok, err)
	}
	if got.Container != "matroska" || got.VideoCodec != "h264" || got.DurationS != 120.5 {
		t.Fatalf("media_info = %+v", got)
	}

	// Ya no debe aparecer como pendiente.
	pending2, err := r.SourcesWithoutMediaInfo(ctx, "localfs")
	if err != nil {
		t.Fatalf("SourcesWithoutMediaInfo (2): %v", err)
	}
	if len(pending2) != 0 {
		t.Fatalf("pending tras sondear = %+v, esperado vacío", pending2)
	}

	// Re-sondeo (nuevo intento): UpsertMediaInfo debe REEMPLAZAR, no duplicar.
	mi.VideoCodec = "hevc"
	if err := r.UpsertMediaInfo(ctx, mi); err != nil {
		t.Fatalf("UpsertMediaInfo (reemplazo): %v", err)
	}
	got2, _, err := r.GetMediaInfo(ctx, srcID)
	if err != nil {
		t.Fatalf("GetMediaInfo tras reemplazo: %v", err)
	}
	if got2.VideoCodec != "hevc" {
		t.Fatalf("VideoCodec = %q, esperado hevc tras reemplazo", got2.VideoCodec)
	}
}

func TestPlaybackGetUpsertAndAttachProgress(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	itemID, _ := insertSourceForStreamTest(t, ctx, r, "peli", "/x/peli.mp4")

	if _, ok, err := r.GetPlayback(ctx, itemID, "device-1"); err != nil || ok {
		t.Fatalf("GetPlayback sin progreso: ok=%v err=%v", ok, err)
	}

	dur := 5400.0
	if err := r.UpsertPlayback(ctx, itemID, "device-1", 120, &dur, false, ""); err != nil {
		t.Fatalf("UpsertPlayback: %v", err)
	}
	p, ok, err := r.GetPlayback(ctx, itemID, "device-1")
	if err != nil || !ok {
		t.Fatalf("GetPlayback tras upsert: ok=%v err=%v", ok, err)
	}
	if p.PositionS != 120 || p.DurationS == nil || *p.DurationS != 5400 || p.Watched {
		t.Fatalf("progress = %+v", p)
	}

	// Un dispositivo distinto no ve el progreso del primero.
	if _, ok, err := r.GetPlayback(ctx, itemID, "device-2"); err != nil || ok {
		t.Fatalf("GetPlayback(device-2): ok=%v err=%v", ok, err)
	}

	// Segundo PUT del mismo dispositivo actualiza in situ (no duplica fila).
	if err := r.UpsertPlayback(ctx, itemID, "device-1", 5100, &dur, true, ""); err != nil {
		t.Fatalf("UpsertPlayback (watched): %v", err)
	}
	p2, _, err := r.GetPlayback(ctx, itemID, "device-1")
	if err != nil {
		t.Fatalf("GetPlayback (2): %v", err)
	}
	if p2.PositionS != 5100 || !p2.Watched {
		t.Fatalf("progress tras 2º PUT = %+v", p2)
	}

	// AttachProgress rellena Item.Progress para el dispositivo pedido.
	items := []catalog.Item{{ID: itemID}}
	if err := r.AttachProgress(ctx, items, "device-1"); err != nil {
		t.Fatalf("AttachProgress: %v", err)
	}
	if items[0].Progress == nil || items[0].Progress.PositionS != 5100 {
		t.Fatalf("Progress tras AttachProgress = %+v", items[0].Progress)
	}

	// Dispositivo vacío: no-op, sin error (Brief M4 §5: lectura sin cookie).
	items2 := []catalog.Item{{ID: itemID}}
	if err := r.AttachProgress(ctx, items2, ""); err != nil {
		t.Fatalf("AttachProgress (sin device): %v", err)
	}
	if items2[0].Progress != nil {
		t.Fatalf("Progress = %+v con deviceID vacío, esperado nil", items2[0].Progress)
	}

	// Un item sin progreso para ESE dispositivo queda con Progress nil.
	otherItemID, _ := insertSourceForStreamTest(t, ctx, r, "otra", "/x/otra.mp4")
	items3 := []catalog.Item{{ID: itemID}, {ID: otherItemID}}
	if err := r.AttachProgress(ctx, items3, "device-1"); err != nil {
		t.Fatalf("AttachProgress (mixto): %v", err)
	}
	if items3[0].Progress == nil {
		t.Error("items3[0].Progress = nil, esperado el progreso guardado")
	}
	if items3[1].Progress != nil {
		t.Errorf("items3[1].Progress = %+v, esperado nil (nunca reproducido)", items3[1].Progress)
	}
}
