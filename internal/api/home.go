package api

import (
	"encoding/json"
	"net/http"

	"github.com/bigbendja/gala/internal/catalog"
)

// handleGenres: GET /api/v1/genres — géneros con contenido y su recuento, de
// más a menos títulos. Alimenta las filas por género de la Home.
func (s *Server) handleGenres(w http.ResponseWriter, r *http.Request) {
	genres, err := s.Catalog.Genres(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if genres == nil {
		genres = []catalog.GenreCount{}
	}
	writeJSON(w, http.StatusOK, genres)
}

type homeSettingsView struct {
	Hero bool `json:"hero"`
}

// handleGetHomeSettings: GET /api/v1/settings/home — preferencias de la Home.
// Las lee cualquier dispositivo (la Home se pinta igual en el móvil).
func (s *Server) handleGetHomeSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, homeSettingsView{Hero: s.Config.HomeHeroEnabled()})
}

// handlePutHomeSettings: PUT /api/v1/settings/home {"hero": bool} — solo desde
// el PC (Ajustes › Inicio), con las mismas defensas que el resto de ajustes.
func (s *Server) handlePutHomeSettings(w http.ResponseWriter, r *http.Request) {
	if !requireOwner(w, r, true) {
		return
	}
	var req struct {
		Hero *bool `json:"hero"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Hero == nil {
		badRequest(w, "hero requerido (true o false)")
		return
	}
	if err := s.Config.SetHomeHero(*req.Hero); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, homeSettingsView{Hero: *req.Hero})
}
