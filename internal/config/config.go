// Package config carga y guarda la configuración portable de GALA.
// Regla: todo vive junto al ejecutable (gala.json, gala.db, cache/). Nada en AppData.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Config es la configuración persistente de la aplicación.
type Config struct {
	// Port es el puerto HTTP local. 0 no es válido; por defecto 7420.
	Port int `json:"port"`
	// Bind es la dirección de escucha. "127.0.0.1" (solo este PC) o "0.0.0.0" (Modo LAN).
	Bind string `json:"bind"`
	// MediaFolders son las carpetas registradas para escaneo (M2).
	MediaFolders []string `json:"media_folders"`
	// LanPIN, si no está vacío, exige PIN en Modo LAN (M7). Vacío = sin PIN.
	LanPIN string `json:"lan_pin"`
	// TMDBAPIKey, si no está vacío, sustituye a identify.DefaultTMDBKey (M3).
	TMDBAPIKey string `json:"tmdb_api_key"`
	// HomeHeroHidden oculta el bloque hero de la Home (Ajustes › Inicio). La
	// polaridad es "oculto" para que un gala.json antiguo, sin el campo,
	// siga mostrando el hero como siempre.
	HomeHeroHidden bool `json:"home_hero_hidden"`
	// AdminLocalhostBypass: localhost es admin sin clave (por defecto sí; nil
	// = true). Con false, localhost también necesita sesión SI ya hay clave
	// puesta; hacerlo detrás de un reverse proxy en el mismo equipo, donde todo
	// el tráfico parecería loopback.
	AdminBypassSetting *bool `json:"admin_localhost_bypass,omitempty"`
	// SessionDays es la duración (deslizante) de la sesión de administración;
	// 0 = 30 días.
	SessionDaysSetting int `json:"session_days,omitempty"`

	dir string // directorio base (junto al ejecutable), no se serializa

	// mu protege Bind y LanPIN (M7): los lee el guardián LAN en CADA petición
	// mientras un handler puede estar cambiándolos. El resto de campos
	// conservan su acceso sin mutex (ver CLAUDE.md, Ajustes).
	mu sync.RWMutex
}

// Loopback y AllInterfaces son los dos valores válidos de Bind.
const (
	BindLoopback      = "127.0.0.1"
	BindAllInterfaces = "0.0.0.0"
)

// LanState devuelve el bind y el PIN vigentes (M7).
func (c *Config) LanState() (bind, pin string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Bind, c.LanPIN
}

// LanEnabled indica si el Modo LAN está activo (bind abierto).
func (c *Config) LanEnabled() bool {
	bind, _ := c.LanState()
	return bind == BindAllInterfaces
}

// SetBind cambia Bind y lo persiste; si no se puede guardar, deja el valor
// anterior.
func (c *Config) SetBind(bind string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev := c.Bind
	c.Bind = bind
	if err := c.save(); err != nil {
		c.Bind = prev
		return err
	}
	return nil
}

// SetLanPIN cambia el PIN ("" lo quita) y lo persiste; si no se puede
// guardar, deja el valor anterior.
func (c *Config) SetLanPIN(pin string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev := c.LanPIN
	c.LanPIN = pin
	if err := c.save(); err != nil {
		c.LanPIN = prev
		return err
	}
	return nil
}

// AdminLocalhostBypass dice si localhost es admin sin sesión (por defecto sí).
func (c *Config) AdminLocalhostBypass() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AdminBypassSetting == nil || *c.AdminBypassSetting
}

// SessionDays devuelve la duración de la sesión de administración en días.
func (c *Config) SessionDays() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.SessionDaysSetting <= 0 {
		return 30
	}
	return c.SessionDaysSetting
}

// HomeHeroEnabled dice si la Home enseña el bloque hero (por defecto sí).
func (c *Config) HomeHeroEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.HomeHeroHidden
}

// SetHomeHero muestra u oculta el hero de la Home y lo persiste; si no se
// puede guardar, deja el valor anterior.
func (c *Config) SetHomeHero(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev := c.HomeHeroHidden
	c.HomeHeroHidden = !enabled
	if err := c.save(); err != nil {
		c.HomeHeroHidden = prev
		return err
	}
	return nil
}

// Default devuelve la configuración por defecto.
func Default() *Config {
	return &Config{Port: 7420, Bind: "127.0.0.1", MediaFolders: []string{}}
}

// NewAt devuelve la configuración por defecto anclada a dir, sin tocar disco.
// Para tests: producción siempre usa Load() (junto al ejecutable).
func NewAt(dir string) *Config {
	c := Default()
	c.dir = dir
	return c
}

// BaseDir devuelve el directorio del ejecutable, donde vive todo lo persistente.
func BaseDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// Load lee gala.json del directorio del ejecutable; si no existe, lo crea con valores por defecto.
func Load() (*Config, error) {
	dir, err := BaseDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "gala.json")

	cfg := Default()
	cfg.dir = dir

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := cfg.Save(); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = 7420
	}
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	return cfg, nil
}

// Save escribe la configuración en gala.json de forma atómica.
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.save()
}

// save escribe sin tomar el mutex: el llamador ya lo tiene.
func (c *Config) save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(c.dir, "gala.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DBPath devuelve la ruta de la base de datos.
func (c *Config) DBPath() string { return filepath.Join(c.dir, "gala.db") }

// CacheDir devuelve (creándolo si falta) el directorio de caché de imágenes/metadatos.
func (c *Config) CacheDir() (string, error) {
	d := filepath.Join(c.dir, "cache")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}
