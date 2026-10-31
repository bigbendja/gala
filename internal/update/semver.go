package update

import (
	"strconv"
	"strings"
)

// Version es una versión mayor.menor.parche. Los sufijos (-rc1, -3-gabc123,
// -dirty, -dev) no se comparan: una compilación de desarrollo se trata como la
// versión base, y lo publicado como «estable» no lleva sufijo.
type Version struct {
	Major, Minor, Patch int
	Valid               bool
	Pre                 bool // llevaba sufijo (prerelease o compilación intermedia)
}

// ParseVersion acepta "v0.9.0", "0.9.0", "v0.9.0-3-gabc1234-dirty"...
func ParseVersion(s string) Version {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "v"))
	core, suffix, _ := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return Version{}
		}
		n[i] = v
	}
	return Version{Major: n[0], Minor: n[1], Patch: n[2], Valid: true, Pre: suffix != ""}
}

// Compare devuelve -1, 0 o 1. Una versión no válida es menor que cualquiera
// válida (una compilación sin versión siempre puede actualizarse).
func (v Version) Compare(o Version) int {
	switch {
	case !v.Valid && !o.Valid:
		return 0
	case !v.Valid:
		return -1
	case !o.Valid:
		return 1
	}
	for _, d := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	return 0
}
