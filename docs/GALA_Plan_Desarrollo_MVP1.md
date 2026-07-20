# GALA — Plan de desarrollo MVP1
**Versión 0.1 · Julio 2026 · Complementa a la Especificación MVP1**

**Decisiones cerradas:** Go + SQLite (modernc) + Svelte (SvelteKit estático embebido) · ffmpeg empaquetado (build LGPL recortada) · AGPL-3.0 + copyright del autor + CLA desde la primera contribución externa.

Las estimaciones de esfuerzo son relativas (S ≈ días, M ≈ 1–2 semanas, L ≈ 2–4 semanas, a tiempo parcial de una persona). Son inferencias, no compromisos.

---

## 1. Mapa de módulos y dependencias

```
F0 Fundamentos
 └─→ M1 Catálogo (núcleo)
      ├─→ M2 Provider LocalFS ──┐
      ├─→ M3 Identificación ────┤──→ M5 Ingesta universal ──→ M8 Deseos
      │        (títulos+TMDB)   │           │
      └─→ M4 Streaming/Play ←───┘           └──→ M9 Formato .gala
               │
               ├─→ M6 Salud de fuentes
               └─→ M7 Modo LAN + progreso
M10 Frontend: transversal, avanza en paralelo desde M1 por pantallas
M11 Empaquetado y release: al final, pero su esqueleto se monta en F0
```

Camino crítico: **F0 → M1 → M2 → M3 → M4**. Con eso ya existe el producto mínimo demostrable (enchufas disco → galería catalogada → play). Todo lo demás suma sobre esa base.

---

## 2. Módulos

### F0 · Fundamentos — S/M
Repo (AGPL-3.0, README con reglas de hierro, CONTRIBUTING + CLA preparado), estructura de proyecto, CI en GitHub Actions (build multiplataforma + tests en cada push), esqueleto del binario: servidor HTTP, `go:embed` de un frontend "hola mundo", SQLite con sistema de migraciones, logging, config en un único archivo junto al .exe (portable de verdad: nada en AppData), icono en bandeja (systray) con Abrir/Salir.
**Hecho cuando:** `gala.exe` arranca, abre el navegador, sirve la SPA vacía y la CI produce binarios de Windows/Linux/macOS.

### M1 · Núcleo de catálogo — M
Modelo de datos completo (tablas de la especificación §4), repositorio de acceso a datos, API `/items` con filtros y paginación, `/items/{id}`. Sin fuentes reales aún: fixtures de desarrollo.
**Hecho cuando:** la API sirve un catálogo semilla con filtros y la galería del frontend lo pinta.
**Riesgo a vigilar:** diseñar bien `items.kind` + `parent_id` (serie→temporada→episodio) ahora; migrarlo después con datos reales es doloroso.

### M2 · Provider LocalFS — M
Interfaz `Provider` (contrato de la especificación §3.2) + primera implementación: registro de carpetas, walker con filtro de extensiones de vídeo, identificación de volúmenes (serial del disco), detección de conexión/desconexión de USB (polling de volúmenes; watcher del FS como mejora), estados `desconectada` en fuentes de un volumen ausente.
**Hecho cuando:** enchufar un USB con vídeos crea/actualiza fuentes automáticamente y desenchufarlo las marca sin borrarlas.

### M3 · Identificación (normalizador + TMDB) — L ⚠ el módulo más delicado
Normalizador de nombres de archivo (limpieza de calidad/códec/grupo/idioma, extracción de año y SxxEyy), cliente TMDB con rate-limit y reintentos, matcher con umbral de confianza, cola "Por confirmar" con candidatos, caché SQLite de metadatos + descarga y caché de imágenes, modo offline completo, cola de enriquecimiento asíncrona (la galería aparece al instante con títulos crudos y se enriquece en vivo).
**Hecho cuando:** un corpus real de ≥300 nombres de archivo (recopílalo desde ya de tus discos y los de conocidos: es tu activo de pruebas más valioso) se identifica con precisión alta y lo dudoso cae en "Por confirmar", nunca en falsos positivos silenciosos.
**Riesgo:** aquí se decide si GALA "parece Netflix" o parece un explorador de archivos. Presupuesto extra de iteración reservado.

### M4 · Streaming y reproducción — L
Endpoint `/stream` con range requests, detección de contenedor/códec (ffprobe), decisión direct-play / remux, remux `-c copy` a fMP4/HLS con ffmpeg empaquetado, reproductor web (vídeo HTML5 + hls.js si aplica), orden de fuentes y fallback automático con toast, aviso honesto + "Abrir en VLC" para códecs no soportados, guardado de posición.
**Hecho cuando:** la matriz de prueba (mp4/mkv/ts × h264/hevc × aac/ac3) reproduce o degrada con mensaje claro, y el seeking funciona también en remux.

### M5 · Ingesta universal — M/L
Detector de tipo de entrada (URL única / lote / M3U / texto sucio / ruta / .gala), extractores, parser M3U básico, provider `http` (enlaces directos con verificación de cabeceras), pipeline completo → normalizador (M3) → dedupe por tmdb_id → resumen de ingesta como en el mockup, tabla `ingest_lists` para trazabilidad y re-sync futuro.
**Hecho cuando:** pegar el texto de WhatsApp del mockup (enlaces + emojis + M3U) produce exactamente el resumen esperado y nada se pierde en silencio.

### M6 · Salud de fuentes — S/M
Worker con presupuesto (al ingerir, cada 24 h, al fallar play), chequeos HTTP con timeout corto, estados y transiciones, `/health/summary`, puntos de color en UI.
**Hecho cuando:** una fuente caída se detecta sin intervención y el play salta a la siguiente.

### M7 · Modo LAN + progreso multi-dispositivo — M
Toggle de bind 127.0.0.1 ↔ 0.0.0.0, QR con IP local, PIN opcional de 4 dígitos, cookie de dispositivo, `playback` por dispositivo, fila "Continuar viendo" coherente entre PC y móvil, prueba explícita en Android de gama baja por navegador (Chrome y el navegador de fábrica de algún Tecno/Infinix: es tu dispositivo objetivo real).
**Hecho cuando:** empiezas una película en el PC y la continúas en un móvil de gama baja por wifi sin instalar nada.

### M8 · Deseos — S
Alta desde búsqueda TMDB, flag `wished`, cruce automático en cada ingesta/escaneo, notificación "N títulos de tus deseos han llegado".
**Hecho cuando:** enchufar un disco que contiene un título deseado lo convierte en entrada con fuente y lo notifica.

### M9 · Formato .gala — M
Esquema versionado, export (biblioteca o colección, con/sin rutas locales), import con sanitización, entradas ajenas → `pendiente de fuente`, miniaturas comprimidas, objetivo de tamaño compartible por WhatsApp.
**Hecho cuando:** exportar en un PC e importar en otro reconstruye la galería con carátulas y las fuentes correctas verificándose en segundo plano.

### M10 · Frontend (transversal) — L en total, por pantallas
Orden de pantallas, cada una cuando su API esté lista: 1) Galería + ficha (M1), 2) Reproductor (M4), 3) Modal ingesta + Por confirmar (M5), 4) LAN/QR + Continuar viendo (M7), 5) Deseos (M8), 6) Ajustes (carpetas, PIN, export/import). Listas virtualizadas desde el principio (probar con 5.000 items falsos), imágenes lazy, diseño del mockup como referencia visual vinculante.

### M11 · Empaquetado y release — M
Build de ffmpeg LGPL recortada por plataforma, empaquetado en el binario/carpeta portable, checksums, GitHub Releases automatizado desde CI, página README con capturas, prueba final en hardware objetivo real (tu banco de pruebas de Malabo) contra los objetivos de rendimiento de la especificación §7.
**Hecho cuando:** una persona sin conocimientos descarga un único archivo, doble clic, y en menos de un minuto ve su primera película.

---

## 3. Orden de construcción propuesto

| Hito | Módulos | Resultado demostrable |
|---|---|---|
| H1 «Existe» | F0 + M1 | Binario portable sirviendo galería con datos semilla |
| H2 «Magia local» | M2 + M3 | Enchufas disco → galería catalogada tipo Netflix, offline |
| H3 «Se ve» | M4 | Play real: direct play + remux + fallback |
| H4 «Traga de todo» | M5 + M6 | Caja "pega lo que sea" + salud de fuentes |
| H5 «Sale de tu PC» | M7 | LAN + QR + continuar viendo en el móvil |
| H6 «Circula» | M8 + M9 + M11 | Deseos, .gala, release público v0.1 |

H2 y H3 admiten solape parcial (M3 y M4 no dependen entre sí). M10 avanza en paralelo todo el tiempo.

Regla de alcance: nada entra en MVP1 que no esté en esta lista. Toda idea nueva (habrá muchas) va a un ISSUES-MVP2.md, no al código.

---

## 4. Estructura de repositorio propuesta

```
gala/
├─ cmd/gala/            # main, systray, arranque
├─ internal/
│  ├─ catalog/          # M1: modelo, repositorio, servicio
│  ├─ providers/        # M2, M5: localfs/, m3u/, http/ + interfaz
│  ├─ identify/         # M3: normalizador, tmdb, caché, cola
│  ├─ stream/           # M4: probe, remux, endpoints
│  ├─ ingest/           # M5: detector, extractores, pipeline
│  ├─ health/           # M6
│  ├─ lan/              # M7
│  ├─ galaformat/       # M9
│  ├─ api/              # handlers REST /api/v1
│  └─ store/            # SQLite, migraciones
├─ web/                 # M10: SvelteKit (build → embed)
├─ third_party/ffmpeg/  # binarios por plataforma + licencias
├─ testdata/            # corpus de nombres, M3U de ejemplo, matriz de vídeos
└─ .github/workflows/   # CI multiplataforma + release
```

## 5. Primeras acciones concretas

1. Crear repo privado (público al llegar a H2, con algo que enseñar) con F0.
2. Empezar HOY el corpus de `testdata/`: nombres de archivo reales, M3U reales anonimizadas, y una matriz de clips cortos en los contenedores/códecs de la especificación.
3. Registrar app en TMDB y verificar términos vigentes de la API key embebida (especificación §3.7).
4. Reservar nombre: repo GitHub + dominio + búsqueda rápida de conflictos de marca "GALA" en software de vídeo (hay riesgo de colisión por ser palabra común; tener un plan B de nombre antes de hacerlo público).
