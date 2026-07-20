# Brief M3 — Identificación (normalizador + TMDB + caché offline)
**Para: Claude Code · Contexto: CLAUDE.md + docs/GALA_Especificacion_MVP1.md (§3.3, §3.7) + plan (M3) + testdata/corpus_nombres.txt**

## Objetivo
Convertir los items provisionales de M2 (nombre de archivo crudo, confirmed=0) en
entradas de catálogo identificadas y enriquecidas (título, año, carátula, sinopsis,
género, clasificación) vía TMDB, con caché offline total y cola "Por confirmar"
para lo dudoso. Criterio del plan: el corpus se identifica con precisión alta y lo
dudoso cae en "Por confirmar" — **nunca falsos positivos silenciosos**. Este módulo
decide si GALA "parece Netflix"; ante la duda entre identificar mal o preguntar,
SIEMPRE preguntar (Por confirmar).

## Alcance
SÍ: normalizador de nombres, cliente TMDB, matcher con umbrales, cola de
enriquecimiento asíncrona, caché de metadatos e imágenes, dedupe por tmdb_id,
endpoints de confirmación, integración con el escaneo de M2, tests contra corpus.
NO (ISSUES-MVP2.md si surge): identificación por hash/huella del archivo, subtítulos,
detección de duración/códecs (M4), edición completa de metadatos, re-identificación
masiva con nuevas versiones del normalizador (anotar la idea).

## 1. Normalizador — `internal/identify/normalize.go`
Entrada: nombre de archivo + nombre de carpeta contenedora (puede ser vacío).
Salida:
```go
type Parsed struct {
    Title    string   // limpio, espacios, sin tokens técnicos
    Year     int      // 0 si no consta
    Kind     string   // "movie" | "series_episode" | "unknown"
    Season   int      // 0 si no aplica
    Episode  int      // 0 si no aplica
    AbsoluteEp int    // numeración absoluta (anime), 0 si no aplica
    Quality  string   // "1080p", "720p", "2160p", "" ...
    Langs    []string // pistas: "es","lat","fr","en","dual","multi"
    Junk     bool     // true = no intentar TMDB (va directo a Por confirmar)
    Extra    bool     // sample/trailer/making-of: excluir del flujo normal
}
```
Pipeline de limpieza (orden importa; cada regla con tests propios):
1. **Junk primero**: patrones `VID-\d{8}-WA\d+`, `WhatsApp Video`, `video_\d{4}-`,
   `\d{8}_\d{6}`, `IMG_\d+`, "descarga", "video sin titulo" y similares → Junk=true,
   fin. También títulos resultantes de <2 caracteres alfabéticos tras limpiar.
2. **Extra**: contiene token aislado `sample`, `trailer`, `teaser`, `making.of`,
   o el archivo se llama exactamente sample/RARBG → Extra=true.
3. Quitar prefijos web `[www.*]`, `[Torrent9]` y cualquier `[...]`/`(...)` cuyo
   contenido sea puramente técnico (grupo, web, hash, calidad); conservar paréntesis
   de año.
4. Detección de serie, por prioridad: `S(\d{1,2})E(\d{1,3})` → `(\d{1,2})x(\d{2,3})`
   → español `Cap.(\d{3,4})` (los 1-2 primeros dígitos = temporada, los 2 últimos =
   episodio; Cap.105→T1E05, Cap.1204→T12E04) → numeración absoluta anime
   (`- (\d{2,4})` tras título con grupo entre corchetes) → AbsoluteEp.
5. **Herencia de carpeta**: si el archivo por sí solo no produce título útil
   (E01.mkv, Capitulo 01.avi, merli_301.mp4) pero sí produce temporada/episodio,
   tomar el título (y temporada si consta: "Temporada 2", "T1", "Season 4") del
   nombre de la carpeta, aplicándole el mismo pipeline. `merli_301.mp4` +
   carpeta "Merli Temporada 3": el 301 se interpreta como Cap → T3E01, coherente
   con la temporada de la carpeta; si contradice, manda la carpeta y se marca menor
   confianza (campo interno, ver matcher).
6. Año: último token `(19[2-9]\d|20[0-4]\d)` que NO sea la primera palabra del
   título; si el título completo ES un año ("2012", "1917") solo hay año si aparece
   un SEGUNDO token de año. "Blade Runner 2049 2017" → título "Blade Runner 2049",
   año 2017 (el último gana; el anterior queda en el título).
7. Tokens técnicos a eliminar (lista exportada, insensible a mayúsculas):
   resoluciones (2160p/1080p/720p/480p/4K/UHD/HD/FULL HD), fuentes (BluRay/BRRip/
   BDRip/WEB-DL/WEBRip/HDTV/DVDRip/HDRip/CAM/TS/NF/AMZN/MA), códecs (x264/x265/
   H264/H.264/HEVC/XviD/10bit/HDR), audio (AAC/AC3/DTS/DDP5.1/DD5.1/Atmos/5.1),
   ediciones (REMASTERED/EXTENDED/UNRATED/COMPLETE/PROPER/REPACK), tamaños
   (\d+MB/GB), grupo final tras guion (`-SPARKS`), `YTS.*`/`YIFY`/`RARBG`,
   "pelicula completa", "download", "(copia)", "(1)".
8. Idiomas → Langs (y fuera del título): Castellano/Español/Spanish/ESP→es,
   Latino→lat, FRENCH/VOSTFR/TRUEFRENCH→fr, GERMAN/KOREAN→(código), DUAL/MULTi→dual/multi.
9. Normalización final: puntos/guiones bajos→espacios, colapsar espacios, conservar
   guiones internos de título (Spider-Man, WALL-E) y apóstrofes (Ocean's).

## 2. Cliente TMDB — `internal/identify/tmdb.go`
- HTTP estándar, sin SDK de terceros. Base api.themoviedb.org/3.
- API key: leer de `config` campo nuevo `tmdb_api_key`; si vacío, usar la constante
  `DefaultTMDBKey` de un archivo `internal/identify/apikey.go` (valor placeholder
  "PON-AQUI-LA-KEY"; el titular la rellenará — no inventar ni buscar keys).
- Endpoints: /search/movie (query+year+language), /search/tv, /movie/{id},
  /tv/{id}, imágenes via image.tmdb.org (poster w342, backdrop w780).
- language=es-ES con fallback en-US si overview vacío.
- Rate limit propio: máx 4 req/s, reintentos con backoff (3 intentos) en 429/5xx/red.
- TODA respuesta de detalle se guarda en `tmdb_cache` (payload JSON crudo); las
  búsquedas también se cachean con clave derivada (kind='search', tmdb_id=hash del
  query — o añadir migración con tabla search_cache propia si queda más limpio;
  decisión libre, documentada).
- Imágenes: descargar a `cache/img/` con nombre determinista; `items.poster_path`
  guarda la ruta LOCAL. Servirlas en `GET /api/v1/img/{archivo}` (validar nombre,
  sin path traversal).
- Sin red: toda función distingue "no hay resultado" de "no hay conexión"
  (error tipado `ErrOffline`).

## 3. Matcher — `internal/identify/match.go`
- Buscar según Parsed.Kind (movie / tv; unknown prueba ambos).
- Puntuación por candidato: similitud de título (comparación normalizada:
  minúsculas, sin acentos, sin signos; usar distancia tipo Levenshtein/Jaro
  implementada en el paquete, sin dependencia nueva) sobre title Y original_title;
  bonus por año exacto (±1 tolerado con menos bonus); desempate por popularidad.
- Umbrales (constantes con nombre, ajustables): ALTA → auto-confirmar (confirmed=1);
  MEDIA → Por confirmar con lista de hasta 5 candidatos; BAJA/sin resultados →
  Por confirmar sin candidatos. La herencia-con-contradicción de carpeta (§1.5)
  degrada un nivel el resultado.
- Series: identificar la SERIE (tmdb_id de la serie). Estructura jerárquica
  serie→temporada→episodio en items (parent_id) creada al identificar; el archivo
  cuelga como source del item episodio. Metadatos de episodio individuales:
  suficiente con número y nombre de temporada/episodio si vienen en la respuesta
  de detalle de la serie; NO llamar al endpoint por-episodio en M3 (coste de red;
  anotar mejora en ISSUES-MVP2).
- **Dedupe**: si ya existe item con ese tmdb_id (y mismo kind/temporada/episodio),
  mover las sources del provisional al existente y borrar el provisional.

## 4. Cola de enriquecimiento — `internal/identify/queue.go`
- Goroutine con canal alimentado por: fin de escaneo de M2 (todos los confirmed=0
  no-Junk no-Extra), arranque de la app (pendientes de sesiones anteriores), y
  confirmaciones manuales.
- Junk → queda confirmed=0 sin candidatos (la UI los agrupa en Por confirmar).
  Extra → source marcada, item no creado o eliminado si provisional (decisión:
  los extras NO aparecen en la galería; documentar en código).
- Offline: los pendientes persisten (confirmed=0 ya lo representa); al recuperar
  red (reintento periódico cada 5 min con jitter), la cola drena sola.
- Respetar context de apagado; no perder trabajo (la cola se reconstruye de BD,
  no de memoria).

## 5. API
- `GET /api/v1/pending` → items Por confirmar con sus candidatos (si los hay).
- `POST /api/v1/items/{id}/confirm` body `{"tmdb_id": 123, "kind": "movie"}` →
  aplica identificación completa (detalle+imágenes+dedupe).
- `POST /api/v1/items/{id}/confirm` body `{"title": "Lo que sea"}` → confirmación
  manual sin TMDB (título editado por el usuario, confirmed=1, sin metadatos).
- `GET /api/v1/identify/status` → tamaño de cola, en curso, errores recientes, online/offline.

## 6. Tests (listón de M3)
- **Test de corpus**: tabla en `internal/identify/normalize_test.go` que recorre
  testdata/corpus_nombres.txt. Para ~50 líneas representativas (elegidas cubriendo
  TODAS las familias del corpus, incluidas todas las trampas de "Casos límite"),
  crear `testdata/corpus_expected.tsv` (linea → Title/Year/Kind/Season/Episode/
  Junk/Extra esperados) y verificar exactitud total. Para el resto del corpus:
  verificación de invariantes (no panics, Junk detecta la familia de basura completa,
  ningún Extra ni Junk produce Kind útil).
- Matcher con TMDB simulado (fixtures JSON en testdata/tmdb/): caso alta confianza,
  caso candidatos múltiples, caso sin resultados, caso año ±1, caso dedupe, caso serie
  con jerarquía.
- Cliente TMDB: rate limit y backoff con servidor httptest; ErrOffline.
- Cola: drenado tras "recuperar" red simulada; reconstrucción desde BD al arrancar.
- E2E ligero: escaneo M2 sobre árbol temporal con nombres del corpus + TMDB fake →
  items confirmados/pendientes como se espera.

## 7. Al terminar
- go test ./..., gofmt, vet limpios. Commit. Estado en CLAUDE.md.
- Informe: tasa de identificación sobre el corpus (identificados/por-confirmar/junk),
  decisiones dentro del margen, dudas anotadas sin ampliar alcance.
- NO tocar el frontend placeholder: la UI de Por confirmar llega en M10.
