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

	// ambiguousScoreGap: por debajo de esta diferencia, los dos primeros
	// candidatos se consideran empatados a efectos de auto-confirmar y la
	// popularidad de TMDB entra a desempatar (corrección post-entrega M3:
	// "Parasite.2019.KOREAN..." se auto-confirmaba como la película oscura
	// "Parasites" en vez de "Parásitos"/Parasite 2019, porque el título
	// traducido al español penaliza la similitud de texto frente al título
	// original en inglés de un candidato homónimo).
	ambiguousScoreGap = 0.15
	// popularityDominanceFactor: una popularidad "órdenes de magnitud" mayor
	// (según el informe del bug) se toma como señal fuerte de cuál es el
	// título real que la gente busca, por encima de un empate reñido de
	// similitud de texto.
	popularityDominanceFactor = 10.0

	// popularitySafetyNetFraction: red de seguridad INDEPENDIENTE de
	// resolveByPopularity (que solo actúa cuando el score está reñido). Si
	// el ganador, aunque gane claramente por texto, tiene una popularidad
	// inferior al 5% de la del candidato más popular del propio conjunto de
	// resultados, es señal de que puede ser un homónimo oscuro y no el
	// título real (informe de la corrección post-entrega M3: "un título
	// oscuro que gana por texto a uno masivamente más popular es señal de
	// riesgo, no de acierto").
	popularitySafetyNetFraction = 0.05
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
// candidatos por similitud de título (sobre title es-ES, original_title y
// title en-US) más bonus de año, con dos redes de seguridad basadas en
// popularidad (resolveByPopularity, isPopularityRisk) antes de dejar auto-
// confirmar. rawName es el nombre de archivo ORIGINAL (sin limpiar): solo se
// usa para detectar packs.
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

	ambiguous := resolveByPopularity(candidates)

	best := candidates[0]
	tier := confidenceFor(best.score)
	popularityRisk := isPopularityRisk(candidates, best)
	if (ambiguous || popularityRisk) && tier == ConfidenceAlta {
		tier = ConfidenceMedia
	}

	pack := isPackHint(rawName)
	if pack && tier == ConfidenceAlta {
		tier = ConfidenceMedia
	}
	// Episodio adivinado por carpeta (nombre de archivo puramente numérico,
	// Episode=0 desconocido): nunca se auto-confirma, tope MEDIA (corrección
	// post-entrega de M3).
	if parsed.LowConfidenceGuess() && tier == ConfidenceAlta {
		tier = ConfidenceMedia
	}
	if parsed.FolderContradiction() && tier > ConfidenceBaja {
		tier--
	}

	note := ""
	switch {
	case pack:
		note = "posible pack/trilogía: nunca se auto-confirma"
	case parsed.LowConfidenceGuess():
		note = "número de episodio desconocido: solo se identificó la serie por la carpeta"
	case parsed.FolderContradiction():
		note = "la carpeta contradice la temporada del archivo"
	case ambiguous:
		note = "candidatos con puntuación y popularidad muy reñidas: no se auto-confirma"
	case popularityRisk:
		note = "el candidato ganador tiene una popularidad muy inferior a otro resultado del mismo conjunto: no se auto-confirma"
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

// resolveByPopularity resuelve el empate entre los dos primeros candidatos
// (ya ordenados por score) cuando su diferencia de puntuación es mínima
// (< ambiguousScoreGap): la similitud de texto por sí sola no es fiable ahí
// (p. ej. un título traducido al español pierde por poco frente al título en
// inglés de un homónimo oscuro). Si uno de los dos tiene una popularidad
// órdenes de magnitud mayor, se le da preferencia (lo reordena a posición 0).
// Si ninguno domina claramente en popularidad, devuelve true: ni el texto ni
// la popularidad separan a los candidatos, así que no debe auto-confirmarse.
func resolveByPopularity(candidates []scoredCandidate) bool {
	if len(candidates) < 2 {
		return false
	}
	top, runner := candidates[0], candidates[1]
	if top.score-runner.score >= ambiguousScoreGap {
		return false
	}
	switch {
	case runner.result.Popularity >= top.result.Popularity*popularityDominanceFactor:
		candidates[0], candidates[1] = candidates[1], candidates[0]
		return false
	case top.result.Popularity >= runner.result.Popularity*popularityDominanceFactor:
		return false
	default:
		return true
	}
}

// isPopularityRisk es la red de seguridad independiente de
// resolveByPopularity (esta última solo entra si el score está reñido, por
// debajo de ambiguousScoreGap). Aunque el ganador tenga el score muy por
// delante del resto, si su popularidad es marginal frente al candidato más
// popular del propio conjunto de resultados, no se auto-confirma: puede ser
// un homónimo oscuro que ganó por texto.
func isPopularityRisk(candidates []scoredCandidate, best scoredCandidate) bool {
	maxPopularity := 0.0
	for _, c := range candidates {
		if c.result.Popularity > maxPopularity {
			maxPopularity = c.result.Popularity
		}
	}
	return maxPopularity > 0 && best.result.Popularity < maxPopularity*popularitySafetyNetFraction
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

	englishTitles := englishTitlesFor(ctx, client, kind, parsed)

	out := make([]scoredCandidate, 0, len(results))
	for _, r := range results {
		out = append(out, scoredCandidate{result: r, kind: kind, score: scoreCandidate(parsed, r, englishTitles[r.ID])})
	}
	return out, nil
}

// englishTitlesFor repite la búsqueda en TMDB con language=en-US (caché
// propia, misma query) para poder comparar también contra el título en
// inglés de cada candidato: en es-ES, TMDB devuelve el título TRADUCIDO en
// "title" y el original en el idioma de origen de la obra (p. ej. coreano),
// que no acerca nada a un nombre de archivo en inglés (corrección
// post-entrega M3: "Parasite.2019.KOREAN..." vs "Parásitos"/기생충 en es-ES,
// "Parasite" en en-US). Un fallo aquí (offline, TMDB caído) no debe tirar
// todo el matching: se ignora y el candidato se puntúa solo con
// title/original_title en es-ES, como antes.
func englishTitlesFor(ctx context.Context, client *Client, kind string, parsed Parsed) map[int64]string {
	var results []SearchResult
	var err error
	if kind == "movie" {
		results, err = client.SearchMovieEN(ctx, parsed.Title, parsed.Year)
	} else {
		results, err = client.SearchTVEN(ctx, parsed.Title, parsed.Year)
	}
	if err != nil {
		return nil
	}
	out := make(map[int64]string, len(results))
	for _, r := range results {
		out[r.ID] = r.EffectiveTitle()
	}
	return out
}

// scoreCandidate puntúa un resultado de búsqueda contra parsed: similitud de
// título (máximo entre title es-ES, original_title, y title en-US si se
// conoce) más bonus de año exacto o ±1 (Brief §3; título en-US añadido en la
// corrección post-entrega de M3).
func scoreCandidate(parsed Parsed, r SearchResult, englishTitle string) float64 {
	titleScore := titleSimilarity(parsed.Title, r.EffectiveTitle())
	if alt := titleSimilarity(parsed.Title, r.EffectiveOriginalTitle()); alt > titleScore {
		titleScore = alt
	}
	if englishTitle != "" {
		if alt := titleSimilarity(parsed.Title, englishTitle); alt > titleScore {
			titleScore = alt
		}
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
