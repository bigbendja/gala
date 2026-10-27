package catalog

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EpisodeRef identifica un episodio ya colgado de su serie de TMDB, a la
// espera de su título/sinopsis/miniatura reales.
type EpisodeRef struct {
	ID           int64
	SeriesTMDBID int64
	Season       int
	Episode      int
}

// EpisodesNeedingDetail devuelve los episodios cuyo título sigue siendo el
// provisional que pone M3 al colgarlos ("Serie T1E01"): aún no se les ha
// pedido a TMDB el detalle de su temporada. Es auto-terminante — una vez
// actualizado el título deja de coincidir — y sirve tanto para los episodios
// recién identificados como para los que ya estaban en la biblioteca.
func (r *Repo) EpisodesNeedingDetail(ctx context.Context) ([]EpisodeRef, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT e.id, ser.tmdb_id, sea.season_number, e.episode_number
		FROM items e
		JOIN items sea ON e.parent_id = sea.id AND sea.kind = ?
		JOIN items ser ON sea.parent_id = ser.id AND ser.kind = ?
		WHERE e.kind = ? AND ser.tmdb_id IS NOT NULL
		  AND sea.season_number IS NOT NULL AND e.episode_number IS NOT NULL
		  AND e.title = ser.title || ' T' || sea.season_number || 'E' || printf('%02d', e.episode_number)
		ORDER BY ser.tmdb_id, sea.season_number, e.episode_number`,
		KindSeason, KindSeries, KindEpisode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EpisodeRef
	for rows.Next() {
		var ref EpisodeRef
		if err := rows.Scan(&ref.ID, &ref.SeriesTMDBID, &ref.Season, &ref.Episode); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// UpdateEpisodeDetail pone el título real de un episodio y, si TMDB los da,
// su sinopsis y miniatura (still, guardada en poster_path: es la imagen del
// episodio). Nunca pisa una imagen ya guardada con NULL.
func (r *Repo) UpdateEpisodeDetail(ctx context.Context, id int64, title string, overview, still *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.q.ExecContext(ctx, `UPDATE items SET title = ?,
			overview = COALESCE(?, overview), poster_path = COALESCE(?, poster_path),
			updated_at = ? WHERE id = ? AND kind = ?`,
		title, overview, still, now, id, KindEpisode)
	return err
}

// AttachSeriesInfo rellena SeriesID/SeriesTitle en los episodios (y
// temporadas) de items: una tarjeta de "Continuar viendo" necesita decir de
// qué serie es el episodio, y el listado solo trae el item suelto.
func (r *Repo) AttachSeriesInfo(ctx context.Context, items []Item) error {
	idx := map[int64][]int{}
	var ids []any
	for i, it := range items {
		if it.Kind == KindEpisode || it.Kind == KindSeason {
			if _, seen := idx[it.ID]; !seen {
				ids = append(ids, it.ID)
			}
			idx[it.ID] = append(idx[it.ID], i)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	// Un episodio cuelga de temporada→serie; una temporada, directamente de la serie.
	rows, err := r.q.QueryContext(ctx, fmt.Sprintf(`SELECT x.id, ser.id, ser.title,
		CASE WHEN p.kind = 'season' THEN p.season_number END FROM items x
		JOIN items p ON x.parent_id = p.id
		JOIN items ser ON ser.id = CASE WHEN p.kind = 'series' THEN p.id ELSE p.parent_id END
		WHERE x.id IN (%s)`, ph), ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, seriesID int64
		var title string
		var seasonNum *int
		if err := rows.Scan(&id, &seriesID, &title, &seasonNum); err != nil {
			return err
		}
		for _, i := range idx[id] {
			sid, st := seriesID, title
			items[i].SeriesID = &sid
			items[i].SeriesTitle = &st
			// El episodio no guarda su temporada: se toma de la que lo contiene
			// ("T1 · E3" en tarjetas y fichas).
			if items[i].Kind == KindEpisode && items[i].SeasonNumber == nil && seasonNum != nil {
				n := *seasonNum
				items[i].SeasonNumber = &n
			}
		}
	}
	return rows.Err()
}

// GenreCount es un género con cuántos títulos de la biblioteca lo tienen.
type GenreCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Genres devuelve los géneros con contenido (películas con alguna fuente y
// series), de más a menos títulos. Alimenta las filas por género de la Home:
// solo existen las que tienen algo que enseñar.
func (r *Repo) Genres(ctx context.Context) ([]GenreCount, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT j.value, COUNT(*) AS n
		FROM items i, json_each(i.genres) j
		WHERE i.identify_status != 'junk'
		  AND ((i.kind = ? AND EXISTS (SELECT 1 FROM sources s WHERE s.item_id = i.id))
		       OR i.kind = ?)
		GROUP BY j.value ORDER BY n DESC, j.value ASC`, KindMovie, KindSeries)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GenreCount{}
	for rows.Next() {
		var g GenreCount
		if err := rows.Scan(&g.Name, &g.Count); err != nil {
			return nil, err
		}
		if strings.TrimSpace(g.Name) != "" {
			out = append(out, g)
		}
	}
	return out, rows.Err()
}
