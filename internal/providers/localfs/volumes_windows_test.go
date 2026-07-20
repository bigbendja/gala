//go:build windows

package localfs

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDriveTypeAllowed(t *testing.T) {
	cases := []struct {
		name string
		typ  uint32
		want bool
	}{
		{"fija", windows.DRIVE_FIXED, true},
		{"extraible", windows.DRIVE_REMOVABLE, true},
		{"cdrom", windows.DRIVE_CDROM, false},
		{"red", windows.DRIVE_REMOTE, false},
		{"ramdisk", windows.DRIVE_RAMDISK, false},
		{"sin_raiz", windows.DRIVE_NO_ROOT_DIR, false},
		{"desconocida", windows.DRIVE_UNKNOWN, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := driveTypeAllowed(c.typ); got != c.want {
				t.Errorf("driveTypeAllowed(%d)=%v, esperado %v", c.typ, got, c.want)
			}
		})
	}
}

// TestWinEnumeratorFiltraTiposDeUnidad inyecta las tres syscalls de las que
// depende winEnumerator.Volumes para comprobar, sin tocar hardware real, que
// solo se consultan (y devuelven) unidades fijas y extraíbles (Addendum M2).
func TestWinEnumeratorFiltraTiposDeUnidad(t *testing.T) {
	origDrives, origType, origInfo := getLogicalDrives, getDriveType, getVolumeInfo
	t.Cleanup(func() {
		getLogicalDrives, getDriveType, getVolumeInfo = origDrives, origType, origInfo
	})

	types := map[string]uint32{
		`C:\`: windows.DRIVE_FIXED,
		`D:\`: windows.DRIVE_REMOVABLE,
		`E:\`: windows.DRIVE_CDROM,
		`F:\`: windows.DRIVE_REMOTE,
		`G:\`: windows.DRIVE_RAMDISK,
		`H:\`: windows.DRIVE_NO_ROOT_DIR,
	}

	var infoCalls []string
	getLogicalDrives = func() (uint32, error) {
		var mask uint32
		for root := range types {
			mask |= 1 << uint(root[0]-'A')
		}
		return mask, nil
	}
	getDriveType = func(root string) uint32 { return types[root] }
	getVolumeInfo = func(root string) (uint32, string, error) {
		infoCalls = append(infoCalls, root)
		return uint32(root[0]), "Vol-" + root[:1], nil
	}

	vols, err := winEnumerator{}.Volumes(context.Background(), nil)
	if err != nil {
		t.Fatalf("volumes: %v", err)
	}

	roots := map[string]bool{}
	for _, v := range vols {
		roots[v.Roots[0]] = true
	}
	if !roots[`C:\`] || !roots[`D:\`] {
		t.Fatalf("esperado incluir C: (fija) y D: (extraíble), got %+v", vols)
	}
	if roots[`E:\`] || roots[`F:\`] || roots[`G:\`] || roots[`H:\`] {
		t.Fatalf("no deberían incluirse CD-ROM/red/ramdisk/sin-raíz: %+v", vols)
	}
	if len(vols) != 2 {
		t.Fatalf("esperadas 2 unidades, got %d: %+v", len(vols), vols)
	}

	// GetVolumeInformation (cara y potencialmente lenta en unidades de red)
	// no debe llamarse para los tipos filtrados.
	if len(infoCalls) != 2 {
		t.Fatalf("getVolumeInfo se llamó %d veces, esperado 2: %v", len(infoCalls), infoCalls)
	}
}
