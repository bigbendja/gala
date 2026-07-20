-- GALA · M3 Identificación (Especificación MVP1 §3.3, §3.7)

-- Jerarquía serie→temporada→episodio: los items 'season'/'episode' necesitan
-- su número; las películas y series no los usan (quedan NULL).
ALTER TABLE items ADD COLUMN season_number INTEGER;
ALTER TABLE items ADD COLUMN episode_number INTEGER;

-- Estado del pipeline de identificación de cada item provisional (M2 los crea
-- con confirmed=0, identify_status='pending' por defecto):
--   pending   → aún no procesado por la cola.
--   junk      → basura detectada por el normalizador; nunca se reintenta TMDB.
--   review    → TMDB consultado; candidatos (o ninguno) en identify_candidates,
--               a la espera de confirmación manual. Estado terminal salvo
--               confirmación del usuario.
--   error     → fallo de red/TMDB durante el intento; la cola lo reintenta.
--   ok        → identificado y confirmado (confirmed=1); se conserva el valor
--               por trazabilidad aunque ya no forme parte de "Por confirmar".
ALTER TABLE items ADD COLUMN identify_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE items ADD COLUMN identify_candidates TEXT;
ALTER TABLE items ADD COLUMN identify_note TEXT;

CREATE INDEX idx_items_identify_status ON items(identify_status);

-- Caché de búsquedas TMDB (search/movie, search/tv): tmdb_cache guarda
-- respuestas de DETALLE (ya con tmdb_id conocido); las búsquedas se indexan
-- por hash de la query porque aún no hay tmdb_id. Tabla propia en vez de
-- forzar tmdb_cache con tmdb_id=hash: mantiene tmdb_cache limpia (su clave
-- primaria sigue siendo un id real de TMDB) — decisión documentada en
-- internal/identify/tmdb.go.
CREATE TABLE search_cache (
  query_hash TEXT NOT NULL,
  kind TEXT NOT NULL,
  lang TEXT NOT NULL,
  payload TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (query_hash, kind, lang)
);
