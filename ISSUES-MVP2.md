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
- 2026-07-24: transcode real de vídeo/audio (más allá del remux -c copy de M4)
  para HEVC/AC3/DTS/etc. sin soporte nativo del navegador. M4 los deja en
  Reject + "Abrir en VLC" a propósito (Brief M4, decisión de producto cerrada:
  "Sin transcode de vídeo en MVP1").
- 2026-07-24: transcodificar SOLO el audio cuando el vídeo ya es compatible
  pero el audio no (p. ej. h264+AC3): hoy internal/media/decide.go manda
  directo a Reject aunque remuxear el vídeo y transcodificar solo el audio
  sería técnicamente resoluble con coste de CPU bajo. Anotado también como
  comentario en decide.go.
- 2026-07-24: subtítulos, pistas de audio múltiples, capítulos, miniaturas de
  preview en la barra de progreso, Chromecast, y ajuste de calidad adaptativo
  del reproductor web. Excluidos explícitamente del alcance de M4 (Brief M4
  "NO", ver CLAUDE.md).
- 2026-07-24: sondeo de fuentes `http` con ffprobe (M4 solo sonda localfs;
  los enlaces directos se sirven siempre en modo direct vía redirect 302, sin
  comprobar contenedor/códecs real).
