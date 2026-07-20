//go:build windows

package localfs

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows"
)

type winEnumerator struct{}

// NewEnumerator devuelve el enumerador de volúmenes real de Windows: unidades
// existentes y su número de serie vía GetVolumeInformation (Brief M2 §Volúmenes).
func NewEnumerator() Enumerator { return winEnumerator{} }

// getLogicalDrives, getDriveType y getVolumeInfo envuelven las syscalls de
// Windows en variables (no llamadas directas) para poder sustituirlas en
// tests por datos falsos, sin depender del hardware real de la máquina de
// desarrollo/CI (Addendum M2: "enumerador inyectable").
var (
	getLogicalDrives = func() (uint32, error) { return windows.GetLogicalDrives() }

	getDriveType = func(root string) uint32 {
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			return windows.DRIVE_UNKNOWN
		}
		return windows.GetDriveType(rootPtr)
	}

	getVolumeInfo = func(root string) (serial uint32, label string, err error) {
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			return 0, "", err
		}
		var volName [261]uint16
		var fsName [261]uint16
		var maxComponentLen, fsFlags uint32
		if err := windows.GetVolumeInformation(rootPtr, &volName[0], uint32(len(volName)),
			&serial, &maxComponentLen, &fsFlags, &fsName[0], uint32(len(fsName))); err != nil {
			return 0, "", err
		}
		return serial, windows.UTF16ToString(volName[:]), nil
	}
)

func (winEnumerator) Volumes(ctx context.Context, _ []string) ([]Volume, error) {
	mask, err := getLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("enumerar unidades: %w", err)
	}
	var vols []Volume
	for d := 0; d < 26; d++ {
		if ctx.Err() != nil {
			return vols, ctx.Err()
		}
		if mask&(1<<uint(d)) == 0 {
			continue
		}
		root := string(rune('A'+d)) + `:\`

		// Solo discos fijos y extraíbles: CD-ROM, unidades de red y ramdisk
		// quedan fuera de MVP1 (Addendum M2) y GetVolumeInformation sobre una
		// unidad de red inalcanzable puede tardar varios segundos.
		if !driveTypeAllowed(getDriveType(root)) {
			continue
		}

		serial, label, err := getVolumeInfo(root)
		if err != nil {
			// Unidad sin medio (p. ej. lector extraíble vacío) u otro fallo
			// transitorio: se omite, no aborta la enumeración.
			continue
		}
		vols = append(vols, Volume{ID: fmt.Sprintf("win-%08X", serial), Label: label, Roots: []string{root}})
	}
	return vols, nil
}

// driveTypeAllowed indica si un tipo de unidad de Windows entra en el
// escaneo de volúmenes de MVP1 (Addendum M2).
func driveTypeAllowed(t uint32) bool {
	switch t {
	case windows.DRIVE_FIXED, windows.DRIVE_REMOVABLE:
		return true
	default:
		return false
	}
}
