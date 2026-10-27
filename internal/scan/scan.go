// Package scan orquesta el escaneo de carpetas locales: recorre las
// carpetas registradas con localfs.WalkFolder y reconcilia el resultado con
// el catálogo (altas, "sigue vivo", bajas), con exclusión mutua y estado
// consultable por la API (Brief M2 §4).
package scan

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/providers/httpprovider"
	"github.com/bigbendja/gala/internal/providers/localfs"
)

// ErrAlreadyRunning se devuelve al pedir un escaneo mientras otro está en curso.
var ErrAlreadyRunning = errors.New("ya hay un escaneo en curso")

// batchSize agrupa las altas en transacciones para no castigar el disco con
// un commit por fichero (Brief M2 §Alta en catálogo).
const batchSize = 200

// strmRemovedProvider es el "provider" bajo el que removed_sources recuerda
// qué item tenía un .strm retirado (la clave es la ruta del .strm).
const strmRemovedProvider = "strm"

// strmDNSTimeout acota la comprobación SSRF de la URL de cada .strm: sin red
// el escaneo no debe quedarse esperando a un DNS que no responde.
const strmDNSTimeout = 3 * time.Second

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

	// AfterScan, si no es nil, se llama al terminar cada escaneo (fin de
	// escaneo con éxito o no). La usa la cola de identificación de M3 para
	// despertarse y procesar los items provisionales recién creados
	// (Brief M3 §4: "fin de escaneo de M2"). Opcional: nil no rompe nada.
	AfterScan func()
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
		if s.AfterScan != nil {
			s.AfterScan()
		}
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

	// Los .strm se leen y validan (SSRF, con DNS) ANTES de abrir la
	// transacción: no se retiene el candado de escritura de SQLite esperando
	// a la red.
	strmURLs := s.prepareStrm(chunk)

	err := s.repo.Tx(s.ctx, func(tx *catalog.Repo) error {
		for _, f := range chunk {
			if f.Strm {
				rawURL, ok := strmURLs[f.Path]
				if !ok {
					continue // inválido: sin fuente; si tenía una, removeStale la retira
				}
				foundSet[localfs.NormalizePath(f.Path)] = struct{}{}
				filesSeen++
				isNew, serr := s.syncStrm(tx, f.Path, rawURL, now)
				if serr != nil {
					return serr
				}
				if isNew {
					itemsNew++
				}
				continue
			}
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

			// ¿Este fichero ya tuvo un item y su fuente se retiró (carpeta
			// quitada y vuelta a añadir, disco que volvió)? Reutilizarlo en
			// vez de duplicar: conserva identificación, deseos y progreso.
			itemID, reused, cerr := tx.ClaimRemovedSource(s.ctx, localfs.ProviderID, localfs.NormalizePath(f.Path))
			if cerr != nil {
				return cerr
			}
			if !reused {
				title := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
				item := catalog.Item{Kind: catalog.KindOther, Title: title, Genres: []string{}, Confirmed: false}
				newID, ierr := tx.InsertItem(s.ctx, &item, now)
				if ierr != nil {
					return ierr
				}
				itemID = newID
			}
			src := catalog.Source{
				ItemID: itemID, Provider: localfs.ProviderID, Locator: f.Path,
				VolumeID: volID, Health: catalog.HealthAlive,
			}
			if _, serr := tx.InsertSource(s.ctx, &src, now); serr != nil {
				return serr
			}
			if !reused {
				itemsNew++
			}
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

// prepareStrm lee cada .strm del lote y devuelve ruta -> URL de los válidos.
// Un .strm vacío, sin URL http/https, ilegible o cuya URL apunta a una red
// interna (SSRF de M5) NO crea fuente: se registra el motivo y se omite, de
// modo que nunca hay una fuente rota ni un item sin fuente por su causa.
// Que el DNS no resuelva (p. ej. sin conexión) no rechaza: la URL es
// sintácticamente válida y la salud (M6) dirá si está viva.
func (s *Scanner) prepareStrm(chunk []localfs.FoundFile) map[string]string {
	var out map[string]string
	hostVerdict := map[string]error{} // un solo DNS por host y lote
	for _, f := range chunk {
		if !f.Strm {
			continue
		}
		rawURL, err := localfs.ReadStrmURL(f.Path)
		if err != nil {
			s.log.Warn(".strm omitido", "path", f.Path, "err", err)
			continue
		}
		if verr := s.checkStrmURL(rawURL, hostVerdict); verr != nil {
			s.log.Warn(".strm omitido: URL no permitida", "path", f.Path, "err", verr)
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[f.Path] = rawURL
	}
	return out
}

// checkStrmURL aplica el bloqueo SSRF a la URL de un .strm. Solo un destino
// interno (ErrBlockedTarget) la rechaza.
func (s *Scanner) checkStrmURL(rawURL string, cache map[string]error) error {
	host := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		host = u.Hostname()
	}
	if verdict, ok := cache[host]; ok {
		return verdict
	}
	ctx, cancel := context.WithTimeout(s.ctx, strmDNSTimeout)
	defer cancel()
	var verdict error
	if err := httpprovider.ValidateExternalURL(ctx, rawURL); errors.Is(err, httpprovider.ErrBlockedTarget) {
		verdict = err
	}
	cache[host] = verdict
	return verdict
}

// syncStrm da de alta o actualiza la fuente http de un .strm. Devuelve true
// si creó un item nuevo. El item se llama como el .strm (sin extensión) y la
// cola de identificación (M3) lo identificará por ese nombre y su carpeta.
func (s *Scanner) syncStrm(tx *catalog.Repo, path, rawURL, now string) (bool, error) {
	existing, err := tx.SourceByOriginPath(s.ctx, path)
	if err != nil && !errors.Is(err, catalog.ErrNotFound) {
		return false, err
	}
	if err == nil {
		// Mismo .strm de siempre. Si su enlace cambió (las herramientas que
		// los escriben rotan URLs), la fuente sigue al archivo.
		if existing.Locator != rawURL {
			if _, cerr := tx.SourceByLocator(s.ctx, httpprovider.ProviderID, rawURL); cerr == nil {
				s.log.Warn(".strm con una URL que ya está en el catálogo; se deja la anterior", "path", path)
				return false, nil
			} else if !errors.Is(cerr, catalog.ErrNotFound) {
				return false, cerr
			}
			return false, tx.UpdateSourceLocator(s.ctx, existing.ID, rawURL)
		}
		return false, nil
	}

	// Una URL ya catalogada (de otro .strm o de una ingesta) no se duplica:
	// UNIQUE(provider, locator) y un mismo enlace no es dos fuentes.
	if _, cerr := tx.SourceByLocator(s.ctx, httpprovider.ProviderID, rawURL); cerr == nil {
		s.log.Warn(".strm omitido: su URL ya está catalogada", "path", path)
		return false, nil
	} else if !errors.Is(cerr, catalog.ErrNotFound) {
		return false, cerr
	}

	itemID, reused, err := tx.ClaimRemovedSource(s.ctx, strmRemovedProvider, localfs.NormalizePath(path))
	if err != nil {
		return false, err
	}
	if !reused {
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		item := catalog.Item{Kind: catalog.KindOther, Title: title, Genres: []string{}, Confirmed: false}
		if itemID, err = tx.InsertItem(s.ctx, &item, now); err != nil {
			return false, err
		}
	}
	origin := path
	src := catalog.Source{
		ItemID: itemID, Provider: httpprovider.ProviderID, Locator: rawURL,
		Health: catalog.HealthUnverified, OriginPath: &origin,
	}
	if _, err := tx.InsertSource(s.ctx, &src, now); err != nil {
		return false, err
	}
	return !reused, nil
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
		// Dejar rastro del item antes de borrar la fuente: si el fichero
		// vuelve (carpeta re-añadida, USB reconectado) se reutiliza el item.
		if err := repo.RememberRemovedSource(ctx, localfs.ProviderID,
			localfs.NormalizePath(src.Locator), src.ItemID, timeNow()); err != nil {
			return n, err
		}
		if err := repo.DeleteSource(ctx, src.ID); err != nil {
			return n, err
		}
		n++
	}

	// Fuentes nacidas de un .strm bajo la carpeta: se retiran si el .strm ya
	// no está (o dejó de ser válido). El item se conserva con su rastro.
	strms, err := repo.SourcesWithOriginPath(ctx)
	if err != nil {
		return n, err
	}
	for _, src := range strms {
		if src.OriginPath == nil || !underFolder(*src.OriginPath, prefix) {
			continue
		}
		key := localfs.NormalizePath(*src.OriginPath)
		if found != nil {
			if _, ok := found[key]; ok {
				continue
			}
		}
		if err := repo.RememberRemovedSource(ctx, strmRemovedProvider, key, src.ItemID, timeNow()); err != nil {
			return n, err
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
