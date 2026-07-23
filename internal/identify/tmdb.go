package identify

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrOffline distingue "TMDB no tiene resultado" de "no hay conexión" o
// TMDB no responde tras agotar reintentos (Brief M3 §2).
var ErrOffline = errors.New("identify: sin conexión con TMDB")

const (
	tmdbBaseURL   = "https://api.themoviedb.org/3"
	tmdbImageBase = "https://image.tmdb.org/t/p"
	posterSize    = "w342"
	backdropSize  = "w780"

	maxAttempts  = 3
	rateInterval = 250 * time.Millisecond // 4 req/s (Brief §2)
)

// Cache abstrae la persistencia de tmdb_cache/search_cache; la implementa
// catalog.Repo (internal/catalog/repo_identify.go) para que este paquete no
// dependa de *sql.DB directamente.
type Cache interface {
	GetTMDBDetail(ctx context.Context, tmdbID int64, kind, lang string) (payload string, ok bool, err error)
	PutTMDBDetail(ctx context.Context, tmdbID int64, kind, lang, payload string) error
	GetSearch(ctx context.Context, queryHash, kind, lang string) (payload string, ok bool, err error)
	PutSearch(ctx context.Context, queryHash, kind, lang, payload string) error
}

// Client es el cliente TMDB: HTTP estándar de la librería estándar, sin SDK
// de terceros (Brief M3 §2).
type Client struct {
	APIKey    string
	CacheDir  string // cache/img/ vive aquí
	Cache     Cache
	HTTP      *http.Client
	BaseURL   string       // sobreescribible en tests (httptest)
	ImageBase string       // sobreescribible en tests
	Log       *slog.Logger // opcional; nil = sin log (Warn tras agotar reintentos de imagen)

	limiterMu sync.Mutex
	lastReq   time.Time
}

// NewClient crea un cliente TMDB. apiKey vacío usa DefaultTMDBKey.
func NewClient(apiKey, cacheDir string, cache Cache) *Client {
	if strings.TrimSpace(apiKey) == "" {
		apiKey = DefaultTMDBKey
	}
	return &Client{
		APIKey: apiKey, CacheDir: cacheDir, Cache: cache,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		BaseURL:   tmdbBaseURL,
		ImageBase: tmdbImageBase,
	}
}

// SearchResult es un candidato de /search/movie o /search/tv.
type SearchResult struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Name          string  `json:"name"`
	OriginalName  string  `json:"original_name"`
	ReleaseDate   string  `json:"release_date"`
	FirstAirDate  string  `json:"first_air_date"`
	PosterPath    string  `json:"poster_path"`
	Popularity    float64 `json:"popularity"`
	Overview      string  `json:"overview"`
}

// EffectiveTitle devuelve title (película) o name (serie), el que exista.
func (r SearchResult) EffectiveTitle() string {
	if r.Title != "" {
		return r.Title
	}
	return r.Name
}

// EffectiveOriginalTitle es el equivalente de EffectiveTitle para el
// título original (Brief §3: el matcher compara contra ambos).
func (r SearchResult) EffectiveOriginalTitle() string {
	if r.OriginalTitle != "" {
		return r.OriginalTitle
	}
	return r.OriginalName
}

// Year devuelve el año de estreno/emisión, 0 si no consta.
func (r SearchResult) Year() int {
	return yearFromDate(firstNonEmpty(r.ReleaseDate, r.FirstAirDate))
}

type searchResponse struct {
	Results []SearchResult `json:"results"`
}

// Genre es un género TMDB (id + nombre).
type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Season es una temporada del detalle de una serie. M3 no llama al
// endpoint por-episodio (coste de red); Brief §3 lo anota como mejora en
// ISSUES-MVP2.md.
type Season struct {
	SeasonNumber int    `json:"season_number"`
	EpisodeCount int    `json:"episode_count"`
	Name         string `json:"name"`
}

// Detail es el detalle de /movie/{id} o /tv/{id}.
type Detail struct {
	ID            int64    `json:"id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title"`
	Name          string   `json:"name"`
	OriginalName  string   `json:"original_name"`
	ReleaseDate   string   `json:"release_date"`
	FirstAirDate  string   `json:"first_air_date"`
	Overview      string   `json:"overview"`
	PosterPath    string   `json:"poster_path"`
	BackdropPath  string   `json:"backdrop_path"`
	Genres        []Genre  `json:"genres"`
	Popularity    float64  `json:"popularity"`
	Seasons       []Season `json:"seasons"`
}

func (d Detail) EffectiveTitle() string {
	if d.Title != "" {
		return d.Title
	}
	return d.Name
}

func (d Detail) EffectiveOriginalTitle() string {
	if d.OriginalTitle != "" {
		return d.OriginalTitle
	}
	return d.OriginalName
}

func (d Detail) Year() int {
	return yearFromDate(firstNonEmpty(d.ReleaseDate, d.FirstAirDate))
}

func (d Detail) GenreNames() []string {
	out := make([]string, 0, len(d.Genres))
	for _, g := range d.Genres {
		out = append(out, g.Name)
	}
	return out
}

// SeasonByNumber devuelve la temporada n del detalle, o nil si no consta.
func (d Detail) SeasonByNumber(n int) *Season {
	for i := range d.Seasons {
		if d.Seasons[i].SeasonNumber == n {
			return &d.Seasons[i]
		}
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func yearFromDate(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}

// SearchMovie busca películas por título (y año, si se conoce).
func (c *Client) SearchMovie(ctx context.Context, query string, year int) ([]SearchResult, error) {
	return c.search(ctx, "movie", query, year)
}

// SearchTV busca series por título (y año de estreno, si se conoce).
func (c *Client) SearchTV(ctx context.Context, query string, year int) ([]SearchResult, error) {
	return c.search(ctx, "tv", query, year)
}

func (c *Client) search(ctx context.Context, kind, query string, year int) ([]SearchResult, error) {
	hash := searchHash(kind, query, year)
	if c.Cache != nil {
		if payload, ok, err := c.Cache.GetSearch(ctx, hash, kind, "es-ES"); err == nil && ok {
			var sr searchResponse
			if jerr := json.Unmarshal([]byte(payload), &sr); jerr == nil {
				return sr.Results, nil
			}
		}
	}

	params := url.Values{"query": {query}, "language": {"es-ES"}}
	if year > 0 {
		if kind == "movie" {
			params.Set("year", strconv.Itoa(year))
		} else {
			params.Set("first_air_date_year", strconv.Itoa(year))
		}
	}
	path := "/search/movie"
	if kind == "tv" {
		path = "/search/tv"
	}

	body, err := c.get(ctx, path, params)
	if err != nil {
		return nil, err
	}
	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("decodificar búsqueda TMDB: %w", err)
	}
	if c.Cache != nil {
		_ = c.Cache.PutSearch(ctx, hash, kind, "es-ES", string(body))
	}
	return sr.Results, nil
}

func searchHash(kind, query string, year int) string {
	h := sha1.Sum([]byte(kind + "|" + strings.ToLower(strings.TrimSpace(query)) + "|" + strconv.Itoa(year)))
	return hex.EncodeToString(h[:])
}

// MovieDetail obtiene (con caché offline en tmdb_cache) el detalle de una
// película.
func (c *Client) MovieDetail(ctx context.Context, id int64) (*Detail, error) {
	return c.detail(ctx, "movie", id)
}

// TVDetail obtiene el detalle de una serie (temporadas incluidas).
func (c *Client) TVDetail(ctx context.Context, id int64) (*Detail, error) {
	return c.detail(ctx, "tv", id)
}

func (c *Client) detail(ctx context.Context, kind string, id int64) (*Detail, error) {
	if c.Cache != nil {
		if payload, ok, err := c.Cache.GetTMDBDetail(ctx, id, kind, "es-ES"); err == nil && ok {
			var d Detail
			if jerr := json.Unmarshal([]byte(payload), &d); jerr == nil {
				return &d, nil
			}
		}
	}

	path := fmt.Sprintf("/movie/%d", id)
	if kind == "tv" {
		path = fmt.Sprintf("/tv/%d", id)
	}

	body, err := c.get(ctx, path, url.Values{"language": {"es-ES"}})
	if err != nil {
		return nil, err
	}
	var d Detail
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("decodificar detalle TMDB: %w", err)
	}

	// Fallback en-US si la sinopsis en español viene vacía (Brief §2).
	if strings.TrimSpace(d.Overview) == "" {
		if fbBody, ferr := c.get(ctx, path, url.Values{"language": {"en-US"}}); ferr == nil {
			var fb Detail
			if json.Unmarshal(fbBody, &fb) == nil && fb.Overview != "" {
				d.Overview = fb.Overview
			}
		}
	}

	if c.Cache != nil {
		if encoded, merr := json.Marshal(d); merr == nil {
			_ = c.Cache.PutTMDBDetail(ctx, id, kind, "es-ES", string(encoded))
		}
	}
	return &d, nil
}

// get hace la petición HTTP con límite de tasa y reintentos con backoff en
// 429/5xx/error de red (Brief §2). Tras agotar los intentos, envuelve el
// error en ErrOffline: la cola de M3 no distingue "TMDB caído" de "sin
// red" a efectos de reintento periódico.
func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	q := cloneValues(params)
	q.Set("api_key", c.APIKey)
	full := c.BaseURL + path + "?" + q.Encode()

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		c.throttle()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			c.backoff(ctx, attempt)
			continue
		}

		body, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			c.backoff(ctx, attempt)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("tmdb %s: %d", path, resp.StatusCode)
			c.backoff(ctx, attempt)
			continue
		default:
			// 4xx (salvo 429): error de API real, no de conectividad.
			return nil, fmt.Errorf("tmdb %s: %d %s", path, resp.StatusCode, string(body))
		}
	}
	return nil, fmt.Errorf("%w: %v", ErrOffline, lastErr)
}

func (c *Client) throttle() {
	c.limiterMu.Lock()
	defer c.limiterMu.Unlock()
	if wait := rateInterval - time.Since(c.lastReq); wait > 0 {
		time.Sleep(wait)
	}
	c.lastReq = time.Now()
}

func (c *Client) backoff(ctx context.Context, attempt int) {
	t := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vals := range v {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

// DownloadPoster descarga (con caché en disco determinista) el póster de
// tmdbPath ("/abc123.jpg") y devuelve el nombre de archivo LOCAL, servido
// luego por GET /api/v1/img/{archivo}. tmdbPath vacío devuelve "".
func (c *Client) DownloadPoster(ctx context.Context, tmdbPath string) (string, error) {
	return c.downloadImage(ctx, tmdbPath, posterSize, "poster")
}

// DownloadBackdrop es el equivalente de DownloadPoster para el fondo.
func (c *Client) DownloadBackdrop(ctx context.Context, tmdbPath string) (string, error) {
	return c.downloadImage(ctx, tmdbPath, backdropSize, "backdrop")
}

// downloadImage descarga tmdbPath a cache/img/ con el mismo esquema de
// reintentos+backoff que get() (Brief §2, 4 req/s, hasta maxAttempts). Antes
// un fallo de red o HTTP transitorio en una sola imagen (429, 5xx, timeout)
// se devolvía sin reintentar, así que en una misma pasada de la cola unos
// pósters se guardaban y otros se perdían en silencio según el azar de la
// red (corrección post-entrega M3, informe de prueba real).
func (c *Client) downloadImage(ctx context.Context, tmdbPath, size, prefix string) (string, error) {
	if strings.TrimSpace(tmdbPath) == "" {
		return "", nil
	}
	ext := filepath.Ext(tmdbPath)
	if ext == "" {
		ext = ".jpg"
	}
	name := fmt.Sprintf("%s_%s_%s%s", prefix, size, imageFileHash(tmdbPath), ext)

	dir := filepath.Join(c.CacheDir, "img")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("crear cache/img: %w", err)
	}
	full := filepath.Join(dir, name)
	if _, err := os.Stat(full); err == nil {
		return name, nil // ya en caché offline
	}

	url := c.ImageBase + "/" + size + tmdbPath
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		c.throttle()
		if err := c.fetchImageOnce(ctx, url, full); err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			c.backoff(ctx, attempt)
			continue
		}
		return name, nil
	}

	if c.Log != nil {
		c.Log.Warn("identify: descargar imagen TMDB tras agotar reintentos",
			"tmdb_path", tmdbPath, "intentos", maxAttempts, "err", lastErr)
	}
	return "", fmt.Errorf("descargar imagen TMDB %s: %w: %v", tmdbPath, ErrOffline, lastErr)
}

// fetchImageOnce es un único intento de descarga+guardado de downloadImage,
// vía archivo temporal + rename atómico para no dejar ficheros a medias si
// falla a mitad de copia.
func (c *Client) fetchImageOnce(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, cerr := io.Copy(out, resp.Body); cerr != nil {
		out.Close()
		os.Remove(tmp)
		return cerr
	}
	out.Close()
	return os.Rename(tmp, dest)
}

func imageFileHash(tmdbPath string) string {
	h := sha1.Sum([]byte(tmdbPath))
	return hex.EncodeToString(h[:])[:12]
}
