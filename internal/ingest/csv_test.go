package ingest

import "testing"

func TestParseCSVBasic(t *testing.T) {
	content := "title,url,year\nPeli A,http://x.com/a.mp4,2020\nPeli B,http://x.com/b.mp4,2021\n"
	entries, err := ParseCSV(content)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, esperado 2", len(entries))
	}
	if entries[0] != (CSVEntry{URL: "http://x.com/a.mp4", Title: "Peli A", Year: 2020}) {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if entries[1] != (CSVEntry{URL: "http://x.com/b.mp4", Title: "Peli B", Year: 2021}) {
		t.Errorf("entries[1] = %+v", entries[1])
	}
}

func TestParseCSVURLOnlyColumn(t *testing.T) {
	entries, err := ParseCSV("url\nhttp://x.com/a.mp4\nhttp://x.com/b.mp4\n")
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(entries) != 2 || entries[0].Title != "" || entries[0].Year != 0 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseCSVSemicolonDelimiter(t *testing.T) {
	entries, err := ParseCSV("url;title\nhttp://x.com/a.mp4;Peli A\n")
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(entries) != 1 || entries[0].Title != "Peli A" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseCSVSkipsRowsWithoutURL(t *testing.T) {
	entries, err := ParseCSV("title,url\nSin URL,\nCon URL,http://x.com/a.mp4\n")
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(entries) != 1 || entries[0].Title != "Con URL" {
		t.Fatalf("entries = %+v, esperado solo la fila con URL", entries)
	}
}

func TestParseCSVNoURLColumnErrors(t *testing.T) {
	_, err := ParseCSV("title,year\nPeli A,2020\n")
	if err == nil {
		t.Fatal("ParseCSV = nil error, esperado error (sin columna url/link/enlace)")
	}
}

func TestParseCSVEmpty(t *testing.T) {
	entries, err := ParseCSV("")
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, esperado vacío", entries)
	}
}
