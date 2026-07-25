package catalog

import "context"

// UpdateSourceHealth guarda el resultado de un chequeo de salud: estado,
// detalle legible (motivo, para la UI) y momento de la verificación (Brief
// M6 §3). detail vacío se guarda como NULL (sin motivo que mostrar, p. ej.
// el caso "viva" no necesita explicación).
func (r *Repo) UpdateSourceHealth(ctx context.Context, id int64, health, detail, now string) error {
	var d *string
	if detail != "" {
		d = &detail
	}
	_, err := r.q.ExecContext(ctx, `UPDATE sources SET health = ?, health_detail = ?, last_checked = ? WHERE id = ?`,
		health, d, now, id)
	return err
}

// SourcesDueForHealthCheck devuelve las fuentes a las que toca verificar
// (Brief M6 §3, "presupuesto": nunca re-verificar antes de tiempo salvo que
// se pida explícitamente). cutoff vacío = todas, ignorando la ventana
// mínima (re-chequeo periódico de 24h o bajo demanda vía POST
// /health/check). cutoff no vacío = solo las nunca verificadas o vistas
// antes de ese momento. Las nunca verificadas van primero (altas recientes
// de un escaneo/ingesta merecen su primer chequeo cuanto antes).
func (r *Repo) SourcesDueForHealthCheck(ctx context.Context, cutoff string) ([]Source, error) {
	query := `SELECT ` + sourceCols + ` FROM sources`
	var args []any
	if cutoff != "" {
		query += ` WHERE last_checked IS NULL OR last_checked < ?`
		args = append(args, cutoff)
	}
	query += ` ORDER BY (last_checked IS NULL) DESC, last_checked ASC`

	rows, err := r.q.QueryContext(ctx, query, args...)
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

// HealthCounts agrega cuántas fuentes hay en cada estado de salud, en TODO
// el catálogo (a diferencia de attachHealthSummaries, que es por item) —
// base de GET /api/v1/health/summary (Brief M6 §5).
func (r *Repo) HealthCounts(ctx context.Context) (map[string]int, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT health, COUNT(*) FROM sources GROUP BY health`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var health string
		var n int
		if err := rows.Scan(&health, &n); err != nil {
			return nil, err
		}
		out[health] = n
	}
	return out, rows.Err()
}
