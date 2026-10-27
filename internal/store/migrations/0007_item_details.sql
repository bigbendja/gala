-- GALA · Ficha enriquecida (Fase 1, Parte B)
--
-- Datos extra que TMDB da de una película o serie y que la ficha de detalle
-- enseña: duración, nota, país, reparto, equipo clave y títulos similares.
-- Una fila por item identificado (movie/series). Se rellena en segundo plano
-- (cola de M3) a partir de UNA petición con append_to_response, y las fotos
-- del reparto y las carátulas de los similares se descargan a cache/img/ con
-- el mismo patrón de reintento que las carátulas.
--
-- cast_json    = [{"id","name","character","profile_path","photo"}]
-- crew_json    = [{"role","names":[...]}]   (Dirección, Guion, Música, Fotografía…)
-- similar_json = [{"tmdb_id","kind","title","year","poster_path","poster"}]
-- "photo"/"poster" son el nombre del fichero en cache/img/ (vacío = aún sin
-- descargar; la web enseña iniciales o un respaldo, nunca un hueco roto).
CREATE TABLE item_details (
  item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
  tmdb_kind TEXT NOT NULL,
  runtime_min INTEGER,
  vote_average REAL,
  original_language TEXT,
  countries TEXT NOT NULL DEFAULT '[]',
  tagline TEXT,
  cast_json TEXT NOT NULL DEFAULT '[]',
  crew_json TEXT NOT NULL DEFAULT '[]',
  similar_json TEXT NOT NULL DEFAULT '[]',
  images_pending INTEGER NOT NULL DEFAULT 0,
  image_attempts INTEGER NOT NULL DEFAULT 0,
  fetched_at TEXT NOT NULL
);
CREATE INDEX idx_item_details_pending ON item_details(images_pending) WHERE images_pending = 1;
