package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/api"
	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/health"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
)

// fakeHealthChecker devuelve siempre el mismo resultado: dicta
// determinísticamente el desenlace de una reconciliación sin I/O real.
type fakeHealthChecker struct {
	health string
	detail string
}

func (f fakeHealthChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	return f.health, f.detail
}

// blockingHealthChecker bloquea hasta que se cierra block: permite observar
// de forma determinista que un health.Worker sigue "en curso" a mitad de
// una verificación (para el 409 de POST /health/check).
type blockingHealthChecker struct{ block <-chan struct{} }

func (b blockingHealthChecker) Check(ctx context.Context, s catalog.Source) (string, string) {
	<-b.block
	return catalog.HealthAlive, ""
}

type healthTestServer struct {
	*httptest.Server
	repo   *catalog.Repo
	worker *health.Worker
}

func newHealthTestServer(t *testing.T, checkers map[string]health.Checker) *healthTestServer {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	repo := catalog.NewRepo(st.DB)
	cfg := config.NewAt(dir)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	sc := scan.New(ctx, cfg, repo, fakeEnum{}, log)

	worker := health.NewWorker(ctx, repo, checkers, log)
	go worker.Run()

	srv := api.New(st, repo, cfg, sc, fakeEnum{}, nil, nil, dir, log, nil, nil, nil, worker)
	mux := http.NewServeMux()
	srv.Routes(mux)
	hts := httptest.NewServer(mux)
	t.Cleanup(func() { hts.Close(); cancel(); st.Close() })
	return &healthTestServer{Server: hts, repo: repo, worker: worker}
}

func insertSourceForHealthAPITest(t *testing.T, repo *catalog.Repo, provider, locator, healthState string) int64 {
	t.Helper()
	ctx := context.Background()
	item := catalog.Item{Kind: catalog.KindMovie, Title: "peli", Genres: []string{}, Confirmed: true}
	itemID, err := repo.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: itemID, Provider: provider, Locator: locator, Health: healthState}
	srcID, err := repo.InsertSource(ctx, &src, "")
	if err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	return srcID
}

func TestHandleHealthSummary(t *testing.T) {
	its := newHealthTestServer(t, nil)
	insertSourceForHealthAPITest(t, its.repo, "localfs", "/x/a.mp4", catalog.HealthAlive)
	insertSourceForHealthAPITest(t, its.repo, "localfs", "/x/b.mp4", catalog.HealthDead)

	resp, err := http.Get(its.URL + "/api/v1/health/summary")
	if err != nil {
		t.Fatalf("GET /health/summary: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var view struct {
		Total, Viva, Lenta, Caida, Desconectada, SinVerificar int
	}
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Total != 2 || view.Viva != 1 || view.Caida != 1 {
		t.Fatalf("summary = %+v, esperado total=2 viva=1 caida=1", view)
	}
}

func TestHandleHealthStatus(t *testing.T) {
	its := newHealthTestServer(t, nil)
	resp, err := http.Get(its.URL + "/api/v1/health/status")
	if err != nil {
		t.Fatalf("GET /health/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var st health.Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.BudgetTotal <= 0 {
		t.Errorf("BudgetTotal = %d, esperado > 0", st.BudgetTotal)
	}
}

func TestHandleHealthCheckTriggersAndConflictsWhileRunning(t *testing.T) {
	block := make(chan struct{})
	checkers := map[string]health.Checker{"localfs": blockingHealthChecker{block: block}}
	its := newHealthTestServer(t, checkers)
	insertSourceForHealthAPITest(t, its.repo, "localfs", "/x/a.mp4", catalog.HealthUnverified)

	resp, err := http.Post(its.URL+"/api/v1/health/check", "", nil)
	if err != nil {
		t.Fatalf("POST /health/check: %v", err)
	}
	resp.Body.Close()
	// 202 (lanzada ahora) o 409 (el arranque del worker ya estaba verificando
	// la fuente recién insertada: carrera legítima, vista en CI). En ambos
	// casos hay una verificación en curso, que es lo que prueba el resto.
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, esperado 202 (o 409 si el arranque ya la estaba haciendo)", resp.StatusCode)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !its.worker.Status().Running {
		time.Sleep(time.Millisecond)
	}
	if !its.worker.Status().Running {
		t.Fatal("el worker no llegó a 'en curso' a tiempo")
	}

	resp2, err := http.Post(its.URL+"/api/v1/health/check", "", nil)
	if err != nil {
		t.Fatalf("POST /health/check (2): %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("status (2) = %d, esperado 409 (ya hay una verificación en curso)", resp2.StatusCode)
	}

	close(block)
}

// TestHandleSourceFailedReconciliationTransientRecoversToAlive cubre la
// reconciliación de Brief M6 §4: un fallo de reproducción marca caída de
// inmediato, pero si el re-check real dice viva (fallo transitorio del
// cliente), la respuesta y la BD terminan en viva, no en caída.
func TestHandleSourceFailedReconciliationTransientRecoversToAlive(t *testing.T) {
	checkers := map[string]health.Checker{"localfs": fakeHealthChecker{health: catalog.HealthAlive}}
	its := newHealthTestServer(t, checkers)
	srcID := insertSourceForHealthAPITest(t, its.repo, "localfs", "/x/a.mp4", catalog.HealthAlive)

	resp, err := http.Post(its.URL+"/api/v1/sources/"+strconv.FormatInt(srcID, 10)+"/failed", "", nil)
	if err != nil {
		t.Fatalf("POST /sources/{id}/failed: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["health"] != catalog.HealthAlive {
		t.Fatalf("health en la respuesta = %q, esperado viva (fallo transitorio reconciliado)", body["health"])
	}

	got, err := its.repo.SourceByID(context.Background(), srcID)
	if err != nil {
		t.Fatalf("SourceByID: %v", err)
	}
	if got.Health != catalog.HealthAlive {
		t.Fatalf("Health en BD = %q, esperado viva", got.Health)
	}
}

// TestHandleSourceFailedReconciliationRealFailureStaysDead cubre el otro
// lado: si el re-check confirma que de verdad está caída, se queda caída.
func TestHandleSourceFailedReconciliationRealFailureStaysDead(t *testing.T) {
	checkers := map[string]health.Checker{
		"localfs": fakeHealthChecker{health: catalog.HealthDead, detail: "archivo no encontrado"},
	}
	its := newHealthTestServer(t, checkers)
	srcID := insertSourceForHealthAPITest(t, its.repo, "localfs", "/x/a.mp4", catalog.HealthAlive)

	resp, err := http.Post(its.URL+"/api/v1/sources/"+strconv.FormatInt(srcID, 10)+"/failed", "", nil)
	if err != nil {
		t.Fatalf("POST /sources/{id}/failed: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["health"] != catalog.HealthDead {
		t.Fatalf("health en la respuesta = %q, esperado caída", body["health"])
	}

	got, err := its.repo.SourceByID(context.Background(), srcID)
	if err != nil {
		t.Fatalf("SourceByID: %v", err)
	}
	if got.Health != catalog.HealthDead {
		t.Fatalf("Health en BD = %q, esperado caída", got.Health)
	}
	if got.HealthDetail == nil || *got.HealthDetail != "archivo no encontrado" {
		t.Fatalf("HealthDetail = %v, esperado el motivo del checker", got.HealthDetail)
	}
}

// TestHandleSourceFailedWithoutHealthWorkerStillMarksDead: sin worker
// configurado (nil-safe, como Ingest/HLS en otros tests), el endpoint sigue
// funcionando como en M4: marca caída sin reconciliar.
func TestHandleSourceFailedWithoutHealthWorkerStillMarksDead(t *testing.T) {
	its := newPlayTestServer(t, nil, nil)
	ctx := context.Background()
	item := catalog.Item{Kind: catalog.KindMovie, Title: "peli", Genres: []string{}, Confirmed: true}
	itemID, err := its.repo.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: itemID, Provider: "localfs", Locator: "/x/a.mp4", Health: catalog.HealthAlive}
	srcID, err := its.repo.InsertSource(ctx, &src, "")
	if err != nil {
		t.Fatalf("InsertSource: %v", err)
	}

	resp, err := http.Post(its.URL+"/api/v1/sources/"+strconv.FormatInt(srcID, 10)+"/failed", "", nil)
	if err != nil {
		t.Fatalf("POST /sources/{id}/failed: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["health"] != catalog.HealthDead {
		t.Fatalf("health = %q, esperado caída (sin worker, sin reconciliación)", body["health"])
	}
}
