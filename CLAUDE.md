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
- `docs/GALA_Plan_Desarrollo_MVP1.md` — módulos, dependencias, orden y criterios de "hecho".
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
- Frontend: SvelteKit con adapter-static, build volcada a `web/dist/`, embebida
  con `go:embed`. Hasta M10, `web/dist/` contiene un reproductor mínimo sin
  framework (M4: galería + `<video>` + hls.js vendorizado localmente).
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
- [ ] M6 Salud ← SIGUIENTE
- [ ] M7 LAN · M8 Deseos ·
      M9 .gala · M10 Frontend · M11 Release
(Actualizar esta lista al completar cada módulo.)

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
web/                 frontend embebido (web.go + dist/: reproductor mínimo M4)
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
