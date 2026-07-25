-- GALA · M6 Salud de fuentes (Especificación MVP1 §3.5)

-- Detalle legible del último chequeo (motivo de "caída"/"lenta", o
-- "pendiente de conector" para drive/mega): sources.health ya existe desde
-- 0001_init.sql, pero es solo el estado; la UI necesita el porqué.
ALTER TABLE sources ADD COLUMN health_detail TEXT;

-- El worker de salud consulta constantemente "qué fuentes tocan verificar"
-- (nunca verificadas primero, luego las más antiguas) — acelera esa
-- consulta en catálogos grandes.
CREATE INDEX idx_sources_last_checked ON sources(last_checked);
