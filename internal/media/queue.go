package media

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/providers/localfs"
)

// Status es el estado consultable de la cola de sondeo (análogo a
// identify.Status, Brief M4 §2).
type Status struct {
	Pending    int  `json:"pending"`
	Processing bool `json:"processing"`
	Processed  int  `json:"processed"`
}

// Queue sondea en segundo plano las fuentes localfs que aún no tienen
// media_info: se engancha al fin del escaneo de M2 igual que la cola de
// identificación de M3 (mismo patrón, Brief M4 §2). Sin ffmpeg disponible,
// drain() es un no-op inmediato: el sistema decide por extensión (Decide) y
// ya se avisará con "no se pudo analizar" o al fallar el navegador.
type Queue struct {
	ctx    context.Context
	repo   *catalog.Repo
	ffmpeg *FFmpeg
	log    *slog.Logger

	signal chan struct{}

	mu     sync.Mutex
	status Status
}

// NewQueue crea la cola. ctx debe vivir mientras viva la aplicación, igual
// que scan.Scanner e identify.Queue.
func NewQueue(ctx context.Context, repo *catalog.Repo, ffmpeg *FFmpeg, log *slog.Logger) *Queue {
	return &Queue{ctx: ctx, repo: repo, ffmpeg: ffmpeg, log: log, signal: make(chan struct{}, 1)}
}

// Status devuelve una copia del estado actual.
func (q *Queue) Status() Status {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.status
}

// Enqueue despierta a la cola para que revise pendientes: al terminar un
// escaneo de M2 (Scanner.AfterScan) o al pedir /play y faltar el sondeo de
// alguna otra fuente del mismo item. No bloquea ni acumula señales.
func (q *Queue) Enqueue() {
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

// Run es el bucle principal: drena lo pendiente y espera a la siguiente
// señal de Enqueue. Termina al cancelarse ctx.
func (q *Queue) Run() {
	q.Enqueue() // pendientes de sesiones anteriores (se reconstruye de BD)
	for {
		q.drain()
		select {
		case <-q.ctx.Done():
			return
		case <-q.signal:
		}
	}
}

// drain sonda todas las fuentes localfs sin media_info que pueda. Sin
// ffmpeg, no hay nada que hacer (Brief M4 §1: nunca abortar, solo degradar).
func (q *Queue) drain() {
	if !q.ffmpeg.Available() {
		return
	}
	q.mu.Lock()
	q.status.Processing = true
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		q.status.Processing = false
		q.mu.Unlock()
	}()

	sources, err := q.repo.SourcesWithoutMediaInfo(q.ctx, localfs.ProviderID)
	if err != nil {
		q.log.Error("media: leer fuentes sin sondear", "err", err)
		return
	}
	q.mu.Lock()
	q.status.Pending = len(sources)
	q.mu.Unlock()

	for _, src := range sources {
		if q.ctx.Err() != nil {
			return
		}
		if _, err := ProbeAndStore(q.ctx, q.repo, q.ffmpeg, src); err != nil {
			q.log.Warn("media: sondear fuente", "source_id", src.ID, "locator", src.Locator, "err", err)
		}
		q.mu.Lock()
		q.status.Pending--
		q.status.Processed++
		q.mu.Unlock()
	}
}

// ProbeAndStore sonda una fuente concreta y persiste el resultado en
// media_info. Se usa tanto desde drain() como bajo demanda al pedir /play si
// aún falta el sondeo de alguna fuente (Brief M4 §2). Sin ffmpeg disponible
// devuelve ErrNoFFmpeg sin tocar la BD.
func ProbeAndStore(ctx context.Context, repo *catalog.Repo, ffmpeg *FFmpeg, src catalog.Source) (*MediaInfo, error) {
	if !ffmpeg.Available() {
		return nil, ErrNoFFmpeg
	}
	info, err := Probe(ctx, ffmpeg.FFprobePath, src.Locator)
	if err != nil {
		return nil, err
	}
	if err := repo.UpsertMediaInfo(ctx, catalog.MediaInfo{
		SourceID: src.ID, Container: info.Container, DurationS: info.DurationS,
		VideoCodec: info.VideoCodec, AudioCodec: info.AudioCodec,
		Width: info.Width, Height: info.Height, Bitrate: info.Bitrate,
		ProbedAt: info.ProbedAt.Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}
	return info, nil
}
