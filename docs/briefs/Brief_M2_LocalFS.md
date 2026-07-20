# Brief M2 — Provider LocalFS
**Para: Claude Code · Contexto: CLAUDE.md + docs/GALA_Especificacion_MVP1.md (§3.2, §3.5) + docs/GALA_Plan_Desarrollo_MVP1.md (M2)**

## Objetivo
Que GALA escanee carpetas y discos locales y los convierta en fuentes del catálogo,
con detección de conexión/desconexión de volúmenes USB. Criterio de "hecho" del plan:
*enchufar un USB con vídeos crea/actualiza fuentes automáticamente y desenchufarlo
las marca sin borrarlas*.

## Alcance
SÍ: interfaz Provider, provider localfs, gestión de carpetas registradas, identificación
de volúmenes, poller de volúmenes, endpoints de carpetas/volúmenes/escaneo, tests.
NO (anotar en ISSUES-MVP2.md si surge la tentación): fsnotify/watchers de FS en tiempo
real (el poller de volúmenes + rescaneo bastan en MVP1), unidades de red SMB, symlinks,
identificación TMDB (es M3), miniaturas, lectura de duración/códecs (es M4).

## 1. Interfaz Provider — `internal/providers/provider.go`
Implementar el contrato de la especificación §3.2 tal cual (Provider, RawItem,
PlayableStream, HealthStatus). `HealthStatus` es un string con los valores de
`catalog.Health*`. En M2 solo localfs implementa la interfaz completa; `Resolve`
devuelve un PlayableStream con URL vacía y el locator (el endpoint real de streaming
llega en M4) — documentarlo en el código.

## 2. Provider localfs — `internal/providers/localfs/`

### Escaneo
- Recorre cada carpeta registrada (config.MediaFolders) con `filepath.WalkDir`.
- Extensiones admitidas (constante exportada): .mp4 .mkv .avi .mov .wmv .flv .webm
  .m4v .mpg .mpeg .ts .m2ts .3gp .ogv
- Ignorar: ocultos (prefijo "."), carpetas del sistema ($RECYCLE.BIN, System Volume
  Information, node_modules), archivos < 10 MB (samples/basura; constante configurable
  en código, no en config de usuario).
- Errores de permiso en subcarpetas: log en debug y continuar; jamás abortar el escaneo.

### Alta en catálogo (provisional, a la espera de M3)
Por cada vídeo nuevo:
- Crear item provisional: `kind='other'`, `confirmed=0`, `title` = nombre de archivo
  sin extensión, sin tmdb_id. M3 lo normalizará/identificará después. NO intentar
  limpiar el título aquí (es trabajo de M3; evitar duplicar lógica).
- Crear source: `provider='localfs'`, `locator` = ruta absoluta, `volume_id` del
  volumen que la contiene, `health='viva'` (el archivo acaba de verse).
- Idempotencia total: el UNIQUE(provider, locator) ya existe; re-escanear no duplica.
  Si la source existe, solo actualizar health→viva y last_checked.
- Archivo desaparecido de un volumen CONECTADO durante un rescaneo: eliminar la
  source (el archivo ya no existe). El item se conserva aunque quede sin fuentes
  (decisión de diseño: catálogo y fuentes son independientes; no borrar items).
- Transacción por lotes (p. ej. cada 200 altas) para no castigar el disco.

### Volúmenes — `internal/providers/localfs/volumes_windows.go` (+ fallback)
- Windows: id de volumen = número de serie del volumen vía
  `golang.org/x/sys/windows` (GetVolumeInformation). Dependencia x/sys APROBADA
  (única nueva permitida en M2). Formato id: `win-%08X`.
- No-Windows (build tag): id = hash SHA-1 corto de la ruta raíz del punto de montaje.
  Suficiente para dev en Linux/CI.
- Tabla `volumes` (ya existe en el esquema): upsert al ver un volumen (label = etiqueta
  del volumen), `connected`, `last_seen`.

### Poller de volúmenes — goroutine en arranque
- Cada 5 s: enumerar volúmenes presentes (Windows: letras de unidad existentes y su
  serial; no-Windows: existencia de las rutas raíz registradas).
- Volumen conocido que desaparece → `volumes.connected=0` y TODAS sus sources
  localfs → `health='desconectada'` (sin borrar nada).
- Volumen que reaparece → `connected=1`, sus sources → `health='viva'`, y disparar
  rescaneo de las carpetas registradas que viven en él (novedades del disco).
- Volumen nuevo nunca visto: solo registrarlo en `volumes`. NO escanearlo entero
  automáticamente (decisión: el usuario decide qué carpetas registra; el escaneo
  automático de discos enteros al enchufar se evaluará en MVP2 como opción opt-in).
  EXCEPCIÓN: si una carpeta registrada apunta a ese volumen, sí se escanea.
- Apagado limpio: el poller respeta el context del main.

## 3. API — añadir a `internal/api`
- `GET  /api/v1/folders` → carpetas registradas + si su volumen está conectado.
- `POST /api/v1/folders` body `{"path": "E:\\Peliculas"}` → valida que existe y es
  directorio, la añade a config (Save), lanza escaneo asíncrono. 409 si ya registrada.
- `DELETE /api/v1/folders?path=...` → la quita de config y elimina sus sources
  localfs (los items se conservan). Confirmación es cosa de la UI, no de la API.
- `POST /api/v1/scan` → rescaneo asíncrono de todo; respuesta inmediata `{"status":"escaneando"}`.
  Un solo escaneo concurrente (si ya hay uno, 409). Estado consultable en
  `GET /api/v1/scan/status` → `{running, last_started, last_finished, files_seen, items_new}`.
- `GET /api/v1/volumes` → volúmenes conocidos con estado.

## 4. Integración en main
- Al arrancar: poller de volúmenes + escaneo inicial asíncrono de carpetas registradas
  (no bloquear el arranque: el servidor escucha ya; la galería se llena en vivo).
- Nuevo paquete orquestador ligero `internal/scan` (o dentro de providers/localfs)
  con el estado del escaneo y la exclusión mutua — a criterio de implementación,
  pero sin dependencias circulares con api.

## 5. Tests (listón de M2)
- Escaneo: árbol temporal con vídeos válidos, extensiones no admitidas, archivo
  < 10 MB, carpeta oculta → altas correctas y exclusiones correctas.
- Idempotencia: doble escaneo, cero duplicados.
- Archivo borrado + rescaneo → source eliminada, item conservado.
- Desconexión simulada (inyectar enumerador de volúmenes falso; diseñar el poller
  con esa interfaz para poder testearlo) → sources en `desconectada`; reconexión →
  `viva`.
- API folders: alta válida, ruta inexistente (400), duplicada (409), borrado.
- Los tests no deben depender de Windows (usar el fallback de volumen no-Windows
  y el enumerador inyectable).

## 6. Al terminar
- `go test ./...`, `gofmt -l .` vacío, `go vet ./...` limpio.
- Actualizar "Estado actual" de CLAUDE.md (marcar M2).
- Commit con mensaje descriptivo.
- Informe corto: decisiones tomadas dentro del margen del brief, y cualquier duda
  de diseño que haya surgido — NO resolverla ampliando alcance.
