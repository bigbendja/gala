package identify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/scan"
)

type fakeEnum struct{}

func (fakeEnum) Volumes(_ context.Context, _ []string) ([]localfs.Volume, error) { return nil, nil }

// mustWriteVideo crea un fichero "vídeo" que supera el MinFileSize de M2
// (10 MB) sin escribir 10 MB de verdad: un fichero disperso con el tamaño
// correcto es instantáneo y localfs.WalkFolder solo mira el tamaño.
func mustWriteVideo(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("crear %s: %v", path, err)
	}
	defer f.Close()
	if err := f.Truncate(localfs.MinFileSize + 1024); err != nil {
		t.Fatalf("truncate %s: %v", path, err)
	}
}

func waitScanIdle(t *testing.T, sc *scan.Scanner) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !sc.Status().Running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout esperando a que el escaneo termine")
}

// TestE2EScanThenIdentify reproduce el flujo completo del Brief M3 §6:
// escaneo M2 real sobre un árbol temporal con nombres representativos del
// corpus, seguido del drenado de la cola de M3 contra un TMDB simulado.
// Verifica los tres desenlaces posibles: confirmado (ALTA), Junk, y "Por
// confirmar" sin candidatos (BAJA/sin resultados).
func TestE2EScanThenIdentify(t *testing.T) {
	dir := t.TempDir()
	mustWriteVideo(t, filepath.Join(dir, "Inception.2010.1080p.BluRay.x264-SPARKS.mkv"))
	mustWriteVideo(t, filepath.Join(dir, "VID-20230412-WA0012.mp4"))
	mustWriteVideo(t, filepath.Join(dir, "Aventura Desconocida Sin Relacion.mkv"))

	// TMDB simulado: enruta por query real (a diferencia de tmdbRoutes,
	// que no distingue query), para que solo "Inception" encuentre
	// resultado y la búsqueda genérica quede sin resultados (BAJA).
	tmdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/movie/27205"):
			w.Write(fixture(t, "movie_27205.json"))
		case strings.Contains(strings.ToLower(r.URL.Query().Get("query")), "inception"):
			w.Write(fixture(t, "search_movie_inception.json"))
		default:
			w.Write(fixture(t, "search_movie_empty.json"))
		}
	}))
	t.Cleanup(tmdbSrv.Close)

	repo := newRepoForTest(t)
	cfg := config.NewAt(t.TempDir())
	cfg.MediaFolders = []string{dir}
	log := discardLogger()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Escaneo M2 real: recorre el árbol temporal y da de alta los items
	// provisionales (confirmed=0, identify_status=pending por defecto).
	sc := scan.New(ctx, cfg, repo, fakeEnum{}, log)
	if err := sc.Start(nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitScanIdle(t, sc)

	n, err := repo.CountItems(ctx)
	if err != nil {
		t.Fatalf("CountItems: %v", err)
	}
	if n != 3 {
		t.Fatalf("CountItems = %d, esperado 3 (el escaneo debe dar de alta los 3 ficheros)", n)
	}

	// Cola de M3: normaliza, busca en TMDB (simulado) y aplica/encola.
	client := NewClient("test-key", t.TempDir(), repo)
	client.BaseURL = tmdbSrv.URL
	client.ImageBase = tmdbSrv.URL
	q := NewQueue(ctx, repo, client, log)
	q.drain()

	list, err := repo.List(ctx, catalog.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var confirmedMovie, junk, pendingNoCandidates bool
	for _, it := range list.Items {
		switch {
		case it.Confirmed && it.Kind == catalog.KindMovie:
			confirmedMovie = true
		case it.IdentifyStatus == catalog.IdentifyJunk:
			junk = true
		case it.IdentifyStatus == catalog.IdentifyReview && !it.Confirmed:
			pendingNoCandidates = true
		}
	}
	if !confirmedMovie {
		t.Error("no se encontró ningún item confirmado como película (ALTA)")
	}
	if !junk {
		t.Error("no se encontró el item Junk (VID-...-WA...)")
	}
	if !pendingNoCandidates {
		t.Error("no se encontró el item 'Por confirmar' sin resultados (BAJA)")
	}

	pending, err := repo.ListPendingConfirmation(ctx)
	if err != nil {
		t.Fatalf("ListPendingConfirmation: %v", err)
	}
	if len(pending) != 2 { // Junk + BAJA sin candidatos
		t.Errorf("ListPendingConfirmation = %d items, esperado 2", len(pending))
	}
}
