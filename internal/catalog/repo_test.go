package catalog_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/store"
)

func newTestRepo(t *testing.T) *catalog.Repo {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return catalog.NewRepo(st.DB)
}

func TestSeedListGet(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	n, err := catalog.SeedDemo(ctx, r)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n == 0 {
		t.Fatal("seed no insertó nada en BD vacía")
	}

	// El seed es idempotente sobre BD con datos.
	n2, err := catalog.SeedDemo(ctx, r)
	if err != nil || n2 != 0 {
		t.Fatalf("seed repetido: n=%d err=%v (esperado 0, nil)", n2, err)
	}

	// Listado completo.
	res, err := r.List(ctx, catalog.ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Total != n || len(res.Items) != n {
		t.Fatalf("list: total=%d items=%d, esperado %d", res.Total, len(res.Items), n)
	}

	// Filtro por kind.
	res, err = r.List(ctx, catalog.ListFilter{Kind: "movie"})
	if err != nil {
		t.Fatalf("list movie: %v", err)
	}
	for _, it := range res.Items {
		if it.Kind != catalog.KindMovie {
			t.Fatalf("filtro kind devolvió %q", it.Kind)
		}
	}

	// Búsqueda por texto (insensible a mayúsculas).
	res, err = r.List(ctx, catalog.ListFilter{Query: "harmattan"})
	if err != nil || res.Total != 1 {
		t.Fatalf("búsqueda: total=%d err=%v", res.Total, err)
	}
	item := res.Items[0]
	if item.SourceCount != 2 {
		t.Fatalf("source_count=%d, esperado 2", item.SourceCount)
	}
	if item.HealthSummary[catalog.HealthAlive] != 1 || item.HealthSummary[catalog.HealthDead] != 1 {
		t.Fatalf("health_summary inesperado: %v", item.HealthSummary)
	}

	// Deseos.
	wished := true
	res, err = r.List(ctx, catalog.ListFilter{Wished: &wished})
	if err != nil || res.Total != 1 || res.Items[0].Title != "El Último Tren" {
		t.Fatalf("deseos: %+v err=%v", res, err)
	}

	// Por confirmar.
	unconfirmed := false
	res, err = r.List(ctx, catalog.ListFilter{Confirmed: &unconfirmed})
	if err != nil || res.Total != 1 {
		t.Fatalf("por confirmar: total=%d err=%v", res.Total, err)
	}

	// Ficha con fuentes ordenadas por prioridad.
	it, err := r.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(it.Sources) != 2 || it.Sources[0].Provider != "localfs" {
		t.Fatalf("fuentes: %+v", it.Sources)
	}

	// Not found.
	if _, err := r.Get(ctx, 99999); err != catalog.ErrNotFound {
		t.Fatalf("esperado ErrNotFound, err=%v", err)
	}

	// Kind inválido rechazado.
	if _, err := r.List(ctx, catalog.ListFilter{Kind: "pelicula"}); err == nil {
		t.Fatal("kind inválido aceptado")
	}
}
