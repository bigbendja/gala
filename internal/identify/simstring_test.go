package identify

import "testing"

func TestNormalizeForCompare(t *testing.T) {
	got := normalizeForCompare("¡El Padrino: Parte II!")
	want := "el padrino parte ii"
	if got != want {
		t.Errorf("normalizeForCompare = %q, esperado %q", got, want)
	}
}

func TestLevenshteinSimilarity(t *testing.T) {
	if s := levenshteinSimilarity("inception", "inception"); s != 1 {
		t.Errorf("similitud de cadenas idénticas = %v, esperado 1", s)
	}
	if s := levenshteinSimilarity("inception", "incepton"); s < 0.8 {
		t.Errorf("similitud con un típo = %v, esperado >= 0.8", s)
	}
	if s := levenshteinSimilarity("batman", "superman"); s > 0.6 {
		t.Errorf("similitud de títulos distintos = %v, esperado baja", s)
	}
}

func TestJaroSimilarity(t *testing.T) {
	if s := jaroSimilarity("martha", "marhta"); s < 0.9 {
		t.Errorf("jaroSimilarity(martha, marhta) = %v, esperado >= 0.9 (caso clásico)", s)
	}
	if s := jaroSimilarity("", ""); s != 1 {
		t.Errorf("jaroSimilarity('','') = %v, esperado 1", s)
	}
}

func TestTitleSimilarityAccentInsensitive(t *testing.T) {
	s := titleSimilarity("Merlí", "Merli")
	if s != 1 {
		t.Errorf("titleSimilarity(Merlí, Merli) = %v, esperado 1 (insensible a acentos)", s)
	}
}
