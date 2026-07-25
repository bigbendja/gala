package ingest

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// CSVEntry es una fila de un CSV de ingesta (Brief M5 §1: "soporte mínimo:
// una columna de URL y opcionalmente title/year").
type CSVEntry struct {
	URL   string
	Title string
	Year  int
}

// ParseCSV interpreta un CSV con cabecera que declara al menos una columna
// url/link/enlace, y opcionalmente title/año. Una fila sin URL se ignora
// (no aborta el resto: "nada se pierde en silencio" es cosa del llamante,
// que cuenta cuántas filas se ignoraron si le interesa).
func ParseCSV(content string) ([]CSVEntry, error) {
	lines := nonEmptyLines(content)
	if len(lines) == 0 {
		return nil, nil
	}
	delim := detectDelimiter(lines[0])
	if delim == 0 {
		delim = ','
	}

	r := csv.NewReader(strings.NewReader(content))
	r.Comma = delim
	r.FieldsPerRecord = -1 // tolerante a filas con nº de campos distinto de la cabecera
	r.TrimLeadingSpace = true
	r.LazyQuotes = true

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("ingest: leer CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	header := rows[0]
	urlCol, titleCol, yearCol := -1, -1, -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "url", "link", "enlace":
			urlCol = i
		case "title", "titulo", "título", "name", "nombre":
			titleCol = i
		case "year", "año", "ano":
			yearCol = i
		}
	}
	if urlCol < 0 {
		return nil, fmt.Errorf("ingest: CSV sin columna url/link/enlace reconocible")
	}

	out := make([]CSVEntry, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if urlCol >= len(row) {
			continue
		}
		url := strings.TrimSpace(row[urlCol])
		if url == "" {
			continue
		}
		e := CSVEntry{URL: url}
		if titleCol >= 0 && titleCol < len(row) {
			e.Title = strings.TrimSpace(row[titleCol])
		}
		if yearCol >= 0 && yearCol < len(row) {
			if y, err := strconv.Atoi(strings.TrimSpace(row[yearCol])); err == nil {
				e.Year = y
			}
		}
		out = append(out, e)
	}
	return out, nil
}
