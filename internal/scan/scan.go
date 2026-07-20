// Package scan orquesta el escaneo de carpetas locales: recorre las
// carpetas registradas con localfs.WalkFolder y reconcilia el resultado con
// el catálogo (altas, "sigue vivo", bajas), con exclusión mutua y estado
// consultable por la API (Brief M2 §4).
package scan

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/providers/localfs"
)

// ErrAlreadyRunning se devuelve al pedir un escaneo mientras otro está en curso.
var ErrAlreadyRunning = errors.New("ya hay un escaneo en curso")

// batchSize agrupa las altas en transacciones para no castigar el disco con
// un commit por fichero (Brief M2 §Alta en catálogo).
const batchSize = 200

// Status es el estado consultable de un escaneo (GET /api/v1/scan/status).
type Status struct {
	Running      bool    `json:"running"`
	LastStarted  *string `json:"last_started,omitempty"`
	LastFinished *string `json:"last_finished,omitempty"`
	FilesSeen    int     `json:"files_seen"`
	ItemsNew     int     `json:"items_new"`
}

// Scanner coordina los escaneos de carpetas locales: un solo escaneo
// concurrente, progreso consultable, y sincronización con el catálogo.
type Scanner struct {
	ctx  context.Context // vida de la aplicación; sobrevive a peticiones HTTP concretas
	cfg  *config.Config
	repo *catalog.Repo
	enum localfs.Enumerator
	log  *slog.Logger

	mu     sync.Mutex
	status Status
}

// New crea un Scanner. ctx debe vivir mientras viva la aplicación: los
// escaneos se lanzan en segundo plano y deben sobrevivir a la petición HTTP
// que los disparó.
func New(ctx context.Context, cfg *config.Config, repo *catalog.Repo, enum localfs.Enumerator, log *slog.Logger) *Scanner {
	return &Scanner{ctx: ctx, cfg: cfg, repo: repo, enum: enum, log: log}
}

// Status devuelve una copia del estado actual.
func (s *Scanner) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Start lanza un escaneo asíncrono. folders nil = todas las carpetas
// registradas en la configuración. Devuelve ErrAlreadyRunning si ya hay uno
// en curso: un solo escaneo concurrente (Brief M2 §3).
func (s *Scanner) Start(folders []string) error {
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	now := timeNow()
	s.status = Status{Running: true, LastStarted: &now, LastFinished: s.status.LastFinished}
	s.mu.Unlock()

	go s.run(folders)
	return nil
}

func (s *Scanner) run(folders []string) {
	defer func() {
		s.mu.Lock()
		s.status.Running = false
		now := timeNow()
		s.status.LastFinished = &now
		s.mu.Unlock()
	}()

	if folders == nil {
		folders = append([]string(nil), s.cfg.MediaFolders...)
	}
	if len(folders) == 0 {
		return
	}

	vols, err := s.enum.Volumes(s.ctx, s.cfg.MediaFolders)
	if err != nil {
		s.log.Error("enumerar volúmenes", "err", err)
		vols = nil
	}

	for _, folder := range folders {
		if s.ctx.Err() != nil {
			return
		}
		if err := s.syncFolder(folder, vols); err != nil {
			s.log.Warn("escaneo de carpeta", "folder", folder, "err", err)
		}
	}
}

// syncFolder recorre folder y reconcilia el resultado con el catálogo:
// altas nuevas, "sigue vivo" para las existentes, y baja de las que ya no
// están (Brief M2 §Alta en catálogo).
func (s *Scanner) syncFolder(folder string, vols []localfs.Volume) error {
	found, err := localfs.WalkFolder(s.ctx, folder, s.log)
	if err != nil {
		return err
	}

	var volID *string
	if mv, ok := localfs.MatchVolume(folder, vols); ok {
		id := mv.ID
		volID = &id
	}

	foundSet := make(map[string]struct{}, len(found))
	for i := 0; i < len(found); i += batchSize {
		end := i + batchSize
		if end > len(found) {
			end = len(found)
		}
		if err := s.syncBatch(found[i:end], volID, foundSet); err != nil {
			return err
		}
	}

	_, err = removeStale(s.ctx, s.repo, folder, foundSet)
	return err
}

// syncBatch da de alta o actualiza un lote de ficheros en una sola
// transacción SQL.
func (s *Scanner) syncBatch(chunk []localfs.FoundFile, volID *string, foundSet map[string]struct{}) error {
	now := timeNow()
	filesSeen, itemsNew := 0, 0

	err := s.repo.Tx(s.ctx, func(tx *catalog.Repo) error {
		for _, f := range chunk {
			foundSet[localfs.NormalizePath(f.Path)] = struct{}{}
			filesSeen++

			existing, gerr := tx.SourceByLocator(s.ctx, localfs.ProviderID, f.Path)
			if gerr != nil && !errors.Is(gerr, catalog.ErrNotFound) {
				return gerr
			}
			if gerr == nil {
				if terr := tx.TouchSource(s.ctx, existing.ID, catalog.HealthAlive, now); terr != nil {
					return terr
				}
				continue
			}

			title := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
			item := catalog.Item{Kind: catalog.KindOther, Title: title, Genres: []string{}, Confirmed: false}
			itemID, ierr := tx.InsertItem(s.ctx, &item, now)
			if ierr != nil {
				return ierr
			}
			src := catalog.Source{
				ItemID: itemID, Provider: localfs.ProviderID, Locator: f.Path,
				VolumeID: volID, Health: catalog.HealthAlive,
			}
			if _, serr := tx.InsertSource(s.ctx, &src, now); serr != nil {
				return serr
			}
			itemsNew++
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.status.FilesSeen += filesSeen
	s.status.ItemsNew += itemsNew
	s.mu.Unlock()
	return nil
}

// RemoveFolderSources elimina todas las fuentes localfs bajo folder (los
// items se conservan). Usado por DELETE /api/v1/folders al desregistrar una
// carpeta entera.
func RemoveFolderSources(ctx context.Context, repo *catalog.Repo, folder string) (int, error) {
	return removeStale(ctx, repo, folder, nil)
}

// removeStale borra las fuentes localfs bajo folder que no estén en found.
// found nil borra todas las de esa carpeta (desregistro completo); found no
// nil borra solo las que desaparecieron del disco en este escaneo (los
// ficheros borrados de un volumen conectado; Brief M2 §Alta en catálogo).
func removeStale(ctx context.Context, repo *catalog.Repo, folder string, found map[string]struct{}) (int, error) {
	existing, err := repo.SourcesByProvider(ctx, localfs.ProviderID)
	if err != nil {
		return 0, err
	}
	prefix := localfs.NormalizePath(folder)

	n := 0
	for _, src := range existing {
		if !underFolder(src.Locator, prefix) {
			continue
		}
		if found != nil {
			if _, ok := found[localfs.NormalizePath(src.Locator)]; ok {
				continue
			}
		}
		if err := repo.DeleteSource(ctx, src.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func underFolder(locator, normFolder string) bool {
	nl := localfs.NormalizePath(locator)
	return nl == normFolder || strings.HasPrefix(nl, normFolder+string(filepath.Separator))
}

func timeNow() string { return time.Now().UTC().Format(time.RFC3339) }
