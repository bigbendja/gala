package catalog

import (
	"context"
	"database/sql"
	"errors"
)

const sourceCols = `id, item_id, provider, locator, volume_id, quality_label,
	health, last_checked, priority, added_at, origin_list_id, health_detail, origin_path`

// SourceByLocator devuelve la fuente con ese provider+locator, o ErrNotFound.
// Base de la idempotencia del escaneo localfs (UNIQUE(provider, locator)).
func (r *Repo) SourceByLocator(ctx context.Context, provider, locator string) (*Source, error) {
	row := r.q.QueryRowContext(ctx, `SELECT `+sourceCols+`
		FROM sources WHERE provider = ? AND locator = ?`, provider, locator)
	s, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// TouchSource actualiza la salud y el momento de verificación de una fuente
// ya existente (re-escaneo de un fichero que sigue ahí).
func (r *Repo) TouchSource(ctx context.Context, id int64, health, now string) error {
	_, err := r.q.ExecContext(ctx, `UPDATE sources SET health = ?, last_checked = ? WHERE id = ?`,
		health, now, id)
	return err
}

// DeleteSource borra una fuente por id. El item asociado se conserva
// (catálogo y fuentes son independientes; ver Brief M2).
func (r *Repo) DeleteSource(ctx context.Context, id int64) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM sources WHERE id = ?`, id)
	return err
}

// SourcesByProvider devuelve todas las fuentes de un provider dado (p. ej.
// "localfs"), usado para reconciliar el escaneo con el estado de la BD.
func (r *Repo) SourcesByProvider(ctx context.Context, provider string) ([]Source, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+sourceCols+` FROM sources WHERE provider = ?`, provider)
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

// SetSourcesHealthByVolume actualiza la salud de todas las fuentes de un
// provider+volumen (desconexión/reconexión de un disco).
func (r *Repo) SetSourcesHealthByVolume(ctx context.Context, provider, volumeID, health string) error {
	_, err := r.q.ExecContext(ctx, `UPDATE sources SET health = ? WHERE provider = ? AND volume_id = ?`,
		health, provider, volumeID)
	return err
}

// UpsertVolume inserta o actualiza un volumen conocido. now es el last_seen.
func (r *Repo) UpsertVolume(ctx context.Context, v Volume, now string) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO volumes (id, label, last_seen, connected)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			label = excluded.label, last_seen = excluded.last_seen, connected = excluded.connected`,
		v.ID, v.Label, now, boolInt(v.Connected))
	return err
}

// SetVolumeConnected marca un volumen conocido como conectado/desconectado.
func (r *Repo) SetVolumeConnected(ctx context.Context, id string, connected bool, now string) error {
	_, err := r.q.ExecContext(ctx, `UPDATE volumes SET connected = ?, last_seen = ? WHERE id = ?`,
		boolInt(connected), now, id)
	return err
}

// ListVolumes devuelve los volúmenes conocidos, más recientes primero.
func (r *Repo) ListVolumes(ctx context.Context) ([]Volume, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT id, label, last_seen, connected
		FROM volumes ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Volume{}
	for rows.Next() {
		var v Volume
		var connected int
		if err := rows.Scan(&v.ID, &v.Label, &v.LastSeen, &connected); err != nil {
			return nil, err
		}
		v.Connected = connected != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanSource(s scanner) (Source, error) {
	var src Source
	err := s.Scan(&src.ID, &src.ItemID, &src.Provider, &src.Locator, &src.VolumeID,
		&src.QualityLabel, &src.Health, &src.LastChecked, &src.Priority, &src.AddedAt,
		&src.OriginListID, &src.HealthDetail, &src.OriginPath)
	return src, err
}

// VolumeConnected indica si el volumen id está conectado ahora mismo. Un
// volumen desconocido (nunca visto por el poller) se asume conectado: no hay
// evidencia de lo contrario, y negarlo por defecto marcaría "desconectada"
// una fuente cuyo volumen simplemente no se ha enumerado todavía (Brief M6
// §2, checker localfs).
func (r *Repo) VolumeConnected(ctx context.Context, id string) (bool, error) {
	var connected int
	err := r.q.QueryRowContext(ctx, `SELECT connected FROM volumes WHERE id = ?`, id).Scan(&connected)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return connected != 0, nil
}
