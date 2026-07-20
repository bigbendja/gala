-- GALA · esquema inicial (Especificación MVP1 §4)

CREATE TABLE items (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('movie','series','season','episode','other')),
  parent_id INTEGER REFERENCES items(id),
  tmdb_id INTEGER,
  title TEXT NOT NULL,
  original_title TEXT,
  year INTEGER,
  overview TEXT,
  poster_path TEXT,
  backdrop_path TEXT,
  genres TEXT,
  rating_cert TEXT,
  confirmed INTEGER NOT NULL DEFAULT 1,
  wished INTEGER NOT NULL DEFAULT 0,
  added_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX idx_items_kind ON items(kind);
CREATE INDEX idx_items_tmdb ON items(tmdb_id);
CREATE INDEX idx_items_parent ON items(parent_id);

CREATE TABLE sources (
  id INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  locator TEXT NOT NULL,
  volume_id TEXT,
  quality_label TEXT,
  health TEXT NOT NULL DEFAULT 'sin_verificar',
  last_checked TEXT,
  priority INTEGER NOT NULL DEFAULT 0,
  origin_list_id INTEGER REFERENCES ingest_lists(id),
  added_at TEXT NOT NULL,
  UNIQUE(provider, locator)
);
CREATE INDEX idx_sources_item ON sources(item_id);
CREATE INDEX idx_sources_volume ON sources(volume_id);

CREATE TABLE ingest_lists (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  source_url TEXT,
  last_sync TEXT,
  item_count INTEGER
);

CREATE TABLE volumes (
  id TEXT PRIMARY KEY,
  label TEXT,
  last_seen TEXT,
  connected INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE playback (
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL,
  position_s REAL NOT NULL,
  duration_s REAL,
  watched INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (item_id, device_id)
);

CREATE TABLE tmdb_cache (
  tmdb_id INTEGER,
  kind TEXT,
  lang TEXT,
  payload TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (tmdb_id, kind, lang)
);
