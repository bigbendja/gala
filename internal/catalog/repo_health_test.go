package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func insertSourceForHealthTest(t *testing.T, ctx context.Context, r *catalog.Repo, locator, health string) int64 {
	t.Helper()
	item := catalog.Item{Kind: catalog.KindMovie, Title: "peli", Genres: []string{}, Confirmed: true}
	itemID, err := r.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: itemID, Provider: "localfs", Locator: locator, Health: health}
	srcID, err := r.InsertSource(ctx, &src, "")
	if err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	return srcID
}

func TestUpdateSourceHealth(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	srcID := insertSourceForHealthTest(t, ctx, r, "/x/a.mp4", catalog.HealthUnverified)

	if err := r.UpdateSourceHealth(ctx, srcID, catalog.HealthDead, "archivo no encontrado", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("UpdateSourceHealth: %v", err)
	}

	src, err := r.SourceByID(ctx, srcID)
	if err != nil {
		t.Fatalf("SourceByID: %v", err)
	}
	if src.Health != catalog.HealthDead {
		t.Fatalf("Health = %q, esperado caída", src.Health)
	}
	if src.HealthDetail == nil || *src.HealthDetail != "archivo no encontrado" {
		t.Fatalf("HealthDetail = %v, esperado \"archivo no encontrado\"", src.HealthDetail)
	}
	if src.LastChecked == nil || *src.LastChecked != "2026-01-01T00:00:00Z" {
		t.Fatalf("LastChecked = %v, esperado 2026-01-01T00:00:00Z", src.LastChecked)
	}

	// detail vacío se guarda como NULL, no como cadena vacía (Brief M6 §3).
	if err := r.UpdateSourceHealth(ctx, srcID, catalog.HealthAlive, "", "2026-01-01T01:00:00Z"); err != nil {
		t.Fatalf("UpdateSourceHealth (sin detalle): %v", err)
	}
	src2, err := r.SourceByID(ctx, srcID)
	if err != nil {
		t.Fatalf("SourceByID (2): %v", err)
	}
	if src2.HealthDetail != nil {
		t.Fatalf("HealthDetail = %v, esperado nil tras detail vacío", src2.HealthDetail)
	}
}

func TestSourcesDueForHealthCheck(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	neverChecked := insertSourceForHealthTest(t, ctx, r, "/x/nunca.mp4", catalog.HealthUnverified)
	recentlyChecked := insertSourceForHealthTest(t, ctx, r, "/x/reciente.mp4", catalog.HealthAlive)
	staleChecked := insertSourceForHealthTest(t, ctx, r, "/x/viejo.mp4", catalog.HealthAlive)

	if err := r.UpdateSourceHealth(ctx, recentlyChecked, catalog.HealthAlive, "", "2026-06-01T00:00:00Z"); err != nil {
		t.Fatalf("UpdateSourceHealth (reciente): %v", err)
	}
	if err := r.UpdateSourceHealth(ctx, staleChecked, catalog.HealthAlive, "", "2020-01-01T00:00:00Z"); err != nil {
		t.Fatalf("UpdateSourceHealth (viejo): %v", err)
	}

	// cutoff no vacío: solo nunca-verificadas o vistas antes del cutoff.
	due, err := r.SourcesDueForHealthCheck(ctx, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("SourcesDueForHealthCheck: %v", err)
	}
	ids := map[int64]bool{}
	for _, s := range due {
		ids[s.ID] = true
	}
	if !ids[neverChecked] || !ids[staleChecked] || ids[recentlyChecked] {
		t.Fatalf("due = %+v, esperado {nunca, viejo}, sin reciente", due)
	}

	// cutoff vacío: todas, ignorando la ventana (re-chequeo forzado).
	all, err := r.SourcesDueForHealthCheck(ctx, "")
	if err != nil {
		t.Fatalf("SourcesDueForHealthCheck (forzado): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all = %d, esperado 3 (todas, sin filtrar)", len(all))
	}
}

func TestHealthCounts(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	insertSourceForHealthTest(t, ctx, r, "/x/a.mp4", catalog.HealthAlive)
	insertSourceForHealthTest(t, ctx, r, "/x/b.mp4", catalog.HealthAlive)
	insertSourceForHealthTest(t, ctx, r, "/x/c.mp4", catalog.HealthDead)
	insertSourceForHealthTest(t, ctx, r, "/x/d.mp4", catalog.HealthUnverified)

	counts, err := r.HealthCounts(ctx)
	if err != nil {
		t.Fatalf("HealthCounts: %v", err)
	}
	if counts[catalog.HealthAlive] != 2 || counts[catalog.HealthDead] != 1 || counts[catalog.HealthUnverified] != 1 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestVolumeConnected(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	// Volumen nunca visto: se asume conectado (Brief M6 §2).
	connected, err := r.VolumeConnected(ctx, "nunca-visto")
	if err != nil {
		t.Fatalf("VolumeConnected (desconocido): %v", err)
	}
	if !connected {
		t.Error("VolumeConnected(desconocido) = false, esperado true (sin evidencia de desconexión)")
	}

	if err := r.UpsertVolume(ctx, catalog.Volume{ID: "vol-1", Connected: false}, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("UpsertVolume: %v", err)
	}
	connected2, err := r.VolumeConnected(ctx, "vol-1")
	if err != nil {
		t.Fatalf("VolumeConnected (desconectado): %v", err)
	}
	if connected2 {
		t.Error("VolumeConnected(vol-1) = true, esperado false")
	}

	if err := r.SetVolumeConnected(ctx, "vol-1", true, "2026-01-01T01:00:00Z"); err != nil {
		t.Fatalf("SetVolumeConnected: %v", err)
	}
	connected3, err := r.VolumeConnected(ctx, "vol-1")
	if err != nil {
		t.Fatalf("VolumeConnected (reconectado): %v", err)
	}
	if !connected3 {
		t.Error("VolumeConnected(vol-1) tras reconectar = false, esperado true")
	}
}
