package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const stillSize = "w300"

// Episode es un episodio de /tv/{id}/season/{n}.
type Episode struct {
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	StillPath     string `json:"still_path"`
}

// SeasonDetail es el detalle de una temporada (sus episodios con título
// propio, sinopsis y miniatura).
type SeasonDetail struct {
	SeasonNumber int       `json:"season_number"`
	Episodes     []Episode `json:"episodes"`
}

// Episode devuelve el episodio número n, o nil.
func (s SeasonDetail) Episode(n int) *Episode {
	for i := range s.Episodes {
		if s.Episodes[i].EpisodeNumber == n {
			return &s.Episodes[i]
		}
	}
	return nil
}

// seasonCacheID compone una clave numérica única (serie, temporada) para
// tmdb_cache, cuya clave primaria es (tmdb_id, kind, lang): una serie real
// de TMDB nunca llega a las 1000 temporadas.
func seasonCacheID(tvID int64, season int) int64 { return tvID*1000 + int64(season) }

// SeasonDetail obtiene (con caché offline en tmdb_cache) el detalle de una
// temporada de una serie.
func (c *Client) SeasonDetail(ctx context.Context, tvID int64, season int) (*SeasonDetail, error) {
	cacheID := seasonCacheID(tvID, season)
	if c.Cache != nil {
		if payload, ok, err := c.Cache.GetTMDBDetail(ctx, cacheID, "tv_season", "es-ES"); err == nil && ok {
			var d SeasonDetail
			if json.Unmarshal([]byte(payload), &d) == nil {
				return &d, nil
			}
		}
	}

	path := fmt.Sprintf("/tv/%d/season/%d", tvID, season)
	body, err := c.get(ctx, path, url.Values{"language": {"es-ES"}})
	if err != nil {
		return nil, err
	}
	var d SeasonDetail
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("decodificar temporada TMDB: %w", err)
	}

	// Como en detail(): los episodios sin sinopsis en español se completan
	// con la versión en inglés si existe (el título casi siempre sí viene).
	needsFallback := false
	for _, e := range d.Episodes {
		if strings.TrimSpace(e.Overview) == "" || strings.TrimSpace(e.Name) == "" {
			needsFallback = true
			break
		}
	}
	if needsFallback {
		if fbBody, ferr := c.get(ctx, path, url.Values{"language": {"en-US"}}); ferr == nil {
			var fb SeasonDetail
			if json.Unmarshal(fbBody, &fb) == nil {
				for i := range d.Episodes {
					if f := fb.Episode(d.Episodes[i].EpisodeNumber); f != nil {
						if strings.TrimSpace(d.Episodes[i].Overview) == "" {
							d.Episodes[i].Overview = f.Overview
						}
						if strings.TrimSpace(d.Episodes[i].Name) == "" {
							d.Episodes[i].Name = f.Name
						}
					}
				}
			}
		}
	}

	if c.Cache != nil {
		if encoded, merr := json.Marshal(d); merr == nil {
			_ = c.Cache.PutTMDBDetail(ctx, cacheID, "tv_season", "es-ES", string(encoded))
		}
	}
	return &d, nil
}

// DownloadStill descarga la miniatura de un episodio a cache/img/.
func (c *Client) DownloadStill(ctx context.Context, tmdbPath string) (string, error) {
	return c.downloadImage(ctx, tmdbPath, stillSize, "still")
}

// enrichEpisodes pide a TMDB el detalle de las temporadas de los episodios
// que aún llevan el título provisional de M3 ("Serie T1E01") y les pone su
// título real, sinopsis y miniatura. Agrupa por (serie, temporada) para pedir
// cada temporada una sola vez. Es de mejor esfuerzo: sin conexión se detiene
// y se reintenta en la siguiente pasada, y un episodio que TMDB no conoce
// queda como "Episodio N" (deja de ser candidato; no se reintenta sin fin).
func (q *Queue) enrichEpisodes() {
	refs, err := q.repo.EpisodesNeedingDetail(q.ctx)
	if err != nil {
		q.log.Error("identify: leer episodios sin detalle", "err", err)
		return
	}
	type key struct {
		tv     int64
		season int
	}
	seasons := map[key]*SeasonDetail{}
	for _, ref := range refs {
		if q.ctx.Err() != nil {
			return
		}
		k := key{ref.SeriesTMDBID, ref.Season}
		sd, loaded := seasons[k]
		if !loaded {
			sd, err = q.client.SeasonDetail(q.ctx, ref.SeriesTMDBID, ref.Season)
			if err != nil {
				if q.ctx.Err() != nil {
					return
				}
				q.log.Warn("identify: detalle de temporada", "serie", ref.SeriesTMDBID, "temporada", ref.Season, "err", err)
				seasons[k] = nil // no insistir con esta temporada en esta pasada
				continue
			}
			seasons[k] = sd
		}
		if sd == nil {
			continue
		}
		title := fmt.Sprintf("Episodio %d", ref.Episode)
		var overview, still *string
		if ep := sd.Episode(ref.Episode); ep != nil {
			if strings.TrimSpace(ep.Name) != "" {
				title = ep.Name
			}
			overview = strPtr(ep.Overview)
			if name, derr := q.client.DownloadStill(q.ctx, ep.StillPath); derr == nil {
				still = strPtr(name)
			}
		}
		if err := q.repo.UpdateEpisodeDetail(q.ctx, ref.ID, title, overview, still); err != nil {
			q.log.Error("identify: guardar detalle de episodio", "id", ref.ID, "err", err)
		}
	}
}
