// Package health verifica en segundo plano el estado real de las fuentes
// del catálogo (Brief M6, Especificación MVP1 §3.5): un worker con
// presupuesto llama a un Checker por provider y persiste el resultado en
// sources.health/health_detail/last_checked.
package health

import (
	"context"

	"github.com/bigbendja/gala/internal/catalog"
)

// Checker comprueba el estado real de UNA fuente concreta. Cada
// implementación aplica su propio timeout corto (Brief M6 §2): un Checker
// nunca debe bloquear indefinidamente al worker.
type Checker interface {
	// Check devuelve un estado de catalog.Health* y un detalle legible para
	// la UI (cadena vacía si el estado se explica solo, p. ej. "viva").
	Check(ctx context.Context, s catalog.Source) (health, detail string)
}

// UnverifiableChecker cubre providers que aún no se pueden comprobar de
// verdad (drive/mega: conectores futuros, Brief M6 §2) o desconocidos: no
// poder verificar NO es lo mismo que estar caído, así que siempre responde
// sin_verificar, nunca caída.
type UnverifiableChecker struct {
	Detail string
}

func (c UnverifiableChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	return catalog.HealthUnverified, c.Detail
}
