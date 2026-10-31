package update

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultRepo    = "bigbendja/gala"
	defaultAPIBase = "https://api.github.com"
	cacheTTL       = 24 * time.Hour
	minForceGap    = time.Minute // entre consultas forzadas, para no machacar GitHub
	maxNotes       = 4000
	sumsAsset      = "SHA256SUMS.txt"
)

// ErrOffline: no se pudo hablar con GitHub (sin red, DNS, 403/5xx...). Nunca
// rompe nada: la UI dice «No se pudo comprobar».
var ErrOffline = errors.New("no se pudo comprobar las actualizaciones")

// Info es el resultado de comprobar si hay una versión nueva.
type Info struct {
	Current         string    `json:"current"`
	Latest          string    `json:"latest"`
	UpdateAvailable bool      `json:"update_available"`
	Notes           string    `json:"notes"`
	PageURL         string    `json:"page_url"`
	AssetName       string    `json:"asset_name"`
	AssetURL        string    `json:"asset_url"`
	AssetSize       int64     `json:"asset_size"`
	Checksum        string    `json:"checksum"`
	CheckedAt       time.Time `json:"checked_at"`
}

// Checker consulta GitHub Releases y cachea el resultado (en memoria y en disco).
type Checker struct {
	Client  *http.Client
	APIBase string
	Repo    string
	GOOS    string
	GOARCH  string
	// Current es la versión en ejecución (buildinfo.Version).
	Current string
	// CacheFile, si no está vacío, persiste el último resultado (sobrevive a reinicios).
	CacheFile string
	// AllowHost decide qué hosts pueden servir el binario; nil = solo GitHub.
	AllowHost func(host string) bool
	Now       func() time.Time

	mu       sync.Mutex
	cached   *Info
	lastPoll time.Time
}

// NewChecker crea el comprobador para esta plataforma.
func NewChecker(goos, goarch, current, cacheFile string) *Checker {
	c := &Checker{
		Client: &http.Client{Timeout: 20 * time.Second}, APIBase: defaultAPIBase, Repo: defaultRepo,
		GOOS: goos, GOARCH: goarch, Current: current, CacheFile: cacheFile, Now: time.Now,
	}
	c.loadCache()
	return c
}

// AssetName es el nombre del binario de Release para una plataforma.
func AssetName(goos, goarch string) string {
	n := "gala-" + goos + "-" + goarch
	if goos == "windows" {
		n += ".exe"
	}
	return n
}

// HostAllowed: solo https y hosts de GitHub (API, descargas y su CDN).
func HostAllowed(host string) bool {
	host = strings.ToLower(host)
	return host == "github.com" || host == "api.github.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

func (c *Checker) allowURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("URL no válida: %q", raw)
	}
	allow := c.AllowHost
	if allow == nil {
		if u.Scheme != "https" {
			return fmt.Errorf("solo se descarga por https: %q", raw)
		}
		allow = HostAllowed
	}
	if !allow(u.Hostname()) {
		return fmt.Errorf("host no permitido para descargar: %q", u.Hostname())
	}
	return nil
}

// Cached devuelve el último resultado conocido sin consultar la red.
func (c *Checker) Cached() (Info, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached == nil {
		return Info{}, false
	}
	info := *c.cached
	info.Current = c.Current
	info.UpdateAvailable = c.newer(info.Latest)
	return info, true
}

func (c *Checker) newer(latest string) bool {
	l := ParseVersion(latest)
	return l.Valid && ParseVersion(c.Current).Compare(l) < 0
}

// Check devuelve si hay una versión nueva. Sin force usa la caché de 24 h; con
// force consulta GitHub salvo que ya se hiciera hace menos de un minuto.
func (c *Checker) Check(ctx context.Context, force bool) (Info, error) {
	c.mu.Lock()
	now := c.Now()
	if c.cached != nil {
		fresh := now.Sub(c.cached.CheckedAt) < cacheTTL
		tooSoon := now.Sub(c.lastPoll) < minForceGap
		if (!force && fresh) || (force && tooSoon) {
			info := *c.cached
			info.Current = c.Current
			info.UpdateAvailable = c.newer(info.Latest)
			c.mu.Unlock()
			return info, nil
		}
	}
	c.lastPoll = now
	c.mu.Unlock()

	info, err := c.fetch(ctx)
	if err != nil {
		return Info{}, err
	}
	info.CheckedAt = c.Now()
	c.mu.Lock()
	c.cached = &info
	c.mu.Unlock()
	c.saveCache(info)
	info.Current = c.Current
	info.UpdateAvailable = c.newer(info.Latest)
	return info, nil
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Body       string    `json:"body"`
	HTMLURL    string    `json:"html_url"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

func (c *Checker) get(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GALA-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOffline, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: GitHub respondió %d", ErrOffline, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOffline, err)
	}
	return b, nil
}

func (c *Checker) fetch(ctx context.Context) (Info, error) {
	body, err := c.get(ctx, fmt.Sprintf("%s/repos/%s/releases/latest", c.APIBase, c.Repo), 1<<20)
	if err != nil {
		return Info{}, err
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return Info{}, fmt.Errorf("%w: respuesta no válida", ErrOffline)
	}
	if rel.Draft || rel.Prerelease || rel.TagName == "" {
		return Info{}, fmt.Errorf("%w: sin versión estable publicada", ErrOffline)
	}
	info := Info{Latest: rel.TagName, PageURL: rel.HTMLURL, Notes: trimNotes(rel.Body)}
	want := AssetName(c.GOOS, c.GOARCH)
	var sumsURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case want:
			info.AssetName, info.AssetURL, info.AssetSize = a.Name, a.URL, a.Size
		case sumsAsset:
			sumsURL = a.URL
		}
	}
	// Sin binario para esta plataforma o sin checksums no se ofrece aplicar:
	// la versión se anuncia, pero Checksum/AssetURL quedan vacíos.
	if info.AssetURL == "" || sumsURL == "" {
		info.AssetURL = ""
		return info, nil
	}
	if err := c.allowURL(info.AssetURL); err != nil {
		return Info{}, err
	}
	if err := c.allowURL(sumsURL); err != nil {
		return Info{}, err
	}
	sums, err := c.get(ctx, sumsURL, 64<<10)
	if err != nil {
		return Info{}, err
	}
	info.Checksum = ParseChecksum(string(sums), want)
	if info.Checksum == "" {
		info.AssetURL = ""
	}
	return info, nil
}

// ParseChecksum busca el SHA256 de name en el contenido de SHA256SUMS.txt
// («<hex>  <fichero>» o «<hex> *<fichero>»).
func ParseChecksum(sums, name string) string {
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name && len(f[0]) == 64 {
			return strings.ToLower(f[0])
		}
	}
	return ""
}

func trimNotes(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	r := []rune(s)
	if len(r) > maxNotes {
		return string(r[:maxNotes]) + "…"
	}
	return s
}

func (c *Checker) loadCache() {
	if c.CacheFile == "" {
		return
	}
	b, err := os.ReadFile(c.CacheFile)
	if err != nil {
		return
	}
	var info Info
	if json.Unmarshal(b, &info) == nil && info.Latest != "" {
		c.cached = &info
	}
}

func (c *Checker) saveCache(info Info) {
	if c.CacheFile == "" {
		return
	}
	if b, err := json.Marshal(info); err == nil {
		_ = os.MkdirAll(filepath.Dir(c.CacheFile), 0o755)
		_ = writeFileAtomic(c.CacheFile, b)
	}
}
