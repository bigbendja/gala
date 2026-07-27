package catalog

import "strings"

// accentFolds son los pares de vocales acentuadas -> planas que la búsqueda
// de texto ignora (Brief M10c §1: "parasitos" debe encontrar "Parásitos").
// Deliberadamente NO se pliega la ñ: en español "ñ" no es una variante
// acentuada de "n", es una letra distinta, y confundirlas cambiaría el
// significado de la búsqueda.
var accentFolds = []struct{ from, to string }{
	{"á", "a"}, {"à", "a"}, {"ä", "a"}, {"â", "a"},
	{"é", "e"}, {"è", "e"}, {"ë", "e"}, {"ê", "e"},
	{"í", "i"}, {"ì", "i"}, {"ï", "i"}, {"î", "i"},
	{"ó", "o"}, {"ò", "o"}, {"ö", "o"}, {"ô", "o"},
	{"ú", "u"}, {"ù", "u"}, {"ü", "u"}, {"û", "u"},
}

// foldAccentsSQL envuelve col (ya en minúsculas vía LOWER) en la misma
// cadena de REPLACE que foldAccents aplica en Go, para que la comparación
// LIKE ignore acentos en ambos lados.
func foldAccentsSQL(col string) string {
	expr := "LOWER(" + col + ")"
	for _, f := range accentFolds {
		expr = "REPLACE(" + expr + ",'" + f.from + "','" + f.to + "')"
	}
	return expr
}

// foldAccents normaliza s para comparar contra foldAccentsSQL: minúsculas y
// vocales acentuadas plegadas a su forma sin acento.
func foldAccents(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		folded := string(r)
		for _, f := range accentFolds {
			if folded == f.from {
				folded = f.to
				break
			}
		}
		b.WriteString(folded)
	}
	return b.String()
}
