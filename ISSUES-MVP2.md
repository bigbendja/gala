# Ideas y mejoras pospuestas (no entran en MVP1)

Formato: una línea por idea, con fecha. Se triarán al cerrar MVP1.

- (ejemplo) 2026-07: ...
- 2026-07-29 (Fase 0 tarea 0.4, diagnóstico integral): quitar una carpeta desde
  Ajustes y volver a añadirla más tarde no reconcilia sus items con la carpeta
  reaparecida — crea items duplicados en vez de restaurar la fuente al item
  original (que queda como "fantasma" de 0 fuentes). Reproducido en vivo.
  Detalle y arreglo propuesto en `docs/GALA_Plan_Estabilizacion_Fases.md`,
  sección "Deuda técnica".
- 2026-07-20: soporte opcional de unidades de red como volúmenes (el Enumerator de
  Windows de M2 solo enumera discos fijos y extraíbles; DRIVE_REMOTE queda fuera).
- 2026-07-20: soporte real de archivos multi-título (packs/trilogías/temporadas
  completas empaquetadas en un único fichero, p. ej. "El Padrino [Trilogia]" o
  "Money.Heist.S01.Complete"). M3 los detecta por palabra clave en el nombre
  (internal/identify/match.go, isPackHint) y los capa en confianza MEDIA como
  mucho —nunca se auto-confirman—, pero solo se cataloga UN item por fichero;
  no se abre el archivo para separar títulos/episodios. Precisión acordada con
  el titular del proyecto para M3.
- 2026-07-20: llamar al endpoint por-episodio de TMDB (/tv/{id}/season/{n}/episode/{m})
  para tener título y sinopsis propios de cada episodio. M3 solo usa el
  detalle de la serie (nombre/número de temporada); el episodio se titula
  genéricamente "Serie TxEyy" si TMDB no aporta más. Coste de red evitado a
  propósito en M3 (Brief M3 §3).
- 2026-07-20: certificación por edades (items.rating_cert) vía TMDB. Requiere
  un endpoint adicional (/movie/{id}/release_dates o /tv/{id}/content_ratings)
  no contemplado en el alcance cerrado de M3 (Brief M3 §2 solo lista 5
  endpoints); rating_cert queda sin rellenar tras M3.
- 2026-07-24: transcode real de VÍDEO (HEVC/MPEG2/WMV/etc. sin soporte nativo
  del navegador) queda en Reject en MVP1 (Brief M4, decisión de producto
  cerrada: "Sin transcode de vídeo en MVP1"). NOTA (2026-07-24, addendum
  post-entrega): el caso de audio incompatible con vídeo COMPATIBLE ya no
  entra aquí — se resolvió con remux + recodificación de solo el audio a AAC
  (internal/media/decide.go, AudioTranscode), que es barato en CPU al no
  tocar el vídeo. Esta entrada queda solo para vídeo incompatible de verdad.
- 2026-07-24 (addendum): conversión bajo demanda de vídeo incompatible (HEVC)
  como FUENTE ADICIONAL del mismo item: ofrecer "convertir este archivo" una
  vez, en segundo plano (programable de noche), y registrar el resultado
  como una fuente más — el modelo catálogo/fuentes de M1 ya lo soporta y el
  orden de /play elegiría sola la fuente compatible. Preferible a
  transcodificar en vivo en hardware modesto (CEMAC, mercado objetivo).
- 2026-07-24 (addendum): transcode de vídeo EN VIVO (al pedir play, no
  precalculado) como última opción, solo para equipos con CPU/GPU capaces;
  la conversión bajo demanda de la entrada anterior es preferible por costo
  en hardware modesto.
- 2026-07-24: subtítulos, pistas de audio múltiples, capítulos, miniaturas de
  preview en la barra de progreso, Chromecast, y ajuste de calidad adaptativo
  del reproductor web. Excluidos explícitamente del alcance de M4 (Brief M4
  "NO", ver CLAUDE.md).
- 2026-07-24: sondeo de fuentes `http` con ffprobe (M4 solo sonda localfs;
  los enlaces directos se sirven siempre en modo direct vía redirect 302, sin
  comprobar contenedor/códecs real).
- 2026-07-25: conectores reales de Google Drive y Mega (Brief M5: "son
  posteriores, solo dejar el contrato listo"). M5 reconoce los dominios y crea
  la fuente con provider="drive"|"mega" y health='sin_verificar', marcada
  "pendiente de conector"; el título mostrado hasta entonces es el último
  segmento de la URL (p. ej. "view" para un enlace de Drive típico), no el
  título real del archivo — solo el conector futuro puede resolverlo.
- 2026-07-25: resolvers de hosts de streaming gris (Streamtape, Filemoon,
  Vidoza, Doodstream y similares). Regla de hierro, no entra NUNCA: M5 solo
  los detecta para omitirlos (internal/ingest/classify.go, unsupportedHosts),
  igual que rechazó la insinuación de facilitarlos de cualquier otra forma.
- 2026-07-25: descarga/anclado a local de fuentes remotas (M5 solo cataloga
  el enlace vía provider http; nunca descarga el archivo al disco del
  usuario).
- 2026-07-25: re-sincronización programada de listas M3U remotas (revisar
  periódicamente si una IPTV list añadió/quitó canales). M5 deja
  `ingest_lists.source_url` guardado para cada M3U remota expandida
  (internal/ingest/pipeline.go, fila anidada tipo "m3u_remoto") precisamente
  para que esto sea posible más adelante; no hay ningún worker que la
  re-consulte todavía.
- 2026-07-25: dedupe por tmdb_id en el momento de la ingesta (dos locators
  distintos que resultan ser la misma película). M5 solo deduplica por
  (provider, locator) de forma síncrona; el dedupe por tmdb_id ya lo hace M3
  (ApplyMovieIdentification) cuando la cola de identificación —asíncrona—
  llega a esas fuentes, así que ocurre solo, pero no en el momento de la
  respuesta de /ingest.
- 2026-07-25: heurística de "host de descarga directa simple" del Brief M5
  §2 ("directo: ...o el host es de descarga directa simple"). Implementado
  solo por extensión de archivo conocida; no hay una lista de hosts de
  descarga directa (a diferencia de unsupportedHosts, el Brief no da
  ejemplos concretos y inventar una lista sin criterio claro parecía más
  arriesgado que útil).
- 2026-07-25: reintentos inteligentes por horario para el worker de salud
  (Brief M6, alcance "NO" explícito). M6 solo tiene el re-chequeo periódico
  fijo de 24h y la ventana mínima de 30 min entre chequeos de una misma
  fuente; no hay lógica de "reintenta de madrugada" ni similar.
- 2026-07-25: notificaciones al usuario de cambios de salud (Brief M6,
  alcance "NO" explícito). El estado se expone vía API
  (`/health/summary`, `health_summary` por item) pero nada empuja un aviso
  activo cuando una fuente cambia de estado.
- 2026-07-25: medición fina de ancho de banda para el checker http (Brief
  M6, alcance "NO" explícito). `HTTPChecker` solo mide latencia de
  respuesta (umbral "lenta"), no throughput real.
- 2026-07-25: re-verificación proactiva de listas M3U remotas COMPLETAS
  (Brief M6, alcance "NO" explícito: "solo lo que ya esté como source"). El
  worker de M6 verifica las fuentes ya dadas de alta (una por una, incluida
  cada entrada expandida de una M3U remota); no vuelve a descargar ni
  re-parsear la lista M3U en sí para detectar canales añadidos/quitados
  (eso es la re-sincronización ya anotada en la entrada de M5 sobre
  `ingest_lists.source_url`).
- 2026-07-25 (resuelto 2026-07-26, M10a-fix): filtro `parent_id`/`in_progress`
  en `GET /items` y endpoint de alta/baja en deseos — los tres quedaron
  anotados aquí tras M10a y se cerraron en el mini-brief M10a-fix (ver
  CLAUDE.md, addendum M10a-fix, para el detalle de la implementación).
- 2026-07-25: procedencia M3U de una fuente visible sin petición extra. El
  badge "LISTA·M3U" del mockup no se implementó en M10a: `Source.Provider`
  no distingue "vino de una M3U" de "enlace directo" (ambos son
  provider="http"); esa distinción solo consta en `ingest_lists.type`, y
  cruzarla exigiría una petición por fuente. Si en algún momento importa
  mostrarlo, lo más barato sería que M5 guardase el origen en la propia
  fuente en vez de derivarlo.
- 2026-07-26 (resuelto 2026-07-27, M10c): descartar manualmente un item de
  "Por confirmar" desde la UI (Brief M10b §3, cerrado en Brief M10c §3/§6:
  `DELETE /api/v1/items/{id}` cablea `catalog.Repo.DeleteItem`, ahora con
  borrado en cascada de hijos directos antes de borrar el item — ver
  CLAUDE.md, entrada M10c, para el detalle).
- 2026-07-26: carátulas de candidatos en "Por confirmar" sin conexión
  (Brief M10b §3). `candidatePosterUrl()` pinta el póster de cada candidato
  tirando en vivo de `image.tmdb.org` (nunca se cachea localmente, a
  diferencia del resto de imágenes del catálogo que sí pasan por
  `/img/{archivo}` una vez confirmadas). Es una excepción deliberada a "todo
  funciona offline": la búsqueda que produjo esos candidatos ya fue un paso
  online, así que la vista previa no añade una dependencia de red nueva,
  pero si el usuario está realmente offline al abrir la cola, esos pósters
  no cargan (fallback a inicial del título, ya cubierto por tests).
- 2026-07-27: "ruta de datos" en Ajustes → Sistema (Brief M10c §2 la pedía
  en la lista de "solo lectura"). No se expone: no hay endpoint que la
  devuelva (`config.BaseDir()`/`DBPath()`/`CacheDir()` son funciones internas
  de Go, no API), y el brief solo autorizaba dos endpoints nuevos ("nada
  más"). Añadir uno solo para esto también implicaría, de cara a M7 (LAN),
  filtrar rutas absolutas del servidor a cualquier cliente de la red local
  — mejor decidirlo junto con el diseño de Modo LAN que colarlo aquí. Se
  muestran versión y disponibilidad de ffmpeg (vía el `GET /ping` ya
  existente, con un campo añadido) y el enlace al repo; nada más.
- 2026-07-27: "Listas" (`/listas`, Brief M10c §4) no cruza cada lista con
  sus items. Es la misma limitación que ya constaba para el badge
  "LISTA·M3U" de M10a: `ingest_lists` no tiene una consulta "dame los items
  de esta lista" expuesta por la API, y el brief pedía "vista mínima
  honesta (no inventar)" en vez de un endpoint nuevo. La vista actual solo
  lista nombre/tipo/recuento/fecha de sincronización; cada fila no enlaza a
  ningún sitio.
- 2026-07-25: puntuación TMDB (`vote_average`) y duración (`media_info`) en
  la ficha/hero. Brief M10a menciona "★ 7,8" y "duración si existe en
  media_info" como en el mockup; ninguno de los dos viaja hoy en
  `GET /items`/`GET /items/{id}` (M3 no persiste vote_average; duration_s
  vive en `media_info`, que solo expone `GET /play/{item_id}` — un endpoint
  de decisión de reproducción, fuera de alcance para una ficha de solo
  lectura en M10a). Se omitieron ambos campos en vez de inventarlos o
  acoplar la ficha a `/play` antes de que exista el reproductor (M10b).
- 2026-10-27 (M7): progreso compartido en LAN (requiere decidir: por
  dispositivo vs por hogar vs por título). La identidad es la cookie
  `gala_device` (una por navegador/dispositivo, decisión cerrada del brief
  M7), así que el progreso de un dispositivo NO aparece en otro: empezar en
  el PC y seguir en el móvil exige un modelo distinto. Decidido por el
  titular: NO entra en M7; tarea pequeña propia, a decidir con calma (¿si
  otra persona pausa una peli en su móvil, debe aparecer a medias en el PC?).
- 2026-10-27 (M7): fantasmas anteriores al arreglo de "carpeta duplicada".
  Los items que ya quedaron duplicados (uno con 0 fuentes, oculto, y otro
  nuevo con la fuente) antes de `0005_removed_sources.sql` no tienen rastro
  y no se reconcilian solos. Limpieza posible: borrar items sin fuentes ni
  hijos cuyo `tmdb_id` ya tenga otro item con fuentes (conservador) — no se
  hizo para no borrar nada del usuario sin pedirlo.
- 2026-10-27 (M7): lista de dispositivos conectados en el panel de Modo LAN
  (opcional en el brief). Se omitió: haría falta guardar `last_seen` por
  `gala_device` y una consulta nueva; no es barato.
- 2026-10-27 (M7): descubrimiento por nombre (mDNS, `gala.local`) — mejora
  opcional; el QR y la URL con IP cubren el caso.
- 2026-10-27 (M7): TLS en LAN. El tráfico va en HTTP plano dentro de la red
  local (el PIN viaja sin cifrar al teclearlo). Aceptado para MVP1 con aviso
  en el panel; HTTPS exigiría un certificado que el móvil acepte sin avisos.
- 2026-10-27 (M7): perfil de transcode 480p para móviles de gama baja por
  ancho de banda (en la especificación); no se hizo.
- 2026-10-27 (M7): las sesiones del PIN viven en memoria: reiniciar GALA
  obliga a los móviles a teclear el PIN otra vez. Persistirlas en la BD si
  molesta en uso real.
