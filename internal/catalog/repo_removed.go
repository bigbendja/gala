package catalog

import (
	"context"
	"database/sql"
	"errors"
)

// RememberRemovedSource anota que la fuente (provider, key) pertenecía a
// itemID justo antes de retirarla, para que un escaneo posterior que vuelva a
// ver ese fichero reutilice el item en vez de duplicarlo. key es la ruta ya
// normalizada por el provider (para localfs, localfs.NormalizePath).
func (r *Repo) RememberRemovedSource(ctx context.Context, provider, key string, itemID int64, now string) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO removed_sources (provider, locator_key, item_id, removed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider, locator_key) DO UPDATE SET item_id = excluded.item_id, removed_at = excluded.removed_at`,
		provider, key, itemID, now)
	return err
}

// ClaimRemovedSource devuelve el item al que pertenecía la fuente (provider,
// key) y consume el rastro. ok=false si no hay rastro (fichero realmente
// nuevo, o su item fue descartado a propósito).
func (r *Repo) ClaimRemovedSource(ctx context.Context, provider, key string) (itemID int64, ok bool, err error) {
	err = r.q.QueryRowContext(ctx, `SELECT item_id FROM removed_sources
		WHERE provider = ? AND locator_key = ?`, provider, key).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if _, err = r.q.ExecContext(ctx, `DELETE FROM removed_sources
		WHERE provider = ? AND locator_key = ?`, provider, key); err != nil {
		return 0, false, err
	}
	return itemID, true, nil
}
