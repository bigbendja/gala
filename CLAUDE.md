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
  con `go:embed`. Hasta M10, `web/dist/` contiene un placeholder estático.
- Todo lo persistente vive JUNTO al ejecutable (`gala.json`, `gala.db`, `cache/`).
  Nada en AppData/registro. Config = JSON con escritura atómica.
- Router: `net/http` estándar de Go 1.22 (patrones con método y {param}). Sin
  frameworks web. Minimizar dependencias: cada `require` nuevo se justifica.
- ffmpeg: empaquetado junto al binario (build LGPL), invocado con exec y argumentos
  construidos, jamás shell. Llega en M4.
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
- [ ] M4 Streaming/reproducción ← SIGUIENTE
- [ ] M5 Ingesta · M6 Salud · M7 LAN · M8 Deseos ·
      M9 .gala · M10 Frontend · M11 Release
(Actualizar esta lista al completar cada módulo.)

## Estructura
```
cmd/gala/            main, flags (--no-browser, --demo)
internal/store/      SQLite + migraciones embebidas
internal/config/     gala.json portable
internal/catalog/    dominio + repositorio (M1)
internal/api/        handlers REST /api/v1
internal/providers/  interfaz Provider + localfs/, m3u/, http/  (desde M2)
internal/scan/       orquestador de escaneo: estado, exclusión mutua (M2)
internal/identify/   normalizador, TMDB, caché                  (desde M3)
web/                 frontend embebido (web.go + dist/)
docs/                especificación, plan, mockup
testdata/            corpus de nombres reales, M3U, matriz de vídeos
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
