package health

import (
	"context"
	"os"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
)

// localfsSlowThreshold: un stat que tarde más que esto se considera disco
// lento, no simplemente vivo (Brief M6 §2). Valor por defecto; ver
// LocalfsChecker.SlowThreshold para sustituirlo (tests deterministas).
const localfsSlowThreshold = 2 * time.Second

// LocalfsChecker comprueba fuentes de disco local: existencia, legibilidad,
// y si el volumen que las contiene sigue conectado. No duplica el poller de
// volúmenes de M2 (internal/providers/localfs.Poller), que ya marca
// desconectada/viva en BLOQUE para todas las fuentes de un volumen a la vez
// con más contexto del que tiene una verificación aislada (reconexión/
// desconexión completa); aquí solo se re-confirma el estado individual del
// archivo cuando le toca su turno de verificación (Brief M6 §2).
type LocalfsChecker struct {
	Repo *catalog.Repo

	// Stat permite sustituir os.Stat en tests, para simular un disco lento
	// de forma determinista sin depender de I/O real. nil usa os.Stat.
	Stat func(name string) (os.FileInfo, error)

	// SlowThreshold sustituye localfsSlowThreshold cuando es > 0 (tests con
	// umbrales pequeños y deterministas).
	SlowThreshold time.Duration
}

func (c *LocalfsChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	if s.VolumeID != nil {
		connected, err := c.Repo.VolumeConnected(ctx, *s.VolumeID)
		if err == nil && !connected {
			return catalog.HealthDisconnected, "el volumen no está conectado"
		}
	}

	stat := c.Stat
	if stat == nil {
		stat = os.Stat
	}
	threshold := c.SlowThreshold
	if threshold <= 0 {
		threshold = localfsSlowThreshold
	}

	start := time.Now()
	info, err := stat(s.Locator)
	elapsed := time.Since(start)
	if err != nil {
		return catalog.HealthDead, "archivo no encontrado o no accesible: " + err.Error()
	}
	if info.IsDir() {
		return catalog.HealthDead, "la ruta es una carpeta, no un archivo"
	}
	if elapsed > threshold {
		return catalog.HealthSlow, "acceso a disco lento"
	}
	return catalog.HealthAlive, ""
}
