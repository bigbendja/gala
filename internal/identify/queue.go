package identify

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"path/filepath"
	"sync"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
)

// retryInterval es el intervalo de reintento cuando no hay red, con jitter
// (Brief M3 §4: "reintento periódico cada 5 min con jitter").
const retryInterval = 5 * time.Minute

// retryJitter es el margen aleatorio añadido a retryInterval para que
// varias instancias no reintenten todas a la vez.
const retryJitter = 30 * time.Second

// Status es el estado consultable de la cola (GET /api/v1/identify/status).
type Status struct {
	Pending    int    `json:"pending"`
	Processing bool   `json:"processing"`
	Online     bool   `json:"online"`
	LastError  string `json:"last_error,omitempty"`
	Processed  int    `json:"processed"`
}

// Queue es la cola de enriquecimiento asíncrona de M3 (Brief §4): por cada
// item provisional (confirmed=0) del catálogo, normaliza el nombre de su
// fuente, busca en TMDB y aplica la identificación o lo deja "Por
// confirmar". Se reconstruye siempre de la BD, nunca de memoria: un
// reinicio a mitad de proceso no pierde trabajo.
type Queue struct {
	ctx    context.Context
	repo   *catalog.Repo
	client *Client
	log    *slog.Logger

	signal chan struct{}

	mu     sync.Mutex
	status Status
}

// NewQueue crea la cola. ctx debe vivir mientras viva la aplicación (igual
// que scan.Scanner): el drenado y el reintento periódico deben sobrevivir
// a peticiones HTTP concretas y solo pararse al apagar GALA.
func NewQueue(ctx context.Context, repo *catalog.Repo, client *Client, log *slog.Logger) *Queue {
	return &Queue{
		ctx: ctx, repo: repo, client: client, log: log,
		signal: make(chan struct{}, 1),
		status: Status{Online: true},
	}
}

// Status devuelve una copia del estado actual.
func (q *Queue) Status() Status {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.status
}

// Enqueue despierta a la cola para que revise pendientes: al terminar un
// escaneo de M2 (Scanner.AfterScan), al confirmar manualmente un item, o al
// arrancar la app. No bloquea ni acumula señales: si ya hay una pendiente,
// esta llamada es un no-op (el próximo drenado ya cubre lo nuevo).
func (q *Queue) Enqueue() {
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

// Run es el bucle principal: drena lo pendiente y, si se queda sin nada
// que hacer (o sin red), espera hasta la siguiente señal de Enqueue o hasta
// el reintento periódico. Debe lanzarse en su propia goroutine; termina al
// cancelarse ctx (Brief §4: "respetar context de apagado").
func (q *Queue) Run() {
	q.Enqueue() // pendientes de sesiones anteriores (Brief §4)
	for {
		offline := q.drain()
		if !offline && q.ctx.Err() == nil {
			q.backfillImages()
			q.enrichEpisodes()
		}

		wait := retryInterval + time.Duration(rand.Int63n(int64(retryJitter)))
		timer := time.NewTimer(wait)
		select {
		case <-q.ctx.Done():
			timer.Stop()
			return
		case <-q.signal:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// drain procesa todos los items pendientes que pueda. Si TMDB no responde
// (ErrOffline), se detiene y devuelve true: los que quedan siguen en BD como
// pending/error y se reintentan en la siguiente pasada (Brief §4). También
// se detiene y devuelve true si una pasada completa no resuelve NINGÚN item
// (todos vuelven a quedar en error): PendingIdentify incluye items en error
// a propósito para reintentarlos, pero un fallo persistente por item (p. ej.
// una API key de TMDB inválida: 401, ni offline ni transitorio) los deja
// exactamente en el mismo estado en el siguiente vistazo — sin este corte,
// el bucle exterior los reprocesaría sin fin, cientos de veces por segundo,
// en vez de esperar al siguiente drenado periódico como si fuera offline
// (bug real observado: cola martillando TMDB sin backoff con una key rota).
func (q *Queue) drain() (offline bool) {
	q.mu.Lock()
	q.status.Processing = true
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		q.status.Processing = false
		q.mu.Unlock()
	}()

	for {
		if q.ctx.Err() != nil {
			return false
		}
		items, err := q.repo.PendingIdentify(q.ctx)
		if err != nil {
			q.log.Error("identify: leer pendientes", "err", err)
			return false
		}
		q.mu.Lock()
		q.status.Pending = len(items)
		q.mu.Unlock()
		if len(items) == 0 {
			return false
		}

		hitOffline := false
		progressed := false
		var lastItemErr error
		for _, it := range items {
			if q.ctx.Err() != nil {
				return false
			}
			if err := q.processOne(it); err != nil {
				if errors.Is(err, ErrOffline) {
					q.mu.Lock()
					q.status.Online = false
					q.status.LastError = err.Error()
					q.mu.Unlock()
					hitOffline = true
					break
				}
				lastItemErr = err
				q.log.Warn("identify: procesar item", "item_id", it.ID, "err", err)
				continue
			}
			progressed = true
			q.mu.Lock()
			q.status.Processed++
			q.mu.Unlock()
		}
		if hitOffline {
			return true
		}
		q.mu.Lock()
		q.status.Online = true
		if progressed {
			q.status.LastError = ""
		} else if lastItemErr != nil {
			q.status.LastError = lastItemErr.Error()
		}
		q.mu.Unlock()

		if !progressed {
			return true
		}
	}
}

// backfillImages reintenta el póster/fondo de items YA confirmados a los que
// se les perdió la imagen (Brief M3, corrección post-entrega: el CDN de
// imágenes de TMDB es menos fiable que su API; un fallo ahí no debe perderse
// para siempre solo porque el item ya salió de "pendientes" y por tanto
// nunca vuelve a pasar por processOne). El detalle viene de tmdb_cache (ya
// se pidió al confirmar), así que esto no repite la búsqueda ni el matching,
// solo reintenta la descarga de la imagen que faltó.
func (q *Queue) backfillImages() {
	items, err := q.repo.ItemsMissingImages(q.ctx)
	if err != nil {
		q.log.Error("identify: leer items sin imagen", "err", err)
		return
	}
	for _, it := range items {
		if q.ctx.Err() != nil {
			return
		}
		if it.TMDBID == nil {
			continue
		}

		var posterSrc, backdropSrc string
		if it.Kind == catalog.KindSeries {
			d, err := q.client.TVDetail(q.ctx, *it.TMDBID)
			if err != nil {
				q.log.Warn("identify: reintentar imagen (detalle serie)", "item_id", it.ID, "err", err)
				continue
			}
			posterSrc, backdropSrc = d.PosterPath, d.BackdropPath
		} else {
			d, err := q.client.MovieDetail(q.ctx, *it.TMDBID)
			if err != nil {
				q.log.Warn("identify: reintentar imagen (detalle película)", "item_id", it.ID, "err", err)
				continue
			}
			posterSrc, backdropSrc = d.PosterPath, d.BackdropPath
		}

		var poster, backdrop *string
		if it.PosterPath == nil {
			if name, err := q.client.DownloadPoster(q.ctx, posterSrc); err != nil {
				q.log.Warn("identify: reintentar póster", "item_id", it.ID, "err", err)
			} else if name != "" {
				poster = &name
			}
		}
		if it.BackdropPath == nil {
			if name, err := q.client.DownloadBackdrop(q.ctx, backdropSrc); err != nil {
				q.log.Warn("identify: reintentar fondo", "item_id", it.ID, "err", err)
			} else if name != "" {
				backdrop = &name
			}
		}
		if poster == nil && backdrop == nil {
			continue
		}
		if err := q.repo.UpdateItemImages(q.ctx, it.ID, poster, backdrop); err != nil {
			q.log.Error("identify: guardar imagen reintentada", "item_id", it.ID, "err", err)
		}
	}
}

// processOne normaliza, busca y aplica (o encola) la identificación de un
// item provisional. El nombre de archivo real y su carpeta contenedora se
// leen de la fuente en disco (sources.locator), no del Title del item: M2
// ya guarda el Title sin extensión, y el normalizador necesita el nombre
// de archivo completo para su propio pipeline de limpieza.
func (q *Queue) processOne(it catalog.Item) error {
	full, err := q.repo.Get(q.ctx, it.ID)
	if err != nil {
		return err
	}
	if len(full.Sources) == 0 {
		note := "item sin fuentes"
		return q.repo.SetIdentifyStatus(q.ctx, it.ID, catalog.IdentifyError, &note)
	}
	locator := full.Sources[0].Locator
	if op := full.Sources[0].OriginPath; op != nil {
		// Fuente nacida de un .strm: la obra se identifica por el nombre del
		// .strm y su carpeta (el locator es la URL, que no dice qué es).
		locator = *op
	}
	filename := filepath.Base(locator)
	folder := filepath.Base(filepath.Dir(locator))

	parsed := Parse(filename, folder)

	if parsed.Junk {
		return q.repo.SetIdentifyStatus(q.ctx, it.ID, catalog.IdentifyJunk, nil)
	}
	if parsed.Extra {
		// Brief §4: los extras no aparecen en la galería.
		return q.repo.DeleteItem(q.ctx, it.ID)
	}

	res, err := Match(q.ctx, q.client, parsed, filename)
	if err != nil {
		return q.markError(it.ID, err)
	}

	if res.Confidence == ConfidenceAlta {
		if err := ApplyIdentification(q.ctx, q.client, q.repo, it.ID, res.Best.ID, res.Kind, parsed); err != nil {
			return q.markError(it.ID, err)
		}
		return nil
	}

	var note *string
	if res.Note != "" {
		note = &res.Note
	}
	return q.repo.SetIdentifyCandidates(q.ctx, it.ID, res.Candidates, parsed.Season, parsed.Episode, note)
}

func (q *Queue) markError(itemID int64, cause error) error {
	note := cause.Error()
	if err := q.repo.SetIdentifyStatus(q.ctx, itemID, catalog.IdentifyError, &note); err != nil {
		q.log.Error("identify: marcar error", "item_id", itemID, "err", err)
	}
	return cause
}
