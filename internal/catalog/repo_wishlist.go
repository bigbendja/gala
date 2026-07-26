package catalog

import (
	"context"
	"time"
)

// SetWished pone o quita el flag wished de un item (Brief M10a-fix B2,
// anticipo mínimo de M8: solo alta/baja sobre un item YA existente en el
// catálogo — la búsqueda-para-desear un título SIN fuente todavía, contra
// TMDB, es el resto de M8 y no entra aquí). Devuelve ErrNotFound si el item
// no existe.
func (r *Repo) SetWished(ctx context.Context, id int64, wished bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.q.ExecContext(ctx, `UPDATE items SET wished = ?, updated_at = ? WHERE id = ?`,
		boolInt(wished), now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
