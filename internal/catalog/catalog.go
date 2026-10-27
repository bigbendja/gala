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

	// SeasonNumber/EpisodeNumber solo aplican a kind season/episode (M3).
	SeasonNumber  *int `json:"season_number,omitempty"`
	EpisodeNumber *int `json:"episode_number,omitempty"`
	// IdentifyStatus es el estado del pipeline de identificación de M3
	// (pending/junk/review/error/ok; ver internal/catalog/repo_identify.go).
	IdentifyStatus string  `json:"identify_status,omitempty"`
	IdentifyNote   *string `json:"identify_note,omitempty"`

	// Sources solo se rellena en la ficha de detalle.
	Sources []Source `json:"sources,omitempty"`
	// SourceCount y HealthSummary se rellenan en listados (para los puntos de la tarjeta).
	SourceCount   int            `json:"source_count"`
	HealthSummary map[string]int `json:"health_summary,omitempty"`
	// Progress es el progreso de reproducción del dispositivo actual (cookie
	// gala_device), cuando existe; lo rellena la capa de API (M4 §5, "Continuar
	// viendo"). nil si el dispositivo nunca reprodujo este item.
	Progress *Progress `json:"progress,omitempty"`
}

// Progress es el progreso de reproducción de un item para UN dispositivo
// (tabla playback, clave item_id+device_id; Brief M4 §5).
type Progress struct {
	PositionS float64  `json:"position_s"`
	DurationS *float64 `json:"duration_s,omitempty"`
	Watched   bool     `json:"watched"`
	UpdatedAt string   `json:"updated_at"`
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
	// OriginListID enlaza con ingest_lists.id cuando la fuente vino de una
	// ingesta (M5, Brief M5 §4: trazabilidad). nil para fuentes de M2 (scan
	// local) o creadas a mano.
	OriginListID *int64 `json:"origin_list_id,omitempty"`
	// HealthDetail es el motivo legible del último chequeo de salud (Brief
	// M6 §3: "por qué" además del estado) — p. ej. "http 404", "archivo no
	// encontrado", "pendiente de conector". nil hasta el primer chequeo.
	HealthDetail *string `json:"health_detail,omitempty"`
	// OriginPath es la ruta del archivo .strm del que salió esta fuente http
	// (nil si no viene de uno). Es una ruta del disco: nunca se serializa.
	OriginPath *string `json:"-"`
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

	// ParentID/TopLevelOnly filtran por jerarquía (Brief M10a-fix B1: la
	// ficha de una serie necesita listar sus temporadas/episodios directos).
	// Ninguno de los dos puesto = comportamiento previo, sin filtrar por
	// parent_id (compatibilidad). ParentID no nil = solo los hijos directos
	// de ese id. TopLevelOnly = solo items sin padre (parent_id IS NULL);
	// se ignora si ParentID también está puesto.
	ParentID     *int64
	TopLevelOnly bool

	// InProgress, con DeviceID, filtra a items con progreso de reproducción
	// real para ESE dispositivo (position_s > 0, no watched), ordenados por
	// el momento del progreso en vez de por added_at (Brief M10a-fix B3:
	// "Continuar viendo" sin depender de qué página del catálogo esté
	// cargada). DeviceID vacío (sin cookie de dispositivo todavía) da
	// resultado vacío, nunca error — mismo criterio que AttachProgress.
	InProgress bool
	DeviceID   string

	Limit  int // <=0 → 60; máx 200
	Offset int
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
