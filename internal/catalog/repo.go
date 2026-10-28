package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound indica que el item o fuente solicitado no existe.
var ErrNotFound = errors.New("item no encontrado")

// dbtx abstrae *sql.DB y *sql.Tx: permite reutilizar las mismas consultas
// dentro o fuera de una transacción explícita (ver Tx).
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Repo es el repositorio de catálogo sobre SQLite.
type Repo struct {
	DB *sql.DB // conexión completa; usar Tx para agrupar varias operaciones
	q  dbtx
}

// NewRepo crea un repositorio.
func NewRepo(db *sql.DB) *Repo { return &Repo{DB: db, q: db} }

// Tx ejecuta fn dentro de una transacción SQL: el *Repo que recibe fn comparte
// esa transacción para todas sus operaciones. Se confirma si fn no devuelve
// error; se deshace en caso contrario. Pensada para altas en lote (M2:
// escaneo de carpetas) que no deben castigar el disco con un commit por fila.
func (r *Repo) Tx(ctx context.Context, fn func(*Repo) error) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Repo{DB: r.DB, q: tx}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

const itemCols = `i.id, i.kind, i.parent_id, i.tmdb_id, i.title, i.original_title,
	i.year, i.overview, i.poster_path, i.backdrop_path, i.genres, i.rating_cert,
	i.confirmed, i.wished, i.added_at, i.updated_at, i.season_number,
	i.episode_number, i.identify_status, i.identify_note`

// List devuelve una página del catálogo según el filtro, con contadores de
// fuentes y resumen de salud por item (para los puntos de las tarjetas).
func (r *Repo) List(ctx context.Context, f ListFilter) (*ListResult, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}

	if f.Kind != "" {
		if !ValidKind(f.Kind) {
			return nil, fmt.Errorf("kind no válido: %q", f.Kind)
		}
		where = append(where, "i.kind = ?")
		args = append(args, f.Kind)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		// Insensible a mayúsculas Y a acentos (Brief M10c §1): "parasitos"
		// debe encontrar "Parásitos". COLLATE NOCASE solo cubre ASCII, así
		// que además plegamos vocales acentuadas en ambos lados (ver
		// search.go); original_title puede ser NULL, de ahí el COALESCE.
		where = append(where, "("+foldAccentsSQL("i.title")+" LIKE ? OR "+
			foldAccentsSQL("COALESCE(i.original_title, '')")+" LIKE ?)")
		like := "%" + foldAccents(q) + "%"
		args = append(args, like, like)
	}
	if f.Year != 0 {
		where = append(where, "i.year = ?")
		args = append(args, f.Year)
	}
	if g := strings.TrimSpace(f.Genre); g != "" {
		// genres es un JSON array; LIKE sobre el valor entrecomillado es
		// suficiente en MVP1 (géneros vienen de TMDB, vocabulario cerrado).
		where = append(where, "i.genres LIKE ?")
		args = append(args, `%"`+g+`"%`)
	}
	if f.Wished != nil {
		where = append(where, "i.wished = ?")
		args = append(args, boolInt(*f.Wished))
	}
	if f.Confirmed != nil {
		where = append(where, "i.confirmed = ?")
		args = append(args, boolInt(*f.Confirmed))
	}
	if f.ParentID != nil {
		where = append(where, "i.parent_id = ?")
		args = append(args, *f.ParentID)
	} else if f.TopLevelOnly {
		where = append(where, "i.parent_id IS NULL")
	}

	// InProgress necesita un JOIN (no cabe en la lista plana de condiciones
	// AND): sus argumentos van ANTES que los de `where`, porque su `?`
	// aparece antes en el SQL final (dentro del FROM).
	from := "items i"
	joinArgs := []any{}
	orderBy := "i.added_at DESC, i.id DESC"
	if f.InProgress {
		from = "items i INNER JOIN playback p ON p.item_id = i.id AND p.device_id = ?"
		joinArgs = append(joinArgs, f.DeviceID)
		where = append(where, "p.position_s > 0", "p.watched = 0")
		orderBy = "p.updated_at DESC, i.id DESC"
	}

	cond := strings.Join(where, " AND ")

	var total int
	countArgs := append(append([]any{}, joinArgs...), args...)
	if err := r.q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM `+from+` WHERE `+cond, countArgs...).Scan(&total); err != nil {
		return nil, err
	}

	query := `SELECT ` + itemCols + `,
		(SELECT COUNT(*) FROM sources s WHERE s.item_id = i.id) AS source_count
		FROM ` + from + ` WHERE ` + cond + `
		ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`
	queryArgs := append(append(append([]any{}, joinArgs...), args...), f.Limit, f.Offset)
	rows, err := r.q.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Item, 0, f.Limit)
	ids := make([]int64, 0, f.Limit)
	for rows.Next() {
		it, err := scanItem(rows, true)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
		ids = append(ids, it.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := r.attachHealthSummaries(ctx, items, ids); err != nil {
		return nil, err
	}
	if err := r.attachSeasonCounts(ctx, items); err != nil {
		return nil, err
	}

	return &ListResult{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}

// Get devuelve un item con todas sus fuentes ordenadas por prioridad.
func (r *Repo) Get(ctx context.Context, id int64) (*Item, error) {
	row := r.q.QueryRowContext(ctx, `SELECT `+itemCols+`,
		(SELECT COUNT(*) FROM sources s WHERE s.item_id = i.id)
		FROM items i WHERE i.id = ?`, id)
	it, err := scanItem(row, true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.q.QueryContext(ctx, `SELECT `+sourceCols+`
		FROM sources WHERE item_id = ? ORDER BY priority DESC, id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		it.Sources = append(it.Sources, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &it, nil
}

// InsertItem inserta un item y devuelve su ID. now en RFC3339; si vacío, ahora.
func (r *Repo) InsertItem(ctx context.Context, it *Item, now string) (int64, error) {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	if !ValidKind(string(it.Kind)) {
		return 0, fmt.Errorf("kind no válido: %q", it.Kind)
	}
	genres, err := json.Marshal(it.Genres)
	if err != nil {
		return 0, err
	}
	status := it.IdentifyStatus
	if status == "" {
		status = IdentifyPending
	}
	res, err := r.q.ExecContext(ctx, `INSERT INTO items
		(kind, parent_id, tmdb_id, title, original_title, year, overview,
		 poster_path, backdrop_path, genres, rating_cert, confirmed, wished,
		 added_at, updated_at, season_number, episode_number, identify_status,
		 identify_note)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		it.Kind, it.ParentID, it.TMDBID, it.Title, it.OriginalTitle, it.Year,
		it.Overview, it.PosterPath, it.BackdropPath, string(genres), it.RatingCert,
		boolInt(it.Confirmed), boolInt(it.Wished), now, now, it.SeasonNumber,
		it.EpisodeNumber, status, it.IdentifyNote)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertSource inserta una fuente para un item.
func (r *Repo) InsertSource(ctx context.Context, s *Source, now string) (int64, error) {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	if s.Health == "" {
		s.Health = HealthUnverified
	}
	res, err := r.q.ExecContext(ctx, `INSERT INTO sources
		(item_id, provider, locator, volume_id, quality_label, health, priority, added_at, origin_list_id, origin_path)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		s.ItemID, s.Provider, s.Locator, s.VolumeID, s.QualityLabel, s.Health,
		s.Priority, now, s.OriginListID, s.OriginPath)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CountItems devuelve el total de items (para saber si la BD está vacía).
func (r *Repo) CountItems(ctx context.Context) (int, error) {
	var n int
	err := r.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&n)
	return n, err
}

// attachHealthSummaries agrega {estado: nº fuentes} a cada item del listado.
func (r *Repo) attachHealthSummaries(ctx context.Context, items []Item, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.q.QueryContext(ctx, `SELECT item_id, health, COUNT(*)
		FROM sources WHERE item_id IN (`+ph+`) GROUP BY item_id, health`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	byItem := map[int64]map[string]int{}
	for rows.Next() {
		var itemID int64
		var health string
		var n int
		if err := rows.Scan(&itemID, &health, &n); err != nil {
			return err
		}
		if byItem[itemID] == nil {
			byItem[itemID] = map[string]int{}
		}
		byItem[itemID][health] = n
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range items {
		items[i].HealthSummary = byItem[items[i].ID]
	}
	return nil
}

// attachSeasonCounts rellena SeasonCount de las series del listado (las
// tarjetas de serie enseñan "N temporadas").
func (r *Repo) attachSeasonCounts(ctx context.Context, items []Item) error {
	var ids []any
	pos := map[int64]int{}
	for i, it := range items {
		if it.Kind == KindSeries {
			ids = append(ids, it.ID)
			pos[it.ID] = i
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	rows, err := r.q.QueryContext(ctx, `SELECT parent_id, COUNT(*) FROM items
		WHERE kind = 'season' AND parent_id IN (`+ph+`) GROUP BY parent_id`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var parent int64
		var n int
		if err := rows.Scan(&parent, &n); err != nil {
			return err
		}
		items[pos[parent]].SeasonCount = n
	}
	return rows.Err()
}

// scanner abstrae *sql.Row y *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanItem(s scanner, withCount bool) (Item, error) {
	var it Item
	var kind string
	var genres sql.NullString
	var confirmed, wished int
	dest := []any{&it.ID, &kind, &it.ParentID, &it.TMDBID, &it.Title,
		&it.OriginalTitle, &it.Year, &it.Overview, &it.PosterPath, &it.BackdropPath,
		&genres, &it.RatingCert, &confirmed, &wished, &it.AddedAt, &it.UpdatedAt,
		&it.SeasonNumber, &it.EpisodeNumber, &it.IdentifyStatus, &it.IdentifyNote}
	if withCount {
		dest = append(dest, &it.SourceCount)
	}
	if err := s.Scan(dest...); err != nil {
		return it, err
	}
	it.Kind = Kind(kind)
	it.Confirmed = confirmed != 0
	it.Wished = wished != 0
	it.Genres = []string{}
	if genres.Valid && genres.String != "" {
		if err := json.Unmarshal([]byte(genres.String), &it.Genres); err != nil {
			// genres corrupto no debe tumbar un listado; se deja vacío.
			it.Genres = []string{}
		}
	}
	return it, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
