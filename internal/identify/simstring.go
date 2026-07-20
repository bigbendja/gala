package identify

import "unicode"

// normalizeForCompare prepara un título para compararlo: minúsculas, sin
// acentos, sin signos de puntuación, espacios colapsados (Brief M3 §3).
func normalizeForCompare(s string) string {
	folded := foldASCII(s)
	out := make([]rune, 0, len(folded))
	prevSpace := false
	for _, r := range folded {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			out = append(out, r)
			prevSpace = false
		default:
			if !prevSpace && len(out) > 0 {
				out = append(out, ' ')
				prevSpace = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// levenshteinSimilarity devuelve 1 - distancia_edición/longitud_máxima, en
// [0,1]. 1 = idénticas, 0 = completamente distintas.
func levenshteinSimilarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1
	}
	d := levenshteinDistance(ra, rb)
	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(d)/float64(maxLen)
}

func levenshteinDistance(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// jaroSimilarity implementa la distancia de Jaro clásica, en [0,1].
func jaroSimilarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 && lb == 0 {
		return 1
	}
	if la == 0 || lb == 0 {
		return 0
	}

	matchDist := la
	if lb > matchDist {
		matchDist = lb
	}
	matchDist = matchDist/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}

	aMatched := make([]bool, la)
	bMatched := make([]bool, lb)
	matches := 0
	for i := 0; i < la; i++ {
		start, end := i-matchDist, i+matchDist+1
		if start < 0 {
			start = 0
		}
		if end > lb {
			end = lb
		}
		for j := start; j < end; j++ {
			if bMatched[j] || ra[i] != rb[j] {
				continue
			}
			aMatched[i] = true
			bMatched[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}

	transpositions := 0
	k := 0
	for i := 0; i < la; i++ {
		if !aMatched[i] {
			continue
		}
		for !bMatched[k] {
			k++
		}
		if ra[i] != rb[k] {
			transpositions++
		}
		k++
	}

	m := float64(matches)
	return (m/float64(la) + m/float64(lb) + (m-float64(transpositions)/2)/m) / 3
}

// titleSimilarity combina Levenshtein y Jaro (el máximo de ambas, ya
// normalizados) sobre títulos ya pasados por normalizeForCompare. Ninguna
// dependencia nueva (Brief M3 §3: "implementada en el paquete").
func titleSimilarity(a, b string) float64 {
	na, nb := normalizeForCompare(a), normalizeForCompare(b)
	if na == nb {
		return 1
	}
	lev := levenshteinSimilarity(na, nb)
	jaro := jaroSimilarity(na, nb)
	if jaro > lev {
		return jaro
	}
	return lev
}
