// Package identify normaliza nombres de archivo, consulta TMDB y decide qué
// se auto-confirma y qué va a "Por confirmar" (Brief M3, Especificación
// MVP1 §3.3, §3.7).
package identify

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Valores de Parsed.Kind (namespace propio del normalizador; no confundir
// con catalog.Kind, que es el enum de la tabla items).
const (
	KindMovie         = "movie"
	KindSeriesEpisode = "series_episode"
	KindUnknown       = "unknown"
)

// Parsed es el resultado de normalizar un nombre de archivo (Brief M3 §1).
type Parsed struct {
	Title      string
	Year       int
	Kind       string
	Season     int
	Episode    int
	AbsoluteEp int
	Quality    string
	Langs      []string
	Junk       bool
	Extra      bool

	// folderContradiction es un campo interno (no exportado, ver Brief §1.5):
	// la carpeta y el archivo dan señales de temporada distintas. El matcher
	// lo usa para degradar un nivel la confianza del resultado.
	folderContradiction bool

	// lowConfidenceGuess es un campo interno: el archivo no aporta ninguna
	// señal propia fiable (nombre puramente numérico, p. ej. "1017.mkv") y
	// Season/Episode salen enteros de la carpeta con Episode=0 (desconocido)
	// — ver seriesEpisodeFromFolder. El matcher nunca debe auto-confirmar
	// esto (corrección post-entrega de M3, no amplía alcance).
	lowConfidenceGuess bool
}

// FolderContradiction expone folderContradiction al paquete identify (match.go).
func (p Parsed) FolderContradiction() bool { return p.folderContradiction }

// LowConfidenceGuess expone lowConfidenceGuess al paquete identify (match.go).
func (p Parsed) LowConfidenceGuess() bool { return p.lowConfidenceGuess }

// Parse normaliza filename (con folder como contexto de herencia, puede
// estar vacío) siguiendo el pipeline del Brief M3 §1. El orden de los pasos
// importa: basura y extras cortan el pipeline entero (no se intenta más).
func Parse(filename, folder string) Parsed {
	base := trimExt(filename)
	folder = strings.TrimSpace(folder)

	// 1. Junk primero — salvo un rescate puntual: un nombre puramente
	// numérico ("1017.mkv", "01.mkv") no dice nada por sí solo, pero si la
	// carpeta contenedora SÍ se identifica como una serie con temporada,
	// no es basura: es un episodio de esa serie con número desconocido
	// (corrección pedida tras la primera entrega de M3). Las señales
	// "duras" de basura (WhatsApp, timestamps de cámara, IMG_/VID_...)
	// nunca se rescatan, aunque sean numéricas: siguen siendo Junk pase lo
	// que pase en la carpeta.
	if isJunk(base) {
		if !isHardJunkSignal(base) && isPureNumeric(base) {
			if p, ok := seriesEpisodeFromFolder(folder); ok {
				return p
			}
		}
		return Parsed{Junk: true, Kind: KindUnknown}
	}
	// 2. Extra (sample/trailer/teaser/making-of): tampoco se cataloga, así
	// que no tiene sentido seguir extrayendo temporada/año/etc. (Brief §4:
	// "los extras NO aparecen en la galería").
	if isExtra(base) {
		return Parsed{Extra: true, Kind: KindUnknown}
	}

	quality := extractQuality(base)
	langs := extractLangs(base)

	work := stripBracketsKeepYear(base)
	// Los guiones bajos cuentan como carácter de palabra para \b: sin este
	// paso "Cars_2006_DVDRip" nunca deja pasar el año (no hay frontera antes
	// de "2006"). A partir de aquí todo el pipeline trabaja con espacios.
	work = strings.ReplaceAll(work, "_", " ")
	season, episode, absEp, notUseful, work := detectSeasonEpisode(work)
	year, work := extractYear(work)
	work = stripTechnicalTokens(work)
	work = stripLangWords(work)
	work = stripTrailingGroup(work)
	title := finalCleanup(work)

	contradiction := false
	if folder != "" {
		fSeason := folderSeason(folder)
		if fSeason > 0 && season > 0 && season != fSeason {
			contradiction = true
		}
		if notUseful {
			if fTitle := folderTitle(folder); fTitle != "" {
				title = fTitle
			}
			if fSeason > 0 {
				season = fSeason // la carpeta manda (Brief §1.5)
			}
		}
	}

	kind := KindUnknown
	switch {
	case season > 0 || episode > 0 || absEp > 0:
		kind = KindSeriesEpisode
	case year > 0:
		kind = KindMovie
	}

	return Parsed{
		Title: title, Year: year, Kind: kind, Season: season, Episode: episode,
		AbsoluteEp: absEp, Quality: quality, Langs: langs,
		folderContradiction: contradiction,
	}
}

func trimExt(filename string) string {
	ext := extOf(filename)
	return filename[:len(filename)-len(ext)]
}

// extOf es como filepath.Ext pero sin depender de path/filepath (aquí solo
// se opera sobre nombres base, nunca rutas completas).
func extOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || strings.ContainsAny(name[i:], `/\`) {
		return ""
	}
	return name[i:]
}

// --- 1. Basura -------------------------------------------------------------

var (
	reDeviceAutoName = regexp.MustCompile(`(?i)^(?:img|vid|video|mov)[_.\s-]*\d{3,}`)
	rePureTimestamp  = regexp.MustCompile(`^\d{8}[_-]\d{6}$`)
	reWhatsAppVideo  = regexp.MustCompile(`(?i)whatsapp[\s._-]*video`)
)

// junkVocab son palabras que, en ausencia de estructura de release
// reconocible (calidad/fuente/códec/SxxEyy...), delatan vídeos personales o
// descargas sin identificar (Brief §1 paso 1, "y similares").
var junkVocab = []string{
	"whatsapp", "telegram", "cloud", "document", "grabacion", "clip",
	"tutorial", "webinar", "recording", "prueba", "cortometraje",
	"animacion", "final", "boda", "untitled",
}

// junkPhrases son frases explícitas del Brief §1 paso 1, comprobadas sin
// necesidad de la puerta de "estructura de release" (son inequívocas).
var junkPhrases = []string{"descarga", "sin titulo", "sin identificar"}

func isJunk(base string) bool {
	if isHardJunkSignal(base) {
		return true
	}
	if hasReleaseStructure(base) {
		return false
	}
	spaced := strings.ToLower(spacify(base))
	for _, w := range junkVocab {
		if containsWord(spaced, w) {
			return true
		}
	}
	// Título resultante de <2 caracteres alfabéticos tras limpiar.
	return countLetters(spaced) < 2
}

// isHardJunkSignal son los patrones inequívocos de basura del Brief §1
// paso 1 (WhatsApp, timestamps de cámara, IMG_/VID_..., frases explícitas):
// a diferencia del resto de isJunk (vocabulario suelto, título demasiado
// corto), estos NUNCA se rescatan por contexto de carpeta — ver
// seriesEpisodeFromFolder y el comentario en Parse.
func isHardJunkSignal(base string) bool {
	if rePureTimestamp.MatchString(base) || reDeviceAutoName.MatchString(base) || reWhatsAppVideo.MatchString(base) {
		return true
	}
	spaced := strings.ToLower(spacify(base))
	for _, phrase := range junkPhrases {
		if strings.Contains(spaced, phrase) {
			return true
		}
	}
	return false
}

// isPureNumeric indica si base no contiene NINGUNA letra (solo dígitos y
// separadores/puntuación), con al menos un dígito: "1017", "01", "1x02"
// (ojo: si tuviera letras no cuenta). Es la única forma de Junk "blando"
// que se rescata por carpeta (Brief, corrección post-M3).
func isPureNumeric(base string) bool {
	hasDigit := false
	for _, r := range base {
		switch {
		case unicode.IsLetter(r):
			return false
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	return hasDigit
}

// seriesEpisodeFromFolder rescata un archivo de nombre puramente numérico
// cuando folder SÍ se identifica como una serie con temporada (aporta
// season Y título). El número del archivo no es fiable (podría ser
// episodio absoluto, numeración interna...), así que Episode queda en 0
// (desconocido) y el resultado se marca lowConfidenceGuess=true: el
// matcher nunca debe auto-confirmarlo (Por confirmar como mucho, tope
// MEDIA). Si la carpeta no aporta season+título, no hay rescate: sigue
// siendo Junk.
func seriesEpisodeFromFolder(folder string) (Parsed, bool) {
	if folder == "" {
		return Parsed{}, false
	}
	season := folderSeason(folder)
	title := folderTitle(folder)
	if season <= 0 || title == "" {
		return Parsed{}, false
	}
	return Parsed{
		Title: title, Kind: KindSeriesEpisode, Season: season, Episode: 0,
		lowConfidenceGuess: true,
	}, true
}

// reReleaseHint son marcadores de alta confianza de que el nombre es un
// release de escena (no vídeo personal): solo tokens con muy bajo riesgo de
// falso positivo, para no tapar basura real que use estos términos sueltos.
var reReleaseHint = regexp.MustCompile(`(?i)\b(1080p|720p|2160p|480p|4K|UHD|BluRay|BRRip|BDRip|WEB-?DL|WEBRip|HDTV|DVDRip|HDRip|x264|x265|H\.?264|HEVC|XviD)\b`)

func hasReleaseStructure(base string) bool {
	return reSxxE.MatchString(base) || reNxNN.MatchString(base) ||
		reCapLiteral.MatchString(base) || reReleaseHint.MatchString(base) ||
		reBareE.MatchString(base) || reEpisodeWrd.MatchString(base)
}

func containsWord(spaced, word string) bool {
	for _, f := range strings.FieldsFunc(spaced, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if f == word {
			return true
		}
	}
	return false
}

func countLetters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// --- 2. Extras (samples, trailers...) --------------------------------------

var (
	reExtraToken = regexp.MustCompile(`(?i)\b(sample|trailer|teaser)\b`)
	reMakingOf   = regexp.MustCompile(`(?i)\bmaking[\s._-]*of\b`)
)

func isExtra(base string) bool {
	lower := strings.ToLower(base)
	if lower == "sample" || lower == "rarbg" {
		return true
	}
	return reExtraToken.MatchString(base) || reMakingOf.MatchString(base)
}

// --- 3. Corchetes/paréntesis (se conserva el año) ---------------------------

var (
	reBracketGroup = regexp.MustCompile(`\[[^\[\]]*\]|\([^()]*\)`)
	reYearExact    = regexp.MustCompile(`^(19[2-9]\d|20[0-4]\d)$`)
)

func stripBracketsKeepYear(s string) string {
	return reBracketGroup.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSpace(m[1 : len(m)-1])
		if reYearExact.MatchString(inner) {
			return " " + inner + " "
		}
		return " "
	})
}

// --- 4. Temporada/episodio ---------------------------------------------------

var (
	reSxxE       = regexp.MustCompile(`(?i)\bS(\d{1,2})E(\d{1,3})\b`)
	reNxNN       = regexp.MustCompile(`\b(\d{1,2})x(\d{2,3})\b`)
	reCapLiteral = regexp.MustCompile(`(?i)\bCap[.\s]*(\d{3,4})\b`)
	reSeasonWord = regexp.MustCompile(`(?i)\b(?:Temporada|Season|Parte)[\s._-]*(\d{1,2})\b`)
	reEpisodeWrd = regexp.MustCompile(`(?i)\b(?:Cap[ií]tulo|Episodio)[\s._-]*(\d{1,3})\b`)
	reBareE      = regexp.MustCompile(`(?i)\bE(\d{1,3})\b`)
	reAnimeAbs   = regexp.MustCompile(`\s-\s(\d{2,4})(?:\s|$)`)
	reDigitFused = regexp.MustCompile(`^([\p{L}]+)[\s._-]+(\d{3,4})$`)
	reYearish4   = regexp.MustCompile(`^(?:19|20)\d\d$`)

	// Solo para nombres de carpeta (Brief §1.5: "T1", "S03" sueltos serían
	// demasiado ambiguos dentro de un título de archivo cualquiera).
	reFolderT = regexp.MustCompile(`(?i)\bT(\d{1,2})\b`)
	reFolderS = regexp.MustCompile(`(?i)\bS(\d{1,2})\b`)
)

// splitCapDigits aplica la regla Cap.105→T1E05, Cap.1204→T12E04: los 1-2
// primeros dígitos son la temporada, los 2 últimos el episodio.
func splitCapDigits(digits string) (season, episode int) {
	cut := len(digits) - 2
	s, _ := strconv.Atoi(digits[:cut])
	e, _ := strconv.Atoi(digits[cut:])
	return s, e
}

func spliceOut(s string, start, end int) string {
	return s[:start] + " " + s[end:]
}

func atoiAt(s string, m []int, groupIdx int) int {
	lo, hi := m[groupIdx*2], m[groupIdx*2+1]
	if lo < 0 {
		return 0
	}
	n, _ := strconv.Atoi(s[lo:hi])
	return n
}

// detectSeasonEpisode aplica la cascada de prioridad del Brief §1 paso 4:
// SxxEyy → NxNN → Cap.NNN → forma en palabras (Temporada/Episodio) → anime
// absoluto → sufijo numérico fusionado (merli_301). Devuelve también si el
// título restante no es útil (Brief §1.5): la cola de mayor prioridad que
// deja título vacío, o el fusionado (siempre de baja confianza).
func detectSeasonEpisode(work string) (season, episode, absEp int, notUseful bool, rest string) {
	work = strings.TrimSpace(work)
	rest = work

	if m := reSxxE.FindStringSubmatchIndex(work); m != nil {
		season, episode = atoiAt(work, m, 1), atoiAt(work, m, 2)
		rest = stripOtherEpisodeMarkers(spliceOut(work, m[0], m[1]))
		notUseful = isResidualUseless(rest)
		return
	}
	if m := reNxNN.FindStringSubmatchIndex(work); m != nil {
		season, episode = atoiAt(work, m, 1), atoiAt(work, m, 2)
		rest = stripOtherEpisodeMarkers(spliceOut(work, m[0], m[1]))
		notUseful = isResidualUseless(rest)
		return
	}
	if m := reCapLiteral.FindStringSubmatchIndex(work); m != nil {
		season, episode = splitCapDigits(work[m[2]:m[3]])
		rest = stripOtherEpisodeMarkers(spliceOut(work, m[0], m[1]))
		notUseful = isResidualUseless(rest)
		return
	}

	found := false
	rest = work
	if m := reSeasonWord.FindStringSubmatchIndex(rest); m != nil {
		season = atoiAt(rest, m, 1)
		rest = spliceOut(rest, m[0], m[1])
		found = true
	}
	if m := reEpisodeWrd.FindStringSubmatchIndex(rest); m != nil {
		episode = atoiAt(rest, m, 1)
		rest = spliceOut(rest, m[0], m[1])
		found = true
	} else if m := reBareE.FindStringSubmatchIndex(rest); m != nil {
		episode = atoiAt(rest, m, 1)
		rest = spliceOut(rest, m[0], m[1])
		found = true
	}
	if found {
		rest = stripOtherEpisodeMarkers(rest)
		notUseful = isResidualUseless(rest)
		return
	}

	rest = work
	if m := reAnimeAbs.FindStringSubmatchIndex(work); m != nil {
		absEp = atoiAt(work, m, 1)
		rest = spliceOut(work, m[0], m[1])
		notUseful = isResidualUseless(rest)
		return
	}

	// Sufijo numérico fusionado (merli_301 → T3E01): solo si el archivo
	// entero es "palabra + 3-4 dígitos" y esos dígitos NO parecen un año
	// (si no, "Joker 2019" se leería como T20E19).
	if m := reDigitFused.FindStringSubmatchIndex(work); m != nil {
		digits := work[m[4]:m[5]]
		if !(len(digits) == 4 && reYearish4.MatchString(digits)) {
			season, episode = splitCapDigits(digits)
			rest = work[m[2]:m[3]]
			notUseful = true
			return
		}
	}

	return 0, 0, 0, false, work
}

// stripOtherEpisodeMarkers limpia del título restos de patrones de
// temporada/episodio de menor prioridad que ya perdieron la cascada (Brief
// "numeración doble": SxN gana, pero "Capitulo NN" no debe quedar en el
// título).
func stripOtherEpisodeMarkers(s string) string {
	for _, re := range []*regexp.Regexp{reSxxE, reNxNN, reCapLiteral, reSeasonWord, reEpisodeWrd, reBareE} {
		s = re.ReplaceAllString(s, " ")
	}
	return s
}

func isResidualUseless(s string) bool {
	s = stripBracketsKeepYear(s)
	s = stripTechnicalTokens(s)
	s = stripLangWords(s)
	s = finalCleanup(s)
	return countLetters(s) == 0
}

// --- 6. Año ------------------------------------------------------------------

var reYear = regexp.MustCompile(`\b(19[2-9]\d|20[0-4]\d)\b`)

func extractYear(work string) (year int, rest string) {
	locs := reYear.FindAllStringIndex(work, -1)
	if len(locs) == 0 {
		return 0, work
	}
	trimmed := strings.TrimLeft(work, " .")
	lead := len(work) - len(trimmed)
	firstIsFirstWord := locs[0][0] <= lead

	if len(locs) == 1 && firstIsFirstWord {
		return 0, work
	}
	last := locs[len(locs)-1]
	yv, _ := strconv.Atoi(work[last[0]:last[1]])
	return yv, spliceOut(work, last[0], last[1])
}

// --- 7. Tokens técnicos (lista exportada, Brief §1 paso 7) ------------------

// TechnicalTokens son los patrones (insensibles a mayúsculas) que el
// normalizador elimina del título: resoluciones, fuentes, códecs, audio,
// ediciones y ruido de release. Expresados como fragmentos de regexp (no
// literales) para tolerar separadores . _ - o espacio indistintamente.
var TechnicalTokens = []string{
	`FULL[\s._-]*HD`, `2160p`, `1080p`, `720p`, `480p`, `4K`, `UHD`, `\bHD\b`,
	`BluRay`, `BRRip`, `BDRip`, `WEB[\s._-]*DL`, `WEBRip`, `\bWEB\b`, `HDTV`, `DVDRip`, `HDRip`, `HDTS`,
	`\bCAM\b`, `\bTS\b`, `\bNF\b`, `\bAMZN\b`, `\bMA\b`, `\bDSNP\b`,
	`x264`, `x265`, `H[\s._-]*264`, `H[\s._-]*265`, `HEVC`, `XviD`, `\bAVC\b`, `10bit`, `\bHDR\b`, `\bDV\b`,
	`\bAAC\b`, `\bAC3\b`, `\bEAC3\b`, `DTS[\s._-]*HD[\s._-]*MA`, `\bDTS\b`,
	`DDP5[\s._-]*1`, `DD5[\s._-]*1`, `TrueHD`, `\bAtmos\b`, `\b7[\s._-]*1\b`, `\b6CH\b`, `\b5[\s._-]*1\b`,
	`REMASTERED`, `EXTENDED`, `UNRATED`, `COMPLETE`, `COMPLETA`, `PROPER`, `REPACK`, `\bFINAL\b`, `\bIMAX\b`, `\bREMUX\b`,
	`YTS[.\w]*`, `YIFY`, `RARBG`,
	`\bSubs?\b`, `pelicula[\s._-]*completa`, `\bdownload\b`, `\bAudio\b`,
	`\d+[\s._-]*[MG]B`,
}

// reEditionSE es "SE" (Special Edition) como sufijo de release: SIEMPRE en
// mayúsculas y con \b, para no comerse el "se" de títulos en español ("La
// que SE avecina") — a diferencia del resto, no es insensible a mayúsculas.
var reEditionSE = regexp.MustCompile(`\bSE\b`)

var reTechTokens = append(compileAll(TechnicalTokens), reEditionSE)

func compileAll(pats []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(pats))
	for i, p := range pats {
		out[i] = regexp.MustCompile(`(?i)` + p)
	}
	return out
}

func stripTechnicalTokens(s string) string {
	for _, re := range reTechTokens {
		s = re.ReplaceAllString(s, " ")
	}
	return s
}

var reTrailingGroup = regexp.MustCompile(`-[\p{L}\p{N}]{2,20}\s*$`)

func stripTrailingGroup(s string) string {
	return reTrailingGroup.ReplaceAllString(s, "")
}

// --- 8. Idiomas (Brief §1 paso 8) -------------------------------------------

var langMap = map[string]string{
	"castellano": "es", "español": "es", "espanol": "es", "spanish": "es", "esp": "es", "cast": "es",
	"latino": "lat", "lat": "lat",
	"french": "fr", "frances": "fr", "francés": "fr", "vostfr": "fr", "truefrench": "fr",
	"german": "de", "aleman": "de", "alemán": "de",
	"korean":   "ko",
	"japanese": "ja", "jap": "ja",
	"catalan": "ca", "catalán": "ca",
	"english": "en", "ingles": "en", "inglés": "en", "eng": "en",
	"hindi": "hi",
	"dual":  "dual", "multi": "multi",
}

var reLangWords = buildLangRegex()

func buildLangRegex() *regexp.Regexp {
	keys := make([]string, 0, len(langMap))
	for k := range langMap {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	// Ordenar por longitud descendente para que "truefrench" no lo tape "french".
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && len(keys[j]) > len(keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(keys, "|") + `)\b`)
}

func extractLangs(s string) []string {
	folded := foldASCII(s)
	matches := reLangWords.FindAllString(folded, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		code := langMap[strings.ToLower(m)]
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}

func stripLangWords(s string) string {
	return reLangWords.ReplaceAllString(s, " ")
}

// --- Calidad (para Parsed.Quality) ------------------------------------------

var qualityPatterns = []struct {
	re  *regexp.Regexp
	val string
}{
	{regexp.MustCompile(`(?i)2160p`), "2160p"},
	{regexp.MustCompile(`(?i)1080p`), "1080p"},
	{regexp.MustCompile(`(?i)720p`), "720p"},
	{regexp.MustCompile(`(?i)480p`), "480p"},
	{regexp.MustCompile(`(?i)\b4K\b`), "4k"},
	{regexp.MustCompile(`(?i)\bUHD\b`), "uhd"},
	{regexp.MustCompile(`(?i)FULL[\s._-]*HD`), "hd"},
	{regexp.MustCompile(`(?i)\bHD\b`), "hd"},
}

func extractQuality(s string) string {
	for _, p := range qualityPatterns {
		if p.re.MatchString(s) {
			return p.val
		}
	}
	return ""
}

// --- 9. Normalización final ---------------------------------------------------

var (
	reSpaceHyphenSpace = regexp.MustCompile(`\s+-\s+`)
	reMultiSpace       = regexp.MustCompile(`\s+`)
	reLeadTrailPunct   = regexp.MustCompile(`^[\s._\-:]+|[\s._\-:]+$`)
)

// spacify convierte puntos y guiones bajos en espacios, colapsa espacios y
// elimina los guiones "-" que actúan como separador ( - ), conservando los
// que forman parte del título (Spider-Man, WALL-E).
func spacify(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ".", " ")
	s = reSpaceHyphenSpace.ReplaceAllString(s, " ")
	s = reMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func finalCleanup(s string) string {
	s = spacify(s)
	s = reLeadTrailPunct.ReplaceAllString(s, "")
	s = reMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// --- Herencia de carpeta (Brief §1.5) ----------------------------------------

func folderSeason(folder string) int {
	if m := reSeasonWord.FindStringSubmatch(folder); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if m := reFolderT.FindStringSubmatch(folder); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if m := reFolderS.FindStringSubmatch(folder); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func folderTitle(folder string) string {
	work := stripBracketsKeepYear(folder)
	work = reSeasonWord.ReplaceAllString(work, " ")
	work = reFolderT.ReplaceAllString(work, " ")
	work = reFolderS.ReplaceAllString(work, " ")
	work = stripTechnicalTokens(work)
	work = stripLangWords(work)
	return finalCleanup(work)
}
