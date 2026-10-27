-- GALA · M7 Reconciliación de fuentes locales retiradas (fix "carpeta duplicada")
--
-- Al quitar una carpeta registrada (o al desaparecer un fichero), la fuente
-- se borra pero el item se conserva. Sin más información, volver a añadir la
-- carpeta no podía saber a qué item pertenecía cada fichero y creaba items
-- nuevos, dejando los originales como fantasmas de 0 fuentes.
--
-- Esta tabla guarda ese rastro: qué item tenía el fichero `locator_key`
-- (ruta normalizada) cuando se retiró. El próximo escaneo que lo vea de
-- nuevo reutiliza ese item y borra el rastro. Si el item se borra a
-- propósito (DELETE /items/{id}), el rastro cae por ON DELETE CASCADE y el
-- fichero se da de alta como item nuevo.
CREATE TABLE removed_sources (
  provider TEXT NOT NULL,
  locator_key TEXT NOT NULL,
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  removed_at TEXT NOT NULL,
  PRIMARY KEY (provider, locator_key)
);
CREATE INDEX idx_removed_sources_item ON removed_sources(item_id);
