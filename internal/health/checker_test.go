package health

import (
	"context"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func TestUnverifiableCheckerNeverDead(t *testing.T) {
	c := UnverifiableChecker{Detail: "pendiente de conector"}
	got, detail := c.Check(context.Background(), catalog.Source{Provider: "drive"})
	if got != catalog.HealthUnverified {
		t.Fatalf("Check = %q, esperado sin_verificar (no poder verificar no es estar caído)", got)
	}
	if detail != "pendiente de conector" {
		t.Errorf("detail = %q, esperado el motivo dado", detail)
	}
}
