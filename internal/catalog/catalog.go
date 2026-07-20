// Package catalog contiene el modelo de dominio y el acceso a datos del catálogo.
package catalog

// Kind es el tipo de entrada de catálogo.
type Kind string

const (
	KindMovie   Kind = "movie"
	KindSeries  Kind = "series"
	KindSeason  Kind = "season"
	KindEpisode Kind = "episode"
	KindOther   Kind = "other"
)

// ValidKind indica si k es un tipo admitido por el esquema.
func ValidKind(k string) bool {
	switch Kind(k) {
	case KindMovie, KindSeries, KindSeason, KindEpisode, KindOther:
		return true
	}
	return false
}

// Item es una entrada de catálogo. Una película/serie NO es un enlace:
// es una entrada con N fuentes asociadas (Source).
type Item struct {
	ID            int64    `json:"id"`
	Kind          Kind     `json:"kind"`
	ParentID      *int64   `json:"parent_id,omitempty"`
	TMDBID        *int64   `json:"tmdb_id,omitempty"`
	Title         string   `json:"title"`
	OriginalTitle *string  `json:"original_title,omitempty"`
	Year          *int     `json:"year,omitempty"`
	Overview      *string  `json:"overview,omitempty"`
	PosterPath    *string  `json:"poster_path,omitempty"`
	BackdropPath  *string  `json:"backdrop_path,omitempty"`
	Genres        []string `json:"genres"`
	RatingCert    *string  `json:"rating_cert,omitempty"`
	Confirmed     bool     `json:"confirmed"`
	Wished        bool     `json:"wished"`
	AddedAt       string   `json:"added_at"`
	UpdatedAt     string   `json:"updated_at"`

	// Sources solo se rellena en la ficha de detalle.
	Sources []Source `json:"sources,omitempty"`
	// SourceCount y HealthSummary se rellenan en listados (para los puntos de la tarjeta).
	SourceCount   int            `json:"source_count"`
	HealthSummary map[string]int `json:"health_summary,omitempty"`
}

// Source es una fuente concreta de un item (archivo local, enlace, entrada de lista...).
type Source struct {
	ID           int64   `json:"id"`
	ItemID       int64   `json:"item_id"`
	Provider     string  `json:"provider"`
	Locator      string  `json:"locator"`
	VolumeID     *string `json:"volume_id,omitempty"`
	QualityLabel *string `json:"quality_label,omitempty"`
	Health       string  `json:"health"`
	LastChecked  *string `json:"last_checked,omitempty"`
	Priority     int     `json:"priority"`
	AddedAt      string  `json:"added_at"`
}

// Estados de salud de una fuente.
const (
	HealthUnverified   = "sin_verificar"
	HealthAlive        = "viva"
	HealthSlow         = "lenta"
	HealthDead         = "caída"
	HealthDisconnected = "desconectada"
)

// Volume es un disco/volumen conocido (interno o USB) sobre el que puede
// haber fuentes localfs. Lo gestiona el poller de volúmenes (M2).
type Volume struct {
	ID        string  `json:"id"`
	Label     *string `json:"label,omitempty"`
	LastSeen  *string `json:"last_seen,omitempty"`
	Connected bool    `json:"connected"`
}

// ListFilter son los filtros del listado del catálogo.
type ListFilter struct {
	Kind      string // vacío = todos
	Query     string // búsqueda en título y título original
	Year      int    // 0 = todos
	Genre     string // vacío = todos
	Wished    *bool  // nil = indiferente
	Confirmed *bool  // nil = indiferente
	Limit     int    // <=0 → 60; máx 200
	Offset    int
}

// Normalize aplica límites seguros al filtro.
func (f *ListFilter) Normalize() {
	if f.Limit <= 0 {
		f.Limit = 60
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}

// ListResult es la página de resultados de un listado.
type ListResult struct {
	Items  []Item `json:"items"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}
