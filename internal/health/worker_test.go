package health

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
)

var workerTestSourceSeq int

func insertSourceForWorkerTest(t *testing.T, repo *catalog.Repo, lastChecked *string) int64 {
	t.Helper()
	ctx := context.Background()
	workerTestSourceSeq++
	locator := "/x/peli-" + strconv.Itoa(workerTestSourceSeq) + ".mp4"

	item := catalog.Item{Kind: catalog.KindMovie, Title: "peli", Genres: []string{}, Confirmed: true}
	itemID, err := repo.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: itemID, Provider: "localfs", Locator: locator}
	srcID, err := repo.InsertSource(ctx, &src, "")
	if err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	if lastChecked != nil {
		if err := repo.UpdateSourceHealth(ctx, srcID, catalog.HealthAlive, "", *lastChecked); err != nil {
			t.Fatalf("UpdateSourceHealth: %v", err)
		}
	}
	return srcID
}

// countingChecker registra cuántas veces se llama y el máximo de llamadas
// solapadas a la vez (Brief M6 §3/§6: "presupuesto").
type countingChecker struct {
	mu          sync.Mutex
	calls       int
	inFlight    int
	maxInFlight int
	sleep       time.Duration
}

func (c *countingChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	c.mu.Lock()
	c.calls++
	c.inFlight++
	if c.inFlight > c.maxInFlight {
		c.maxInFlight = c.inFlight
	}
	c.mu.Unlock()

	if c.sleep > 0 {
		time.Sleep(c.sleep)
	}

	c.mu.Lock()
	c.inFlight--
	c.mu.Unlock()
	return catalog.HealthAlive, ""
}

func (c *countingChecker) snapshot() (calls, maxInFlight int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, c.maxInFlight
}

func TestWorkerBudgetNeverExceedsConcurrencyLimit(t *testing.T) {
	repo := newTestRepo(t)
	const n = maxConcurrentChecks * 3
	for i := 0; i < n; i++ {
		insertSourceForWorkerTest(t, repo, nil)
	}
	checker := &countingChecker{sleep: 20 * time.Millisecond}
	w := NewWorker(context.Background(), repo, map[string]Checker{"localfs": checker}, discardLogger())

	w.drain()

	calls, maxInFlight := checker.snapshot()
	if calls != n {
		t.Fatalf("calls = %d, esperado %d (todas verificadas)", calls, n)
	}
	if maxInFlight > maxConcurrentChecks {
		t.Fatalf("maxInFlight = %d, esperado <= %d (presupuesto)", maxInFlight, maxConcurrentChecks)
	}
	if maxInFlight < maxConcurrentChecks {
		t.Errorf("maxInFlight = %d, esperado exactamente %d (si no, no se está paralelizando de verdad)", maxInFlight, maxConcurrentChecks)
	}
}

func TestWorkerMinRecheckWindowSkipsRecentlyCheckedUnlessForced(t *testing.T) {
	repo := newTestRepo(t)
	recent := time.Now().UTC().Format(time.RFC3339)
	insertSourceForWorkerTest(t, repo, &recent)

	checker := &countingChecker{}
	w := NewWorker(context.Background(), repo, map[string]Checker{"localfs": checker}, discardLogger())

	w.drain()
	if calls, _ := checker.snapshot(); calls != 0 {
		t.Fatalf("calls = %d, esperado 0 (fuente vista hace muy poco, sin forzar)", calls)
	}

	if err := w.TriggerFull(); err != nil {
		t.Fatalf("TriggerFull: %v", err)
	}
	w.drain()
	if calls, _ := checker.snapshot(); calls != 1 {
		t.Fatalf("calls tras forzar = %d, esperado 1", calls)
	}
}

func TestWorkerTriggerFullReturnsErrAlreadyRunningWhileRunning(t *testing.T) {
	repo := newTestRepo(t)
	insertSourceForWorkerTest(t, repo, nil)

	block := make(chan struct{})
	checker := blockingChecker{block: block}
	w := NewWorker(context.Background(), repo, map[string]Checker{"localfs": checker}, discardLogger())

	done := make(chan struct{})
	go func() {
		w.drain()
		close(done)
	}()

	waitUntil(t, func() bool { return w.Status().Running })

	if err := w.TriggerFull(); err != ErrAlreadyRunning {
		t.Fatalf("TriggerFull mientras corre = %v, esperado ErrAlreadyRunning", err)
	}

	close(block)
	<-done
	if w.Status().Running {
		t.Error("Running sigue true tras terminar drain()")
	}
}

// stoppingChecker cancela el contexto del worker tras la primera
// verificación completada, simulando un apagado a media pasada (Brief M6
// §6: "reconstrucción desde BD tras apagado").
type stoppingChecker struct {
	mu      sync.Mutex
	checked int
	cancel  context.CancelFunc
}

func (c *stoppingChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Millisecond):
	}
	c.mu.Lock()
	c.checked++
	n := c.checked
	c.mu.Unlock()
	if n == 1 {
		c.cancel()
	}
	return catalog.HealthAlive, ""
}

func TestWorkerRebuildsPendingWorkFromDBAcrossInstances(t *testing.T) {
	repo := newTestRepo(t)
	const n = maxConcurrentChecks * 3
	for i := 0; i < n; i++ {
		insertSourceForWorkerTest(t, repo, nil)
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	stopper := &stoppingChecker{}
	w1 := NewWorker(ctx1, repo, map[string]Checker{"localfs": stopper}, discardLogger())
	stopper.cancel = cancel1
	w1.drain()
	cancel1() // por si stopAfter no llegó a dispararse en esta pasada

	// Cuántas fuentes quedaron SIN marca de verificación (last_checked nil)
	// tras "apagar" el primer worker a media pasada.
	stillUnchecked := func() int {
		srcs, err := repo.SourcesByProvider(context.Background(), "localfs")
		if err != nil {
			t.Fatalf("SourcesByProvider: %v", err)
		}
		n := 0
		for _, s := range srcs {
			if s.LastChecked == nil {
				n++
			}
		}
		return n
	}

	if n := stillUnchecked(); n == 0 {
		t.Fatal("no quedó nada sin verificar tras 'apagar' el primer worker a media verificación; el test no prueba lo que dice probar")
	}

	// Un SEGUNDO worker, instancia nueva sin relación con w1 salvo el mismo
	// repo: reconstruye el trabajo pendiente de la BD, no de memoria.
	checker2 := &countingChecker{}
	w2 := NewWorker(context.Background(), repo, map[string]Checker{"localfs": checker2}, discardLogger())
	w2.drain()

	if n := stillUnchecked(); n != 0 {
		t.Fatalf("fuentes sin verificar tras el segundo worker = %d, esperado 0", n)
	}
}

func TestWorkerRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := newTestRepo(t)
	w := NewWorker(ctx, repo, map[string]Checker{}, discardLogger())
	cancel() // ya cancelado antes incluso de arrancar Run()

	done := make(chan struct{})
	go func() {
		w.Run()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() no terminó tras cancelar el contexto (Brief M6 §3: debe respetar el apagado)")
	}
}

func TestWorkerCheckNowForcesImmediateCheckAndPersists(t *testing.T) {
	repo := newTestRepo(t)
	srcID := insertSourceForWorkerTest(t, repo, nil)

	checker := &fixedChecker{health: catalog.HealthDead, detail: "archivo no encontrado"}
	w := NewWorker(context.Background(), repo, map[string]Checker{"localfs": checker}, discardLogger())

	src, err := repo.SourceByID(context.Background(), srcID)
	if err != nil {
		t.Fatalf("SourceByID: %v", err)
	}
	gotHealth, gotDetail, err := w.CheckNow(context.Background(), *src)
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if gotHealth != catalog.HealthDead || gotDetail != "archivo no encontrado" {
		t.Fatalf("CheckNow = (%q, %q), esperado (caída, archivo no encontrado)", gotHealth, gotDetail)
	}

	persisted, err := repo.SourceByID(context.Background(), srcID)
	if err != nil {
		t.Fatalf("SourceByID (2): %v", err)
	}
	if persisted.Health != catalog.HealthDead {
		t.Fatalf("Health persistido = %q, esperado caída", persisted.Health)
	}
	if persisted.HealthDetail == nil || *persisted.HealthDetail != "archivo no encontrado" {
		t.Fatalf("HealthDetail persistido = %v, esperado \"archivo no encontrado\"", persisted.HealthDetail)
	}
	if persisted.LastChecked == nil {
		t.Error("LastChecked no se rellenó")
	}
	if w.Status().Checked != 1 {
		t.Errorf("Status().Checked = %d, esperado 1", w.Status().Checked)
	}
}

func TestWorkerUnknownProviderFallsBackToUnverified(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	item := catalog.Item{Kind: catalog.KindMovie, Title: "peli", Genres: []string{}, Confirmed: true}
	itemID, err := repo.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: itemID, Provider: "futuro-provider", Locator: "loc"}
	srcID, err := repo.InsertSource(ctx, &src, "")
	if err != nil {
		t.Fatalf("InsertSource: %v", err)
	}

	w := NewWorker(ctx, repo, map[string]Checker{}, discardLogger())
	full, err := repo.SourceByID(ctx, srcID)
	if err != nil {
		t.Fatalf("SourceByID: %v", err)
	}
	got, _, err := w.CheckNow(ctx, *full)
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if got != catalog.HealthUnverified {
		t.Fatalf("CheckNow(provider desconocido) = %q, esperado sin_verificar", got)
	}
}

// fixedChecker devuelve siempre el mismo resultado, para probar
// determinísticamente la reconciliación y la persistencia.
type fixedChecker struct {
	health string
	detail string
}

func (c *fixedChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	return c.health, c.detail
}

// blockingChecker bloquea hasta que se cierra block, para poder observar
// Status().Running de forma determinista en mitad de una verificación.
type blockingChecker struct{ block <-chan struct{} }

func (c blockingChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	<-c.block
	return catalog.HealthAlive, ""
}

// waitUntil espera hasta 2s a que cond() sea true, comprobando cada
// milisegundo; falla el test si no se cumple a tiempo. Sincronización de
// test estándar para una condición de una goroutine en marcha, no un reintento
// de una operación que pueda fallar.
func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condición no se cumplió a tiempo")
}
