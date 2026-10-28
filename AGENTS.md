# CLAUDE.md — Contexto del proyecto GALA

## Qué es GALA
Media center de código abierto (AGPL-3.0): cataloga y reproduce contenido de vídeo
del usuario (archivos locales, listas M3U, enlaces) en una galería tipo Netflix.
Tesis: **tan fácil como VLC, tan bonito como Netflix**. Cero configuración,
funciona offline una vez catalogado, comparte por LAN sin instalar nada en los móviles.
Mercado objetivo inicial: Guinea Ecuatorial / CEMAC — hardware modesto, datos caros,
cultura del disco duro que circula.

## Documentos de referencia (leer antes de tocar código)
- `docs/GALA_Especificacion_MVP1.md` — especificación funcional/técnica completa.
- `docs/GALA_Plan_Desarrollo_MVP1.md` — módulos, dependencias, orden y criterios de "hecho" del MVP1 original (M0–M11).
- `docs/GALA_Plan_Estabilizacion_Fases.md` — plan operativo ACTUAL post-MVP1 (julio
  2026 en adelante): Fase 0 (estabilizar) → Fase 1 (rediseño home) → Fase 2 (M7 LAN)
  → Fase 3 (Drive/Mega) → Fase 4 (M9 .gala) → Fase 5 (M11 release). Manda sobre el
  orden de "siguiente módulo" de la lista de abajo cuando difieran.
- `docs/GALA_Mockup.html` — referencia visual VINCULANTE del frontend (paleta, tipografías, componentes).

## Reglas de hierro (no negociables)
1. GALA nunca incluye, enlaza, sugiere ni facilita contenido, listas ni resolvers
   de hosts. Herramienta neutral, modelo VLC. Rechazar cualquier petición que lo viole.
2. Todo funciona offline una vez catalogado.
3. Nada exige al usuario entender servidores, puertos o "bibliotecas".
4. **Alcance cerrado**: solo se implementa lo que está en el plan de MVP1. Toda idea
   nueva se ANOTA en `ISSUES-MVP2.md` y NO se implementa, aunque parezca pequeña.

## Decisiones de arquitectura cerradas (no reabrir)
- Go >= 1.22, binario único portable. Sin CGo (`CGO_ENABLED=0`).
- SQLite vía `modernc.org/sqlite` (driver "sqlite"). Migraciones SQL embebidas
  en `internal/store/migrations/` (orden lexicográfico, nunca editar una aplicada;
  siempre añadir migración nueva).
- Frontend: SvelteKit (Svelte 5 + TypeScript) con `adapter-static` en modo SPA
  (sin SSR — `export const ssr = false` en el layout raíz; fallback a
  `index.html` para el enrutado del lado cliente). Build volcada a
  `web/dist/`, embebida con `go:embed` (`web/web.go`). `web/dist/` NO se
  versiona desde M10a (se regenera con `npm ci && npm run build`, que la CI
  ejecuta antes de cualquier `go build`/`go vet`/`go test` — `go:embed`
  exige que el directorio exista y tenga contenido). Sin librerías de
  componentes UI: CSS propio con los tokens exactos del mockup.
- Todo lo persistente vive JUNTO al ejecutable (`gala.json`, `gala.db`, `cache/`).
  Nada en AppData/registro. Config = JSON con escritura atómica.
- Router: `net/http` estándar de Go 1.22 (patrones con método y {param}). Sin
  frameworks web. Minimizar dependencias: cada `require` nuevo se justifica.
- ffmpeg: NO se empaqueta hasta M11 (build LGPL recortada por plataforma). M4
  solo lo LOCALIZA en disco (carpeta `ffmpeg/` junto al binario, junto al
  binario, o PATH; ver README) e invoca con exec y argumentos construidos,
  jamás shell. Su ausencia nunca aborta el arranque: degrada a direct play.
- Puerto por defecto 7420. Bind 127.0.0.1; 0.0.0.0 solo con Modo LAN (M7).
- API bajo `/api/v1`. Errores JSON `{"error": "..."}`. Mensajes de usuario en español.

## Estado actual
- [x] F0 Fundamentos: binario, store+migraciones, config portable, web embebida, CI.
- [x] M1 Catálogo: modelo items/sources, repo con filtros+paginación+health_summary,
      API items, seed `--demo`, tests de repositorio.
- [x] M2 Provider LocalFS: interfaz `Provider`, provider localfs (escaneo con filtros,
      alta/baja idempotente en catálogo), volúmenes (Windows real + fallback no-Windows),
      poller de volúmenes (conexión/desconexión, rescaneo dirigido), orquestador
      `internal/scan` (exclusión mutua, estado, lotes de 200), endpoints
      folders/scan/volumes, tests.
- [x] M3 Identificación: normalizador (internal/identify/normalize.go, pipeline
      completo del brief + corpus de tests), cliente TMDB (búsqueda/detalle/
      imágenes, rate limit 4 req/s, reintentos con backoff, ErrOffline, caché
      offline en tmdb_cache/search_cache), matcher (umbrales ALTA/MEDIA/BAJA,
      similitud Levenshtein/Jaro propia, degradación por contradicción de
      carpeta, cap MEDIA para packs, dedupe y jerarquía serie→temporada→episodio),
      cola de enriquecimiento asíncrona (internal/identify/queue.go: engancha
      fin de escaneo/confirmación manual/arranque, reintento offline cada
      5 min, se reconstruye de BD), endpoints /pending, /items/{id}/confirm,
      /identify/status, /img/{archivo}.
- [x] M4 Streaming y reproducción: `internal/media` (localización de ffmpeg/ffprobe
      sin abortar el arranque si faltan, sonda ffprobe con timeout+caché en
      `media_info`, cola de sondeo en segundo plano + bajo demanda SÍNCRONA en
      /play, `Decide` direct/remux/reject/analyzing por contenedor+códecs con
      motivo en español — análisis pendiente o fallido NUNCA es reject, es
      analyzing transitorio; audio incompatible con vídeo compatible ya no es
      reject, es remux con `AudioTranscode`), `internal/stream` (direct play
      con range requests/ETag/416 vía `http.ServeContent`, remux a HLS con
      sesiones —`-c copy`, o `-c:v copy -c:a aac` si hace falta recodificar
      audio— límite global 3, máx. 2 por fuente, barrido de huérfanas cada
      10 min, cancelación por contexto), API `/play` (orden de fuentes
      salud→provider→prioridad→calidad; reject/analyzing incluidas al final,
      `path` del archivo solo si la petición es de localhost),
      `/stream/{source_id}[/hls/...]`, `/sources/{id}/failed`,
      `/playback/{item_id}` (cookie `gala_device`, umbral watched 92%),
      `progress` en /items, reproductor web mínimo (`web/dist/`: galería +
      `<video>` + hls.js vendorizado en `vendor/`; sin botón vlc://, mensaje
      honesto + "Copiar enlace"/"Copiar ruta del archivo"), script
      `scripts/make_testmedia.(ps1|sh)` para la matriz de prueba, tests (con
      ffmpeg/ffprobe falsos compilados en caliente para no depender de tener
      ffmpeg real en CI).
- [x] M5 Ingesta universal: `internal/ingest` (detector de tipo M3U/CSV/texto
      suelto — DetectInputType, heurística de CSV por cabecera+columna
      url/link/enlace; extractor de URLs y de rutas locales de texto sucio;
      clasificador de URLs — directo/m3u_remoto/drive/mega/host_no_soportado/
      desconocido, unsupportedHosts mantenible SOLO para omitir, nunca
      resolver; parser M3U robusto a CRLF/comentarios/atributos, con
      heurística lista-de-canales-vs-HLS-de-un-vídeo; parser CSV mínimo;
      Pipeline que reutiliza identify.Parse para basura y despierta la MISMA
      cola de M3 al terminar — sin lógica de matching propia), provider
      `internal/providers/httpprovider` (Check/Resolve + ValidateExternalURL:
      SSRF básico contra 127.0.0.0/8, 10/8, 172.16/12, 192.168/16, 169.254/16
      y equivalentes IPv6, con resolución DNS sustituible en tests),
      `catalog.IngestList`/`Source.OriginListID` (trazabilidad, fila propia
      por cada M3U remota expandida con su source_url para un re-sync
      futuro), API `POST /ingest` (texto plano o multipart, límite 5 MB→413,
      límite 5000 candidatos) y `GET /ingest/lists`, tests (incl. el texto
      EXACTO del mockup de ingesta, con DNS y descarga de M3U simulados).
      Addendum (corrección de contadores post-entrega): `ResumenIngesta` no
      lleva `por_confirmar` (el enriquecimiento es asíncrono — un campo así
      siempre valdría igual que `anadidos`, mentira por omisión); `total` se
      DERIVA como la suma exacta de los seis resultados mutuamente
      excluyentes (invariante garantizada por construcción, testeada en
      todos los tests del pipeline vía `assertResumenInvariant`); nuevo
      campo `detectados` cuenta todo lo que entró antes de truncar por
      límite o expandir M3U remotas (puede ser mayor que `total` si se corta
      por límite, o menor si una M3U se expande en N entradas); un error de
      BD real al dar de alta cae en `sin_clasificar`, nunca fuera de los
      seis resultados.
- [x] M6 Salud de fuentes: `internal/health` (interfaz `Checker` —
      `Check(ctx, source) (health, detail string)`—, `LocalfsChecker`: stat +
      legibilidad, consulta `VolumeConnected` ANTES de tocar disco —un volumen
      desconectado nunca se marca caída, respeta la semántica de M2—, y
      umbral de lentitud inyectable para tests; `HTTPChecker`: HEAD con
      respaldo GET Range parcial, SSRF de M5 antes de cualquier petición,
      Content-Type text/html cuenta como caída, latencia alta cuenta como
      lenta; `UnverifiableChecker` para drive/mega/desconocidos — no poder
      verificar nunca es caída, siempre sin_verificar), `Worker` con el mismo
      patrón de cola que M3/M4 (`internal/health/worker.go`: `Enqueue`/
      `Run`/`drain` con concurrencia acotada a 4 verificaciones simultáneas,
      ventana mínima de 30 min entre chequeos de una misma fuente salvo
      forzado, re-chequeo periódico completo cada 24h, `TriggerFull` con
      `ErrAlreadyRunning` simétrico a `scan.ErrAlreadyRunning`, `CheckNow`
      síncrono para la reconciliación de fallos, reconstrucción desde BD vía
      `SourcesDueForHealthCheck` — nunca de memoria), migración
      `0004_health.sql` (columna `sources.health_detail` + índice sobre
      `last_checked`), API `GET /health/summary` (agregado global, `total`
      derivado de la suma de los cinco estados — mismo principio que
      `ResumenIngesta` de M5), `POST /health/check` (409 si ya hay una
      verificación en curso) y `GET /health/status`; `POST
      /sources/{id}/failed` (M4) ahora reconcilia: marca caída de inmediato
      y dispara un `CheckNow` real, corrigiendo a viva si el fallo fue
      transitorio; la ingesta de M5 despierta el worker tras altas nuevas
      vía la interfaz mínima `ingest.HealthNotifier` (evita que M5 dependa
      del paquete internal/health); tests (incl. presupuesto de
      concurrencia, ventana mínima, reconstrucción desde BD tras "apagado",
      cancelación por context, reconciliación transitoria/real, SSRF).
- [x] M10a Frontend — esqueleto + Galería + Ficha: proyecto SvelteKit real en
      `web/` (Svelte 5 + TS, `sv create`), sustituye al placeholder de
      `web/dist/`. Sistema de diseño (`web/src/lib/styles/`: tokens EXACTOS
      del mockup, fuentes empaquetadas — Bricolage Grotesque/Instrument
      Sans/Spline Sans Mono como `.woff2` propios, self-hosted, NUNCA Google
      Fonts — y `prefers-reduced-motion` respetado). Cliente de API tipado
      (`web/src/lib/api/`: tipos que reflejan el JSON real, `ApiError` con
      mensajes en español para fallos de red/HTTP). Componentes:
      `HealthDot`/`Card`/`Row`/`Hero`/`SourceStack`/`Topbar`/`Footer`. Rutas
      `/` (galería: hero opcional con item real que tenga backdrop,
      "Continuar viendo"/"Tu galería"/"Deseos") y `/item/{id}` (ficha: hero +
      stack de fuentes + metadatos); ambas sin SSR (`ssr=false`). Galería
      virtualizada SIN librerías (`Row.svelte`: `grid-auto-flow:column` +
      `grid-auto-columns` fijo reserva el ancho de scroll aunque solo se
      rendericen las tarjetas visibles, con `grid-column` explícito por
      índice) + paginación perezosa sobre `GET /items`; probado con 3.000
      items sintéticos. Estados de carga (skeletons)/vacío/error por
      sección. Tests: Vitest + `@testing-library/svelte` (jsdom) para
      componentes y cliente de API; `svelte-check` limpio. CI actualizada:
      `npm ci && npm run build` antes de cualquier paso de Go (`go:embed`
      exige `web/dist/` con contenido). `web/dist/` deja de versionarse.
      Endpoints que faltaron en la entrega inicial (los 3 principales ya se
      cerraron, ver addendum abajo); el origen M3U de una fuente sigue sin
      ser visible sin cruzar `ingest_lists` (badge "LISTA·M3U" omitido,
      exigiría una petición extra por fuente — ver ISSUES-MVP2.md).
      Reproductor, modal de ingesta, "Por confirmar", ajustes y búsqueda
      funcional quedan fuera a propósito (M10b/M10c).
      Addendum M10a-fix (correcciones visuales + los 3 endpoints anotados):
      badge "NUEVO" ahora a 48h (antes 24h, salía en casi todo) y con menor
      peso visual (halo difuso en vez de chapa opaca, para no tapar el
      póster); el detalle de salud en la ficha ya no muestra el nombre del
      paquete Go interno (`httpprovider:` etc. — `cleanHealthDetail` en
      `web/src/lib/format.ts`, limpieza SOLO de presentación); items sin
      identificar (`confirmed=false` o sin `tmdb_id`) muestran el nombre de
      archivo limpio (puntos/guiones bajos → espacios, `displayTitle`) y una
      marca sobria "Sin identificar" en vez del año, sin tocar la lógica de
      M3. Backend: `GET /items` gana `parent_id` (hijos directos; vacío o
      "null" = solo nivel superior) y `in_progress` (items con progreso real
      del dispositivo de la cookie, ordenados por el momento del progreso);
      `POST`/`DELETE /items/{id}/wish` marca/quita deseado (anticipo mínimo
      de M8 — la búsqueda-para-desear un título sin fuente sigue siendo M8
      completo). Frontend ya consume los tres: ficha de serie con
      temporadas/episodios reales, botón "+ Mi lista" funcional en hero y
      ficha, "Continuar viendo" alimentada por `in_progress` (ya no limitada
      a la página cargada de "Tu galería").
- [x] M10b Frontend — Reproductor + Ingesta + Por confirmar: reproductor real
      (`web/src/lib/components/Player.svelte`, ruta `/ver/{id}`) que sustituye
      al mínimo de M4 — consume `GET /play/{item_id}`, selector de fuentes con
      `HealthDot`+modo+calidad, direct play nativo, remux vía hls.js
      (`^1.6.16`, dependencia npm normal para que Vite lo empaquete — nunca
      CDN), fallback automático a la siguiente fuente reproducible si el
      `<video>` dispara `error` (avisa a `POST /sources/{id}/failed` y
      muestra "La fuente falló, probando la siguiente…"), reject/analyzing
      mostrados con el motivo en español + "Copiar enlace"/"Copiar ruta del
      archivo" (nunca vlc://), aviso si `ffmpeg_available` es falso, progreso
      guardado cada ~10 s y en pausa vía `PUT /playback/{item_id}` y
      reanudado al abrir (offset del lado del cliente para direct, parámetro
      `t` de arranque de sesión para remux — la sesión HLS siempre empieza su
      propio timeline en 0 en el punto de seek). Modal "Pega lo que sea"
      (`IngestModal.svelte`) conectado a `POST /api/v1/ingest`: pega texto o
      suelta un archivo, muestra el `ResumenIngesta` REAL (no las cifras de
      fantasía del mockup) con sus `detalles` si algo quedó sin clasificar;
      "Elegir carpeta"/"Escanear disco USB"/"Importar .gala" quedan inertes a
      propósito (gestión de carpetas es M10c, `.gala` es M9) en vez de
      improvisar alcance nuevo. "Por confirmar" (`/pendientes`, contador en
      el topbar): consume `GET /pending`; separa items `review` (candidatos
      TMDB como `CandidateCard` con póster — única excepción consciente a
      "todo offline", ya que la búsqueda que los generó ya fue online — o
      título manual) de `junk` (sección "Descartados", solo lectura) de
      `pending`/`error` (se cuentan como "siguen procesándose en segundo
      plano", no se listan como accionables); confirmar por candidato usa
      `POST /items/{id}/confirm {tmdb_id,kind}`, por título manual
      `{title}`; ninguno de los dos endpoints es nuevo. No existe endpoint
      para descartar un item de la cola sin confirmarlo (se anota en
      ISSUES-MVP2.md, no se inventó uno). Tests: Vitest para
      `pickNextSource`/fallback simulado con `video.error`/`ResumenIngesta`/
      `CandidateCard` (49 tests, `svelte-check` limpio). Verificación E2E
      manual contra `gala.exe` real con la matriz de M4: direct play y remux
      (HLS real vía ffmpeg), reject HEVC con mensaje honesto, fallback
      forzado borrando el archivo de una fuente, ingesta con el texto EXACTO
      del mockup (ver `internal/ingest/pipeline_test.go`,
      `TestPipelineRunMockupTextExact`) mostrando un resumen real con un
      fallo de DNS real en sus `detalles`, resolución manual de un item de
      "Por confirmar" reflejada al instante en galería y topbar, y reanudado
      de progreso tras recargar la página — capturas en
      `docs/screenshots/m10b/`. La confirmación de candidato TMDB no se pudo
      probar en vivo en este entorno (sin API key de TMDB válida,
      `ApplyIdentification` fallaría con 401); queda cubierta por los tests
      automatizados de M3 y por la ruta de título manual, que sí es 100%
      offline y se verificó en vivo.
- [x] M10c Frontend — Ajustes + Búsqueda + Gestión + Responsive (cierra M10):
      búsqueda funcional en la topbar (`SearchBox.svelte`, debounce ~300 ms
      sobre `GET /items?q=...`, desplegable con `<Card>` reales, estado vacío,
      botón de limpiar); búsqueda insensible a acentos AÑADIDA en este brief
      (`internal/catalog/search.go` — el brief la daba por resuelta, pero no
      lo estaba: `COLLATE NOCASE` de M1 solo cubre mayúsculas/minúsculas ASCII,
      no vocales acentuadas; se pliegan tanto la columna vía SQL
      `REPLACE(LOWER(...))` encadenado como el término buscado en Go, sin
      migración ni columna nueva). Pantalla `/ajustes` (nueva, sin mockup —
      diseño propio sobrio, ver informe): Carpetas (listar/añadir/quitar con
      confirmación inline, reusa `GET·POST·DELETE /folders` de M2), Escaneo
      (`POST /scan` + `GET /scan/status` con sondeo cada 2 s mientras
      `running`), Identificación (`GET /identify/status`,
      `friendlyIdentifyError` en `web/src/lib/format.ts` traduce un 401 crudo
      de TMDB a "La clave de TMDB no es válida" en vez de enseñar el JSON de
      error — verificado en vivo con una carpeta y una clave falsa reales),
      Clave TMDB (endpoints nuevos, ver abajo; nunca se muestra la clave,
      solo si hay una configurada), Salud (`GET /health/summary` +
      `POST /health/check` con sondeo), Sistema (versión y disponibilidad de
      ffmpeg vía `GET /ping` — solo se le añadió el campo `ffmpeg_available`,
      sin montar un endpoint nuevo solo para esto; "ruta de datos" NO se
      expone, ver ISSUES-MVP2.md). Descartar pendientes en `/pendientes`
      (cierra el hueco anotado en M10b): confirmación inline por item,
      `DELETE /api/v1/items/{id}` (endpoint nuevo, ver abajo). Navegación real
      de la topbar: `/peliculas` y `/series` (`Grid.svelte`, rejilla con
      "Cargar más" — reemplaza la fila de scroll horizontal de `Row.svelte`
      para un listado completo), `/deseos` (variant "wish"), `/listas`
      (`GET /ingest/lists`, vista mínima honesta de solo lectura — sin
      cruzarla con items, exigiría un endpoint que no se pidió). Responsive
      360–414px: topbar colapsa a menú hamburguesa con buscador y navegación
      accesibles (`Topbar.svelte`); corregido un desbordamiento horizontal
      real de la ficha en móvil (`.info` es un track de grid `1fr` sin
      `min-width:0` — un track "1fr" sigue teniendo `min-width:auto` por
      defecto, así que el texto sin ajuste de línea de `SourceStack`
      forzaba tanto el track como la página entera más anchos que el
      viewport; encontrado y verificado con Chrome DevTools Protocol
      emulando 360–390px contra `gala.exe` real, no solo mirándolo).
      Endpoints nuevos (mínimos, según brief): `DELETE /api/v1/items/{id}`
      (cablea `catalog.Repo.DeleteItem`, ahora con borrado en cascada de
      hijos — serie→temporada→episodio — antes de borrar el item, ya que
      `parent_id` no tiene `ON DELETE CASCADE` a propósito y sin esto una
      serie con temporadas fallaría por la restricción de clave foránea);
      `GET`/`PUT /api/v1/settings/tmdb` (nunca devuelve la clave; `PUT` la
      valida con una búsqueda de prueba real contra TMDB antes de guardarla
      — 422 si es inválida, 503 si no hay conexión para probarla — y toma
      efecto de inmediato en la cola de identificación, mismo `*identify.
      Client` compartido). Tests: Go (`TestDeleteItemCascadesToChildren`,
      `TestSearchIsAccentInsensitive`, `TestHandlePutTMDBSettings*`, etc.) y
      Vitest (`SearchBox`, `/ajustes`, descartar en `/pendientes`; 62 tests
      en total, `svelte-check` limpio). Verificación E2E manual contra
      `gala.exe` real: añadir una carpeta desde Ajustes y ver aparecer su
      contenido sin curl, escribir una clave TMDB inválida desde la UI y ver
      el 422 real, buscar "parasitos" y encontrar "Parásitos" (con acento) en
      un item real, descartar un pendiente basura y verlo desaparecer de la
      cola y del backend (404 al pedirlo después), navegar
      Películas/Series/Listas/Deseos, y emulación móvil 360/375/390px en
      galería/ficha/reproductor/ajustes/pendientes/modal de ingesta/menú —
      capturas en `docs/screenshots/m10c/`. Con esto M10 (a+b+c) queda
      cerrado.
- [x] Fase 0 (`docs/GALA_Plan_Estabilizacion_Fases.md`) — Estabilizar lo que
      estaba roto tras M10: 0.1 CI (test `TestLocatePrefersFfmpegSubfolder`
      arreglado — colisión archivo/carpeta solo en Linux, no hacía falta
      tocar `internal/media`; el resto de 0.1, y 0.2/0.3/0.5 completos,
      resultaron ya resueltos por M10c, diagnóstico desactualizado), 0.4
      (bug real: "Tu galería" exigía `confirmed=true` Y `kind IN
      (movie,series)`, escondiendo cualquier enlace directo sin match en
      TMDB — casi todos. `web/src/lib/gallery.ts` deja ver `kind=other` con
      `source_count > 0` sin exigir `confirmed`; "Por confirmar" gana la
      acción "Dejar sin identificar" y trata `identify_status=error` como
      accionable, no como limbo eterno; verificado en vivo con la URL real
      de Big Buck Bunny del reporte original). Diagnóstico integral
      (pedido explícito del titular) encontró y corrigió además un bug de
      presentación en la tarjeta "Deseos" (decía "Sin fuente · buscando…"
      aunque el item ya tuviera fuente real) y documentó sin corregir un
      bug de reconciliación de carpetas quitadas/vueltas a añadir (ver
      ISSUES-MVP2.md y la propia Fase 0 en el plan). Pendiente de que el
      titular confirme el run verde en GitHub Actions antes de dar la fase
      por cerrada del todo.
- [x] Fase 1 (rediseño de la Home + acabado visual; rama feat/home-redesign,
      pendiente de que el titular verifique en vivo): Sistema de Diseño GALA
      aplicado a toda la app. **Home**: `HeroSlider` (rota solo cada 8 s, se
      detiene con ratón/foco/pestaña oculta/`prefers-reduced-motion`; filtros
      Destacados = selección diaria estable de lo identificado con sinopsis
      —no hay puntuación TMDB guardada, no es un ranking—, Nuevos = más
      recientes, Sin ver = lo que este dispositivo no ha empezado; el filtro
      se recuerda en localStorage; se apaga desde Ajustes › Inicio, ajuste
      `home_hero_hidden` en gala.json vía `GET/PUT /settings/home`, lectura
      permitida a clientes LAN, escritura solo del PC) y filas en carrusel
      con flechas ‹ › (`Row`, virtualización de M10a intacta): Continuar
      viendo (por dispositivo), Películas, Series, Documentales (género TMDB
      «Documental»), Añadidas recientemente, Mis Listas (= los deseos de «+
      Mi lista») y filas por género dinámicas (`GET /genres`, solo con
      contenido, máx. 6). Cada fila (`HomeSection`) carga sola y perezosa.
      **Reglas transversales**: sin pantalla negra (cascarón en `app.html`,
      skeletons, barra de progreso al navegar), errores en español con qué
      hacer (`errorMessage`; nunca `ApiError:` ni texto de un 5xx), fechas
      humanas, salud sin jerga, estados vacíos con acción, botones
      (`.btn-primary/-secondary/-quiet-danger` globales; deshabilitado =
      apagado + tooltip). **Fichas**: episodios con título, sinopsis y
      miniatura reales de TMDB (`/tv/{id}/season/{n}`, `Queue.enrichEpisodes`,
      cubre también los ya existentes), estado de fuentes de una serie
      derivado de sus episodios, «Continuar/Empezar T1·E1», ficha de
      episodio, ficha sin identificar con Identificar/Descartar y fuente con
      nombre de archivo destacado. Por confirmar, modal de ingesta,
      Listas, topbar móvil y Ajustes pulidos. Detalle y huecos en
      ISSUES-MVP2.md.
- [x] Fase 1 — segunda pasada (mockups de la Home y de la Ficha de
      `docs/briefs/`; rama feat/home-redesign, pendiente de verificación en
      vivo): **Diseño**: hero de la Home con el backdrop apaisado de TMDB a toda
      la anchura y degradado hacia el fondo (diapositivas apiladas con fundido,
      filtros arriba a la derecha, puntos-barra, flechas), flechas de los
      carruseles como bandas laterales, ficha con hero atmosférico, póster con
      elevación y secciones. Los fondos nuevos se descargan en w1280 y
      `Queue.upgradeBackdrops` pasa los antiguos (w780) a w1280. **Ficha
      enriquecida** (migración `0007_item_details.sql`, tabla `item_details`):
      UNA petición a TMDB con `append_to_response` (créditos, similares,
      clasificación) en `Client.FullDetail` (caché offline `movie_full`/
      `tv_full`); duración, nota, países en español, reparto (12), equipo
      (Dirección/Guion/Música/Fotografía; «Creación» en series), similares (10)
      y la edad de España (`rating_cert`, solo numérica). Fotos del reparto
      (w185) y carátulas de similares con el mismo reintento/caché que las
      carátulas; si una falla queda `images_pending` y se reintenta hasta 3
      veces (la web enseña iniciales). `Queue.enrichDetails` rellena también lo
      que ya estaba en la biblioteca. `GET /items/{id}/details` (siempre 200;
      `has_details=false` si TMDB aún no se ha consultado; ficha técnica de
      `media_info` + tamaño en disco; un similar que ya tienes lleva su
      `item_id`; lectura permitida a clientes LAN, sin rutas). Frontend:
      Reparto, Equipo, «Este archivo» y «Más como esto» (`CastRail`,
      `FileSpecs`, `SimilarRail`); un título sin identificar NO enseña
      secciones de TMDB. El stack de fuentes de GALA se conserva.
      Tercera pasada (fidelidad visual de la Home, comparada con capturas en
      `docs/screenshots/fidelidad/`): medidas del mockup (contenedor 1296,
      tarjeta 168/300, hueco 14, cabeceras a 28px, sin barra de scroll nativa,
      «Ver todo» al pasar el ratón); una fila diferida aún sin pedir NO pinta
      cajas (el skeleton es solo mientras carga de verdad), se pide sola a los
      1,5 s si no llegó a verse y, vacía, desaparece.
      Retoques: las series enseñan «año · N temp.» (`season_count` en el
      listado; el año que faltaba en las series identificadas por episodio se
      rellena desde TMDB en `enrichDetails`) y el hero se acota a 5 títulos.
- [x] M7 Modo LAN (Fase 2; pendiente de la prueba en móvil real y del CI
      verde, que confirma el titular): `internal/lan` (detección de la IP
      local — `Candidates()` ordena IPv4 física privada > pública > virtual
      (WSL/VPN/Tailscale…) y la API devuelve la mejor como `lan_ip` más la
      lista completa; `Controller`: listener reiniciable sobre EL MISMO
      `http.Server` — se cierra el listener viejo y se abre el nuevo, así que
      una reproducción en curso del PC no se corta, y al DESACTIVAR se cortan
      las conexiones de dispositivos remotos; `Guard`: middleware que envuelve
      todo el mux). `config.Config` gana mutex para `Bind`/`LanPIN`
      (`LanState`, `SetBind`, `SetLanPIN`). API: `GET /lan/status`, `POST
      /lan/toggle`, `PUT /lan/pin` (solo desde el PC: loopback + Host
      loopback + `application/json`, contra CSRF/DNS rebinding) y
      `GET /lan/session`, `POST /lan/auth` (los usa el móvil). PIN de 4
      dígitos en `gala.json` (`lan_pin`), nunca devuelto por la API (solo
      `pin_set`); al acertarlo se planta la cookie `gala_lan` con un token
      aleatorio guardado en memoria del servidor (30 días; reiniciar GALA o
      cambiar/quitar el PIN invalida todas las sesiones); rate-limit
      progresivo por IP (5 fallos → 30 s, se duplica hasta 15 min) más tope
      global (30 fallos/10 min → 5 min). QR generado EN EL NAVEGADOR con
      `qrcode-generator` (MIT, sin dependencias; evita una librería QR en
      Go) a partir de `qr_payload` = `http://{lan_ip}:{port}`, nunca con el
      PIN. **Qué puede un cliente LAN** (lista cerrada en
      `lan.RemoteAllowed`; una ruta nueva de la API es solo-PC hasta que se
      añada ahí a propósito): cargar la web, ver la galería y fichas
      (`GET /items`), carátulas, `/play`, `/stream`, guardar su progreso
      (`/playback`), "Mi lista" (`/wish`), y `/sources/{id}/failed`. Todo lo
      demás (carpetas, escaneo, ingesta, "Por confirmar", borrar, ajustes,
      clave TMDB, salud, Modo LAN) da 403. A un cliente LAN la ficha le llega
      sin rutas del disco (solo el nombre del archivo), sin `volume_id` ni
      `health_detail` (`redactForRemote`); `/play` ya ocultaba `path`. Rutas
      con `..` o sin normalizar → 403. Frontend: píldora "Modo LAN"
      funcional (`LanPanel.svelte`: aviso antes de activar, QR + URL,
      candidatas alternativas, PIN poner/cambiar/quitar), `PinGate.svelte`
      (pantalla de PIN para el móvil), el layout consulta `/lan/session` y
      en dispositivos remotos oculta Añadir/Ajustes/Por confirmar/Modo LAN.
      El tráfico LAN va en HTTP plano (decisión: aviso en el panel, sin
      TLS en MVP1). **Bug "carpeta duplicada" corregido** (migración
      `0005_removed_sources.sql`): la causa era que al quitar una carpeta se
      borraban sus fuentes y con ellas la única pista de a qué item
      pertenecía cada fichero (`syncBatch` solo buscaba por `locator`, así
      que al re-añadir creaba items nuevos y el original quedaba fantasma);
      lo mismo pasaba con un fichero que desaparecía y volvía (USB
      intermitente). Ahora `removeStale` anota en `removed_sources` el item
      de cada fuente retirada y el siguiente escaneo que ve ese fichero
      reutiliza el item (conserva identificación TMDB, deseos y progreso).
      Si el item se borra a propósito, el rastro cae por `ON DELETE
      CASCADE` y el fichero vuelve como item nuevo. Los fantasmas YA
      duplicados antes de este arreglo no se limpian solos (ver
      ISSUES-MVP2.md).
- [x] Fase A' CERRADA — Soporte de `.strm` (PR #2 mergeado; CI verde y prueba
      en vivo con TMDB real: Big Buck Bunny -> tmdb 10378 (2008), serie por
      carpeta -> Breaking Bad tmdb 1396 T1E02): el escaneo local (`localfs.WalkFolder`) reconoce
      `.strm` —un puntero de texto, formato abierto de Kodi/Jellyfin— sin
      aplicarle el tamaño mínimo de los vídeos. `localfs.ParseStrm` toma la
      primera URL http/https válida (ignora vacías, `#` y lo que no sea URL).
      La fuente es de tipo `http` (reutiliza `/play` 302, `HTTPChecker` de M6 y
      el SSRF de M5, que se aplica ANTES de la transacción y solo rechaza
      destinos internos: un DNS que no resuelve no rechaza). El item se llama
      como el .strm y la cola de M3 lo identifica por el nombre del .strm y su
      carpeta (`sources.origin_path`, migración `0006`, nunca serializada por
      la API), sin lógica nueva de matching. .strm inválido/vacío/SSRF = no
      crea item ni fuente (aviso en log); bajas, regreso del .strm y
      re-añadir carpeta reconcilian con `removed_sources` como en M7. Detalle
      y lo descartado en ISSUES-MVP2.md.
- [x] Fixes críticos de la auditoría visual (PR #3; auditoría con capturas en
      `docs/screenshots/audit/`, antes del acabado visual): (1) con la BD vacía
      `GET /pending` devolvía `null` y el layout raíz (`.filter` sobre `null`)
      tumbaba TODAS las rutas con "500 Internal Error": la API devuelve siempre
      `[]` y el cliente tolera `null` en las listas (`orEmpty`). REGLA: toda
      lista que salga por la API es `[]`, nunca `null`. (2) Remux en bucle:
      `setupRemux` leía `knownPosition` dentro del `$effect` de montaje y cada
      `timeupdate` lo relanzaba (una sesión HLS por fotograma hasta agotar el
      límite de 3; existía desde M10b, que solo verificó el arranque); el
      efecto depende ahora solo de fuente+modo+`<video>`. Nuevo `DELETE
      /stream/{source}/hls/{session}` (`Manager.StopSessionOf`, idempotente,
      solo cierra la sesión de su fuente, permitido a clientes LAN) y el
      reproductor libera su sesión al salir/cambiar de fuente/`pagehide`;
      antes solo se barrían a los 30 min y el servidor quedaba en 503.
      (3) SSRF en ingesta: los enlaces directos pasan por el mismo bloqueo de
      M5 que M3U remotas, `.strm` y salud (un DNS caído no rechaza);
      `ValidateExternalURL` bloquea `localhost`/`*.localhost` por nombre.
      Pendiente de la auditoría (acabado visual, aún sin hacer): pantalla
      negra durante la carga, fichas de serie/episodio/sin identificar,
      carátulas de candidatos, textos crudos (ISO, `ApiError:`), topbar móvil.
- [x] Ingesta: las páginas web no se catalogan (rama feat/fix-ingesta-paginas):
      un enlace con extensión de documento web (.html, .php…) o sin extensión
      cuyo servidor responde HTML (comprobación acotada en `internal/ingest/
      pages.go`: SSRF antes, 5 s por enlace, 45 s por ingesta; sin conexión o
      ante cualquier duda NO se rechaza) no crea item ni fuente: cuenta en
      `omitidos_no_directos` (parte de la suma de `total`) y los detalles
      llevan el mensaje claro, sin nombres de hosts ni de herramientas.
- [ ] Fase 3 (Drive/Mega) · Fase 4 (M9 .gala) · Fase 5 (M11 release)
(Actualizar esta lista al completar cada módulo/fase.)

## Estructura
```
cmd/gala/            main, flags (--no-browser, --demo)
internal/store/      SQLite + migraciones embebidas
internal/config/     gala.json portable
internal/catalog/    dominio + repositorio (M1)
internal/api/        handlers REST /api/v1
internal/providers/  interfaz Provider + localfs/ (M2), httpprovider/ (M5)
internal/scan/       orquestador de escaneo: estado, exclusión mutua (M2)
internal/identify/   normalizador, TMDB, caché                  (desde M3)
internal/media/      ffmpeg/ffprobe: localización, sonda, decide  (desde M4)
internal/stream/     direct play + remux HLS con sesiones         (desde M4)
internal/ingest/     detector, clasificador, parsers M3U/CSV, pipeline (M5)
internal/health/     checkers por provider + worker con presupuesto  (M6)
internal/lan/        Modo LAN: IP local, listener reiniciable, guardián+PIN (M7)
web/                 frontend: proyecto SvelteKit (src/) + web.go (go:embed de
                     dist/, generado en build, no versionado)             (M10a)
docs/                especificación, plan, mockup
testdata/            corpus de nombres reales, M3U, matriz de vídeos (media/ generado, no versionado)
scripts/             generación de datos de prueba (make_testmedia.ps1/.sh)
```

## Convenciones
- `gofmt` obligatorio (la CI lo exige). `go vet ./...` limpio.
- Todo módulo nuevo lleva tests; los criterios de "hecho" del plan son el listón.
- Comentarios y mensajes de error/log en español; identificadores en inglés.
- Contratos estables: la interfaz `Provider` de la especificación §3.2 no se cambia
  sin actualizar antes la especificación en docs/.
- Errores: envolver con `fmt.Errorf("contexto: %w", err)`. Sentinelas como
  `catalog.ErrNotFound` para casos esperables.
- Seguridad: validar SIEMPRE rutas de archivo contra las carpetas registradas
  (anti path-traversal); tratar locators importados como datos, nunca ejecutarlos.

## Comandos
```
go mod tidy                          # tras cualquier cambio de dependencias
go build -o gala.exe ./cmd/gala      # build local (Windows)
.\gala.exe --demo                    # arrancar con datos de demostración
go test ./...                        # tests (listón de cada módulo)
gofmt -l .                           # debe salir vacío
```

## Flujo de trabajo de este repo
El diseño (pantallas, contratos, decisiones, briefs de módulo) se hace fuera y
llega como documentos en `docs/` o como brief en el prompt. Claude Code implementa
EXACTAMENTE el alcance del brief: si durante la implementación surge una decisión
de diseño no cubierta, se pregunta o se anota, no se improvisa amplación de alcance.
