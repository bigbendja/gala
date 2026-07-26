package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/bigbendja/gala/internal/catalog"
)

// --- /api/v1/items/{id}/wish (Brief M10a-fix B2; anticipo mínimo de M8) ---
//
// Solo alta/baja del flag wished sobre un item YA existente en el catálogo:
// el botón "+ Mi lista" de la ficha llama aquí. La búsqueda-para-desear un
// título SIN fuente todavía (contra TMDB, creando el item) es el resto de
// M8 y no entra en este endpoint.

// handleWishAdd: POST /api/v1/items/{id}/wish.
func (s *Server) handleWishAdd(w http.ResponseWriter, r *http.Request) {
	s.setWished(w, r, true)
}

// handleWishRemove: DELETE /api/v1/items/{id}/wish.
func (s *Server) handleWishRemove(w http.ResponseWriter, r *http.Request) {
	s.setWished(w, r, false)
}

func (s *Server) setWished(w http.ResponseWriter, r *http.Request, wished bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, "id no válido")
		return
	}
	if err := s.Catalog.SetWished(r.Context(), id, wished); err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no encontrado"})
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "wished": wished})
}
