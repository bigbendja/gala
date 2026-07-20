//go:build !windows

package localfs

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
)

type fallbackEnumerator struct{}

// NewEnumerator devuelve el enumerador de volúmenes de plataformas
// no-Windows: no enumera discos reales (requeriría leer /proc/mounts o
// similar, fuera de alcance de MVP1). Trata cada carpeta registrada que
// exista como su propio "volumen" (Brief M2 §Volúmenes: "suficiente para
// dev en Linux/CI").
func NewEnumerator() Enumerator { return fallbackEnumerator{} }

func (fallbackEnumerator) Volumes(ctx context.Context, folders []string) ([]Volume, error) {
	var vols []Volume
	for _, f := range folders {
		if ctx.Err() != nil {
			return vols, ctx.Err()
		}
		abs, err := filepath.Abs(f)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		vols = append(vols, Volume{ID: hashVolumeID(abs), Label: filepath.Base(abs), Roots: []string{abs}})
	}
	return vols, nil
}

func hashVolumeID(path string) string {
	sum := sha1.Sum([]byte(filepath.Clean(path)))
	return "nix-" + hex.EncodeToString(sum[:])[:8]
}
