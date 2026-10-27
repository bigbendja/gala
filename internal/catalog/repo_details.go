package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Person es una persona del reparto de un título.
type Person struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character,omitempty"`
	ProfilePath string `json:"profile_path,omitempty"` // ruta TMDB (para reintentar la descarga)
	Photo       string `json:"photo,omitempty"`        // fichero en cache/img/ ("" = sin foto)
}

// CrewGroup agrupa a quienes hicieron un mismo trabajo ("Dirección": [...]).
type CrewGroup struct {
	Role  string   `json:"role"`
	Names []string `json:"names"`
}

// SimilarTitle es un título que TMDB recomienda "como este".
type SimilarTitle struct {
	TMDBID     int64  `json:"tmdb_id"`
	Kind       string `json:"kind"` // "movie" | "tv"
	Title      string `json:"title"`
	Year       int    `json:"year,omitempty"`
	PosterPath string `json:"poster_path,omitempty"`
	Poster     string `json:"poster,omitempty"`
}

// Details son los datos enriquecidos de TMDB de un item (tabla item_details).
type Details struct {
	ItemID           int64
	TMDBKind         string
	RuntimeMin       *int
	VoteAverage      *float64
	OriginalLanguage string
	Countries        []string
	Tagline          string
	Cast             []Person
	Crew             []CrewGroup
	Similar          []SimilarTitle
	ImagesPending    bool
	ImageAttempts    int
	FetchedAt        string
}

// UpsertDetails guarda (o reemplaza) los detalles de un item.
func (r *Repo) UpsertDetails(ctx context.Context, d *Details) error {
	if d.FetchedAt == "" {
		d.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if d.Countries == nil {
		d.Countries = []string{}
	}
	if d.Cast == nil {
		d.Cast = []Person{}
	}
	if d.Crew == nil {
		d.Crew = []CrewGroup{}
	}
	if d.Similar == nil {
		d.Similar = []SimilarTitle{}
	}
	cj, _ := json.Marshal(d.Countries)
	pj, _ := json.Marshal(d.Cast)
	wj, _ := json.Marshal(d.Crew)
	sj, _ := json.Marshal(d.Similar)
	_, err := r.q.ExecContext(ctx, `INSERT INTO item_details
		(item_id, tmdb_kind, runtime_min, vote_average, original_language, countries, tagline,
		 cast_json, crew_json, similar_json, images_pending, image_attempts, fetched_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET
		  tmdb_kind=excluded.tmdb_kind, runtime_min=excluded.runtime_min,
		  vote_average=excluded.vote_average, original_language=excluded.original_language,
		  countries=excluded.countries, tagline=excluded.tagline, cast_json=excluded.cast_json,
		  crew_json=excluded.crew_json, similar_json=excluded.similar_json,
		  images_pending=excluded.images_pending, image_attempts=excluded.image_attempts,
		  fetched_at=excluded.fetched_at`,
		d.ItemID, d.TMDBKind, d.RuntimeMin, d.VoteAverage, d.OriginalLanguage, string(cj), d.Tagline,
		string(pj), string(wj), string(sj), boolInt(d.ImagesPending), d.ImageAttempts, d.FetchedAt)
	return err
}

// GetDetails devuelve los detalles de un item, o ErrNotFound si aún no se
// han pedido a TMDB (item sin identificar, o recién identificado).
func (r *Repo) GetDetails(ctx context.Context, itemID int64) (*Details, error) {
	row := r.q.QueryRowContext(ctx, detailsSelect+` WHERE item_id = ?`, itemID)
	d, err := scanDetails(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

const detailsSelect = `SELECT item_id, tmdb_kind, runtime_min, vote_average, original_language, countries,
	tagline, cast_json, crew_json, similar_json, images_pending, image_attempts, fetched_at FROM item_details`

func scanDetails(s scanner) (*Details, error) {
	var d Details
	var lang, tagline sql.NullString
	var countries, cast, crew, similar string
	var pending int
	if err := s.Scan(&d.ItemID, &d.TMDBKind, &d.RuntimeMin, &d.VoteAverage, &lang, &countries,
		&tagline, &cast, &crew, &similar, &pending, &d.ImageAttempts, &d.FetchedAt); err != nil {
		return nil, err
	}
	d.OriginalLanguage, d.Tagline, d.ImagesPending = lang.String, tagline.String, pending != 0
	_ = json.Unmarshal([]byte(countries), &d.Countries)
	_ = json.Unmarshal([]byte(cast), &d.Cast)
	_ = json.Unmarshal([]byte(crew), &d.Crew)
	_ = json.Unmarshal([]byte(similar), &d.Similar)
	return &d, nil
}

// ItemsNeedingDetails devuelve las películas y series identificadas (con
// tmdb_id) que todavía no tienen fila en item_details: las recién
// identificadas y las que ya estaban en la biblioteca antes de existir esta
// tabla. Auto-terminante: al guardar sus detalles dejan de aparecer.
func (r *Repo) ItemsNeedingDetails(ctx context.Context) ([]Item, error) {
	return r.itemsQuery(ctx, `SELECT `+itemCols+`, 0 FROM items i
		WHERE i.confirmed = 1 AND i.tmdb_id IS NOT NULL AND i.kind IN (?, ?)
		  AND NOT EXISTS (SELECT 1 FROM item_details d WHERE d.item_id = i.id)
		ORDER BY i.added_at DESC`, KindMovie, KindSeries)
}

// ItemsWithSmallBackdrop devuelve los items cuyo fondo se descargó en la
// resolución antigua (w780): se pasa a w1280 para que el hero no se vea
// borroso a pantalla completa.
func (r *Repo) ItemsWithSmallBackdrop(ctx context.Context) ([]Item, error) {
	return r.itemsQuery(ctx, `SELECT `+itemCols+`, 0 FROM items i
		WHERE i.tmdb_id IS NOT NULL AND i.kind IN (?, ?) AND i.backdrop_path LIKE 'backdrop_w780_%'`,
		KindMovie, KindSeries)
}

func (r *Repo) itemsQuery(ctx context.Context, query string, args ...any) ([]Item, error) {
	rows, err := r.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// DetailsNeedingImages devuelve los detalles con fotos o carátulas sin
// descargar (imágenes que fallaron por la red) para reintentarlas.
func (r *Repo) DetailsNeedingImages(ctx context.Context, maxAttempts int) ([]*Details, error) {
	rows, err := r.q.QueryContext(ctx, detailsSelect+` WHERE images_pending = 1 AND image_attempts < ?`, maxAttempts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Details
	for rows.Next() {
		d, err := scanDetails(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetRatingCert guarda la clasificación por edad (solo si el item no tenía).
func (r *Repo) SetRatingCert(ctx context.Context, itemID int64, cert string) error {
	_, err := r.q.ExecContext(ctx, `UPDATE items SET rating_cert = ? WHERE id = ? AND rating_cert IS NULL`, cert, itemID)
	return err
}

// SetBackdrop sustituye el fichero del fondo de un item.
func (r *Repo) SetBackdrop(ctx context.Context, itemID int64, file string) error {
	_, err := r.q.ExecContext(ctx, `UPDATE items SET backdrop_path = ? WHERE id = ?`, file, itemID)
	return err
}

// LibraryRef es el item de la biblioteca que corresponde a un título de TMDB.
type LibraryRef struct {
	ItemID int64
	Kind   Kind
}

// LibraryByTMDB devuelve, por tmdb_id, el item de la biblioteca que se puede
// abrir: una película con alguna fuente, o una serie. Alimenta "Más como
// esto": un similar que ya tienes enlaza a su ficha.
func (r *Repo) LibraryByTMDB(ctx context.Context, ids []int64) (map[int64]LibraryRef, error) {
	out := map[int64]LibraryRef{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(ids))
	ph := ""
	for i, id := range ids {
		if i > 0 {
			ph += ","
		}
		ph += "?"
		args = append(args, id)
	}
	rows, err := r.q.QueryContext(ctx, `SELECT i.tmdb_id, i.id, i.kind FROM items i
		WHERE i.tmdb_id IN (`+ph+`) AND i.kind IN ('movie','series')
		  AND (i.kind = 'series' OR EXISTS (SELECT 1 FROM sources s WHERE s.item_id = i.id))`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tmdb, id int64
		var kind string
		if err := rows.Scan(&tmdb, &id, &kind); err != nil {
			return nil, err
		}
		out[tmdb] = LibraryRef{ItemID: id, Kind: Kind(kind)}
	}
	return out, rows.Err()
}

// MediaFile es la ficha técnica de una fuente (media_info).
type MediaFile struct {
	SourceID   int64   `json:"source_id"`
	Container  string  `json:"container,omitempty"`
	VideoCodec string  `json:"video_codec,omitempty"`
	AudioCodec string  `json:"audio_codec,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	Bitrate    int64   `json:"bitrate,omitempty"`
	DurationS  float64 `json:"duration_s,omitempty"`
}

// MediaFilesForItem devuelve la ficha técnica de las fuentes de un item que
// ya tienen media_info (las locales analizadas por ffprobe), la mejor primero.
func (r *Repo) MediaFilesForItem(ctx context.Context, itemID int64) ([]MediaFile, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT m.source_id, m.container, m.video_codec, m.audio_codec,
			m.width, m.height, m.bitrate, m.duration_s
		FROM media_info m JOIN sources s ON s.id = m.source_id
		WHERE s.item_id = ? ORDER BY s.priority DESC, s.id ASC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MediaFile{}
	for rows.Next() {
		var f MediaFile
		var c, v, a sql.NullString
		var w, h, br sql.NullInt64
		var dur sql.NullFloat64
		if err := rows.Scan(&f.SourceID, &c, &v, &a, &w, &h, &br, &dur); err != nil {
			return nil, err
		}
		f.Container, f.VideoCodec, f.AudioCodec = c.String, v.String, a.String
		f.Width, f.Height, f.Bitrate, f.DurationS = int(w.Int64), int(h.Int64), br.Int64, dur.Float64
		out = append(out, f)
	}
	return out, rows.Err()
}
