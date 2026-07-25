package catalog

import (
	"context"
	"time"
)

// IngestList es una lista/ingesta registrada (tabla ingest_lists, Brief M5
// §4): trazabilidad de qué se pegó/subió, y base para una futura
// re-sincronización de M3U remotos (fuera de alcance de M5: solo se guarda
// source_url para entonces).
type IngestList struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"` // "text","m3u","csv","m3u_remoto"...
	SourceURL *string `json:"source_url,omitempty"`
	LastSync  *string `json:"last_sync,omitempty"`
	ItemCount int     `json:"item_count"`
}

// InsertIngestList registra una ingesta y devuelve su ID (para
// sources.origin_list_id).
func (r *Repo) InsertIngestList(ctx context.Context, l *IngestList, now string) (int64, error) {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := r.q.ExecContext(ctx, `INSERT INTO ingest_lists
		(name, type, source_url, last_sync, item_count) VALUES (?,?,?,?,?)`,
		l.Name, l.Type, l.SourceURL, now, l.ItemCount)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListIngestLists devuelve las ingestas registradas, más recientes primero
// (GET /api/v1/ingest/lists, Brief M5 §5).
func (r *Repo) ListIngestLists(ctx context.Context) ([]IngestList, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT id, name, type, source_url, last_sync, item_count
		FROM ingest_lists ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []IngestList{}
	for rows.Next() {
		var l IngestList
		if err := rows.Scan(&l.ID, &l.Name, &l.Type, &l.SourceURL, &l.LastSync, &l.ItemCount); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
