package catalog_test

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func TestSetWished(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	id, err := r.InsertItem(ctx, &catalog.Item{
		Kind: catalog.KindMovie, Title: "El Último Tren", Genres: []string{}, Confirmed: false,
	}, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}

	it, err := r.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.Wished {
		t.Fatal("Wished = true recién creado, esperado false")
	}

	if err := r.SetWished(ctx, id, true); err != nil {
		t.Fatalf("SetWished(true): %v", err)
	}
	it2, err := r.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get (2): %v", err)
	}
	if !it2.Wished {
		t.Fatal("Wished = false tras SetWished(true)")
	}

	if err := r.SetWished(ctx, id, false); err != nil {
		t.Fatalf("SetWished(false): %v", err)
	}
	it3, err := r.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get (3): %v", err)
	}
	if it3.Wished {
		t.Fatal("Wished = true tras SetWished(false)")
	}

	if err := r.SetWished(ctx, 999999, true); err != catalog.ErrNotFound {
		t.Fatalf("SetWished(id inexistente) = %v, esperado ErrNotFound", err)
	}
}
