// Package buildinfo guarda la versión del binario, que se fija en la
// compilación con -ldflags (ver .github/workflows/ci.yml).
package buildinfo

import "fmt"

// Se sobreescriben con: -X github.com/bigbendja/gala/internal/buildinfo.Version=...
var (
	Version = "0.1.0-dev"
	Commit  = "desconocido"
	Date    = "desconocida"
)

// String es la salida de `gala --version`.
func String() string {
	return fmt.Sprintf("GALA %s (commit %s, compilado %s)", Version, Commit, Date)
}
