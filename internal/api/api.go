// Package api expone la API REST /api/v1 de GALA.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/identify"
	"github.com/bigbendja/gala/internal/media"
	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
	"github.com/bigbendja/gala/internal/stream"
)

// Version de la aplicación; se sobreescribe en build con -ldflags.
var Version = "0.1.0-dev"

// Server agrupa las dependencias de los handlers.
type Server struct {
	Store    *store.Store
	Catalog  *catalog.Repo
	Config   *config.Config
	Scanner  *scan.Scanner
	Enum     localfs.Enumerator
	Identify *identify.Queue  // nil = endpoints de identificación degradan a estado vacío
	TMDB     *identify.Client // nil solo en tests que no ejercitan M3
	CacheDir string           // raíz de cache/ (img/ y hls/ viven dentro), Brief M3 §2 / M4 §4
	Log      *slog.Logger

	FFmpeg *media.FFmpeg   // nunca nil (Locate() siempre devuelve algo); Available() puede ser false
	HLS    *stream.Manager // nil solo en tests que no ejercitan streaming/remux (M4)
}

// New crea el servidor de API con sus dependencias. repo, cfg, sc, enum, q,
// tmdb, ffmpeg y hls se comparten con el resto de la aplicación (poller,
// escaneo inicial, cola de identificación y cola de sondeo de medios en main).
func New(st *store.Store, repo *catalog.Repo, cfg *config.Config, sc *scan.Scanner,
	enum localfs.Enumerator, q *identify.Queue, tmdb *identify.Client, cacheDir string, log *slog.Logger,
	ffmpeg *media.FFmpeg, hls *stream.Manager) *Server {
	return &Server{Store: st, Catalog: repo, Config: cfg, Scanner: sc, Enum: enum,
		Identify: q, TMDB: tmdb, CacheDir: cacheDir, Log: log, FFmpeg: ffmpeg, HLS: hls}
}

// Routes registra las rutas de la API en el mux dado (requiere Go >= 1.22).
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/ping", s.handlePing)
	mux.HandleFunc("GET /api/v1/items", s.handleListItems)
	mux.HandleFunc("GET /api/v1/items/{id}", s.handleGetItem)

	mux.HandleFunc("GET /api/v1/folders", s.handleListFolders)
	mux.HandleFunc("POST /api/v1/folders", s.handleAddFolder)
	mux.HandleFunc("DELETE /api/v1/folders", s.handleDeleteFolder)
	mux.HandleFunc("POST /api/v1/scan", s.handleStartScan)
	mux.HandleFunc("GET /api/v1/scan/status", s.handleScanStatus)
	mux.HandleFunc("GET /api/v1/volumes", s.handleListVolumes)

	mux.HandleFunc("GET /api/v1/pending", s.handleListPending)
	mux.HandleFunc("POST /api/v1/items/{id}/confirm", s.handleConfirmItem)
	mux.HandleFunc("GET /api/v1/identify/status", s.handleIdentifyStatus)
	mux.HandleFunc("GET /api/v1/img/{archivo}", s.handleImg)

	mux.HandleFunc("GET /api/v1/play/{item_id}", s.handlePlay)
	mux.HandleFunc("GET /api/v1/stream/{source_id}", s.handleStreamDirect)
	mux.HandleFunc("GET /api/v1/stream/{source_id}/hls/start", s.handleHLSStart)
	mux.HandleFunc("GET /api/v1/stream/{source_id}/hls/{session_id}/{file}", s.handleHLSFile)
	mux.HandleFunc("POST /api/v1/sources/{id}/failed", s.handleSourceFailed)
	mux.HandleFunc("GET /api/v1/playback/{item_id}", s.handleGetPlayback)
	mux.HandleFunc("PUT /api/v1/playback/{item_id}", s.handlePutPlayback)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok", "app": "gala", "version": Version,
	})
}

// handleListItems: GET /api/v1/items?kind=movie&q=texto&year=2023&genre=Drama
//
//	&wished=true&confirmed=false&limit=60&offset=0
func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := catalog.ListFilter{
		Kind:  q.Get("kind"),
		Query: q.Get("q"),
		Genre: q.Get("genre"),
	}
	var err error
	if f.Year, err = intParam(q.Get("year"), 0); err != nil {
		badRequest(w, "year no válido")
		return
	}
	if f.Limit, err = intParam(q.Get("limit"), 0); err != nil {
		badRequest(w, "limit no válido")
		return
	}
	if f.Offset, err = intParam(q.Get("offset"), 0); err != nil {
		badRequest(w, "offset no válido")
		return
	}
	if f.Wished, err = boolParam(q.Get("wished")); err != nil {
		badRequest(w, "wished no válido")
		return
	}
	if f.Confirmed, err = boolParam(q.Get("confirmed")); err != nil {
		badRequest(w, "confirmed no válido")
		return
	}

	res, err := s.Catalog.List(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	// "Continuar viendo" (Brief M4 §5): solo LEE la cookie si ya existe, sin
	// crearla en un listado de solo lectura; se crea la primera vez que el
	// dispositivo reproduce algo de verdad (PUT /playback).
	if err := s.Catalog.AttachProgress(r.Context(), res.Items, readDeviceID(r)); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, "id no válido")
		return
	}
	it, err := s.Catalog.Get(r.Context(), id)
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no encontrado"})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	items := []catalog.Item{*it}
	if err := s.Catalog.AttachProgress(r.Context(), items, readDeviceID(r)); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items[0])
}

func intParam(v string, def int) (int, error) {
	if v == "" {
		return def, nil
	}
	return strconv.Atoi(v)
}

func boolParam(v string) (*bool, error) {
	if v == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.Log.Error("api", "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "error interno"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
