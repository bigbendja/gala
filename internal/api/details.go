package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/stream"
)

type castView struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	Photo     string `json:"photo,omitempty"` // fichero para /img/{archivo}; vacío = iniciales
}

type similarView struct {
	TMDBID int64  `json:"tmdb_id"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Year   int    `json:"year,omitempty"`
	Poster string `json:"poster,omitempty"`
	// ItemID es el item de la biblioteca que corresponde a este título (se
	// puede abrir su ficha); nil = no está en tu galería.
	ItemID *int64 `json:"item_id,omitempty"`
}

type fileView struct {
	catalog.MediaFile
	SizeBytes int64 `json:"size_bytes,omitempty"`
}

// detailsView es la parte "enriquecida" de la ficha: lo que TMDB da además de
// los metadatos básicos (duración, nota, reparto, equipo, similares) y la
// ficha técnica de los archivos locales analizados.
type detailsView struct {
	HasDetails       bool                `json:"has_details"`
	RuntimeMin       *int                `json:"runtime_min,omitempty"`
	VoteAverage      *float64            `json:"vote_average,omitempty"`
	OriginalLanguage string              `json:"original_language,omitempty"`
	Countries        []string            `json:"countries"`
	Tagline          string              `json:"tagline,omitempty"`
	Cast             []castView          `json:"cast"`
	Crew             []catalog.CrewGroup `json:"crew"`
	Similar          []similarView       `json:"similar"`
	Files            []fileView          `json:"files"`
}

// handleItemDetails: GET /api/v1/items/{id}/details — reparto, equipo,
// similares, duración, nota y ficha técnica. Siempre 200 (con listas vacías y
// has_details=false si TMDB aún no se ha consultado o el item no está
// identificado): la web decide qué secciones enseñar.
func (s *Server) handleItemDetails(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(w, "id no válido")
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

	view := detailsView{
		Countries: []string{}, Cast: []castView{}, Crew: []catalog.CrewGroup{},
		Similar: []similarView{}, Files: []fileView{},
	}

	d, err := s.Catalog.GetDetails(r.Context(), id)
	if err != nil && !errors.Is(err, catalog.ErrNotFound) {
		s.fail(w, err)
		return
	}
	if err == nil {
		view.HasDetails = true
		view.RuntimeMin, view.VoteAverage = d.RuntimeMin, d.VoteAverage
		view.OriginalLanguage, view.Tagline = d.OriginalLanguage, d.Tagline
		if d.Countries != nil {
			view.Countries = d.Countries
		}
		for _, p := range d.Cast {
			view.Cast = append(view.Cast, castView{ID: p.ID, Name: p.Name, Character: p.Character, Photo: p.Photo})
		}
		if d.Crew != nil {
			view.Crew = d.Crew
		}
		ids := make([]int64, 0, len(d.Similar))
		for _, sm := range d.Similar {
			ids = append(ids, sm.TMDBID)
		}
		lib, err := s.Catalog.LibraryByTMDB(r.Context(), ids)
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, sm := range d.Similar {
			v := similarView{TMDBID: sm.TMDBID, Kind: sm.Kind, Title: sm.Title, Year: sm.Year, Poster: sm.Poster}
			if ref, ok := lib[sm.TMDBID]; ok && ref.ItemID != id {
				itemID := ref.ItemID
				v.ItemID = &itemID
			}
			view.Similar = append(view.Similar, v)
		}
	}

	files, err := s.Catalog.MediaFilesForItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, f := range files {
		fv := fileView{MediaFile: f}
		// Tamaño en disco: solo de archivos locales dentro de las carpetas
		// registradas (la ruta nunca sale por la API).
		for _, src := range item.Sources {
			if src.ID == f.SourceID && src.Provider == localfs.ProviderID &&
				stream.ValidatePath(src.Locator, s.Config.MediaFolders) == nil {
				if st, err := os.Stat(src.Locator); err == nil && !st.IsDir() {
					fv.SizeBytes = st.Size()
				}
			}
		}
		view.Files = append(view.Files, fv)
	}
	writeJSON(w, http.StatusOK, view)
}
