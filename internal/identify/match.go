package identify

import (
	"context"
	"regexp"
	"sort"

	"github.com/bigbendja/gala/internal/catalog"
)

// Umbrales de confianza del matcher (Brief M3 §3; constantes con nombre,
// ajustables sin tocar el resto del pipeline).
const (
	// ThresholdAlta: puntuación mínima para auto-confirmar (confirmed=1).
	ThresholdAlta = 0.88
	// ThresholdMedia: puntuación mínima para "Por confirmar" con candidatos;
	// por debajo, BAJA/sin resultados → "Por confirmar" sin candidatos.
	ThresholdMedia = 0.55
	// altaMinTitleScore evita que un año exacto compense un título muy
	// distinto: sin un mínimo de similitud de título, nunca se auto-confirma
	// aunque el año coincida.
	altaMinTitleScore = 0.80

	yearBonusExact = 0.12
	yearBonusNear  = 0.05 // año ±1 (Brief §3)

	// maxCandidates es el máximo de candidatos que se muestran en "Por
	// confirmar" (Brief §3).
	maxCandidates = 5
)

// Confidence es el nivel de confianza de un resultado del matcher.
type Confidence int

const (
	ConfidenceBaja Confidence = iota
	ConfidenceMedia
	ConfidenceAlta
)

// rePackHint detecta packs/trilogías/temporadas completas en un solo
// archivo: Trilogía, Colección, Saga completa, o "S01.Complete"/
// "Complete.S01" (temporada completa empaquetada). Precisión acordada con
// el titular del proyecto: confianza máxima MEDIA, nunca auto-confirmar
// (el soporte real de archivos multi-título se anota en ISSUES-MVP2.md).
var rePackHint = regexp.MustCompile(`(?i)(trilog[ií]a|trilogy|colecci[oó]n|collection|saga\s*completa|\bsaga\b|s\d{1,2}\W*complete|complete\W*s\d{1,2})`)

func isPackHint(rawName string) bool {
	return rePackHint.MatchString(rawName)
}

type scoredCandidate struct {
	result SearchResult
	kind   string // "movie" | "tv"
	score  float64
}

// MatchResult es el resultado de buscar y puntuar candidatos para un item
// provisional (Brief M3 §3).
type MatchResult struct {
	Confidence Confidence
	Kind       string // "movie" | "tv"; vacío si no hubo ningún candidato
	Best       *SearchResult
	Candidates []catalog.Candidate // hasta maxCandidates, para MEDIA/BAJA
	Note       string
}

// Match busca en TMDB según parsed.Kind (unknown prueba ambos) y puntúa los
// candidatos por similitud de título (sobre title Y original_title) más
// bonus de año, con desempate por popularidad. rawName es el nombre de
// archivo ORIGINAL (sin limpiar): solo se usa para detectar packs.
func Match(ctx context.Context, client *Client, parsed Parsed, rawName string) (MatchResult, error) {
	var candidates []scoredCandidate

	tryMovie := parsed.Kind == KindMovie || parsed.Kind == KindUnknown
	tryTV := parsed.Kind == KindSeriesEpisode || parsed.Kind == KindUnknown

	if tryMovie {
		cs, err := searchScored(ctx, client, "movie", parsed)
		if err != nil {
			return MatchResult{}, err
		}
		candidates = append(candidates, cs...)
	}
	if tryTV {
		cs, err := searchScored(ctx, client, "tv", parsed)
		if err != nil {
			return MatchResult{}, err
		}
		candidates = append(candidates, cs...)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].result.Popularity > candidates[j].result.Popularity
	})

	if len(candidates) == 0 {
		return MatchResult{Confidence: ConfidenceBaja, Note: "sin resultados en TMDB"}, nil
	}

	best := candidates[0]
	tier := confidenceFor(best.score)

	pack := isPackHint(rawName)
	if pack && tier == ConfidenceAlta {
		tier = ConfidenceMedia
	}
	if parsed.FolderContradiction() && tier > ConfidenceBaja {
		tier--
	}

	note := ""
	switch {
	case pack:
		note = "posible pack/trilogía: nunca se auto-confirma"
	case parsed.FolderContradiction():
		note = "la carpeta contradice la temporada del archivo"
	}

	res := MatchResult{Confidence: tier, Kind: best.kind, Note: note}
	if tier == ConfidenceAlta {
		r := best.result
		res.Best = &r
		return res, nil
	}

	n := maxCandidates
	if len(candidates) < n {
		n = len(candidates)
	}
	for _, c := range candidates[:n] {
		res.Candidates = append(res.Candidates, catalog.Candidate{
			TMDBID: c.result.ID, Kind: c.kind, Title: c.result.EffectiveTitle(),
			OriginalTitle: c.result.EffectiveOriginalTitle(), Year: c.result.Year(),
			PosterPath: c.result.PosterPath, Score: c.score,
		})
	}
	return res, nil
}

func confidenceFor(score float64) Confidence {
	switch {
	case score >= ThresholdAlta:
		return ConfidenceAlta
	case score >= ThresholdMedia:
		return ConfidenceMedia
	default:
		return ConfidenceBaja
	}
}

func searchScored(ctx context.Context, client *Client, kind string, parsed Parsed) ([]scoredCandidate, error) {
	var results []SearchResult
	var err error
	if kind == "movie" {
		results, err = client.SearchMovie(ctx, parsed.Title, parsed.Year)
	} else {
		results, err = client.SearchTV(ctx, parsed.Title, parsed.Year)
	}
	if err != nil {
		return nil, err
	}
	out := make([]scoredCandidate, 0, len(results))
	for _, r := range results {
		out = append(out, scoredCandidate{result: r, kind: kind, score: scoreCandidate(parsed, r)})
	}
	return out, nil
}

// scoreCandidate puntúa un resultado de búsqueda contra parsed: similitud
// de título (máximo entre title y original_title) más bonus de año exacto
// o ±1 (Brief §3).
func scoreCandidate(parsed Parsed, r SearchResult) float64 {
	titleScore := titleSimilarity(parsed.Title, r.EffectiveTitle())
	if alt := titleSimilarity(parsed.Title, r.EffectiveOriginalTitle()); alt > titleScore {
		titleScore = alt
	}

	score := titleScore
	if parsed.Year > 0 && r.Year() > 0 {
		diff := parsed.Year - r.Year()
		if diff < 0 {
			diff = -diff
		}
		switch diff {
		case 0:
			score += yearBonusExact
		case 1:
			score += yearBonusNear
		}
	}

	// El año no debe poder compensar un título que en realidad no se
	// parece: sin un mínimo de similitud de título, el resultado nunca
	// alcanza ALTA aunque el año encaje.
	if titleScore < altaMinTitleScore && score >= ThresholdAlta {
		score = ThresholdAlta - 0.01
	}
	return score
}

// ApplyIdentification descarga el detalle+imágenes de tmdbID (kind "movie"
// o "tv") y aplica la identificación completa sobre itemID. Comparte
// camino la auto-confirmación (ALTA) y la confirmación manual desde "Por
// confirmar" (Brief §3, §5).
func ApplyIdentification(ctx context.Context, client *Client, repo *catalog.Repo,
	itemID, tmdbID int64, kind string, parsed Parsed) error {
	if kind == "movie" {
		d, err := client.MovieDetail(ctx, tmdbID)
		if err != nil {
			return err
		}
		poster, _ := client.DownloadPoster(ctx, d.PosterPath)
		backdrop, _ := client.DownloadBackdrop(ctx, d.BackdropPath)
		return repo.ApplyMovieIdentification(ctx, itemID, tmdbID, d.EffectiveTitle(),
			strPtr(d.EffectiveOriginalTitle()), strPtr(d.Overview), strPtr(poster), strPtr(backdrop),
			nil, intPtr(d.Year()), d.GenreNames(), "")
	}

	d, err := client.TVDetail(ctx, tmdbID)
	if err != nil {
		return err
	}
	poster, _ := client.DownloadPoster(ctx, d.PosterPath)
	backdrop, _ := client.DownloadBackdrop(ctx, d.BackdropPath)

	// parsed.Season = 0 cuando el normalizador no pudo determinar la
	// temporada (p. ej. archivo numérico bajo una carpeta genérica); se
	// pasa tal cual y queda como "Temporada 0" (duda de diseño anotada en
	// el informe final, no se improvisa un valor por defecto).
	seasonName := ""
	if s := d.SeasonByNumber(parsed.Season); s != nil {
		seasonName = s.Name
	}

	return repo.ApplyEpisodeIdentification(ctx, itemID, tmdbID, d.EffectiveTitle(),
		strPtr(d.EffectiveOriginalTitle()), strPtr(d.Overview), strPtr(poster), strPtr(backdrop),
		d.GenreNames(), parsed.Season, parsed.Episode, seasonName, "", "")
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
