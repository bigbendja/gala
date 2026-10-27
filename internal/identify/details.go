package identify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/bigbendja/gala/internal/catalog"
)

const (
	profileSize      = "w185"
	smallPosterSize  = "w185"
	maxCast          = 12
	maxSimilar       = 10
	maxCrewNames     = 4
	maxDetailAttempt = 3 // reintentos de imágenes de reparto/similares
)

// CastMember es una persona del reparto (o creador, en series) de TMDB.
type CastMember struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
	Order       int    `json:"order"`
}

// CrewMember es una persona del equipo técnico de TMDB.
type CrewMember struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Job   string `json:"job"`
	Dept  string `json:"department"`
	Order int    `json:"-"`
}

// Credits es el bloque credits de TMDB.
type Credits struct {
	Cast []CastMember `json:"cast"`
	Crew []CrewMember `json:"crew"`
}

// Country es un país de producción de TMDB.
type Country struct {
	ISO  string `json:"iso_3166_1"`
	Name string `json:"name"`
}

// FullDetail es el detalle de /movie/{id} o /tv/{id} con créditos, similares
// y clasificación por edad (append_to_response): UNA petición para todo.
type FullDetail struct {
	Detail
	Runtime             int       `json:"runtime"`
	EpisodeRunTime      []int     `json:"episode_run_time"`
	VoteAverage         float64   `json:"vote_average"`
	OriginalLanguage    string    `json:"original_language"`
	Tagline             string    `json:"tagline"`
	ProductionCountries []Country `json:"production_countries"`
	OriginCountry       []string  `json:"origin_country"`
	CreatedBy           []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"created_by"`
	Credits Credits `json:"credits"`
	Similar struct {
		Results []SearchResult `json:"results"`
	} `json:"similar"`
	ReleaseDates struct {
		Results []struct {
			ISO   string `json:"iso_3166_1"`
			Dates []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	ContentRatings struct {
		Results []struct {
			ISO    string `json:"iso_3166_1"`
			Rating string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
}

// FullDetail obtiene (con caché offline en tmdb_cache) el detalle completo
// de una película ("movie") o serie ("tv").
func (c *Client) FullDetail(ctx context.Context, id int64, kind string) (*FullDetail, error) {
	cacheKind := kind + "_full"
	if c.Cache != nil {
		if payload, ok, err := c.Cache.GetTMDBDetail(ctx, id, cacheKind, "es-ES"); err == nil && ok {
			var d FullDetail
			if json.Unmarshal([]byte(payload), &d) == nil {
				return &d, nil
			}
		}
	}
	path := fmt.Sprintf("/movie/%d", id)
	append := "credits,similar,release_dates"
	if kind == "tv" {
		path = fmt.Sprintf("/tv/%d", id)
		append = "credits,similar,content_ratings"
	}
	body, err := c.get(ctx, path, url.Values{"language": {"es-ES"}, "append_to_response": {append}})
	if err != nil {
		return nil, err
	}
	var d FullDetail
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("decodificar detalle completo TMDB: %w", err)
	}
	if c.Cache != nil {
		if encoded, merr := json.Marshal(d); merr == nil {
			_ = c.Cache.PutTMDBDetail(ctx, id, cacheKind, "es-ES", string(encoded))
		}
	}
	return &d, nil
}

// DownloadProfile descarga la foto de una persona del reparto.
func (c *Client) DownloadProfile(ctx context.Context, tmdbPath string) (string, error) {
	return c.downloadImage(ctx, tmdbPath, profileSize, "profile")
}

// DownloadSmallPoster descarga una carátula pequeña (títulos similares).
func (c *Client) DownloadSmallPoster(ctx context.Context, tmdbPath string) (string, error) {
	return c.downloadImage(ctx, tmdbPath, smallPosterSize, "sposter")
}

// crewRoles traduce el trabajo de TMDB (en inglés) al rol que enseña la ficha,
// en el orden en que se muestran.
var crewRoles = []struct {
	label string
	jobs  map[string]bool
}{
	{"Dirección", map[string]bool{"Director": true}},
	{"Guion", map[string]bool{"Screenplay": true, "Writer": true, "Story": true, "Novel": true}},
	{"Música", map[string]bool{"Original Music Composer": true, "Music": true, "Composer": true}},
	{"Fotografía", map[string]bool{"Director of Photography": true}},
}

// BuildDetails convierte la respuesta de TMDB en los datos que guarda GALA
// (sin fotos: las rutas se descargan después) y la clasificación por edad
// numérica ("16") si TMDB la da para España.
func BuildDetails(itemID int64, kind string, fd *FullDetail) (*catalog.Details, string) {
	d := &catalog.Details{ItemID: itemID, TMDBKind: kind, OriginalLanguage: fd.OriginalLanguage, Tagline: strings.TrimSpace(fd.Tagline)}
	if fd.VoteAverage > 0 {
		v := float64(int(fd.VoteAverage*10+0.5)) / 10
		d.VoteAverage = &v
	}
	runtime := fd.Runtime
	if runtime == 0 && len(fd.EpisodeRunTime) > 0 {
		runtime = fd.EpisodeRunTime[0]
	}
	if runtime > 0 {
		d.RuntimeMin = &runtime
	}

	seen := map[string]bool{}
	for _, c := range fd.ProductionCountries {
		name := countryName(c.ISO, c.Name)
		if name != "" && !seen[name] {
			seen[name] = true
			d.Countries = append(d.Countries, name)
		}
	}
	if len(d.Countries) == 0 {
		for _, iso := range fd.OriginCountry {
			if name := countryName(iso, ""); name != "" && !seen[name] {
				seen[name] = true
				d.Countries = append(d.Countries, name)
			}
		}
	}

	cast := append([]CastMember(nil), fd.Credits.Cast...)
	sort.SliceStable(cast, func(i, j int) bool { return cast[i].Order < cast[j].Order })
	for _, m := range cast {
		if strings.TrimSpace(m.Name) == "" {
			continue
		}
		d.Cast = append(d.Cast, catalog.Person{ID: m.ID, Name: m.Name, Character: strings.TrimSpace(m.Character), ProfilePath: m.ProfilePath})
		if len(d.Cast) >= maxCast {
			break
		}
	}

	if kind == "tv" && len(fd.CreatedBy) > 0 {
		g := catalog.CrewGroup{Role: "Creación"}
		for _, p := range fd.CreatedBy {
			if p.Name != "" && len(g.Names) < maxCrewNames {
				g.Names = append(g.Names, p.Name)
			}
		}
		if len(g.Names) > 0 {
			d.Crew = append(d.Crew, g)
		}
	}
	for _, role := range crewRoles {
		g := catalog.CrewGroup{Role: role.label}
		names := map[string]bool{}
		for _, m := range fd.Credits.Crew {
			if role.jobs[m.Job] && m.Name != "" && !names[m.Name] && len(g.Names) < maxCrewNames {
				names[m.Name] = true
				g.Names = append(g.Names, m.Name)
			}
		}
		if len(g.Names) > 0 {
			d.Crew = append(d.Crew, g)
		}
	}

	for _, s := range fd.Similar.Results {
		title := s.EffectiveTitle()
		if s.ID == 0 || strings.TrimSpace(title) == "" {
			continue
		}
		d.Similar = append(d.Similar, catalog.SimilarTitle{
			TMDBID: s.ID, Kind: kind, Title: title, Year: s.Year(), PosterPath: s.PosterPath})
		if len(d.Similar) >= maxSimilar {
			break
		}
	}
	return d, spanishCert(kind, fd)
}

// spanishCert devuelve la edad mínima numérica ("16") de la clasificación
// española, si existe; "TP" u otras etiquetas no numéricas no se muestran.
func spanishCert(kind string, fd *FullDetail) string {
	numeric := func(s string) string {
		s = strings.TrimSpace(s)
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return s
		}
		return ""
	}
	if kind == "tv" {
		for _, r := range fd.ContentRatings.Results {
			if r.ISO == "ES" {
				return numeric(r.Rating)
			}
		}
		return ""
	}
	for _, r := range fd.ReleaseDates.Results {
		if r.ISO != "ES" {
			continue
		}
		for _, rd := range r.Dates {
			if n := numeric(rd.Certification); n != "" {
				return n
			}
		}
	}
	return ""
}

var countryNames = map[string]string{
	"ES": "España", "US": "Estados Unidos", "GB": "Reino Unido", "FR": "Francia", "DE": "Alemania",
	"IT": "Italia", "PT": "Portugal", "MX": "México", "AR": "Argentina", "CO": "Colombia", "CL": "Chile",
	"PE": "Perú", "VE": "Venezuela", "BR": "Brasil", "CA": "Canadá", "AU": "Australia", "NZ": "Nueva Zelanda",
	"IE": "Irlanda", "JP": "Japón", "KR": "Corea del Sur", "CN": "China", "HK": "Hong Kong", "TW": "Taiwán",
	"IN": "India", "RU": "Rusia", "SE": "Suecia", "NO": "Noruega", "DK": "Dinamarca", "FI": "Finlandia",
	"NL": "Países Bajos", "BE": "Bélgica", "CH": "Suiza", "AT": "Austria", "PL": "Polonia", "TR": "Turquía",
	"GR": "Grecia", "IL": "Israel", "IR": "Irán", "EG": "Egipto", "ZA": "Sudáfrica", "NG": "Nigeria",
	"GQ": "Guinea Ecuatorial", "CM": "Camerún", "GA": "Gabón", "SN": "Senegal", "MA": "Marruecos",
	"TH": "Tailandia", "NZL": "Nueva Zelanda", "UA": "Ucrania", "CZ": "Chequia", "HU": "Hungría",
	"RO": "Rumanía", "AE": "Emiratos Árabes Unidos", "SA": "Arabia Saudí",
}

func countryName(iso, fallback string) string {
	if n, ok := countryNames[strings.ToUpper(iso)]; ok {
		return n
	}
	return strings.TrimSpace(fallback)
}

// enrichDetails pide a TMDB el detalle completo (reparto, equipo, similares,
// duración, nota) de las películas y series identificadas que aún no lo
// tienen, descarga las fotos y carátulas y lo guarda. De mejor esfuerzo: sin
// conexión se detiene y reintenta en la siguiente pasada; lo ya cacheado se
// sigue enseñando.
func (q *Queue) enrichDetails() {
	items, err := q.repo.ItemsNeedingDetails(q.ctx)
	if err != nil {
		q.log.Error("identify: leer items sin detalles", "err", err)
		return
	}
	for _, it := range items {
		if q.ctx.Err() != nil || it.TMDBID == nil {
			return
		}
		kind := "movie"
		if it.Kind == catalog.KindSeries {
			kind = "tv"
		}
		fd, err := q.client.FullDetail(q.ctx, *it.TMDBID, kind)
		if err != nil {
			if q.ctx.Err() != nil {
				return
			}
			q.log.Warn("identify: detalle completo", "item_id", it.ID, "err", err)
			if isOffline(err) {
				return
			}
			continue
		}
		d, cert := BuildDetails(it.ID, kind, fd)
		q.fetchDetailImages(d)
		if err := q.repo.UpsertDetails(q.ctx, d); err != nil {
			q.log.Error("identify: guardar detalles", "item_id", it.ID, "err", err)
			continue
		}
		if cert != "" {
			_ = q.repo.SetRatingCert(q.ctx, it.ID, cert)
		}
	}
}

// retryDetailImages reintenta las fotos y carátulas de detalles que se
// quedaron a medias por la red (pocos intentos: el CDN de imágenes es menos
// fiable que la API).
func (q *Queue) retryDetailImages() {
	pending, err := q.repo.DetailsNeedingImages(q.ctx, maxDetailAttempt)
	if err != nil {
		q.log.Error("identify: leer imágenes pendientes", "err", err)
		return
	}
	for _, d := range pending {
		if q.ctx.Err() != nil {
			return
		}
		d.ImageAttempts++
		q.fetchDetailImages(d)
		if err := q.repo.UpsertDetails(q.ctx, d); err != nil {
			q.log.Error("identify: guardar imágenes de detalles", "item_id", d.ItemID, "err", err)
		}
	}
}

// fetchDetailImages descarga lo que falte y marca ImagesPending si algo no
// llegó. Nunca falla: una foto que no baja se enseña como iniciales.
func (q *Queue) fetchDetailImages(d *catalog.Details) {
	missing := false
	for i := range d.Cast {
		p := &d.Cast[i]
		if p.Photo != "" || p.ProfilePath == "" {
			continue
		}
		if q.ctx.Err() != nil {
			missing = true
			continue
		}
		if name, err := q.client.DownloadProfile(q.ctx, p.ProfilePath); err == nil {
			p.Photo = name
		} else {
			missing = true
		}
	}
	for i := range d.Similar {
		s := &d.Similar[i]
		if s.Poster != "" || s.PosterPath == "" {
			continue
		}
		if q.ctx.Err() != nil {
			missing = true
			continue
		}
		if name, err := q.client.DownloadSmallPoster(q.ctx, s.PosterPath); err == nil {
			s.Poster = name
		} else {
			missing = true
		}
	}
	d.ImagesPending = missing
}

// upgradeBackdrops vuelve a descargar en w1280 los fondos que quedaron en la
// resolución antigua (w780): a pantalla completa en el hero se veían borrosos.
func (q *Queue) upgradeBackdrops() {
	items, err := q.repo.ItemsWithSmallBackdrop(q.ctx)
	if err != nil {
		q.log.Error("identify: leer fondos pequeños", "err", err)
		return
	}
	for _, it := range items {
		if q.ctx.Err() != nil || it.TMDBID == nil {
			return
		}
		var path string
		if it.Kind == catalog.KindSeries {
			d, err := q.client.TVDetail(q.ctx, *it.TMDBID)
			if err != nil {
				if isOffline(err) {
					return
				}
				continue
			}
			path = d.BackdropPath
		} else {
			d, err := q.client.MovieDetail(q.ctx, *it.TMDBID)
			if err != nil {
				if isOffline(err) {
					return
				}
				continue
			}
			path = d.BackdropPath
		}
		if path == "" {
			continue
		}
		name, err := q.client.DownloadBackdrop(q.ctx, path)
		if err != nil || name == "" {
			if isOffline(err) {
				return
			}
			continue
		}
		_ = q.repo.SetBackdrop(q.ctx, it.ID, name)
	}
}

func isOffline(err error) bool {
	return errors.Is(err, ErrOffline)
}
