package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// SourceByID devuelve una fuente por su id, o ErrNotFound.
func (r *Repo) SourceByID(ctx context.Context, id int64) (*Source, error) {
	row := r.q.QueryRowContext(ctx, `SELECT `+sourceCols+` FROM sources WHERE id = ?`, id)
	s, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// MediaInfo es el resultado persistido de sondear una fuente con ffprobe
// (Brief M4 §2; tabla media_info, migración 0003). Duplicado a propósito del
// media.MediaInfo del paquete internal/media: catalog no depende de media,
// igual que ya no depende de identify para sus Candidate (mismo patrón).
type MediaInfo struct {
	SourceID   int64   `json:"source_id"`
	Container  string  `json:"container,omitempty"`
	DurationS  float64 `json:"duration_s,omitempty"`
	VideoCodec string  `json:"video_codec,omitempty"`
	AudioCodec string  `json:"audio_codec,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	Bitrate    int64   `json:"bitrate,omitempty"`
	ProbedAt   string  `json:"probed_at"`
}

// UpsertMediaInfo guarda (o reemplaza) el resultado de sondear una fuente.
func (r *Repo) UpsertMediaInfo(ctx context.Context, mi MediaInfo) error {
	if mi.ProbedAt == "" {
		mi.ProbedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := r.q.ExecContext(ctx, `INSERT INTO media_info
		(source_id, container, duration_s, video_codec, audio_codec, width, height, bitrate, probed_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source_id) DO UPDATE SET
			container = excluded.container, duration_s = excluded.duration_s,
			video_codec = excluded.video_codec, audio_codec = excluded.audio_codec,
			width = excluded.width, height = excluded.height, bitrate = excluded.bitrate,
			probed_at = excluded.probed_at`,
		mi.SourceID, mi.Container, mi.DurationS, mi.VideoCodec, mi.AudioCodec,
		mi.Width, mi.Height, mi.Bitrate, mi.ProbedAt)
	return err
}

// GetMediaInfo devuelve el resultado de sondeo de una fuente, si existe.
func (r *Repo) GetMediaInfo(ctx context.Context, sourceID int64) (*MediaInfo, bool, error) {
	row := r.q.QueryRowContext(ctx, `SELECT source_id, container, duration_s, video_codec,
		audio_codec, width, height, bitrate, probed_at FROM media_info WHERE source_id = ?`, sourceID)
	var mi MediaInfo
	err := row.Scan(&mi.SourceID, &mi.Container, &mi.DurationS, &mi.VideoCodec,
		&mi.AudioCodec, &mi.Width, &mi.Height, &mi.Bitrate, &mi.ProbedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &mi, true, nil
}

// SourcesWithoutMediaInfo devuelve las fuentes de provider sin sondear
// todavía (Brief M4 §2: sondeo en background tras el escaneo de M2).
func (r *Repo) SourcesWithoutMediaInfo(ctx context.Context, provider string) ([]Source, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+sourceCols+` FROM sources s
		WHERE s.provider = ? AND NOT EXISTS (SELECT 1 FROM media_info m WHERE m.source_id = s.id)
		ORDER BY s.added_at ASC`, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetPlayback devuelve el progreso de un item para un dispositivo, si existe
// (Brief M4 §5).
func (r *Repo) GetPlayback(ctx context.Context, itemID int64, deviceID string) (*Progress, bool, error) {
	row := r.q.QueryRowContext(ctx, `SELECT position_s, duration_s, watched, updated_at
		FROM playback WHERE item_id = ? AND device_id = ?`, itemID, deviceID)
	var p Progress
	var watched int
	err := row.Scan(&p.PositionS, &p.DurationS, &watched, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	p.Watched = watched != 0
	return &p, true, nil
}

// UpsertPlayback guarda el progreso de reproducción de un item para un
// dispositivo (Brief M4 §5; PUT /api/v1/playback/{item_id}). watched lo
// decide la capa de API (umbral del 92%, regla de producto, no de
// persistencia).
func (r *Repo) UpsertPlayback(ctx context.Context, itemID int64, deviceID string,
	positionS float64, durationS *float64, watched bool, now string) error {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := r.q.ExecContext(ctx, `INSERT INTO playback
		(item_id, device_id, position_s, duration_s, watched, updated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(item_id, device_id) DO UPDATE SET
			position_s = excluded.position_s, duration_s = excluded.duration_s,
			watched = excluded.watched, updated_at = excluded.updated_at`,
		itemID, deviceID, positionS, durationS, boolInt(watched), now)
	return err
}

// AttachProgress rellena Item.Progress para cada item de items, del
// dispositivo deviceID, cuando exista (Brief M4 §5: extender /items y
// /items/{id} con "progress"). deviceID vacío es un no-op (sin cookie
// gala_device no hay "Continuar viendo" que mostrar).
func (r *Repo) AttachProgress(ctx context.Context, items []Item, deviceID string) error {
	if deviceID == "" || len(items) == 0 {
		return nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(items)), ",")
	args := make([]any, 0, len(items)+1)
	args = append(args, deviceID)
	for i := range items {
		args = append(args, items[i].ID)
	}
	rows, err := r.q.QueryContext(ctx, `SELECT item_id, position_s, duration_s, watched, updated_at
		FROM playback WHERE device_id = ? AND item_id IN (`+ph+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	byItem := map[int64]*Progress{}
	for rows.Next() {
		var itemID int64
		var p Progress
		var watched int
		if err := rows.Scan(&itemID, &p.PositionS, &p.DurationS, &watched, &p.UpdatedAt); err != nil {
			return err
		}
		p.Watched = watched != 0
		byItem[itemID] = &p
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range items {
		items[i].Progress = byItem[items[i].ID]
	}
	return nil
}
