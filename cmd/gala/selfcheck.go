package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/store"
	"github.com/bigbendja/gala/internal/update"
)

// runSelfcheck es `gala --selfcheck`: lo ejecuta el preflight con el binario
// recién instalado ANTES de arrancarlo de verdad. Carga la configuración y abre
// la BD (aplicando migraciones) pero no liga ningún puerto ni lanza workers.
// Imprime `migraciones_aplicadas=N` (las que aplicó ESTA ejecución) y sale 0 si
// todo está bien; si no, explica el motivo por stderr y sale 1.
func runSelfcheck(stdout, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "configuración:", err)
		return 1
	}
	before, err := store.MigrationCount(cfg.DBPath())
	if err != nil {
		fmt.Fprintln(stderr, "base de datos:", err)
		return 1
	}
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		fmt.Fprintln(stderr, "base de datos:", err)
		return 1
	}
	defer st.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := st.DB.PingContext(ctx); err != nil {
		fmt.Fprintln(stderr, "base de datos:", err)
		return 1
	}
	fmt.Fprintf(stdout, "migraciones_aplicadas=%d\n", st.AppliedMigrations()-before)
	return 0
}

// autoCheckUpdates consulta GitHub una vez al día si update.auto_check está
// activo. Solo comprueba y cachea; nunca instala. Sin red no pasa nada.
func autoCheckUpdates(ctx context.Context, c *update.Checker, log *slog.Logger) {
	timer := time.NewTimer(time.Minute) // no en el arranque: que primero esté todo en marcha
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if info, err := c.Check(ctx, false); err != nil {
			log.Info("actualizaciones: no se pudo comprobar", "err", err)
		} else if info.UpdateAvailable {
			log.Info("hay una versión nueva", "actual", info.Current, "nueva", info.Latest)
		}
		timer.Reset(24 * time.Hour)
	}
}
