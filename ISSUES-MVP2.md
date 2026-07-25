# Ideas y mejoras pospuestas (no entran en MVP1)

Formato: una línea por idea, con fecha. Se triarán al cerrar MVP1.

- (ejemplo) 2026-07: ...
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
