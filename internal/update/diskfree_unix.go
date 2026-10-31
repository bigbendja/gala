//go:build !windows

package update

import "syscall"

// freeBytes devuelve los bytes que un proceso sin privilegios puede escribir
// todavía en el sistema de ficheros de dir.
func freeBytes(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
