package catalog

import (
	"context"
	"database/sql"
	"errors"
)

// SourceByOriginPath devuelve la fuente creada a partir del .strm en esa ruta
// exacta, o ErrNotFound. Base de la idempotencia del escaneo de .strm.
func (r *Repo) SourceByOriginPath(ctx context.Context, path string) (*Source, error) {
	row := r.q.QueryRowContext(ctx, `SELECT `+sourceCols+`
		FROM sources WHERE origin_path = ?`, path)
	s, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SourcesWithOriginPath devuelve todas las fuentes nacidas de un .strm, para
// reconciliar el escaneo con el estado del disco.
func (r *Repo) SourcesWithOriginPath(ctx context.Context) ([]Source, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT `+sourceCols+` FROM sources WHERE origin_path IS NOT NULL`)
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

// UpdateSourceLocator cambia la URL de una fuente (el .strm se reescribió
// con otro enlace) y la devuelve a "sin verificar" para que M6 la compruebe.
func (r *Repo) UpdateSourceLocator(ctx context.Context, id int64, locator string) error {
	_, err := r.q.ExecContext(ctx, `UPDATE sources
		SET locator = ?, health = ?, health_detail = NULL, last_checked = NULL WHERE id = ?`,
		locator, HealthUnverified, id)
	return err
}
