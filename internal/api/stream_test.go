package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/bigbendja/gala/internal/api"
	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/media"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
	"github.com/bigbendja/gala/internal/stream"
)

// --- servidor de prueba dedicado a M4 (play/stream/hls/playback) ----------

type playTestServer struct {
	*httptest.Server
	repo      *catalog.Repo
	mediaRoot string
}

func newPlayTestServer(t *testing.T, ffmpeg *media.FFmpeg, hls *stream.Manager) *playTestServer {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	repo := catalog.NewRepo(st.DB)

	mediaRoot := t.TempDir()
	cfg := config.NewAt(dir)
	cfg.MediaFolders = []string{mediaRoot}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	sc := scan.New(ctx, cfg, repo, fakeEnum{}, log)

	if ffmpeg == nil {
		ffmpeg = &media.FFmpeg{}
	}
	srv := api.New(st, repo, cfg, sc, fakeEnum{}, nil, nil, dir, log, ffmpeg, hls)
	mux := http.NewServeMux()
	srv.Routes(mux)
	hts := httptest.NewServer(mux)
	t.Cleanup(func() { hts.Close(); cancel(); st.Close() })
	return &playTestServer{Server: hts, repo: repo, mediaRoot: mediaRoot}
}

func (ps *playTestServer) writeMediaFile(t *testing.T, name string, size int) string {
	t.Helper()
	full := filepath.Join(ps.mediaRoot, name)
	if err := os.WriteFile(full, bytes.Repeat([]byte("x"), size), 0o644); err != nil {
		t.Fatalf("escribir %s: %v", full, err)
	}
	return full
}

func (ps *playTestServer) insertSource(t *testing.T, itemID int64, locator, quality, health string, priority int, provider string) int64 {
	t.Helper()
	var q *string
	if quality != "" {
		q = &quality
	}
	if provider == "" {
		provider = "localfs"
	}
	src := catalog.Source{ItemID: itemID, Provider: provider, Locator: locator, QualityLabel: q, Health: health, Priority: priority}
	id, err := ps.repo.InsertSource(context.Background(), &src, "")
	if err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	return id
}

func (ps *playTestServer) insertMovie(t *testing.T, title string) int64 {
	t.Helper()
	item := catalog.Item{Kind: catalog.KindMovie, Title: title, Genres: []string{}, Confirmed: true}
	id, err := ps.repo.InsertItem(context.Background(), &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	return id
}

// --- DTOs mínimos para decodificar las respuestas JSON de la API ----------
// (los tipos reales viven en el paquete api, no exportados: se decodifica
// por forma, igual que haría cualquier cliente HTTP real).

type playSourceDTO struct {
	SourceID     int64   `json:"source_id"`
	Provider     string  `json:"provider"`
	QualityLabel *string `json:"quality_label"`
	Health       string  `json:"health"`
	Mode         string  `json:"mode"`
	URL          string  `json:"url"`
	Reason       *string `json:"reason"`
	Media        *struct {
		DurationS float64 `json:"duration_s"`
	} `json:"media"`
}

type playResponseDTO struct {
	ItemID          int64           `json:"item_id"`
	Sources         []playSourceDTO `json:"sources"`
	ChosenSourceID  *int64          `json:"chosen_source_id"`
	FFmpegAvailable bool            `json:"ffmpeg_available"`
}

func getPlay(t *testing.T, ps *playTestServer, itemID int64) playResponseDTO {
	t.Helper()
	resp, err := http.Get(ps.URL + "/api/v1/play/" + strconv.FormatInt(itemID, 10))
	if err != nil {
		t.Fatalf("GET /play: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, data)
	}
	var got playResponseDTO
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

// --- /play: modo por extensión sin ffmpeg ---------------------------------

func TestHandlePlayDirectByExtensionNoFFmpeg(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Peli MP4")
	locator := ps.writeMediaFile(t, "peli.mp4", 1000)
	srcID := ps.insertSource(t, itemID, locator, "1080p", catalog.HealthAlive, 0, "")

	got := getPlay(t, ps, itemID)
	if got.FFmpegAvailable {
		t.Error("FFmpegAvailable = true, esperado false (ffmpeg no configurado)")
	}
	if len(got.Sources) != 1 || got.Sources[0].Mode != "direct" {
		t.Fatalf("Sources = %+v, esperado 1 fuente en modo direct", got.Sources)
	}
	if got.ChosenSourceID == nil || *got.ChosenSourceID != srcID {
		t.Fatalf("ChosenSourceID = %v, esperado %d", got.ChosenSourceID, srcID)
	}
	if got.Sources[0].URL != "/api/v1/stream/"+strconv.FormatInt(srcID, 10) {
		t.Errorf("URL = %q", got.Sources[0].URL)
	}
}

func TestHandlePlayRejectMKVWithoutFFmpeg(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Peli MKV")
	locator := ps.writeMediaFile(t, "peli.mkv", 1000)
	ps.insertSource(t, itemID, locator, "", catalog.HealthAlive, 0, "")

	got := getPlay(t, ps, itemID)
	if got.Sources[0].Mode != "reject" {
		t.Fatalf("Mode = %q, esperado reject (mkv sin ffmpeg no se puede remuxear)", got.Sources[0].Mode)
	}
	if got.Sources[0].Reason == nil || *got.Sources[0].Reason == "" {
		t.Error("Reason vacío en modo reject")
	}
}

func TestHandlePlayHTTPProviderAlwaysDirect(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Peli remota")
	ps.insertSource(t, itemID, "http://ejemplo.test/peli.mkv", "", catalog.HealthUnverified, 0, "http")

	got := getPlay(t, ps, itemID)
	if got.Sources[0].Mode != "direct" {
		t.Fatalf("Mode = %q, esperado direct para provider http", got.Sources[0].Mode)
	}
}

func TestHandlePlayUnknownItem404(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	resp, err := http.Get(ps.URL + "/api/v1/play/999999")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", resp.StatusCode)
	}
}

// --- orden de fuentes (Brief M4 §5/§8) -------------------------------------

// TestHandlePlaySourceOrdering cubre la tabla salud × provider × prioridad
// × calidad, con una fuente reject que debe caer al final pese a tener la
// mejor salud/prioridad/calidad de todas.
func TestHandlePlaySourceOrdering(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Multi-fuente")

	locA := ps.writeMediaFile(t, "a.mp4", 100)
	locB := ps.writeMediaFile(t, "b.mp4", 100)
	locD := ps.writeMediaFile(t, "d.mkv", 100) // mkv sin ffmpeg → reject

	idA := ps.insertSource(t, itemID, locA, "720p", catalog.HealthAlive, 0, "localfs")
	idB := ps.insertSource(t, itemID, locB, "1080p", catalog.HealthDead, 0, "localfs")
	idC := ps.insertSource(t, itemID, "http://ejemplo.test/c.mp4", "480p", catalog.HealthUnverified, 0, "http")
	idD := ps.insertSource(t, itemID, locD, "2160p", catalog.HealthAlive, 5, "localfs")

	got := getPlay(t, ps, itemID)
	if len(got.Sources) != 4 {
		t.Fatalf("Sources = %d, esperado 4", len(got.Sources))
	}
	gotOrder := make([]int64, len(got.Sources))
	for i, s := range got.Sources {
		gotOrder[i] = s.SourceID
	}
	wantOrder := []int64{idA, idC, idB, idD} // viva-localfs, sin_verificar-http, caída-localfs, reject al final
	if !int64SliceEqual(gotOrder, wantOrder) {
		t.Fatalf("orden = %v, esperado %v (A=viva/localfs, C=sin_verificar/http, B=caída, D=reject al final)", gotOrder, wantOrder)
	}
	if got.ChosenSourceID == nil || *got.ChosenSourceID != idA {
		t.Fatalf("ChosenSourceID = %v, esperado %d (la mejor no-reject)", got.ChosenSourceID, idA)
	}
	// Reject incluida, no descartada (Brief §5: "SE INCLUYEN").
	if got.Sources[3].SourceID != idD || got.Sources[3].Mode != "reject" {
		t.Fatalf("última fuente = %+v, esperada D en reject", got.Sources[3])
	}
}

func int64SliceEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- /stream/{source_id} ----------------------------------------------------

func TestHandleStreamDirectServesFile(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Peli")
	locator := ps.writeMediaFile(t, "peli.mp4", 500)
	srcID := ps.insertSource(t, itemID, locator, "", catalog.HealthAlive, 0, "")

	resp, err := http.Get(ps.URL + "/api/v1/stream/" + strconv.FormatInt(srcID, 10))
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 500 {
		t.Errorf("body = %d bytes, esperado 500", len(body))
	}
}

func TestHandleStreamDirectOutsideRegisteredFolders403(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Peli fuera")
	outside := t.TempDir()
	locator := filepath.Join(outside, "peli.mp4")
	if err := os.WriteFile(locator, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	srcID := ps.insertSource(t, itemID, locator, "", catalog.HealthAlive, 0, "")

	resp, err := http.Get(ps.URL + "/api/v1/stream/" + strconv.FormatInt(srcID, 10))
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, esperado 403 (fuera de carpetas registradas)", resp.StatusCode)
	}
}

func TestHandleStreamHTTPProviderRedirects(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Remota")
	srcID := ps.insertSource(t, itemID, "http://ejemplo.test/video.mp4", "", catalog.HealthUnverified, 0, "http")

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(ps.URL + "/api/v1/stream/" + strconv.FormatInt(srcID, 10))
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, esperado 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "http://ejemplo.test/video.mp4" {
		t.Errorf("Location = %q", loc)
	}
}

func TestHandleStreamUnsupportedProvider501(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "M3U")
	srcID := ps.insertSource(t, itemID, "entrada-m3u", "", catalog.HealthUnverified, 0, "m3u")

	resp, err := http.Get(ps.URL + "/api/v1/stream/" + strconv.FormatInt(srcID, 10))
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, esperado 501", resp.StatusCode)
	}
}

func TestHandleStreamUnknownSource404(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	resp, err := http.Get(ps.URL + "/api/v1/stream/999999")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", resp.StatusCode)
	}
}

// --- /sources/{id}/failed reordena /play (Brief M4 §5/§8) -----------------

func TestHandleSourceFailedMarksDeadAndReordersPlay(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Dos fuentes")
	locHealthy := ps.writeMediaFile(t, "buena.mp4", 100)
	locFailing := ps.writeMediaFile(t, "mala.mp4", 100)

	idHealthy := ps.insertSource(t, itemID, locHealthy, "", catalog.HealthUnverified, 0, "")
	idFailing := ps.insertSource(t, itemID, locFailing, "", catalog.HealthAlive, 0, "")

	// Antes: idFailing (viva) va primero.
	before := getPlay(t, ps, itemID)
	if *before.ChosenSourceID != idFailing {
		t.Fatalf("antes de fallar, chosen = %v, esperado %d", before.ChosenSourceID, idFailing)
	}

	resp, err := http.Post(ps.URL+"/api/v1/sources/"+strconv.FormatInt(idFailing, 10)+"/failed", "", nil)
	if err != nil {
		t.Fatalf("POST /failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", resp.StatusCode)
	}

	src, err := ps.repo.SourceByID(context.Background(), idFailing)
	if err != nil {
		t.Fatalf("SourceByID: %v", err)
	}
	if src.Health != catalog.HealthDead {
		t.Fatalf("Health = %q, esperado %q", src.Health, catalog.HealthDead)
	}

	after := getPlay(t, ps, itemID)
	if *after.ChosenSourceID != idHealthy {
		t.Fatalf("tras fallar, chosen = %v, esperado %d (la otra fuente)", after.ChosenSourceID, idHealthy)
	}
}

func TestHandleSourceFailedUnknown404(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	resp, err := http.Post(ps.URL+"/api/v1/sources/999999/failed", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", resp.StatusCode)
	}
}

// --- /playback/{item_id}: cookie de dispositivo y umbral watched ----------

func TestHandlePlaybackSetsDeviceCookieAndRoundtrips(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Peli")

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}

	getResp, err := client.Get(ps.URL + "/api/v1/playback/" + strconv.FormatInt(itemID, 10))
	if err != nil {
		t.Fatalf("GET /playback: %v", err)
	}
	getResp.Body.Close()
	u, err := url.Parse(ps.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	found := false
	for _, c := range jar.Cookies(u) {
		if c.Name == "gala_device" {
			found = true
		}
	}
	if !found {
		t.Fatal("no se creó la cookie gala_device en el primer GET")
	}

	body, _ := json.Marshal(map[string]any{"position_s": 30.0, "duration_s": 100.0})
	putReq, _ := http.NewRequest(http.MethodPut, ps.URL+"/api/v1/playback/"+strconv.FormatInt(itemID, 10), bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := client.Do(putReq)
	if err != nil {
		t.Fatalf("PUT /playback: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(putResp.Body)
		t.Fatalf("status = %d, body=%s", putResp.StatusCode, data)
	}

	getResp2, err := client.Get(ps.URL + "/api/v1/playback/" + strconv.FormatInt(itemID, 10))
	if err != nil {
		t.Fatalf("GET /playback (2): %v", err)
	}
	defer getResp2.Body.Close()
	var p catalog.Progress
	if err := json.NewDecoder(getResp2.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.PositionS != 30 || p.DurationS == nil || *p.DurationS != 100 || p.Watched {
		t.Fatalf("progress = %+v", p)
	}
}

func TestHandlePlaybackWatchedThreshold(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Peli")

	putAndCheck := func(position float64, wantWatched bool) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"position_s": position, "duration_s": 100.0})
		req, _ := http.NewRequest(http.MethodPut, ps.URL+"/api/v1/playback/"+strconv.FormatInt(itemID, 10), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out["watched"] != wantWatched {
			t.Fatalf("position=%v watched=%v, esperado %v", position, out["watched"], wantWatched)
		}
	}
	putAndCheck(90, false)  // 90% < 92%
	putAndCheck(93, true)   // 93% > 92%
	putAndCheck(92.1, true) // justo por encima
}

func TestHandlePlaybackUnknownItem404(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	body, _ := json.Marshal(map[string]any{"position_s": 1.0})
	req, _ := http.NewRequest(http.MethodPut, ps.URL+"/api/v1/playback/999999", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", resp.StatusCode)
	}
}

// --- HLS a través de la API (Brief M4 §8) -----------------------------------

func TestHandleHLSStartAndServeFile(t *testing.T) {
	fake := buildFakeBinary(t)
	t.Setenv("HELPER_MODE", "hls")
	ffmpeg := &media.FFmpeg{FFmpegPath: fake, FFprobePath: fake}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hlsMgr := stream.NewManager(ctx, fake, t.TempDir(), discardTestLogger())

	ps := newPlayTestServer(t, ffmpeg, hlsMgr)
	itemID := ps.insertMovie(t, "Peli MKV")
	locator := ps.writeMediaFile(t, "peli.mkv", 1000)
	srcID := ps.insertSource(t, itemID, locator, "", catalog.HealthAlive, 0, "")

	resp, err := http.Get(ps.URL + "/api/v1/stream/" + strconv.FormatInt(srcID, 10) + "/hls/start")
	if err != nil {
		t.Fatalf("GET /hls/start: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, data)
	}
	var out struct {
		SessionID   string `json:"session_id"`
		PlaylistURL string `json:"playlist_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.SessionID == "" || out.PlaylistURL == "" {
		t.Fatalf("respuesta incompleta: %+v", out)
	}

	plResp, err := http.Get(ps.URL + out.PlaylistURL)
	if err != nil {
		t.Fatalf("GET playlist: %v", err)
	}
	defer plResp.Body.Close()
	if plResp.StatusCode != http.StatusOK {
		t.Fatalf("status playlist = %d, esperado 200", plResp.StatusCode)
	}
	plBody, _ := io.ReadAll(plResp.Body)
	if len(plBody) == 0 {
		t.Fatal("playlist vacía")
	}
}

func TestHandleHLSStartWithoutFFmpeg503(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil) // ffmpeg no disponible, HLS nil
	itemID := ps.insertMovie(t, "Peli MKV")
	locator := ps.writeMediaFile(t, "peli.mkv", 1000)
	srcID := ps.insertSource(t, itemID, locator, "", catalog.HealthAlive, 0, "")

	resp, err := http.Get(ps.URL + "/api/v1/stream/" + strconv.FormatInt(srcID, 10) + "/hls/start")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, esperado 503", resp.StatusCode)
	}
}

func TestHandleHLSStartTooManySessions503(t *testing.T) {
	fake := buildFakeBinary(t)
	t.Setenv("HELPER_MODE", "hls")
	t.Setenv("HELPER_SLEEP_MS", "2000") // mantener las sesiones vivas durante el test
	ffmpeg := &media.FFmpeg{FFmpegPath: fake, FFprobePath: fake}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hlsMgr := stream.NewManager(ctx, fake, t.TempDir(), discardTestLogger())

	ps := newPlayTestServer(t, ffmpeg, hlsMgr)
	itemID := ps.insertMovie(t, "Peli MKV")

	// Cuatro FUENTES DISTINTAS (el límite por fuente no debe interferir con
	// el límite global de 3).
	const globalLimit = 3
	var lastStatus int
	for i := 0; i < globalLimit+1; i++ {
		locator := ps.writeMediaFile(t, "peli"+strconv.Itoa(i)+".mkv", 1000)
		srcID := ps.insertSource(t, itemID, locator, "", catalog.HealthAlive, 0, "")
		resp, err := http.Get(ps.URL + "/api/v1/stream/" + strconv.FormatInt(srcID, 10) + "/hls/start")
		if err != nil {
			t.Fatalf("GET #%d: %v", i, err)
		}
		lastStatus = resp.StatusCode
		resp.Body.Close()
		if i < globalLimit && lastStatus != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			t.Fatalf("sesión #%d: status = %d, esperado 200, body=%s", i, lastStatus, data)
		}
	}
	if lastStatus != http.StatusServiceUnavailable {
		t.Fatalf("última sesión (#%d, por encima del límite): status = %d, esperado 503", globalLimit, lastStatus)
	}
}

func discardTestLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
