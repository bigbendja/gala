package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/identify"
)

// handleListPending: GET /api/v1/pending — items "Por confirmar" (Junk
// incluidos) con sus candidatos, si los hay (Brief M3 §5).
func (s *Server) handleListPending(w http.ResponseWriter, r *http.Request) {
	items, err := s.Catalog.ListPendingConfirmation(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if items == nil {
		items = []catalog.PendingItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// confirmRequest acepta las dos variantes del Brief §5: confirmación desde
// un candidato TMDB ({"tmdb_id":123,"kind":"movie"}) o edición manual sin
// metadatos ({"title":"..."}).
type confirmRequest struct {
	TMDBID *int64 `json:"tmdb_id"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
}

// handleConfirmItem: POST /api/v1/items/{id}/confirm (Brief M3 §5).
func (s *Server) handleConfirmItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, "id no válido")
		return
	}
	var req confirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "cuerpo no válido")
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

	switch {
	case req.TMDBID != nil:
		if req.Kind != "movie" && req.Kind != "tv" {
			badRequest(w, "kind debe ser 'movie' o 'tv'")
			return
		}
		if s.TMDB == nil {
			s.fail(w, errors.New("cliente TMDB no configurado"))
			return
		}
		// season/episode se guardaron en el propio provisional al dejarlo
		// "Por confirmar" (ver catalog.Repo.SetIdentifyCandidates).
		var parsed identify.Parsed
		if it.SeasonNumber != nil {
			parsed.Season = *it.SeasonNumber
		}
		if it.EpisodeNumber != nil {
			parsed.Episode = *it.EpisodeNumber
		}
		if err := identify.ApplyIdentification(r.Context(), s.TMDB, s.Catalog, id, *req.TMDBID, req.Kind, parsed); err != nil {
			if errors.Is(err, identify.ErrOffline) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sin conexión con TMDB"})
				return
			}
			s.fail(w, err)
			return
		}
	case strings.TrimSpace(req.Title) != "":
		if err := s.Catalog.ApplyManualConfirm(r.Context(), id, req.Title, ""); err != nil {
			s.fail(w, err)
			return
		}
	default:
		badRequest(w, "se requiere 'tmdb_id'+'kind', o 'title'")
		return
	}

	if s.Identify != nil {
		s.Identify.Enqueue()
	}

	updated, err := s.Catalog.Get(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleIdentifyStatus: GET /api/v1/identify/status (Brief M3 §5).
func (s *Server) handleIdentifyStatus(w http.ResponseWriter, r *http.Request) {
	if s.Identify == nil {
		writeJSON(w, http.StatusOK, identify.Status{})
		return
	}
	writeJSON(w, http.StatusOK, s.Identify.Status())
}

// reImgFilename son los nombres deterministas que genera Client.downloadImage
// (prefijo_size_hash.ext); cualquier otra cosa se rechaza.
var reImgFilename = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// handleImg: GET /api/v1/img/{archivo} — sirve cache/img/{archivo}. Valida
// el nombre contra path traversal (CLAUDE.md: nunca confiar en rutas que
// vienen de fuera).
func (s *Server) handleImg(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("archivo")
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") || !reImgFilename.MatchString(name) {
		badRequest(w, "nombre de archivo no válido")
		return
	}

	dir := filepath.Join(s.CacheDir, "img")
	full := filepath.Join(dir, name)
	if full != filepath.Clean(full) || !strings.HasPrefix(full, dir+string(filepath.Separator)) {
		badRequest(w, "ruta no válida")
		return
	}

	if _, err := os.Stat(full); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no encontrado"})
		return
	}
	http.ServeFile(w, r, full)
}
