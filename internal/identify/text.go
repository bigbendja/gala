package identify

import (
	"strings"
	"unicode"
)

// foldASCII devuelve s en minúsculas y sin diacríticos (á→a, ñ→n, ç→c...),
// para comparaciones insensibles a mayúsculas/acentos (normalizador y
// matcher). No usa golang.org/x/text (dependencia nueva innecesaria para el
// alfabeto latino que cubre el corpus).
func foldASCII(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		r = unicode.ToLower(r)
		if repl, ok := accentFold[r]; ok {
			b.WriteRune(repl)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var accentFold = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ñ': 'n', 'ç': 'c', 'ý': 'y',
}
