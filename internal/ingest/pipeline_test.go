package ingest

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/providers/httpprovider"
	"github.com/bigbendja/gala/internal/store"
)

func newRepoForTest(t *testing.T) *catalog.Repo {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return catalog.NewRepo(st.DB)
}

// fakeM3UTransport sirve contenido M3U fijo para hosts/rutas conocidos; el
// resto falla explícitamente (para detectar peticiones no esperadas en los
// tests, en vez de colgarse intentando red real).
type fakeM3UTransport struct {
	responses map[string]fakeM3UResponse
}

type fakeM3UResponse struct {
	status      int
	body        string
	contentType string
}

func (f fakeM3UTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key := req.URL.Host + req.URL.Path
	resp, ok := f.responses[key]
	if !ok {
		return nil, errors.New("fakeM3UTransport: petición no esperada a " + key)
	}
	status := resp.status
	if status == 0 {
		status = http.StatusOK
	}
	header := make(http.Header)
	if resp.contentType != "" {
		header.Set("Content-Type", resp.contentType)
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(resp.body)),
		Header:     header,
	}, nil
}

// withFakeDNS sustituye la resolución DNS de httpprovider por una que
// siempre devuelve una IP pública fija, y la restaura al terminar el test:
// evita depender de que dominios de prueba (p. ej. "ejemplo.net", el que usa
// el texto exacto del mockup) resuelvan de verdad.
func withFakeDNS(t *testing.T) {
	t.Helper()
	orig := httpprovider.LookupIPAddr
	httpprovider.LookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	t.Cleanup(func() { httpprovider.LookupIPAddr = orig })
}

func newPipelineForTest(t *testing.T, responses map[string]fakeM3UResponse) (*Pipeline, *catalog.Repo) {
	t.Helper()
	withFakeDNS(t)
	repo := newRepoForTest(t)
	p := NewPipeline(repo, nil, nil)
	if responses != nil {
		p.HTTPClient = &http.Client{Transport: fakeM3UTransport{responses: responses}}
	}
	return p, repo
}

const testNow = "2026-01-01T00:00:00Z"

// assertResumenInvariant comprueba la invariante de ResumenIngesta pedida
// tras el addendum de corrección de contadores: Total debe ser SIEMPRE la
// suma exacta de los seis resultados mutuamente excluyentes, para cualquier
// texto de entrada. Se llama en todos los tests del pipeline, no solo en uno
// dedicado, porque cualquier caso nuevo (dedupe, SSRF, límite, etc.) podría
// reintroducir el mismo descuadre que motivó la corrección.
func assertResumenInvariant(t *testing.T, res *ResumenIngesta) {
	t.Helper()
	sum := res.Anadidos + res.Duplicados + res.PendientesConector +
		res.OmitidosNoSoportados + res.OmitidosNoDirectos + res.SinClasificar + res.Basura
	if sum != res.Total {
		t.Errorf("invariante rota: Anadidos+Duplicados+PendientesConector+OmitidosNoSoportados+SinClasificar+Basura = %d, pero Total = %d (%+v)",
			sum, res.Total, res)
	}
}

// --- E2E: el texto EXACTO del mockup (docs/GALA_Mockup.html) ---------------

// TestPipelineRunMockupTextExact pega EXACTAMENTE el texto de ejemplo del
// modal de ingesta del mockup (Brief M5, criterio de "hecho" del plan) y
// comprueba el ResumenIngesta campo por campo. La lista M3U del mockup
// apunta a un dominio ficticio ("ejemplo.net") que no existe de verdad; se
// sirve con un Transport falso + DNS falso para poder correr el texto
// literal sin red real (ver informe: los números del propio mockup son
// ilustrativos de diseño, no reproducibles al pie de la letra — p. ej.
// muestra un título ya resuelto para el enlace de Drive, que M5 explícita-
// mente NO resuelve).
func TestPipelineRunMockupTextExact(t *testing.T) {
	const mockupText = "🎬 mira lo q me pasaron!!\n" +
		"https://drive.google.com/file/d/1xK2…/view\n" +
		"http://ejemplo.net/pack_series.m3u\n" +
		"https://cdn.ejemplo.net/v/pelicula_hd.mp4 👌👌"

	m3uBody := "#EXTM3U\n" +
		"#EXTINF:-1,Serie Uno S01E01\nhttp://cdn.ejemplo.net/serie1_e1.mp4\n" +
		"#EXTINF:-1,Serie Dos S01E01\nhttp://cdn.ejemplo.net/serie2_e1.mp4\n" +
		"#EXTINF:-1,Serie Tres (embed gris)\nhttps://streamtape.com/e/abc123\n"

	p, repo := newPipelineForTest(t, map[string]fakeM3UResponse{
		"ejemplo.net/pack_series.m3u": {body: m3uBody},
	})

	res, err := p.Run(context.Background(), mockupText, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Detectados: los 3 candidatos de primer nivel tal como entraron (drive,
	// la URL de la M3U, el mp4) — ANTES de que la M3U se expanda.
	if res.Detectados != 3 {
		t.Errorf("Detectados = %d, esperado 3 (drive + URL de la M3U + mp4, antes de expandir)", res.Detectados)
	}
	// Total: tras expandir la M3U en sus 3 entradas, quedan 5 candidatos
	// procesados (drive + mp4 + 3 entradas) — Detectados(3) < Total(5),
	// justo el caso de "se expande" que motivó separar los dos campos.
	if res.Total != 5 {
		t.Errorf("Total = %d, esperado 5 (drive + mp4 + 3 entradas de la M3U expandida)", res.Total)
	}
	assertResumenInvariant(t, res)

	if res.PendientesConector != 1 {
		t.Errorf("PendientesConector = %d, esperado 1 (el enlace de Drive)", res.PendientesConector)
	}
	if res.OmitidosNoSoportados != 1 {
		t.Errorf("OmitidosNoSoportados = %d, esperado 1 (el embed de streamtape dentro de la M3U)", res.OmitidosNoSoportados)
	}
	if res.Anadidos != 3 {
		t.Errorf("Anadidos = %d, esperado 3 (el mp4 directo + las 2 series de la M3U)", res.Anadidos)
	}
	if res.Duplicados != 0 || res.SinClasificar != 0 || res.Basura != 0 {
		t.Errorf("Duplicados/SinClasificar/Basura = %d/%d/%d, esperado 0/0/0", res.Duplicados, res.SinClasificar, res.Basura)
	}
	if res.PorTipo["url"] != 2 {
		t.Errorf("PorTipo[url] = %d, esperado 2 (drive + mp4)", res.PorTipo["url"])
	}
	if res.PorTipo["m3u"] != 3 {
		t.Errorf("PorTipo[m3u] = %d, esperado 3 (entradas de la lista expandida)", res.PorTipo["m3u"])
	}
	if res.ListID == 0 {
		t.Error("ListID = 0, esperado id de la ingest_list creada")
	}

	// Trazabilidad: la M3U remota debe tener su PROPIA fila ingest_lists,
	// con source_url, para un futuro re-sync (Brief §4).
	lists, err := repo.ListIngestLists(context.Background())
	if err != nil {
		t.Fatalf("ListIngestLists: %v", err)
	}
	var nested *catalog.IngestList
	for i := range lists {
		if lists[i].Type == "m3u_remoto" {
			nested = &lists[i]
		}
	}
	if nested == nil {
		t.Fatal("no se registró ninguna ingest_list de tipo m3u_remoto")
	}
	if nested.SourceURL == nil || *nested.SourceURL != "http://ejemplo.net/pack_series.m3u" {
		t.Errorf("SourceURL = %v, esperado la URL de la M3U", nested.SourceURL)
	}
	if nested.ItemCount != 3 {
		t.Errorf("ItemCount de la lista anidada = %d, esperado 3", nested.ItemCount)
	}

	// Nada se pierde en silencio: el ruido ("🎬 mira lo q me pasaron!!" y los
	// emojis) simplemente no genera candidatos (Brief §1: "se ignora el
	// resto"), y no hay ninguna entrada de Detalles quejándose de nada.
	if len(res.Detalles) != 0 {
		t.Errorf("Detalles = %v, esperado vacío para este texto (todo se resolvió limpio)", res.Detalles)
	}
}

// --- Dedupe (Brief M5 §7) --------------------------------------------------

func TestPipelineDedupeSameURLTwice(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	blob := "https://cdn.ejemplo.net/v/pelicula.mp4\nhttps://cdn.ejemplo.net/v/pelicula.mp4\n"

	res, err := p.Run(context.Background(), blob, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Anadidos != 1 || res.Duplicados != 1 {
		t.Fatalf("Anadidos=%d Duplicados=%d, esperado 1/1", res.Anadidos, res.Duplicados)
	}
	assertResumenInvariant(t, res)

	items, err := repo.List(context.Background(), catalog.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items.Total != 1 {
		t.Fatalf("items en catálogo = %d, esperado 1", items.Total)
	}
}

// TestPipelineDedupeSameMovieTwoRoutesCreatesTwoProvisionalSources cubre la
// parte que le toca a M5 de "misma película por dos rutas → un item, dos
// sources": la ingesta SOLO deduplica por (provider, locator) — dos
// locators distintos deben quedar como dos items/sources provisionales
// SEPARADOS, listos para que la cola de M3 los funda por tmdb_id al
// identificar (ese dedupe ya está probado en
// internal/catalog/repo_identify_test.go: TestApplyMovieIdentificationDedupe;
// no hace falta un TMDB falso aquí para volver a probarlo).
func TestPipelineDedupeSameMovieTwoRoutesCreatesTwoProvisionalSources(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	blob := "https://cdn.ejemplo.net/v/Inception.2010.1080p-GRUPO1.mp4\n" +
		"https://cdn.ejemplo.net/v/Inception.2010.720p-GRUPO2.mp4\n"

	res, err := p.Run(context.Background(), blob, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Anadidos != 2 || res.Duplicados != 0 {
		t.Fatalf("Anadidos=%d Duplicados=%d, esperado 2/0 (locators distintos, ingesta no dedupe por título)", res.Anadidos, res.Duplicados)
	}
	assertResumenInvariant(t, res)

	items, err := repo.List(context.Background(), catalog.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items.Total != 2 {
		t.Fatalf("items en catálogo = %d, esperado 2 (uno por locator; M3 los fusionará al identificar)", items.Total)
	}
	for _, it := range items.Items {
		if it.Confirmed {
			t.Errorf("item %q ya viene confirmed=true; debería quedar provisional para que M3 lo identifique", it.Title)
		}
	}
}

// --- SSRF (Brief M5 §6/§7) --------------------------------------------------

func TestPipelineM3URemoteBlockedBySSRF(t *testing.T) {
	repo := newRepoForTest(t)
	p := NewPipeline(repo, nil, nil)
	// SIN withFakeDNS: 127.0.0.1 es una IP literal, ValidateExternalURL la
	// bloquea sin necesidad de resolver nada.
	blob := "http://127.0.0.1:9/lista.m3u\n"

	res, err := p.Run(context.Background(), blob, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.SinClasificar != 1 {
		t.Fatalf("SinClasificar = %d, esperado 1 (bloqueado por SSRF)", res.SinClasificar)
	}
	assertResumenInvariant(t, res)
	if len(res.Detalles) != 1 || !strings.Contains(res.Detalles[0], "127.0.0.1") {
		t.Fatalf("Detalles = %v, esperado que mencione la URL bloqueada", res.Detalles)
	}

	items, err := repo.List(context.Background(), catalog.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items.Total != 0 {
		t.Fatalf("items en catálogo = %d, esperado 0 (nada se creó de una URL bloqueada)", items.Total)
	}
}

// --- host_no_soportado: sin entrada, contado (Brief §2/§7) ------------------

func TestPipelineUnsupportedHostCreatesNoEntry(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	blob := "https://streamtape.com/e/abc123\nhttps://cdn.ejemplo.net/v/pelicula.mp4\n"

	res, err := p.Run(context.Background(), blob, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.OmitidosNoSoportados != 1 || res.Anadidos != 1 {
		t.Fatalf("OmitidosNoSoportados=%d Anadidos=%d, esperado 1/1", res.OmitidosNoSoportados, res.Anadidos)
	}
	assertResumenInvariant(t, res)
	if len(res.Detalles) != 0 {
		t.Errorf("Detalles = %v; host_no_soportado no debe generar detalle (sin nombres, sin cómo)", res.Detalles)
	}

	items, err := repo.List(context.Background(), catalog.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items.Total != 1 {
		t.Fatalf("items en catálogo = %d, esperado 1 (solo el directo; streamtape NO crea entrada)", items.Total)
	}
}

// --- Basura (Brief §4: identify.Parse detecta junk/extra) ------------------

func TestPipelineJunkURLCountsAsBasura(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	// Nombre de archivo que el normalizador de M3 ya reconoce como basura
	// (ver internal/identify/normalize_test.go: vídeos de WhatsApp sin título real).
	blob := "https://cdn.ejemplo.net/v/VID-20230412-WA0012.mp4\n"

	res, err := p.Run(context.Background(), blob, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Basura != 1 || res.Anadidos != 0 {
		t.Fatalf("Basura=%d Anadidos=%d, esperado 1/0", res.Basura, res.Anadidos)
	}
	assertResumenInvariant(t, res)

	items, err := repo.List(context.Background(), catalog.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items.Total != 0 {
		t.Fatalf("items en catálogo = %d, esperado 0 (basura no se cataloga)", items.Total)
	}
}

// --- Límite de candidatos (Brief §6) ----------------------------------------

func TestPipelineCandidateLimit(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	var b strings.Builder
	for i := 0; i < maxCandidates+50; i++ {
		fmtLine(&b, i)
	}

	res, err := p.Run(context.Background(), b.String(), "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != maxCandidates {
		t.Fatalf("Total = %d, esperado %d (límite aplicado)", res.Total, maxCandidates)
	}
	if res.Detectados != maxCandidates+50 {
		t.Fatalf("Detectados = %d, esperado %d (todo lo que entró, antes del límite; Detectados > Total es el caso de \"se corta\")", res.Detectados, maxCandidates+50)
	}
	assertResumenInvariant(t, res)
	foundNotice := false
	for _, d := range res.Detalles {
		if strings.Contains(d, "límite") {
			foundNotice = true
		}
	}
	if !foundNotice {
		t.Error("Detalles no menciona el límite aplicado (nada se pierde en silencio)")
	}

	items, err := repo.List(context.Background(), catalog.ListFilter{Limit: 1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items.Total != maxCandidates {
		t.Fatalf("items en catálogo = %d, esperado %d", items.Total, maxCandidates)
	}
}

func fmtLine(b *strings.Builder, i int) {
	b.WriteString("https://cdn.ejemplo.net/v/pelicula")
	b.WriteString(itoaForTest(i))
	b.WriteString(".mp4\n")
}

func itoaForTest(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

// --- Rutas locales (Brief §1: se informan, no se procesan) -----------------

func TestPipelineLocalPathReportedNotProcessed(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	dir := t.TempDir()
	blob := "mira esto:\n" + dir + "\nhttps://cdn.ejemplo.net/v/pelicula.mp4\n"

	res, err := p.Run(context.Background(), blob, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("Total = %d, esperado 1 (la ruta local no es un candidato)", res.Total)
	}
	if res.Detectados != 1 {
		t.Fatalf("Detectados = %d, esperado 1 (la ruta local no cuenta como candidato; solo la URL)", res.Detectados)
	}
	assertResumenInvariant(t, res)
	found := false
	for _, d := range res.Detalles {
		if strings.Contains(d, dir) {
			found = true
		}
	}
	if !found {
		t.Errorf("Detalles = %v, esperado que mencione la ruta local detectada", res.Detalles)
	}

	items, err := repo.List(context.Background(), catalog.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items.Total != 1 {
		t.Fatalf("items en catálogo = %d, esperado 1 (solo el mp4 directo)", items.Total)
	}
}

// --- Error de BD a mitad de ingesta (Brief: "nada se pierde en silencio") --

// TestPipelineDBErrorCountsAsSinClasificar cubre el tercer hueco encontrado
// en el addendum de corrección de contadores: un error de BD real (no
// catalog.ErrNotFound, p.ej. la conexión ya cerrada) al comprobar duplicados
// caía FUERA de los seis resultados, rompiendo la invariante Total = suma.
// Ahora cae en SinClasificar (ver Pipeline.applyOutcome) y queda explicado en
// Detalles.
func TestPipelineDBErrorCountsAsSinClasificar(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	repo := catalog.NewRepo(st.DB)
	p := NewPipeline(repo, nil, nil)

	listID, err := repo.InsertIngestList(context.Background(), &catalog.IngestList{
		Name: "test", Type: "text", ItemCount: 1,
	}, testNow)
	if err != nil {
		t.Fatalf("InsertIngestList: %v", err)
	}

	// Cerrar el store DESPUÉS de conseguir un listID válido simula un fallo
	// real de BD a mitad de la ingesta (SourceByLocator fallará con un error
	// de conexión, no con catalog.ErrNotFound); se llama a processCandidate
	// directamente, sin pasar por Run, para no depender del store ya cerrado
	// en ningún paso previo.
	if err := st.Close(); err != nil {
		t.Fatalf("cerrar store: %v", err)
	}

	c := candidateFromURL("https://cdn.ejemplo.net/v/pelicula.mp4", "", "url")
	res := &ResumenIngesta{PorTipo: map[string]int{}}
	p.processCandidate(context.Background(), c, listID, testNow, res)
	res.Total = res.Anadidos + res.Duplicados + res.PendientesConector +
		res.OmitidosNoSoportados + res.OmitidosNoDirectos + res.SinClasificar + res.Basura

	if res.SinClasificar != 1 {
		t.Fatalf("SinClasificar = %d, esperado 1 (el error de BD no debe perderse ni caer fuera de los seis resultados)", res.SinClasificar)
	}
	assertResumenInvariant(t, res)
	if len(res.Detalles) != 1 {
		t.Fatalf("Detalles = %v, esperado 1 entrada explicando el error de BD", res.Detalles)
	}
}

// --- Aviso al worker de salud de M6 (disparador "a": altas de una ingesta) --

type fakeHealthNotifier struct{ enqueued int }

func (f *fakeHealthNotifier) Enqueue() { f.enqueued++ }

func TestPipelineWakesHealthWorkerOnNewSources(t *testing.T) {
	p, _ := newPipelineForTest(t, nil)
	notifier := &fakeHealthNotifier{}
	p.Health = notifier

	if _, err := p.Run(context.Background(), "https://cdn.ejemplo.net/v/pelicula.mp4\n", "", testNow); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if notifier.enqueued != 1 {
		t.Fatalf("Health.Enqueue() llamado %d veces, esperado 1 (hubo altas nuevas)", notifier.enqueued)
	}
}

func TestPipelineDoesNotWakeHealthWorkerWithoutNewSources(t *testing.T) {
	p, _ := newPipelineForTest(t, nil)
	notifier := &fakeHealthNotifier{}
	p.Health = notifier

	// Solo basura: sin altas, no hay nada nuevo que verificar.
	if _, err := p.Run(context.Background(), "https://cdn.ejemplo.net/v/VID-20230412-WA0012.mp4\n", "", testNow); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if notifier.enqueued != 0 {
		t.Fatalf("Health.Enqueue() llamado %d veces, esperado 0 (sin altas nuevas)", notifier.enqueued)
	}
}
