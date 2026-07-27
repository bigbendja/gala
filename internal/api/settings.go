package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bigbendja/gala/internal/identify"
)

// tmdbSettingsView nunca devuelve la clave en sí (Brief M10c §2: "no
// mostrar la clave entera, solo si existe/está vacía").
type tmdbSettingsView struct {
	Configured bool `json:"configured"`
}

// handleGetTMDBSettings: GET /api/v1/settings/tmdb.
func (s *Server) handleGetTMDBSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, tmdbSettingsView{Configured: strings.TrimSpace(s.Config.TMDBAPIKey) != ""})
}

type putTMDBRequest struct {
	APIKey string `json:"api_key"`
}

// handlePutTMDBSettings: PUT /api/v1/settings/tmdb  body {"api_key": "..."}.
// Valida la clave con una búsqueda de prueba real antes de guardarla (Brief
// M10c §2: "una llamada de prueba a TMDB para confirmar que la clave
// sirve"), para que el usuario sepa si sirve y no solo si se guardó. Si no
// hay cliente TMDB configurado (algunos tests), se guarda sin validar.
func (s *Server) handlePutTMDBSettings(w http.ResponseWriter, r *http.Request) {
	var req putTMDBRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "cuerpo no válido")
		return
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		badRequest(w, "api_key requerido")
		return
	}

	if s.TMDB != nil {
		validator := &identify.Client{APIKey: key, HTTP: s.TMDB.HTTP, BaseURL: s.TMDB.BaseURL}
		if _, err := validator.SearchMovie(r.Context(), "test", 0); err != nil {
			if errors.Is(err, identify.ErrOffline) {
				writeJSON(w, http.StatusServiceUnavailable,
					map[string]string{"error": "sin conexión con TMDB: no se pudo comprobar la clave"})
				return
			}
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "la clave de TMDB no es válida"})
			return
		}
	}

	s.Config.TMDBAPIKey = key
	if err := s.Config.Save(); err != nil {
		s.fail(w, err)
		return
	}
	// Toma efecto de inmediato para la API y para la cola de identificación
	// (comparten el mismo *identify.Client; ver internal/config §Ajustes en
	// CLAUDE.md para el porqué de la falta de mutex aquí, igual que
	// Config.MediaFolders en internal/api/localfs.go).
	if s.TMDB != nil {
		s.TMDB.APIKey = key
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
