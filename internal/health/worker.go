package health

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
)

// periodicInterval es el re-chequeo completo periódico (Brief M6 §3: "cada
// 24 h para re-chequear todo").
const periodicInterval = 24 * time.Hour

// minRecheckWindow: una fuente vista hace menos de esto no se vuelve a
// verificar en una pasada normal, salvo fallo explícito o petición de
// verificación completa (Brief M6 §3, "presupuesto": evita tormentas de
// I/O/red en hardware modesto).
const minRecheckWindow = 30 * time.Minute

// maxConcurrentChecks limita cuántas verificaciones corren a la vez (Brief
// M6 §3, análogo al límite de sesiones HLS de M4 — stream.Manager).
const maxConcurrentChecks = 4

// ErrAlreadyRunning se devuelve al pedir una verificación completa mientras
// otra ya está en curso (simétrico a scan.ErrAlreadyRunning).
var ErrAlreadyRunning = errors.New("ya hay una verificación de salud en curso")

// Status es el estado consultable del worker (GET /api/v1/health/status).
type Status struct {
	Running      bool    `json:"running"`
	LastStarted  *string `json:"last_started,omitempty"`
	LastFinished *string `json:"last_finished,omitempty"`
	Checked      int     `json:"checked"`
	Pending      int     `json:"pending"`
	InFlight     int     `json:"in_flight"`
	BudgetTotal  int     `json:"budget_total"`
}

// Worker verifica en segundo plano la salud de las fuentes (Brief M6): se
// despierta al terminar un escaneo/ingesta (fuentes nuevas), cada 24h
// (re-chequeo completo periódico) y bajo demanda (POST /health/check, o un
// fallo de reproducción vía CheckNow). Mismo patrón que scan.Scanner/
// identify.Queue/media.Queue: se reconstruye siempre de BD, nunca de
// memoria — un reinicio a media verificación no pierde ni duplica trabajo.
type Worker struct {
	ctx      context.Context
	repo     *catalog.Repo
	checkers map[string]Checker
	fallback Checker
	log      *slog.Logger

	signal chan struct{}

	mu     sync.Mutex
	status Status
	force  bool // la próxima pasada ignora minRecheckWindow para todas las fuentes
}

// NewWorker crea el worker. ctx debe vivir mientras viva la aplicación,
// igual que scan.Scanner/identify.Queue/media.Queue. checkers mapea
// provider → Checker; cualquier provider sin entrada usa el fallback
// (sin_verificar, nunca caída).
func NewWorker(ctx context.Context, repo *catalog.Repo, checkers map[string]Checker, log *slog.Logger) *Worker {
	return &Worker{
		ctx: ctx, repo: repo, checkers: checkers, log: log,
		fallback: UnverifiableChecker{Detail: "provider desconocido"},
		signal:   make(chan struct{}, 1),
		status:   Status{BudgetTotal: maxConcurrentChecks},
	}
}

// Status devuelve una copia del estado actual.
func (w *Worker) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

// Enqueue despierta al worker sin forzar un re-chequeo completo: solo
// recoge lo nunca verificado o fuera de la ventana mínima (fuentes nuevas
// de un escaneo/ingesta recién terminados, Brief M6 §3 disparador "a"). No
// bloquea ni acumula señales.
func (w *Worker) Enqueue() {
	select {
	case w.signal <- struct{}{}:
	default:
	}
}

// CheckNow verifica UNA fuente concreta de inmediato, ignorando la ventana
// mínima, y persiste el resultado (Brief M6 §4: reconciliación de
// /sources/{id}/failed — un fallo de reproducción puede ser transitorio;
// esto dispara un re-check real en vez de fiarse solo del aviso del
// cliente). Se ejecuta síncronamente dentro de la petición que la llama:
// cada Checker acota su propio tiempo (como mucho los 8 s de HTTPChecker),
// un límite razonable para la respuesta a un solo clic de "esto ha
// fallado".
func (w *Worker) CheckNow(ctx context.Context, src catalog.Source) (healthState, detail string, err error) {
	healthState, detail = w.checkOne(ctx, src)
	if err = w.repo.UpdateSourceHealth(ctx, src.ID, healthState, detail, timeNow()); err != nil {
		return healthState, detail, err
	}
	w.mu.Lock()
	w.status.Checked++
	w.mu.Unlock()
	return healthState, detail, nil
}

// TriggerFull pide una verificación completa bajo demanda (POST
// /health/check, Brief M6 §5): ignora la ventana mínima para TODAS las
// fuentes en la próxima pasada. Un solo run concurrente, como
// scan.Scanner.Start.
func (w *Worker) TriggerFull() error {
	w.mu.Lock()
	if w.status.Running {
		w.mu.Unlock()
		return ErrAlreadyRunning
	}
	w.force = true
	w.mu.Unlock()
	w.Enqueue()
	return nil
}

// Run es el bucle principal (mismo patrón que identify.Queue/media.Queue):
// drena lo pendiente y espera a la siguiente señal de Enqueue/TriggerFull o
// al re-chequeo periódico de 24h. Termina al cancelarse ctx.
func (w *Worker) Run() {
	w.Enqueue() // pendientes de sesiones anteriores (se reconstruye de BD)
	ticker := time.NewTicker(periodicInterval)
	defer ticker.Stop()
	for {
		w.drain()
		select {
		case <-w.ctx.Done():
			return
		case <-w.signal:
		case <-ticker.C:
			w.mu.Lock()
			w.force = true
			w.mu.Unlock()
		}
	}
}

// drain verifica todas las fuentes que tocan en esta pasada, con
// concurrencia acotada a maxConcurrentChecks (Brief M6 §3, "presupuesto").
// Nota de apagado: si ctx se cancela con verificaciones en curso, drain deja
// de LANZAR nuevas pero espera a que las ya en marcha terminen (acotadas por
// su propio timeout, como mucho 8 s) antes de que Run() pueda volver a
// mirar ctx.Done(); no las cancela a media petición.
func (w *Worker) drain() {
	w.mu.Lock()
	w.status.Running = true
	started := timeNow()
	w.status.LastStarted = &started
	force := w.force
	w.force = false
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.status.Running = false
		finished := timeNow()
		w.status.LastFinished = &finished
		w.mu.Unlock()
	}()

	cutoff := ""
	if !force {
		cutoff = time.Now().Add(-minRecheckWindow).UTC().Format(time.RFC3339)
	}
	sources, err := w.repo.SourcesDueForHealthCheck(w.ctx, cutoff)
	if err != nil {
		w.log.Error("health: leer fuentes pendientes", "err", err)
		return
	}
	w.mu.Lock()
	w.status.Pending = len(sources)
	w.mu.Unlock()

	sem := make(chan struct{}, maxConcurrentChecks)
	var wg sync.WaitGroup
	for _, src := range sources {
		if w.ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		w.mu.Lock()
		w.status.InFlight++
		w.mu.Unlock()
		wg.Add(1)
		go func(s catalog.Source) {
			defer wg.Done()
			defer func() {
				<-sem
				w.mu.Lock()
				w.status.InFlight--
				w.mu.Unlock()
			}()
			w.checkAndStore(s)
		}(src)
	}
	wg.Wait()
}

func (w *Worker) checkAndStore(src catalog.Source) {
	healthState, detail := w.checkOne(w.ctx, src)
	if err := w.repo.UpdateSourceHealth(w.ctx, src.ID, healthState, detail, timeNow()); err != nil {
		w.log.Error("health: guardar resultado", "source_id", src.ID, "err", err)
	}
	w.mu.Lock()
	w.status.Checked++
	if w.status.Pending > 0 {
		w.status.Pending--
	}
	w.mu.Unlock()
}

func (w *Worker) checkOne(ctx context.Context, src catalog.Source) (string, string) {
	checker, ok := w.checkers[src.Provider]
	if !ok {
		checker = w.fallback
	}
	return checker.Check(ctx, src)
}

func timeNow() string { return time.Now().UTC().Format(time.RFC3339) }
