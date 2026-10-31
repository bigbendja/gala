package update

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// BootTiming son los plazos de la confirmación del arranque (inyectables en tests).
type BootTiming struct {
	Health time.Duration // máximo para que /healthz responda 200
	Stable time.Duration // tiempo vivo y sano antes de dar la actualización por buena
	Poll   time.Duration
}

// DefaultBootTiming: 30 s para estar sano y 60 s estable.
var DefaultBootTiming = BootTiming{Health: 30 * time.Second, Stable: 60 * time.Second, Poll: time.Second}

// ConfirmBoot confirma (o da por fallido) el primer arranque tras una
// actualización. Solo el camino feliz vive aquí; el rollback es del script
// gala-preflight, que corre ANTES de cada arranque y no depende de este binario.
//
//   - Sin marcador: no hace nada.
//   - Con marcador: espera /healthz (Health). Si no responde, llama a onFail
//     (salir con código 1: el intento ya contó y el supervisor reintenta).
//     Si responde y el proceso aguanta Stable, da la actualización por buena:
//     escribe el resultado, promueve la copia de la BD y borra el marcador.
//
// ConfirmBoot bloquea hasta terminar: se lanza en una goroutine.
func ConfirmBoot(ctx context.Context, exePath, healthURL, running string, log *slog.Logger, onFail func(), t BootTiming) {
	dir := filepath.Dir(exePath)
	m, ok, err := ReadMarker(dir)
	if err != nil {
		log.Warn("actualización: no se pudo leer el marcador", "err", err)
		return
	}
	if !ok {
		return
	}
	// «preparing» + .new ausente + .prev presente = intercambio hecho al que
	// solo le faltó la anotación; «preparing» con .new presente = el
	// intercambio nunca ocurrió (marcador huérfano: se limpia).
	if m.State == StateReady {
		if exists(exePath+NewSuffix) || !exists(exePath+PrevSuffix) {
			log.Warn("actualización: marcador huérfano de un intercambio que no llegó a hacerse; se limpia")
			_ = os.Remove(exePath + NewSuffix)
			_ = os.Remove(filepath.Join(dir, PendingFile))
			return
		}
	}

	if !waitHealthy(ctx, healthURL, t.Health, t.Poll) {
		if ctx.Err() != nil {
			return
		}
		log.Error("actualización: la versión nueva no responde a /healthz; se sale para que se reintente o se revierta", "version", running)
		onFail()
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(t.Stable):
	}
	// Una última comprobación: que siga sano tras el periodo de estabilidad.
	if !waitHealthy(ctx, healthURL, t.Health, t.Poll) {
		if ctx.Err() == nil {
			log.Error("actualización: la versión nueva dejó de responder durante el periodo de estabilidad")
			onFail()
		}
		return
	}
	finalize(dir, m, running, log)
}

func finalize(dir string, m Marker, running string, log *slog.Logger) {
	if err := WriteResult(dir, Result{Status: ResultOK, From: m.From, To: running}); err != nil {
		log.Warn("actualización: no se pudo escribir el resultado", "err", err)
	}
	// La copia de esta actualización pasa a ser LA copia (sustituye a la de la
	// actualización anterior, que se conservaba hasta ahora).
	if exists(filepath.Join(dir, DBBackupNew)) {
		if err := os.Rename(filepath.Join(dir, DBBackupNew), filepath.Join(dir, DBBackupFile)); err != nil {
			log.Warn("actualización: no se pudo promover la copia de la BD", "err", err)
		}
	}
	if err := os.Remove(filepath.Join(dir, PendingFile)); err != nil && !os.IsNotExist(err) {
		log.Warn("actualización: no se pudo borrar el marcador", "err", err)
		return
	}
	log.Info("actualización confirmada", "version", running)
}

func waitHealthy(ctx context.Context, url string, max, poll time.Duration) bool {
	deadline := time.Now().Add(max)
	client := &http.Client{Timeout: 3 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			if resp, err := client.Do(req); err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true
				}
			}
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(poll):
		}
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Describe resume el marcador para los logs.
func (m Marker) Describe() string {
	return fmt.Sprintf("%s→%s (%s, intentos=%d)", m.From, m.New, m.State, m.Attempts)
}
