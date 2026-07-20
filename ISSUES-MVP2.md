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
