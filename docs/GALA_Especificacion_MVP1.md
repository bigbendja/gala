# GALA — Especificación MVP1
**Versión 0.1 · Julio 2026 · Nombre de producto provisional**

---

## 1. Propósito y tesis de producto

Software libre (web local + escritorio, futura app) para catalogar y reproducir contenido de vídeo propio del usuario, agregando fuentes heterogéneas (archivos locales, listas M3U, enlaces Drive/HTTP) en una sola galería tipo Netflix.

**Tesis:** tan fácil como VLC, tan bonito como Netflix. Cero configuración: doble clic → detecta → cataloga → reproduce. Diseñado para el contexto CEMAC: funcionamiento offline, compartición por LAN sin internet, hardware modesto, disco duro que circula.

**Reglas de hierro:**
1. El software nunca incluye, enlaza ni sugiere contenido o listas. Herramienta neutral (modelo VLC/Kodi core).
2. Todo funciona sin internet una vez catalogado (caché TMDB agresiva).
3. Ninguna acción de usuario requiere entender conceptos de servidor, puertos o bibliotecas.

---

## 2. Alcance

### MVP1 (este documento)
- Escaneo de carpetas y discos locales con detección automática de nuevos volúmenes (disco USB enchufado → catalogado).
- Identificación y enriquecimiento vía TMDB (carátula, sinopsis, año, género, clasificación) con caché offline.
- Modelo catálogo/fuentes: una entrada de catálogo con N fuentes.
- Caja de ingesta universal ("pega lo que sea"): enlaces sueltos, múltiples enlaces, M3U/M3U8, texto con enlaces mezclados.
- Reproductor web con direct play; remux al vuelo si solo falla el contenedor. Sin transcode completo en MVP1.
- Fallback de fuentes al reproducir (orden: local → LAN → HTTP directo).
- Salud de fuentes: verificación en segundo plano, estados viva / lenta / caída.
- Modo LAN: el binario sirve la web en la red local; conexión por QR desde móviles (navegador, sin instalar nada).
- Continuar viendo (estado en servidor, compartido entre dispositivos de la LAN).
- Lista de deseos: títulos sin fuente, cruce automático al ingerir nuevas fuentes.
- Exportar/importar biblioteca o colecciones en formato `.gala`.

### Fuera del MVP1 (roadmap)
- **MVP2:** provider M3U avanzado (EPG, favoritos, re-sincronización programada), transcode ffmpeg con perfiles de bajo consumo, perfil 480p para LAN.
- **MVP3:** provider Google Drive (OAuth por usuario), anclar a local (descarga programada), subtítulos OpenSubtitles.
- **MVP4:** provider Mega, perfiles multiusuario con modo niños + PIN, wrapper móvil (Capacitor), interfaz de resolvers plugables (yt-dlp embebido opcional, resolvers de terceros fuera del proyecto oficial).
- **Comercial (paralelo, no en core):** edición vendedor / white-label.

---

## 3. Arquitectura

### 3.1 Visión general

Binario único autocontenido (sin instalador, sin dependencias, portable) que embebe:

```
┌─────────────────────────────────────────────┐
│  GALA (binario Go)                          │
│  ┌──────────┐ ┌──────────┐ ┌─────────────┐  │
│  │ Providers│ │  Core    │ │ HTTP server │  │
│  │ LocalFS  │→│ Catálogo │→│ API REST    │  │
│  │ M3U      │ │ Ingesta  │ │ Streaming   │  │
│  │ HTTP     │ │ Salud    │ │ Web estática│  │
│  └──────────┘ │ TMDB     │ └─────────────┘  │
│               └────┬─────┘        ↑         │
│               SQLite + caché   navegadores  │
│               de imágenes      (local y LAN)│
└─────────────────────────────────────────────┘
```

- **Lenguaje:** Go (binario único multiplataforma, bajo consumo, cross-compile a Windows/Linux/macOS).
- **BD:** SQLite embebido (modernc.org/sqlite, sin CGo, para no complicar el cross-compile).
- **Frontend:** SPA (Vue 3 o Svelte — decisión pendiente, criterio: peso final del bundle) embebida en el binario con `go:embed`. La misma UI sirve en local (`http://localhost:PUERTO`) y en LAN.
- **Escritorio:** el binario abre el navegador por defecto al arrancar + icono en bandeja del sistema (systray) con: Abrir, Modo LAN on/off, QR, Salir. Nada de Electron.
- **Plataforma inicial:** Windows 10/11 x64. Linux/macOS como builds secundarias desde el día 1 (el cross-compile en Go es gratis).

### 3.2 Interfaz de providers (contrato estable desde MVP1)

```go
type Provider interface {
    ID() string                                  // "localfs", "m3u", "http"
    Discover(ctx context.Context) ([]RawItem, error) // escaneo/sincronización
    Resolve(ctx context.Context, src Source) (PlayableStream, error)
    Check(ctx context.Context, src Source) (HealthStatus, error)
}

type RawItem struct {
    ProviderID string
    Locator    string            // ruta, URL, entrada M3U
    Hints      map[string]string // título crudo, año, temporada/episodio...
}

type PlayableStream struct {
    URL       string  // URL servible al reproductor (local: endpoint propio de streaming)
    Container string  // mp4, mkv, ts...
    NeedsRemux bool
}
```

MVP1 implementa `localfs`, `m3u` (parseo básico: ingesta y salud; lo avanzado en MVP2) y `http` (enlaces directos). Drive, Mega y resolvers encajan después sin tocar el core.

### 3.3 Pipeline de ingesta ("pega lo que sea")

```
Entrada (texto/archivo/carpeta/disco)
  → Detector de tipo (M3U | lista de URLs | URL única | texto sucio | ruta)
  → Extractor (regex URLs, parser M3U, walker de FS con extensiones de vídeo)
  → Normalizador de título (guessit-like propio: limpia calidad, códec, grupo,
    idioma, año, SxxEyy; p.ej. "Pelicula.2019.1080p.x264-GRUPO.mkv" → "Pelicula", 2019)
  → Matcher TMDB (búsqueda con año; umbral de confianza)
      ├─ confianza alta → alta automática en catálogo
      └─ confianza baja → cola "Por confirmar" (el usuario elige entre candidatos o edita)
  → Deduplicación (match por tmdb_id → misma entrada, nueva fuente)
  → Cruce con lista de deseos → notificación "N títulos de tus deseos disponibles"
```

Nada se descarta en silencio: lo no identificado queda en "Por confirmar" con el nombre crudo, reproducible igualmente.

### 3.4 Reproducción

1. Cliente pide play de una entrada → API devuelve fuentes ordenadas por prioridad (local > LAN > http, y dentro de cada tipo por salud y calidad declarada).
2. Direct play si contenedor/códec compatibles con el navegador (MP4/H.264/AAC, WebM).
3. Si el códec es compatible pero el contenedor no (MKV con H.264/AAC): **remux al vuelo** con ffmpeg `-c copy` (coste CPU casi nulo). ffmpeg se distribuye junto al binario (licencia LGPL, build sin componentes GPL) o se descarga al primer uso — decisión pendiente por peso del ejecutable.
4. Si el códec no es compatible (HEVC en navegador sin soporte): MVP1 muestra aviso claro + botón "Abrir en VLC" (protocolo `vlc://` / apertura local). Transcode real llega en MVP2.
5. Fallo de fuente en reproducción → salto automático a la siguiente fuente, con toast informativo.
6. Seeking: soporte de range requests en el endpoint de streaming local; para remux, segmentación HLS simple.

### 3.5 Salud de fuentes

- Worker en segundo plano con presupuesto configurable (por defecto: comprobación al ingerir + re-chequeo cada 24 h + al fallar reproducción).
- HTTP: HEAD/GET parcial con timeout corto. Local: existencia + legibilidad (distingue "archivo borrado" de "disco no conectado": un volumen ausente marca las fuentes como `desconectada`, no `caída`).
- Estados: `viva` · `lenta` · `caída` · `desconectada` · `sin_verificar`. Visibles como puntos de color en la ficha; nunca bloquean el play (el usuario puede intentarlo igualmente).

### 3.6 Modo LAN

- Toggle en la UI y en la bandeja. Al activarlo: bind en `0.0.0.0`, muestra QR con `http://IP_LOCAL:PUERTO`.
- Sin cuentas en MVP1; opcionalmente PIN de acceso de 4 dígitos (activable). Multiusuario real en MVP4.
- "Continuar viendo" y progreso se guardan por dispositivo (cookie de dispositivo) en el servidor.
- mDNS (`gala.local`) como mejora si el coste es bajo; el QR es el camino principal.

### 3.7 TMDB y modo offline

- API key de TMDB propia del proyecto embebida (permitido por TMDB para apps no comerciales con atribución; revisar términos antes de release — si cambia, fallback a key por usuario con asistente de 1 pantalla).
- Caché SQLite de metadatos + caché de imágenes en disco (posters w342, backdrops w780; tamaños fijos para controlar espacio).
- Sin conexión: búsqueda y matching contra la caché local; lo no resuelto queda en "Por confirmar" y se resuelve automáticamente cuando vuelva la conexión.
- Idioma de metadatos: es-ES por defecto con fallback en-US; fr-FR seleccionable (CEMAC).

### 3.8 Formato `.gala`

- ZIP con: `manifest.json` (versión de esquema, autor opcional, fecha), `catalog.json` (entradas + fuentes exportables), `covers/` (miniaturas cacheadas).
- Al exportar: opción de incluir o excluir fuentes locales (rutas). Al importar: entradas con fuente local ajena se marcan `pendiente de fuente` (alimentan deseos); enlaces se verifican en segundo plano.
- Esquema versionado desde el día 1. Tamaño objetivo: compartible por WhatsApp (< 20 MB para cientos de títulos con miniaturas comprimidas).

---

## 4. Modelo de datos (SQLite)

```sql
-- Entradas de catálogo (película, serie, episodio, otro)
CREATE TABLE items (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('movie','series','season','episode','other')),
  parent_id INTEGER REFERENCES items(id),     -- episodio → temporada → serie
  tmdb_id INTEGER,                            -- NULL si no identificado
  title TEXT NOT NULL,
  original_title TEXT,
  year INTEGER,
  overview TEXT,
  poster_path TEXT, backdrop_path TEXT,       -- rutas en caché local
  genres TEXT,                                -- JSON array
  rating_cert TEXT,                           -- clasificación por edades
  confirmed INTEGER NOT NULL DEFAULT 1,       -- 0 = en "Por confirmar"
  wished INTEGER NOT NULL DEFAULT 0,          -- 1 = en deseos (puede no tener fuentes)
  added_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

-- Fuentes: N por item
CREATE TABLE sources (
  id INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,                     -- 'localfs' | 'm3u' | 'http' | ...
  locator TEXT NOT NULL,                      -- ruta absoluta o URL
  volume_id TEXT,                             -- id de disco para localfs
  quality_label TEXT,                         -- '1080p', '720p'... si consta
  health TEXT NOT NULL DEFAULT 'sin_verificar',
  last_checked TEXT,
  priority INTEGER NOT NULL DEFAULT 0,
  origin_list_id INTEGER REFERENCES ingest_lists(id),
  added_at TEXT NOT NULL,
  UNIQUE(provider, locator)
);

-- Listas ingeridas (M3U, .gala, lotes pegados) para re-sincronizar y trazar
CREATE TABLE ingest_lists (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL,                         -- 'm3u' | 'gala' | 'paste'
  source_url TEXT,                            -- si vino de URL, para re-sync
  last_sync TEXT, item_count INTEGER
);

-- Volúmenes (discos) conocidos
CREATE TABLE volumes (
  id TEXT PRIMARY KEY,                        -- serial/uuid del volumen
  label TEXT, last_seen TEXT, connected INTEGER NOT NULL DEFAULT 0
);

-- Progreso de reproducción por dispositivo
CREATE TABLE playback (
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL,
  position_s REAL NOT NULL, duration_s REAL,
  watched INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (item_id, device_id)
);

-- Caché TMDB
CREATE TABLE tmdb_cache (
  tmdb_id INTEGER, kind TEXT, lang TEXT,
  payload TEXT NOT NULL,                      -- JSON crudo
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (tmdb_id, kind, lang)
);
```

---

## 5. API (REST, prefijo `/api/v1`)

| Método | Ruta | Función |
|---|---|---|
| GET | `/items` | Catálogo con filtros (kind, género, texto, salud, deseos, por confirmar) |
| GET | `/items/{id}` | Ficha con fuentes y salud |
| POST | `/ingest` | Caja universal: body texto o archivo; devuelve resumen (altas, duplicados, por confirmar) |
| POST | `/items/{id}/confirm` | Resolver "Por confirmar" (elegir candidato TMDB o título manual) |
| GET | `/play/{item_id}` | Fuentes ordenadas + URL de stream de la primera |
| GET | `/stream/{source_id}` | Streaming (range requests; remux si procede) |
| GET/PUT | `/playback/{item_id}` | Progreso del dispositivo |
| POST | `/wishlist` | Alta en deseos desde búsqueda TMDB |
| GET/POST | `/export` · `/import` | Formato `.gala` |
| GET | `/lan/status` · POST `/lan/toggle` | Modo LAN + datos del QR |
| GET | `/health/summary` | Resumen de salud de fuentes |

---

## 6. Seguridad

- Bind por defecto solo `127.0.0.1`; `0.0.0.0` únicamente con Modo LAN activo y aviso claro.
- Streaming local: validación estricta de rutas (solo dentro de carpetas registradas; anti path-traversal).
- PIN LAN opcional; rate limit básico en LAN.
- Sin telemetría. Actualizaciones: comprobación manual (botón), nunca automática silenciosa.
- ffmpeg invocado con argumentos construidos, jamás shell interpolado.
- `.gala` importado: se tratan los locators como datos, nunca se ejecuta nada; sanitización de rutas.

## 7. Rendimiento (objetivos en hardware modesto: dual-core, 4 GB RAM)

- Arranque < 3 s. Escaneo inicial de 1.000 archivos < 2 min (matching TMDB en cola, no bloqueante: la galería aparece al momento con títulos crudos y se enriquece en vivo).
- RAM en reposo < 150 MB. Remux: overhead CPU < 10%.
- UI: listas virtualizadas (una galería de 5.000 títulos debe hacer scroll fluido); imágenes lazy con miniaturas.

## 8. Pruebas

- Unitarias: normalizador de títulos (corpus de nombres reales de archivos, el punto más frágil), parser M3U, detector de ingesta, dedupe.
- Integración: pipeline completo con TMDB simulado (fixtures); reproducción sobre matriz de contenedores/códecs (mp4/mkv/ts × h264/hevc/vp9 × aac/ac3).
- E2E mínimo (Playwright): flujo enchufar disco → ver galería → play → continuar en otro "dispositivo".
- Manual en hardware objetivo real antes de cada release (tienes el banco de pruebas perfecto en tu entorno).

## 9. Distribución

- GitHub Releases: `gala-win-x64.exe` (portable), zip Linux/macOS. Checksums firmados.
- Licencia propuesta: **AGPL-3.0** — protege el core de que terceros lo cierren y lo revendan como SaaS, y te deja a ti (titular del copyright) la vía de doble licencia para la edición vendedor/white-label. Alternativa MIT si priorizas adopción máxima sobre protección; decisión tuya antes del primer release público.
- Nombre GALA: verificar disponibilidad de marca/dominio antes de publicar.

## 10. Riesgos principales

| Riesgo | Impacto | Mitigación |
|---|---|---|
| Matching de títulos mediocre (archivos con nombres caóticos) | La galería "no parece Netflix" y muere la magia | Corpus real de pruebas; cola "Por confirmar" bien diseñada; edición manual fácil |
| TMDB cambia términos de API key embebida | Alta fricción de onboarding | Fallback a key por usuario asistida; caché hace el día a día offline |
| HEVC sin transcode en MVP1 | Parte del contenido no reproduce en navegador | Aviso honesto + "Abrir en VLC"; transcode priorizado en MVP2 |
| Asociación de marca con piratería | Mata stores, comunidad y edición comercial | Reglas de hierro §1; resolvers fuera del proyecto oficial |
| Peso de ffmpeg junto al binario | Descarga pesada para datos caros | Build LGPL mínima o descarga bajo demanda con reanudación |

---

*Documento editable. Los puntos marcados "decisión pendiente" requieren cierre antes de iniciar desarrollo: framework frontend, distribución de ffmpeg, licencia.*
