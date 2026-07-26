package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

// TestListInProgressFiltersByDeviceAndProgress cubre el filtro in_progress
// añadido para que "Continuar viendo" mire todo el catálogo, no solo la
// página ya cargada (Brief M10a-fix B3).
func TestListInProgressFiltersByDeviceAndProgress(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	newMovie := func(title string) int64 {
		id, err := r.InsertItem(ctx, &catalog.Item{
			Kind: catalog.KindMovie, Title: title, Genres: []string{}, Confirmed: true,
		}, "")
		if err != nil {
			t.Fatalf("InsertItem %s: %v", title, err)
		}
		return id
	}

	inProgressID := newMovie("En curso")
	watchedID := newMovie("Ya vista")
	zeroProgressID := newMovie("Sin empezar")
	otherDeviceID := newMovie("De otro dispositivo")
	_ = newMovie("Sin ningún progreso registrado")

	dur := 5400.0
	if err := r.UpsertPlayback(ctx, inProgressID, "dev-1", 120, &dur, false, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("UpsertPlayback en curso: %v", err)
	}
	if err := r.UpsertPlayback(ctx, watchedID, "dev-1", 5300, &dur, true, "2026-01-02T00:00:00Z"); err != nil {
		t.Fatalf("UpsertPlayback vista: %v", err)
	}
	if err := r.UpsertPlayback(ctx, zeroProgressID, "dev-1", 0, &dur, false, "2026-01-03T00:00:00Z"); err != nil {
		t.Fatalf("UpsertPlayback sin empezar: %v", err)
	}
	if err := r.UpsertPlayback(ctx, otherDeviceID, "dev-2", 300, &dur, false, "2026-01-04T00:00:00Z"); err != nil {
		t.Fatalf("UpsertPlayback otro dispositivo: %v", err)
	}

	res, err := r.List(ctx, catalog.ListFilter{InProgress: true, DeviceID: "dev-1"})
	if err != nil {
		t.Fatalf("List(in_progress): %v", err)
	}
	if res.Total != 1 || res.Items[0].ID != inProgressID {
		t.Fatalf("in_progress para dev-1 = %+v, esperado solo %q", res.Items, "En curso")
	}

	// Sin dispositivo (cookie ausente): vacío, nunca error.
	empty, err := r.List(ctx, catalog.ListFilter{InProgress: true, DeviceID: ""})
	if err != nil {
		t.Fatalf("List(in_progress, sin device): %v", err)
	}
	if empty.Total != 0 {
		t.Fatalf("in_progress sin device = %d, esperado 0", empty.Total)
	}

	// Orden por el momento del progreso (updated_at de playback), no por
	// added_at del item: añadimos un segundo en-curso más reciente y debe
	// salir primero.
	secondInProgressID := newMovie("En curso más reciente")
	if err := r.UpsertPlayback(ctx, secondInProgressID, "dev-1", 60, &dur, false, "2026-01-05T00:00:00Z"); err != nil {
		t.Fatalf("UpsertPlayback segundo en curso: %v", err)
	}
	res2, err := r.List(ctx, catalog.ListFilter{InProgress: true, DeviceID: "dev-1"})
	if err != nil {
		t.Fatalf("List(in_progress) tras segundo: %v", err)
	}
	if res2.Total != 2 || res2.Items[0].ID != secondInProgressID || res2.Items[1].ID != inProgressID {
		t.Fatalf("orden in_progress = %+v, esperado [más reciente, en curso]", res2.Items)
	}
}
