-- GALA · M4 Streaming y reproducción (Especificación MVP1 §3.4)

-- Resultado de sondear una fuente con ffprobe: contenedor/códecs reales,
-- para decidir direct/remux/reject sin tener que invocar ffprobe en cada
-- petición de play (Brief M4 §2). 1:1 con sources; se borra en cascada si la
-- fuente desaparece (fichero borrado del disco).
CREATE TABLE media_info (
  source_id INTEGER PRIMARY KEY REFERENCES sources(id) ON DELETE CASCADE,
  container TEXT,
  duration_s REAL,
  video_codec TEXT,
  audio_codec TEXT,
  width INTEGER,
  height INTEGER,
  bitrate INTEGER,
  probed_at TEXT NOT NULL
);
