package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/media"
	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/stream"
)

// --- /api/v1/play/{item_id} (Brief M4 §5) -----------------------------

type playMediaView struct {
	Container  string  `json:"container,omitempty"`
	VideoCodec string  `json:"video_codec,omitempty"`
	AudioCodec string  `json:"audio_codec,omitempty"`
	DurationS  float64 `json:"duration_s,omitempty"`
}

type playSourceView struct {
	SourceID     int64          `json:"source_id"`
	Provider     string         `json:"provider"`
	QualityLabel *string        `json:"quality_label,omitempty"`
	Health       string         `json:"health"`
	Mode         media.PlayMode `json:"mode"`
	URL          string         `json:"url"`
	Reason       *string        `json:"reason,omitempty"`
	Media        *playMediaView `json:"media,omitempty"`
}

type playResponse struct {
	ItemID          int64            `json:"item_id"`
	Sources         []playSourceView `json:"sources"`
	ChosenSourceID  *int64           `json:"chosen_source_id,omitempty"`
	FFmpegAvailable bool             `json:"ffmpeg_available"`
}

// healthRank y providerRank son el orden de preferencia de fuentes (Brief M4
// §5): salud, luego provider, luego priority declarada, luego calidad.
// Cualquier valor no listado (dato corrupto o provider futuro) cae al final.
var healthRank = map[string]int{
	catalog.HealthAlive: 0, catalog.HealthSlow: 1, catalog.HealthUnverified: 2,
	catalog.HealthDisconnected: 3, catalog.HealthDead: 4,
}
var providerRank = map[string]int{localfs.ProviderID: 0, "http": 1}

const unknownRank = 99

func rankOf(m map[string]int, key string) int {
	if r, ok := m[key]; ok {
		return r
	}
	return unknownRank
}

var reQualityDigits = regexp.MustCompile(`\d+`)

// qualityRank extrae los dígitos de una etiqueta de calidad ("1080p" → 1080)
// para poder ordenar de mayor a menor; sin etiqueta o sin dígitos, 0 (al
// final entre las de su mismo grupo salud/provider/priority).
func qualityRank(label *string) int {
	if label == nil {
		return 0
	}
	m := reQualityDigits.FindString(*label)
	if m == "" {
		return 0
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return 0
	}
	return n
}

// handlePlay: GET /api/v1/play/{item_id} — decisión completa de
// reproducción para la UI (Brief M4 §5). Sonda bajo demanda las fuentes
// localfs que aún no tengan media_info (si hay ffmpeg disponible).
func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("item_id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, "item_id no válido")
		return
	}
	item, err := s.Catalog.Get(r.Context(), id)
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no encontrado"})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}

	type scored struct {
		view   playSourceView
		reject bool
		src    catalog.Source
	}
	out := make([]scored, 0, len(item.Sources))

	for _, src := range item.Sources {
		view := playSourceView{
			SourceID: src.ID, Provider: src.Provider,
			QualityLabel: src.QualityLabel, Health: src.Health,
			URL: "/api/v1/stream/" + strconv.FormatInt(src.ID, 10),
		}

		switch src.Provider {
		case localfs.ProviderID:
			var info *media.MediaInfo
			if mi, ok, err := s.Catalog.GetMediaInfo(r.Context(), src.ID); err != nil {
				s.fail(w, err)
				return
			} else if ok {
				info = &media.MediaInfo{Container: mi.Container, DurationS: mi.DurationS,
					VideoCodec: mi.VideoCodec, AudioCodec: mi.AudioCodec,
					Width: mi.Width, Height: mi.Height, Bitrate: mi.Bitrate}
			} else if s.FFmpeg.Available() {
				// Sonda bajo demanda (Brief §2): si falla (archivo ilegible,
				// disco desconectado...), se decide igualmente por extensión.
				if probed, perr := media.ProbeAndStore(r.Context(), s.Catalog, s.FFmpeg, src); perr == nil {
					info = probed
				}
			}
			decision := media.Decide(info, filepath.Ext(src.Locator), s.FFmpeg.Available())
			view.Mode = decision.Mode
			if decision.Reason != "" {
				view.Reason = &decision.Reason
			}
			if decision.Mode == media.ModeRemux {
				view.URL = "/api/v1/stream/" + strconv.FormatInt(src.ID, 10) + "/hls/start"
			}
			if info != nil {
				view.Media = &playMediaView{Container: info.Container, VideoCodec: info.VideoCodec,
					AudioCodec: info.AudioCodec, DurationS: info.DurationS}
			}

		case "http":
			// M4 no sondea enlaces remotos (fuera de alcance): direct
			// siempre, y el navegador ya avisará si de verdad no puede.
			view.Mode = media.ModeDirect

		default:
			view.Mode = media.ModeReject
			reason := "proveedor no soportado todavía"
			view.Reason = &reason
		}

		out = append(out, scored{view: view, reject: view.Mode == media.ModeReject, src: src})
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.reject != b.reject {
			return !a.reject // no-reject primero; reject al final (Brief §5) pero incluidas
		}
		if hr := rankOf(healthRank, a.src.Health) - rankOf(healthRank, b.src.Health); hr != 0 {
			return hr < 0
		}
		if pr := rankOf(providerRank, a.src.Provider) - rankOf(providerRank, b.src.Provider); pr != 0 {
			return pr < 0
		}
		if a.src.Priority != b.src.Priority {
			return a.src.Priority > b.src.Priority
		}
		return qualityRank(a.src.QualityLabel) > qualityRank(b.src.QualityLabel)
	})

	res := playResponse{ItemID: id, FFmpegAvailable: s.FFmpeg.Available(), Sources: make([]playSourceView, 0, len(out))}
	for _, sc := range out {
		res.Sources = append(res.Sources, sc.view)
	}
	if len(out) > 0 {
		id := out[0].view.SourceID
		res.ChosenSourceID = &id
	}
	writeJSON(w, http.StatusOK, res)
}

// --- /api/v1/stream/{source_id} (Brief M4 §4) --------------------------

// handleStreamDirect: GET /api/v1/stream/{source_id}. localfs se sirve con
// range requests (stream.ServeDirect); http redirige 302 al locator; el
// resto de providers, 501 (aún no soportados para reproducir).
func (s *Server) handleStreamDirect(w http.ResponseWriter, r *http.Request) {
	src, ok := s.mustSource(w, r, "source_id")
	if !ok {
		return
	}
	switch src.Provider {
	case localfs.ProviderID:
		if err := stream.ServeDirect(w, r, src.Locator, s.Config.MediaFolders); err != nil {
			if errors.Is(err, stream.ErrOutsideRegisteredFolders) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "ruta fuera de las carpetas registradas"})
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "archivo no accesible"})
			return
		}
	case "http":
		http.Redirect(w, r, src.Locator, http.StatusFound)
	default:
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "provider aún no soportado para reproducir"})
	}
}

// --- /api/v1/stream/{source_id}/hls/... — remux (Brief M4 §4) ----------

// handleHLSStart: GET /api/v1/stream/{source_id}/hls/start?t=123.4 — crea
// (o recrea, en un seek) una sesión de remux y devuelve dónde está su
// playlist. El reproductor web la pasa a hls.js.
func (s *Server) handleHLSStart(w http.ResponseWriter, r *http.Request) {
	src, ok := s.mustSource(w, r, "source_id")
	if !ok {
		return
	}
	if src.Provider != localfs.ProviderID {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "remux solo disponible para archivos locales"})
		return
	}
	if err := stream.ValidatePath(src.Locator, s.Config.MediaFolders); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "ruta fuera de las carpetas registradas"})
		return
	}
	if s.HLS == nil || !s.FFmpeg.Available() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "falta ffmpeg: no se puede convertir este contenedor"})
		return
	}

	start := 0.0
	if t := r.URL.Query().Get("t"); t != "" {
		if v, err := strconv.ParseFloat(t, 64); err == nil && v >= 0 {
			start = v
		}
	}

	sess, err := s.HLS.StartSession(r.Context(), src.ID, src.Locator, start)
	if err != nil {
		switch {
		case errors.Is(err, stream.ErrTooManySessions):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "demasiadas conversiones en curso; inténtalo de nuevo en unos segundos"})
		case errors.Is(err, stream.ErrFFmpegUnavailable):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "falta ffmpeg"})
		default:
			s.fail(w, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"session_id":   sess.ID,
		"playlist_url": "/api/v1/stream/" + strconv.FormatInt(src.ID, 10) + "/hls/" + sess.ID + "/index.m3u8",
	})
}

// handleHLSFile: GET /api/v1/stream/{source_id}/hls/{session_id}/{file} —
// sirve la playlist o un segmento de una sesión ya creada.
func (s *Server) handleHLSFile(w http.ResponseWriter, r *http.Request) {
	sourceID, err := strconv.ParseInt(r.PathValue("source_id"), 10, 64)
	if err != nil || sourceID <= 0 {
		badRequest(w, "source_id no válido")
		return
	}
	if s.HLS == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "sesión no encontrada"})
		return
	}
	sessionID := r.PathValue("session_id")
	file := r.PathValue("file")

	sess, ok := s.HLS.Session(sessionID)
	if !ok || sess.SourceID != sourceID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "sesión no encontrada"})
		return
	}
	if err := s.HLS.ServeFile(w, r, sessionID, file); err != nil {
		if errors.Is(err, stream.ErrSessionNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sesión no encontrada"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "fichero no encontrado"})
	}
}

// mustSource resuelve {param} como source_id y devuelve la fuente, o escribe
// la respuesta de error correspondiente (400/404) y devuelve ok=false.
func (s *Server) mustSource(w http.ResponseWriter, r *http.Request, param string) (*catalog.Source, bool) {
	id, err := strconv.ParseInt(r.PathValue(param), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, param+" no válido")
		return nil, false
	}
	src, err := s.Catalog.SourceByID(r.Context(), id)
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "fuente no encontrada"})
		return nil, false
	}
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return src, true
}

// --- /api/v1/sources/{id}/failed (Brief M4 §5, anticipo mínimo de M6) --

// handleSourceFailed: POST /api/v1/sources/{id}/failed — el reproductor la
// llama cuando el elemento <video> dispara "error"; marca la fuente caída
// para que el siguiente /play la mande al final.
func (s *Server) handleSourceFailed(w http.ResponseWriter, r *http.Request) {
	src, ok := s.mustSource(w, r, "id")
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.Catalog.TouchSource(r.Context(), src.ID, catalog.HealthDead, now); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "health": catalog.HealthDead})
}

// --- /api/v1/playback/{item_id} (Brief M4 §5) ---------------------------

// watchedThreshold: a partir de este porcentaje de la duración se considera
// visto (Brief M4 §5: "marcar watched=1 si position > 92% de duración").
const watchedThreshold = 0.92

type playbackRequest struct {
	PositionS float64  `json:"position_s"`
	DurationS *float64 `json:"duration_s"`
}

// handleGetPlayback: GET /api/v1/playback/{item_id}.
func (s *Server) handleGetPlayback(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("item_id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, "item_id no válido")
		return
	}
	device := ensureDeviceID(w, r)
	p, _, err := s.Catalog.GetPlayback(r.Context(), id, device)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p) // p nil → "null": el dispositivo nunca reprodujo esto
}

// handlePutPlayback: PUT /api/v1/playback/{item_id}  body {"position_s":123.4,"duration_s":7261.4}.
func (s *Server) handlePutPlayback(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("item_id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, "item_id no válido")
		return
	}
	var req playbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PositionS < 0 {
		badRequest(w, "cuerpo no válido")
		return
	}
	if _, err := s.Catalog.Get(r.Context(), id); errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no encontrado"})
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}

	device := ensureDeviceID(w, r)
	watched := req.DurationS != nil && *req.DurationS > 0 && req.PositionS > *req.DurationS*watchedThreshold
	if err := s.Catalog.UpsertPlayback(r.Context(), id, device, req.PositionS, req.DurationS, watched, ""); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "watched": watched})
}

// --- cookie de dispositivo (Brief M4 §5 / M7) ---------------------------

const deviceCookieName = "gala_device"
const deviceCookieMaxAge = 10 * 365 * 24 * 3600 // ~10 años: identifica el dispositivo, no una sesión

// readDeviceID lee la cookie de dispositivo SIN crearla; usado en lecturas
// (listado/ficha) donde no tiene sentido plantar una cookie nueva solo por
// mirar la galería.
func readDeviceID(r *http.Request) string {
	c, err := r.Cookie(deviceCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// ensureDeviceID lee la cookie de dispositivo o la crea si falta (Brief M4
// §5, base de "Continuar viendo" multi-dispositivo de M7).
func ensureDeviceID(w http.ResponseWriter, r *http.Request) string {
	if id := readDeviceID(r); id != "" {
		return id
	}
	id := newDeviceID()
	http.SetCookie(w, &http.Cookie{
		Name: deviceCookieName, Value: id, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: deviceCookieMaxAge,
	})
	return id
}

func newDeviceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}
