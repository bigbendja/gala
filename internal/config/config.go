// Package config carga y guarda la configuración portable de GALA.
// Regla: todo vive junto al ejecutable (gala.json, gala.db, cache/). Nada en AppData.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

	dir string // directorio base (junto al ejecutable), no se serializa
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
