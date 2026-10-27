# GALA — Plan de estabilización y evolución
**Julio 2026 · Estado: backend M0–M6 + frontend M10(a/b/c) implementados. Quedan bugs abiertos, el módulo M7 (LAN), M8/M9, el release M11, y mejoras de producto.**

Este plan recoge TODO lo pendiente y disperso y lo ordena en fases ejecutables. Cada
fase se cierra antes de pasar a la siguiente. Claude Code implementa una tarea a la
vez; el usuario verifica en vivo (con gala.exe real y su key TMDB) antes de dar por
cerrada cada tarea. Regla: la CI solo cuenta como "verde" si el usuario lo confirma en
github.com/bigbendja/gala/actions, no si se reporta a ciegas.

---

## FASE 0 — Estabilizar lo que está roto (BLOQUEANTE, va primero)
Objetivo: dejar GALA sin nada roto ni inalcanzable. Nada nuevo hasta cerrar esto.

**Estado (2026-07-29): 0.1–0.5 implementadas y verificadas en vivo por Claude Code
contra `gala.exe` real (incl. la URL real de Big Buck Bunny). Pendiente de que el
usuario confirme el run verde en github.com/bigbendja/gala/actions antes de dar la
fase por cerrada del todo — ver regla de CI del encabezado de este documento.**

**0.1 — CI en verde de verdad. ✅ implementada.**
- Arreglar el test frágil `TestLocatePrefersFfmpegSubfolder` (falla solo en Linux:
  intenta crear un archivo "ffmpeg" donde ya hay un directorio; usar t.TempDir bien
  aislado, sin colisión archivo/directorio). El código de localización es correcto;
  el defecto es del test.
  → Hecho: el test original creaba un archivo "ffmpeg" en la raíz del directorio de
  prueba justo donde ya existía la carpeta "ffmpeg/" (mismo nombre, distinto
  significado) — en Linux/macOS el binario raíz se llama literalmente "ffmpeg" (sin
  `.exe`), así que archivo y carpeta colisionaban. Ese archivo raíz ni falta: la
  función real (`locateBinary`) comprueba la subcarpeta PRIMERO y devuelve en cuanto
  la encuentra. Se quitó esa parte del test, no se tocó el código de producción.
- En ci.yml: construir el frontend (npm ci && npm run build en web/) ANTES del
  go build/test (go:embed necesita web/dist); tests que requieran ffmpeg/red → t.Skip
  limpio; actualizar actions deprecadas (checkout/setup-go/setup-node a v4/v5).
  → Ya estaba así (esta parte del diagnóstico original estaba desactualizada):
  ci.yml ya construye el frontend antes de cualquier paso de Go en ambos jobs, y ya
  usa actions/checkout@v4, actions/setup-go@v5, actions/setup-node@v4.
- Cierre: el usuario ve el run VERDE en la web. **Pendiente de confirmación del
  usuario** (Claude Code no tiene forma de comprobar Actions por sí mismo).

**0.2 — Acceso a Ajustes. ✅ ya estaba resuelto (diagnóstico desactualizado).**
- La página /ajustes existe pero no hay forma de llegar desde la UI. Añadir acceso
  visible (engranaje en la topbar junto a Modo LAN / Añadir; también en el menú
  hamburguesa móvil).
  → El engranaje ⚙️ ya existe en `Topbar.svelte` (desktop y menú móvil) desde M10c;
  se verificó en vivo que el enlace funciona.

**0.3 — Botones muertos del modal de ingesta. ✅ ya estaba resuelto (diagnóstico desactualizado).**
- "Elegir carpeta", "Escanear disco USB", "Importar .gala" son clicables sin efecto.
  Conectar "Elegir carpeta" al flujo real de añadir carpeta (o llevar a Ajustes ›
  Carpetas); "Importar .gala" y "Escanear USB" → deshabilitados con tooltip
  "próximamente" (son M9/futuro). Nada de botones muertos.
  → "Elegir carpeta" y "Escanear disco" ya enlazan a /ajustes desde M10c (verificado
  en vivo). "Importar .gala" sigue deshabilitado con tooltip "M9", correcto.

**0.4 — Bug: enlace directo ingerido no aparece en la galería. ✅ implementada y verificada en vivo.**
- Síntoma: pegar un .mp4 directo dice "añadido" (y luego "duplicado"), pero no se ve.
- Diagnóstico: el item SÍ se creaba y persistía (confirmado con el propio caso real
  del usuario, id 28), pero como `kind=other`/`confirmed=false`, y "Tu galería"
  exigía `confirmed=true` Y `kind IN (movie,series)` — un enlace directo casi nunca
  tiene match en TMDB, así que ambos filtros lo escondían siempre.
- Corrección: `web/src/lib/gallery.ts` (`galleryVisible`) — un item `other` se ve en
  cuanto tiene ≥1 fuente (`source_count > 0`), sin exigir `confirmed`, salvo que esté
  marcado `junk`. `movie`/`series` se comportan exactamente igual que antes (sin
  cambios de riesgo ahí). `/play` ya funcionaba para items sin confirmar — no hacía
  falta tocarlo (test nuevo: `TestHandlePlayWorksForUnconfirmedItem`).
- "Por confirmar" gana una 4ª acción real: "Dejar sin identificar" (además de
  candidato/título manual/descartar) — reutiliza `ApplyManualConfirm` con el nombre
  de archivo ya limpio, sin backend nuevo. Además, los items en `identify_status=
  error` (típicamente clave TMDB inválida) pasaban a estar atascados para siempre
  como "procesándose en segundo plano" sin ninguna acción disponible; ahora son
  tan accionables como los `review`, con el motivo del error en español.
- Efecto colateral encontrado y corregido durante la verificación: la tarjeta
  "wish" (Deseos) decía siempre "Sin fuente · buscando…" aunque el item deseado
  YA tuviera una fuente real y reproducible (M10a-fix permite desear cualquier item
  existente, no solo aspiracionales) — mentía sobre el estado. Corregido en
  `Card.svelte`: ese texto solo aparece si `source_count === 0` de verdad.
- Verificado en vivo con la URL real
  `https://test-videos.co.uk/vids/bigbuckbunny/mp4/h264/360/Big_Buck_Bunny_360_10s_1MB.mp4`
  (la misma que dio pie al bug): ingerida por la UI real, aparece en "Tu galería"
  con "Sin identificar", se reproduce de verdad (redirect 302 real a la URL,
  `Content-Type: video/mp4`) sin confirmar, y "Dejar sin identificar" la resuelve.
- Tests: `web/src/lib/gallery.test.ts`, `web/src/routes/pendientes/page.test.ts`
  (dejar sin identificar + error accionable), `internal/api/stream_test.go`
  (`TestHandlePlayWorksForUnconfirmedItem`), `Card.svelte.test.ts` (wish con/sin
  fuente).

**0.5 — Buscador y navegación de secciones. ✅ ya estaba resuelto (M10c) — reverificado en vivo.**
- Confirmar que quedan operativos tras 0.2 (eran parte del fix de M10c ya escrito):
  buscador clicable y funcional; Películas/Series/Deseos/Listas navegables.
  → Verificado en vivo: buscar "buck bunny" encuentra el item sin identificar desde
  el desplegable; Películas/Series/Deseos/Listas navegan correctamente.

**Bug encontrado en el diagnóstico integral, NO corregido en esta sesión — anotado
en ISSUES-MVP2.md, requiere diseño propio, no es parte de 0.4:** quitar una carpeta
desde Ajustes borra sus fuentes pero conserva el item como un "fantasma" de 0
fuentes (ya documentado como decisión deliberada); si luego se vuelve a añadir la
MISMA carpeta, no se reconcilia con ese fantasma — se crea un item NUEVO duplicado
en vez de restaurarle la fuente al original. Reproducido en vivo (quitar → volver a
añadir `media2` → dos items "otra pelicula", uno con 0 fuentes y otro con 1). El
fantasma de 0 fuentes ya no aparece en la galería gracias al fix de 0.4
(`source_count > 0`), así que el síntoma visible se atenúa, pero el duplicado en sí
sigue ahí.

Cierre de FASE 0: CI verde confirmada por el usuario; Ajustes accesible; sin botones
muertos; un enlace directo pegado aparece y se reproduce; buscador y navegación
funcionan. Commit(s) y push.

---

## FASE 1 — Rediseño de la Home (tipo Netflix)
Objetivo: la pantalla de inicio que el usuario quiere. NO empezar sin mockup acordado.

**1.1 — Mockup nuevo de la home (lo acuerda el usuario con su asesor de producto
antes de tocar código).** Debe definir:
- Hero-slider superior (varios títulos destacados en carrusel) con filtros por tipo
  de contenido y categoría.
- Debajo, VARIAS secciones en carrusel, no solo "Tu galería": Películas, Series,
  Documentales, Mis Listas, y filas por categoría/género.
- Cada carrusel navegable con botones (flechas ‹ ›) además del scroll/teclado.
- Coherencia con el sistema de diseño ya existente (tokens, fuentes, tono cálido).

**1.2 — Implementación del rediseño** contra el mockup 1.1, reusando componentes de
M10 (Card, HealthDot, api client). Requisitos:
- Distinguir Documentales: hoy no hay señal fiable de "documental" en el modelo;
  decidir origen (género TMDB "Documentary" → sección) y documentarlo.
- Los carruseles con botones deben funcionar en desktop y táctil (móvil).
- Rendimiento: mantener virtualización/lazy de M10a; muchas filas no deben cargar
  miles de imágenes de golpe.
- Filtros del hero y por categoría: definir si son cliente (sobre lo ya cargado) o
  servidor (nuevos query params). Preferible servidor para escalar; anotar endpoints
  nuevos si hacen falta (p. ej. `?genre=`, ya existe; `?kind=`, ya existe).

Cierre de FASE 1: la home nueva funciona, es fluida en desktop y móvil, y el usuario
la aprueba contra su mockup. Capturas. CI verde. Commit/push.

---

## FASE 2 — Modo LAN (M7)
Objetivo: el diferencial de mercado — "Netflix de la casa" por wifi. Se hace DESPUÉS
de estabilizar y rediseñar, para probarlo sobre una UI sólida y responsive.
- Toggle bind 127.0.0.1 ↔ 0.0.0.0 (la píldora "Modo LAN" ya está en la topbar).
- QR con la IP local para conectar móviles sin instalar nada.
- PIN opcional de 4 dígitos.
- Progreso "Continuar viendo" por dispositivo (cookie), ya soportado en backend.
- Prueba real en un móvil de gama baja por wifi: reproducir local y remux.
- Seguridad: no exponer rutas del sistema de archivos en la UI accesible por LAN
  (ya anotado en M10c). Revisar SSRF y validación de rutas con bind abierto.
(Brief detallado de M7 se escribe al llegar aquí.)

**Estado (2026-10-27): M7 implementado (toggle de bind, QR, PIN con rate-limit,
guardián de rutas, UI, y el arreglo del bug de carpeta duplicada, que se movió
aquí desde la deuda técnica). Pendiente: prueba en un móvil real por wifi y CI
verde confirmada por el titular. Ver CLAUDE.md, entrada M7.**

Cierre de FASE 2: abrir GALA en el móvil del salón por wifi y ver una peli, sin
instalar nada en el móvil.

---

## FASE 3 — Contenido remoto legítimo: provider Google Drive / Mega (parte de M-providers)
Objetivo: resolver el caso de uso REAL y limpio del usuario (compartir/ver su propio
contenido en Drive/Mega: el documental comprado, la carpeta que le pasaron).
- Provider Drive con OAuth del usuario (cada usuario conecta SU cuenta).
- Provider Mega (descifrado en cliente; más complejo — evaluar esfuerzo).
- Se integran en el pipeline de fuentes ya existente (M5 ya los detecta y marca
  "pendiente de conector").
- Esto es contenido del propio usuario: legítimo y defendible. NO es scraping de
  hosts.
(Brief detallado al llegar aquí. Prioridad sobre Fase 4 si el usuario lo prefiere.)

---

## FASE 4 — Formato .gala e importación (M9)
Objetivo: exportar/importar biblioteca o colecciones como archivo `.gala` compartible.
- Export (con/sin rutas locales), import con sanitización, entradas ajenas →
  "pendiente de fuente".
- Sirve además como VÍA DE IMPORTACIÓN LIMPIA: cualquier herramienta EXTERNA del
  usuario (incluida una extensión de navegador propia, proyecto SEPARADO y con otro
  nombre — ver Anexo) puede generar un `.gala` con enlaces ya resueltos, y GALA lo
  importa como cualquier otro. GALA no scrapea nada; solo recibe un archivo con
  enlaces directos.
(Brief detallado al llegar aquí.)

---

## FASE 5 — Release portable (M11)
Objetivo: el .exe autónomo para el usuario final (el "primo de Malabo").
- Empaquetar ffmpeg (build LGPL, no la GPL de desarrollo) dentro del binario.
- Ejecutable único portable, sin necesidad de Go ni ffmpeg instalados.
- Instalador/carpeta portable, checksums, GitHub Releases automatizado.
- Licencia: sustituir el LICENSE placeholder por AGPL-3.0 completo; CLA operativo.
- Prueba en una máquina limpia (solo el .exe) de todo el ciclo.

---

## Deuda técnica y mejoras anotadas (intercalar donde encaje, no bloquean)
- 2026-07-29 (diagnóstico integral de la tarea 0.4): quitar una carpeta desde
  Ajustes y volver a añadir la MISMA carpeta más tarde crea un item duplicado en
  vez de reconciliarse con el original (que queda como "fantasma" de 0 fuentes,
  ya oculto de la galería gracias al fix de 0.4 pero no eliminado). Reproducido en
  vivo. Arreglo real: en `internal/scan/scan.go` (`syncBatch`), antes de crear un
  item nuevo para un locator sin fuente existente, buscar si hay un item huérfano
  (0 fuentes, mismo título/carpeta) que reconciliar. Requiere diseño propio (qué
  heurística de "es el mismo item" es segura) — no se ha tocado en esta sesión.
- Descartar pendientes: endpoint DELETE + UI (ya en el fix de M10c; verificar que
  entra en Fase 0).
- Aviso claro de "clave TMDB inválida" en Ajustes (ya en M10c; verificar).
- Búsqueda-para-desear en TMDB (M8): añadir a deseos títulos aún sin fuente.
- Rendimiento de la cola de identificación (paralelizar si con biblioteca real se hace
  lento; hoy en observación).
- Badge "LISTA·M3U" y cross-link de /listas a sus items (requiere cruzar ingest_lists;
  menor).
- Transcode de solo-audio ya resuelto; transcode de vídeo (HEVC) como "convertir una
  vez → fuente adicional" (MVP2).

---

## ANEXO — Regla del muro (extensión de navegador personal del usuario)
El usuario puede construir para SÍ MISMO una extensión que extraiga enlaces de vídeo y
prepare un `.gala` para su instancia privada. Condición innegociable para no dañar
GALA como producto:
- Proyecto SEPARADO: otro repo (o ninguno), OTRO nombre, sin "GALA" en él.
- El repo/web/marca/comunidad de GALA NUNCA lo menciona, enlaza ni sugiere.
- GALA permanece neutral: solo recibe enlaces directos / archivos .gala; nunca
  scrapea ni resuelve hosts. La "extracción" vive entera fuera, en la extensión.
- Con ese muro, ambas cosas coexisten sin contaminarse (modelo yt-dlp). Sin el muro,
  GALA-producto queda inservible como proyecto abierto/comercial.

---

### Orden de ejecución recomendado
FASE 0 (bloqueante) → FASE 1 (home) → FASE 2 (LAN) → FASE 3 (Drive/Mega) →
FASE 4 (.gala) → FASE 5 (release).
Fase 3 y 4 pueden intercambiarse según prioridad del usuario. Fase 0 nunca se salta.
