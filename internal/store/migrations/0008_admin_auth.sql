-- Autenticación de administrador (M-Servidor). Un único admin.
CREATE TABLE admin_auth (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  password_hash TEXT NOT NULL,                 -- $argon2id$v=19$m=65536,t=2,p=1$sal$hash
  network_admin INTEGER NOT NULL DEFAULT 1,    -- «Exigir clave para administrar desde la red»
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- id_hash = SHA-256 del token de la cookie; el token nunca se guarda.
CREATE TABLE admin_sessions (
  id_hash TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  last_seen TEXT NOT NULL,
  user_agent TEXT NOT NULL,
  ip TEXT NOT NULL
);
CREATE INDEX idx_admin_sessions_expires ON admin_sessions(expires_at);

-- Secreto del servidor (deriva el token CSRF). Nunca sale por la API.
CREATE TABLE server_secret (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  secret BLOB NOT NULL
);
