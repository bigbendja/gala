package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Estados de items.identify_status (migración 0002, Brief M3 §4).
const (
	IdentifyPending = "pending"
	IdentifyJunk    = "junk"
	IdentifyReview  = "review"
	IdentifyError   = "error"
	IdentifyOK      = "ok"
)

// Candidate es un candidato TMDB propuesto en "Por confirmar" (confianza
// MEDIA o BAJA, Brief M3 §3). PosterPath es la ruta TMDB tal cual (no se
// descarga hasta que el usuario confirma, para no gastar red en candidatos
// descartados).
type Candidate struct {
	TMDBID        int64   `json:"tmdb_id"`
	Kind          string  `json:"kind"` // "movie" | "tv"
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title,omitempty"`
	Year          int     `json:"year,omitempty"`
	PosterPath    string  `json:"poster_path,omitempty"`
	Score         float64 `json:"score"`
}

// PendingIdentify devuelve los items provisionales (confirmed=0) listos
// para que la cola de M3 los procese: pendientes de un primer intento, o
// que fallaron antes por un problema de red (Brief §4). Los Junk y los ya
// revisados (review) son estados terminales y no se reintentan solos.
func (r *Repo) PendingIdentify(ctx context.Context) ([]Item, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+itemCols+`, 0 FROM items i
		WHERE i.confirmed = 0 AND i.identify_status IN (?, ?)
		ORDER BY i.added_at ASC`, IdentifyPending, IdentifyError)
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

// PendingItem es un item "Por confirmar" con sus candidatos, si los hay
// (GET /api/v1/pending, Brief §5).
type PendingItem struct {
	Item
	Candidates []Candidate `json:"candidates,omitempty"`
}

// ListPendingConfirmation devuelve todos los items confirmed=0 (Junk
// incluidos: la UI los agrupa en "Por confirmar", Brief §4) con sus
// candidatos.
func (r *Repo) ListPendingConfirmation(ctx context.Context) ([]PendingItem, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+itemCols+`,
		(SELECT COUNT(*) FROM sources s WHERE s.item_id = i.id), i.identify_candidates
		FROM items i WHERE i.confirmed = 0 ORDER BY i.added_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingItem
	for rows.Next() {
		var it Item
		var kind string
		var genresRaw, candidatesRaw sql.NullString
		var confirmedInt, wishedInt int
		err := rows.Scan(&it.ID, &kind, &it.ParentID, &it.TMDBID, &it.Title,
			&it.OriginalTitle, &it.Year, &it.Overview, &it.PosterPath, &it.BackdropPath,
			&genresRaw, &it.RatingCert, &confirmedInt, &wishedInt, &it.AddedAt, &it.UpdatedAt,
			&it.SeasonNumber, &it.EpisodeNumber, &it.IdentifyStatus, &it.IdentifyNote,
			&it.SourceCount, &candidatesRaw)
		if err != nil {
			return nil, err
		}
		it.Kind = Kind(kind)
		it.Confirmed = confirmedInt != 0
		it.Wished = wishedInt != 0
		it.Genres = []string{}
		if genresRaw.Valid && genresRaw.String != "" {
			_ = json.Unmarshal([]byte(genresRaw.String), &it.Genres)
		}
		pi := PendingItem{Item: it}
		if candidatesRaw.Valid && candidatesRaw.String != "" {
			_ = json.Unmarshal([]byte(candidatesRaw.String), &pi.Candidates)
		}
		out = append(out, pi)
	}
	return out, rows.Err()
}

// SetIdentifyStatus marca el estado del pipeline de un item sin tocar sus
// metadatos (Junk, o error de red a reintentar; Brief §4).
func (r *Repo) SetIdentifyStatus(ctx context.Context, itemID int64, status string, note *string) error {
	_, err := r.q.ExecContext(ctx,
		`UPDATE items SET identify_status = ?, identify_note = ? WHERE id = ?`,
		status, note, itemID)
	return err
}

// SetIdentifyCandidates deja un item "Por confirmar" con hasta 5
// candidatos propuestos por el matcher (confianza MEDIA/BAJA, Brief §3).
// season/episode (0 = desconocido) se guardan en el propio provisional para
// que una confirmación manual posterior (POST /items/{id}/confirm con
// tmdb_id elegido) sepa a qué temporada/episodio aplicarlo sin tener que
// volver a analizar el nombre de archivo.
func (r *Repo) SetIdentifyCandidates(ctx context.Context, itemID int64, candidates []Candidate, season, episode int, note *string) error {
	payload, err := json.Marshal(candidates)
	if err != nil {
		return err
	}
	var sn, en *int
	if season > 0 {
		sn = &season
	}
	if episode > 0 {
		en = &episode
	}
	_, err = r.q.ExecContext(ctx,
		`UPDATE items SET identify_status = ?, identify_candidates = ?, identify_note = ?,
			season_number = ?, episode_number = ? WHERE id = ?`,
		IdentifyReview, string(payload), note, sn, en, itemID)
	return err
}

// ApplyMovieIdentification aplica la identificación completa de una
// película sobre el item provisional itemID. Dedupe (Brief §3): si ya
// existe un item con ese tmdb_id, mueve las fuentes del provisional al
// existente y borra el provisional; si no, transforma el provisional en el
// item identificado in situ (conserva sus fuentes).
func (r *Repo) ApplyMovieIdentification(ctx context.Context, itemID, tmdbID int64,
	title string, originalTitle, overview, posterPath, backdropPath, ratingCert *string,
	year *int, genres []string, now string) error {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	existing, err := r.findItemByTMDB(ctx, tmdbID, KindMovie)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != itemID {
		return r.moveSourcesAndDelete(ctx, itemID, existing.ID)
	}

	genresJSON, err := json.Marshal(genres)
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `UPDATE items SET kind = ?, tmdb_id = ?, title = ?,
		original_title = ?, year = ?, overview = ?, poster_path = ?, backdrop_path = ?,
		genres = ?, rating_cert = ?, confirmed = 1, identify_status = ?,
		identify_candidates = NULL, identify_note = NULL, updated_at = ?
		WHERE id = ?`,
		KindMovie, tmdbID, title, originalTitle, year, overview, posterPath, backdropPath,
		string(genresJSON), ratingCert, IdentifyOK, now, itemID)
	return err
}

// ApplyEpisodeIdentification identifica el item provisional itemID como el
// episodio (seasonNum, episodeNum) de la serie tmdb_id=seriesTMDBID: crea o
// reutiliza la jerarquía serie→temporada (Brief §3), y aplica el mismo
// dedupe que ApplyMovieIdentification a nivel de episodio.
func (r *Repo) ApplyEpisodeIdentification(ctx context.Context, itemID, seriesTMDBID int64,
	seriesTitle string, seriesOriginalTitle, seriesOverview, seriesPoster, seriesBackdrop *string,
	genres []string, seasonNum, episodeNum int, seasonName, episodeTitle, now string) error {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	seriesID, err := r.findOrCreateSeries(ctx, seriesTMDBID, seriesTitle, seriesOriginalTitle,
		seriesOverview, seriesPoster, seriesBackdrop, genres, now)
	if err != nil {
		return fmt.Errorf("crear/reutilizar serie: %w", err)
	}
	seasonID, err := r.findOrCreateSeason(ctx, seriesID, seasonNum, seasonName, now)
	if err != nil {
		return fmt.Errorf("crear/reutilizar temporada: %w", err)
	}

	existing, err := r.findChildByNumber(ctx, seasonID, KindEpisode, episodeNum)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != itemID {
		return r.moveSourcesAndDelete(ctx, itemID, existing.ID)
	}

	title := episodeTitle
	if title == "" {
		title = fmt.Sprintf("%s T%dE%02d", seriesTitle, seasonNum, episodeNum)
	}
	_, err = r.q.ExecContext(ctx, `UPDATE items SET kind = ?, parent_id = ?, tmdb_id = NULL,
		title = ?, episode_number = ?, confirmed = 1, identify_status = ?,
		identify_candidates = NULL, identify_note = NULL, updated_at = ?
		WHERE id = ?`,
		KindEpisode, seasonID, title, episodeNum, IdentifyOK, now, itemID)
	return err
}

// ApplyManualConfirm confirma manualmente un item sin pasar por TMDB: solo
// título, confirmed=1, sin metadatos adicionales (Brief §5).
func (r *Repo) ApplyManualConfirm(ctx context.Context, itemID int64, title, now string) error {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := r.q.ExecContext(ctx, `UPDATE items SET title = ?, confirmed = 1,
		identify_status = ?, identify_candidates = NULL, identify_note = NULL,
		updated_at = ? WHERE id = ?`, title, IdentifyOK, now, itemID)
	return err
}

// DeleteItem borra un item y sus fuentes (ON DELETE CASCADE). Lo usa la
// cola de M3 para los extras (sample/trailer/teaser): "los extras NO
// aparecen en la galería" (Brief §4).
func (r *Repo) DeleteItem(ctx context.Context, itemID int64) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, itemID)
	return err
}

func (r *Repo) findOrCreateSeries(ctx context.Context, tmdbID int64, title string,
	originalTitle, overview, posterPath, backdropPath *string, genres []string, now string) (int64, error) {
	existing, err := r.findItemByTMDB(ctx, tmdbID, KindSeries)
	if err != nil {
		return 0, err
	}
	if existing != nil {
		return existing.ID, nil
	}
	id := tmdbID
	item := Item{
		Kind: KindSeries, TMDBID: &id, Title: title, OriginalTitle: originalTitle,
		Overview: overview, PosterPath: posterPath, BackdropPath: backdropPath,
		Genres: genres, Confirmed: true, IdentifyStatus: IdentifyOK,
	}
	return r.InsertItem(ctx, &item, now)
}

func (r *Repo) findOrCreateSeason(ctx context.Context, seriesID int64, seasonNum int, seasonName, now string) (int64, error) {
	existing, err := r.findChildByNumber(ctx, seriesID, KindSeason, seasonNum)
	if err != nil {
		return 0, err
	}
	if existing != nil {
		return existing.ID, nil
	}
	title := seasonName
	if title == "" {
		title = fmt.Sprintf("Temporada %d", seasonNum)
	}
	sn := seasonNum
	item := Item{
		Kind: KindSeason, ParentID: &seriesID, Title: title, SeasonNumber: &sn,
		Genres: []string{}, Confirmed: true, IdentifyStatus: IdentifyOK,
	}
	return r.InsertItem(ctx, &item, now)
}

func (r *Repo) findItemByTMDB(ctx context.Context, tmdbID int64, kind Kind) (*Item, error) {
	row := r.q.QueryRowContext(ctx, `SELECT `+itemCols+`, 0 FROM items i
		WHERE i.tmdb_id = ? AND i.kind = ? LIMIT 1`, tmdbID, kind)
	it, err := scanItem(row, true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// findChildByNumber busca un hijo directo de parentID por kind (season o
// episode) y su número (season_number o episode_number según kind).
func (r *Repo) findChildByNumber(ctx context.Context, parentID int64, kind Kind, number int) (*Item, error) {
	col := "season_number"
	if kind == KindEpisode {
		col = "episode_number"
	}
	row := r.q.QueryRowContext(ctx, `SELECT `+itemCols+`, 0 FROM items i
		WHERE i.parent_id = ? AND i.kind = ? AND i.`+col+` = ? LIMIT 1`, parentID, kind, number)
	it, err := scanItem(row, true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// moveSourcesAndDelete mueve todas las fuentes de fromItemID a toItemID y
// borra el item origen (dedupe por tmdb_id, Brief §3).
func (r *Repo) moveSourcesAndDelete(ctx context.Context, fromItemID, toItemID int64) error {
	if fromItemID == toItemID {
		return nil
	}
	if _, err := r.q.ExecContext(ctx, `UPDATE sources SET item_id = ? WHERE item_id = ?`,
		toItemID, fromItemID); err != nil {
		return err
	}
	_, err := r.q.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, fromItemID)
	return err
}

// --- Caché TMDB (implementa identify.Cache sin que catalog dependa de
// identify: identify.Cache es una interfaz estructural) -------------------

// GetTMDBDetail devuelve el detalle cacheado de /movie/{id} o /tv/{id}
// (tabla tmdb_cache), o ok=false si no está en caché (Brief §2).
func (r *Repo) GetTMDBDetail(ctx context.Context, tmdbID int64, kind, lang string) (string, bool, error) {
	var payload string
	err := r.q.QueryRowContext(ctx, `SELECT payload FROM tmdb_cache
		WHERE tmdb_id = ? AND kind = ? AND lang = ?`, tmdbID, kind, lang).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return payload, true, nil
}

// PutTMDBDetail guarda (o actualiza) el detalle crudo de TMDB en caché.
func (r *Repo) PutTMDBDetail(ctx context.Context, tmdbID int64, kind, lang, payload string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.q.ExecContext(ctx, `INSERT INTO tmdb_cache (tmdb_id, kind, lang, payload, fetched_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(tmdb_id, kind, lang) DO UPDATE SET
			payload = excluded.payload, fetched_at = excluded.fetched_at`,
		tmdbID, kind, lang, payload, now)
	return err
}

// GetSearch/PutSearch cachean /search/movie y /search/tv en su propia
// tabla search_cache (aún no hay tmdb_id: la clave es un hash de la
// query+año; decisión documentada en internal/identify/tmdb.go y en la
// migración 0002).
func (r *Repo) GetSearch(ctx context.Context, queryHash, kind, lang string) (string, bool, error) {
	var payload string
	err := r.q.QueryRowContext(ctx, `SELECT payload FROM search_cache
		WHERE query_hash = ? AND kind = ? AND lang = ?`, queryHash, kind, lang).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return payload, true, nil
}

func (r *Repo) PutSearch(ctx context.Context, queryHash, kind, lang, payload string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.q.ExecContext(ctx, `INSERT INTO search_cache (query_hash, kind, lang, payload, fetched_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(query_hash, kind, lang) DO UPDATE SET
			payload = excluded.payload, fetched_at = excluded.fetched_at`,
		queryHash, kind, lang, payload, now)
	return err
}
