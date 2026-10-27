-- GALA · Soporte de archivos .strm
--
-- Un .strm es un puntero (texto con una URL), no un vídeo. Su fuente es de
-- tipo http (locator = la URL), pero el nombre de archivo y la carpeta del
-- .strm siguen siendo lo que identifica la obra (M3) y lo que enlaza la
-- fuente con el disco para reconciliar altas y bajas en el escaneo.
-- origin_path guarda la ruta del .strm; NULL para toda fuente que no venga
-- de uno (localfs, ingesta, etc.).
ALTER TABLE sources ADD COLUMN origin_path TEXT;
CREATE INDEX idx_sources_origin_path ON sources(origin_path) WHERE origin_path IS NOT NULL;
