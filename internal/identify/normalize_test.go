package identify

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// --- Test de corpus (Brief M3 §6) -------------------------------------------

type expectedRow struct {
	filename, folder       string
	title                  string
	year                   int
	kind                   string
	season, episode, absep int
	junk, extra            bool
	line                   int
}

func loadExpected(t *testing.T) []expectedRow {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "corpus_expected.tsv")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("abrir corpus_expected.tsv: %v", err)
	}
	defer f.Close()

	var rows []expectedRow
	sc := bufio.NewScanner(f)
	lineNo := 0
	header := false
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		if !header {
			header = true
			continue // línea de cabecera
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 10 {
			t.Fatalf("línea %d: se esperaban 10 columnas, hay %d: %q", lineNo, len(cols), line)
		}
		folder := cols[1]
		if folder == "-" {
			folder = ""
		}
		row := expectedRow{
			filename: cols[0], folder: folder, title: cols[2],
			kind: cols[4], line: lineNo,
		}
		row.year = atoi(t, lineNo, cols[3])
		row.season = atoi(t, lineNo, cols[5])
		row.episode = atoi(t, lineNo, cols[6])
		row.absep = atoi(t, lineNo, cols[7])
		row.junk = cols[8] == "true"
		row.extra = cols[9] == "true"
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("leer corpus_expected.tsv: %v", err)
	}
	if len(rows) < 45 {
		t.Fatalf("corpus_expected.tsv tiene %d líneas; se esperaban ~50 (Brief M3 §6)", len(rows))
	}
	return rows
}

func atoi(t *testing.T, line int, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("línea %d: %q no es un entero", line, s)
	}
	return n
}

func TestCorpusExpected(t *testing.T) {
	for _, row := range loadExpected(t) {
		row := row
		t.Run(fmt.Sprintf("%d_%s", row.line, row.filename), func(t *testing.T) {
			p := Parse(row.filename, row.folder)
			if p.Title != row.title {
				t.Errorf("Title = %q, esperado %q", p.Title, row.title)
			}
			if p.Year != row.year {
				t.Errorf("Year = %d, esperado %d", p.Year, row.year)
			}
			if p.Kind != row.kind {
				t.Errorf("Kind = %q, esperado %q", p.Kind, row.kind)
			}
			if p.Season != row.season {
				t.Errorf("Season = %d, esperado %d", p.Season, row.season)
			}
			if p.Episode != row.episode {
				t.Errorf("Episode = %d, esperado %d", p.Episode, row.episode)
			}
			if p.AbsoluteEp != row.absep {
				t.Errorf("AbsoluteEp = %d, esperado %d", p.AbsoluteEp, row.absep)
			}
			if p.Junk != row.junk {
				t.Errorf("Junk = %v, esperado %v", p.Junk, row.junk)
			}
			if p.Extra != row.extra {
				t.Errorf("Extra = %v, esperado %v", p.Extra, row.extra)
			}
		})
	}
}

// --- Invariantes sobre el corpus completo (Brief M3 §6) --------------------

// TestCorpusInvariants recorre testdata/corpus_nombres.txt entero (no solo
// las ~50 líneas curadas): ninguna línea debe hacer panic, la familia
// "Basura realista" debe detectarse entera como Junk, la familia "Trampas"
// entera como Extra, y ningún item Junk o Extra debe producir un Kind útil.
func TestCorpusInvariants(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "corpus_nombres.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("abrir corpus_nombres.txt: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var family, dir string
	seenBasura, seenTrampas := false, false
	total := 0

	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			family = line
			dir = ""
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "DIR:") {
			dir = strings.TrimSpace(strings.TrimPrefix(line, "DIR:"))
			continue
		}

		total++
		var p Parsed
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Parse(%q, %q) hizo panic: %v", line, dir, r)
				}
			}()
			p = Parse(line, dir)
		}()

		if (p.Junk || p.Extra) && p.Kind != KindUnknown {
			t.Errorf("%q: Junk=%v Extra=%v pero Kind=%q (debería ser %q)",
				line, p.Junk, p.Extra, p.Kind, KindUnknown)
		}

		switch {
		case strings.Contains(family, "Basura realista"):
			seenBasura = true
			if !p.Junk {
				t.Errorf("%q: se esperaba Junk=true (familia %q)", line, family)
			}
		case strings.Contains(family, "Trampas"):
			seenTrampas = true
			if !p.Extra {
				t.Errorf("%q: se esperaba Extra=true (familia %q)", line, family)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("leer corpus_nombres.txt: %v", err)
	}
	if total < 200 {
		t.Fatalf("solo se procesaron %d líneas del corpus; ¿cambió el formato del archivo?", total)
	}
	if !seenBasura {
		t.Fatal("no se encontró la familia 'Basura realista' en el corpus")
	}
	if !seenTrampas {
		t.Fatal("no se encontró la familia 'Trampas' en el corpus")
	}
}

// --- Tests por regla (Brief M3 §1: "cada regla con tests propios") --------

func TestJunkPatterns(t *testing.T) {
	cases := []string{
		"VID-20230412-WA0012.mp4",
		"WhatsApp Video 2024-11-02 at 09.14.55.mp4",
		"video_2024-06-15_18-22-10.mp4",
		"20230101_123456.mp4",
		"IMG_4032.MOV",
		"descarga (3).mp4",
		"video sin titulo.mp4",
		"pelicula.sin.identificar.mkv",
	}
	for _, name := range cases {
		if p := Parse(name, ""); !p.Junk {
			t.Errorf("Parse(%q).Junk = false, esperado true", name)
		}
	}
}

func TestJunkNotOverEager(t *testing.T) {
	// Estos NO son basura: un release con estructura reconocible (SxxEyy o
	// calidad/fuente) no debe caer en la heurística de vocabulario de basura
	// aunque contenga palabras sueltas parecidas ("FINAL" como tag de escena).
	cases := []string{
		"Vikings.S06E20.FINAL.720p.HDTV.x264-CRAVERS[eztv].mkv",
		"Inception.2010.1080p.BluRay.x264-SPARKS.mkv",
	}
	for _, name := range cases {
		if p := Parse(name, ""); p.Junk {
			t.Errorf("Parse(%q).Junk = true, esperado false", name)
		}
	}
}

func TestExtraPatterns(t *testing.T) {
	cases := []string{
		"Inception.2010.1080p.BluRay.x264-SPARKS.sample.mkv",
		"sample.mkv",
		"sample.avi",
		"Dune.2021.TRAILER.1080p.mp4",
		"Making.of.The.Lord.of.the.Rings.mkv",
		"RARBG.mp4",
		"SAMPLE-Inception.2010.mkv",
	}
	for _, name := range cases {
		if p := Parse(name, ""); !p.Extra {
			t.Errorf("Parse(%q).Extra = false, esperado true", name)
		}
	}
}

func TestBracketStripping(t *testing.T) {
	p := Parse("El Laberinto del Fauno [2006] [DVDRip] [Español Latino].avi", "")
	if p.Title != "El Laberinto del Fauno" || p.Year != 2006 {
		t.Errorf("Title=%q Year=%d, esperado 'El Laberinto del Fauno' / 2006", p.Title, p.Year)
	}
	// El año en paréntesis se conserva aunque todo lo demás entre
	// corchetes/paréntesis se descarte.
	p2 := Parse("Whiplash (2014) [1080p] [YTS.AG].mp4", "")
	if p2.Title != "Whiplash" || p2.Year != 2014 || p2.Quality != "1080p" {
		t.Errorf("Title=%q Year=%d Quality=%q, esperado 'Whiplash' / 2014 / 1080p", p2.Title, p2.Year, p2.Quality)
	}
}

func TestSeasonEpisodePriority(t *testing.T) {
	// SxxEyy gana sobre cualquier otro patrón presente en el mismo nombre.
	p := Parse("La Reina del Sur - 3x01 - Capitulo 41.mp4", "")
	if p.Season != 3 || p.Episode != 1 || p.Title != "La Reina del Sur" {
		t.Errorf("got Season=%d Episode=%d Title=%q, esperado 3/1/'La Reina del Sur'", p.Season, p.Episode, p.Title)
	}

	// Cap.NNN literal: los 1-2 primeros dígitos son la temporada, los 2
	// últimos el episodio (Brief §1.4).
	s, e := splitCapDigits("105")
	if s != 1 || e != 5 {
		t.Errorf("splitCapDigits(105) = %d,%d; esperado 1,5 (Cap.105→T1E05)", s, e)
	}
	s, e = splitCapDigits("1204")
	if s != 12 || e != 4 {
		t.Errorf("splitCapDigits(1204) = %d,%d; esperado 12,4 (Cap.1204→T12E04)", s, e)
	}

	// Forma en palabras (sin SxxEyy ni NxNN).
	p2 := Parse("Peaky.Blinders.Temporada.5.Episodio.3.ESPAÑOL.1080p.mkv", "")
	if p2.Season != 5 || p2.Episode != 3 || p2.Kind != KindSeriesEpisode {
		t.Errorf("got Season=%d Episode=%d Kind=%q, esperado 5/3/series_episode", p2.Season, p2.Episode, p2.Kind)
	}

	// Numeración absoluta anime.
	p3 := Parse("[HorribleSubs] Attack on Titan - 25 [480p].mkv", "")
	if p3.AbsoluteEp != 25 || p3.Season != 0 || p3.Episode != 0 {
		t.Errorf("got AbsoluteEp=%d Season=%d Episode=%d, esperado 25/0/0", p3.AbsoluteEp, p3.Season, p3.Episode)
	}
}

func TestFolderInheritance(t *testing.T) {
	// E01.mkv por sí solo no produce título útil; la carpeta lo aporta.
	p := Parse("E01.mkv", "Breaking Bad Temporada 2 [720p][Castellano]")
	if p.Title != "Breaking Bad" || p.Season != 2 || p.Episode != 1 {
		t.Errorf("got Title=%q Season=%d Episode=%d, esperado 'Breaking Bad'/2/1", p.Title, p.Season, p.Episode)
	}

	// merli_301 + carpeta "Merli Temporada 3": 301 se interpreta como
	// Cap→T3E01, coherente con la temporada de la carpeta (Brief §1.5).
	p2 := Parse("merli_301.mp4", "Merli Temporada 3")
	if p2.Title != "Merli" || p2.Season != 3 || p2.Episode != 1 {
		t.Errorf("got Title=%q Season=%d Episode=%d, esperado 'Merli'/3/1", p2.Title, p2.Season, p2.Episode)
	}
	if p2.FolderContradiction() {
		t.Error("FolderContradiction = true, esperado false (301 coincide con Temporada 3)")
	}

	// Contradicción: el archivo tiene identidad propia y fuerte (SxxEyy);
	// se conserva su propia temporada pero se marca contradicción interna
	// para que el matcher baje la confianza (Brief §1.5, decisión anotada
	// en el informe final: "manda la carpeta" solo aplica cuando la señal
	// del archivo es débil/fusionada, no cuando ya es un SxxEyy explícito).
	p3 := Parse("Better.Call.Saul.S01E05.SPANISH.HDTV.x264.mkv", "Breaking Bad Temporada 3 [HDTV][Castellano]")
	if p3.Title != "Better Call Saul" || p3.Season != 1 || p3.Episode != 5 {
		t.Errorf("got Title=%q Season=%d Episode=%d, esperado 'Better Call Saul'/1/5", p3.Title, p3.Season, p3.Episode)
	}
	if !p3.FolderContradiction() {
		t.Error("FolderContradiction = false, esperado true (carpeta anuncia Breaking Bad T3)")
	}
}

func TestYearExtraction(t *testing.T) {
	cases := []struct {
		name  string
		year  int
		title string
	}{
		// Último año gana; el anterior (que también parece año) queda en el título.
		{"Blade.Runner.2049.2017.1080p.BluRay.x264.mkv", 2017, "Blade Runner 2049"},
		// El título completo ES un año: solo cuenta como año el SEGUNDO token.
		{"2012.2009.1080p.BluRay.x264.mkv", 2009, "2012"},
		{"1917.2019.720p.BluRay.x264-YOL0W.mkv", 2019, "1917"},
	}
	for _, c := range cases {
		p := Parse(c.name, "")
		if p.Year != c.year || p.Title != c.title {
			t.Errorf("Parse(%q) = Title=%q Year=%d; esperado Title=%q Year=%d",
				c.name, p.Title, p.Year, c.title, c.year)
		}
	}

	// Sin segundo token de año: "1899" no cae en el rango 1920-2049 del
	// regex de año, así que se queda como título tal cual (no hace falta
	// caso especial: la propia familia "título que es un año" del corpus
	// depende de este límite del regex).
	p := Parse("1899.S01E01.MULTI.1080p.WEB.H264-CiELOS.mkv", "")
	if p.Title != "1899" || p.Year != 0 {
		t.Errorf("got Title=%q Year=%d, esperado '1899'/0", p.Title, p.Year)
	}
}

func TestTechnicalTokenRemoval(t *testing.T) {
	p := Parse("La La Land 2016 1080p BluRay REMUX AVC DTS-HD MA 5.1-EPSiLON.mkv", "")
	if p.Title != "La La Land" {
		t.Errorf("Title = %q, esperado 'La La Land'", p.Title)
	}
	p2 := Parse("Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H265-FLUX.mkv", "")
	if p2.Title != "Dune Part Two" {
		t.Errorf("Title = %q, esperado 'Dune Part Two'", p2.Title)
	}
}

func TestLanguageExtraction(t *testing.T) {
	p := Parse("Le Comte de Monte-Cristo.2024.MULTi.1080p.WEB.H264-FW.mkv", "")
	found := false
	for _, l := range p.Langs {
		if l == "multi" {
			found = true
		}
	}
	if !found {
		t.Errorf("Langs = %v, esperaba incluir 'multi'", p.Langs)
	}

	p2 := Parse("El Viaje de Chihiro (2001) [BDRip 1080p][Cast-Jap+Subs].mkv", "")
	want := map[string]bool{"es": true, "ja": true}
	got := map[string]bool{}
	for _, l := range p2.Langs {
		got[l] = true
	}
	for l := range want {
		if !got[l] {
			t.Errorf("Langs = %v, esperaba incluir %q", p2.Langs, l)
		}
	}
}

func TestFinalNormalizationPreservesInternalPunctuation(t *testing.T) {
	cases := map[string]string{
		"WALL-E.2008.1080p.BluRay.x264.mkv":                   "WALL-E",
		"Ocean's.Eleven.2001.1080p.BluRay.x264.mkv":           "Ocean's Eleven",
		"Merlí.Sapere.Aude.S01E01.CATALAN.720p.HDTV.x264.mkv": "Merlí Sapere Aude",
		"Océanos.mp4": "Océanos",
	}
	for name, want := range cases {
		if p := Parse(name, ""); p.Title != want {
			t.Errorf("Parse(%q).Title = %q, esperado %q", name, p.Title, want)
		}
	}
}
