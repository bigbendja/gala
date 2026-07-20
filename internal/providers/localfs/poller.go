package localfs

import (
	"context"
	"log/slog"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
)

// PollInterval es el periodo por defecto del poller de volúmenes (Brief M2).
const PollInterval = 5 * time.Second

// slowPassThreshold: por encima de esto, una pasada del poller se registra
// en debug (Addendum M2), para detectar degradaciones futuras (p. ej.
// unidades lentas de enumerar).
const slowPassThreshold = 2 * time.Second

// Poller vigila la conexión/desconexión de volúmenes y mantiene la tabla
// volumes y la salud de las fuentes localfs coherentes con lo que hay
// realmente conectado.
type Poller struct {
	Enum     Enumerator
	Folders  func() []string
	Repo     *catalog.Repo
	Rescan   func(folders []string) // dispara un re-escaneo asíncrono
	Log      *slog.Logger
	Interval time.Duration // 0 = PollInterval
}

// NewPoller crea un Poller.
func NewPoller(enum Enumerator, folders func() []string, repo *catalog.Repo, rescan func([]string), log *slog.Logger) *Poller {
	return &Poller{Enum: enum, Folders: folders, Repo: repo, Rescan: rescan, Log: log}
}

// Run comprueba los volúmenes cada Interval hasta que ctx se cancele
// (apagado limpio: el poller respeta el context del main).
func (p *Poller) Run(ctx context.Context) {
	interval := p.Interval
	if interval <= 0 {
		interval = PollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()
			err := p.tick(ctx)
			if elapsed := time.Since(start); elapsed > slowPassThreshold {
				p.Log.Debug("pasada del poller lenta", "duración", elapsed)
			}
			if err != nil {
				p.Log.Error("poller de volúmenes", "err", err)
			}
		}
	}
}

// tick hace una comprobación. No exportado; los tests del poller lo llaman
// directamente para no depender de temporizadores reales.
func (p *Poller) tick(ctx context.Context) error {
	folders := p.Folders()
	present, err := p.Enum.Volumes(ctx, folders)
	if err != nil {
		return err
	}

	known, err := p.Repo.ListVolumes(ctx)
	if err != nil {
		return err
	}
	knownByID := make(map[string]catalog.Volume, len(known))
	for _, k := range known {
		knownByID[k.ID] = k
	}

	// Carpeta registrada -> volumen que la contiene ahora mismo (si lo hay).
	folderVol := make(map[string]string, len(folders))
	for _, f := range folders {
		if mv, ok := MatchVolume(f, present); ok {
			folderVol[f] = mv.ID
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	presentIDs := make(map[string]bool, len(present))

	for _, v := range present {
		presentIDs[v.ID] = true
		prev, wasKnown := knownByID[v.ID]

		var label *string
		if v.Label != "" {
			label = &v.Label
		}
		if err := p.Repo.UpsertVolume(ctx, catalog.Volume{ID: v.ID, Label: label, Connected: true}, now); err != nil {
			p.Log.Error("registrar volumen", "id", v.ID, "err", err)
			continue
		}

		reconnected := wasKnown && !prev.Connected
		if reconnected {
			if err := p.Repo.SetSourcesHealthByVolume(ctx, ProviderID, v.ID, catalog.HealthAlive); err != nil {
				p.Log.Error("reactivar fuentes de volumen", "id", v.ID, "err", err)
			}
		}

		// Volumen nuevo o que reaparece: solo se re-escanean las carpetas
		// registradas que viven en él (nunca el disco entero automáticamente).
		if !wasKnown || reconnected {
			var toRescan []string
			for f, vid := range folderVol {
				if vid == v.ID {
					toRescan = append(toRescan, f)
				}
			}
			if len(toRescan) > 0 && p.Rescan != nil {
				p.Rescan(toRescan)
			}
		}
	}

	for id, k := range knownByID {
		if k.Connected && !presentIDs[id] {
			if err := p.Repo.SetVolumeConnected(ctx, id, false, now); err != nil {
				p.Log.Error("marcar volumen desconectado", "id", id, "err", err)
				continue
			}
			if err := p.Repo.SetSourcesHealthByVolume(ctx, ProviderID, id, catalog.HealthDisconnected); err != nil {
				p.Log.Error("marcar fuentes desconectadas", "id", id, "err", err)
			}
		}
	}
	return nil
}
