package ingest

import "testing"

func TestClassifyURLDirect(t *testing.T) {
	cases := []string{
		"https://cdn.ejemplo.net/v/pelicula_hd.mp4",
		"http://ejemplo.net/serie.mkv",
		"https://ejemplo.net/video.webm",
		"http://ejemplo.net/pelicula.avi",
		"https://ejemplo.net/clip.ts?token=abc",
		"https://ejemplo.net/pelicula.MP4", // extensión en mayúsculas
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			got, err := ClassifyURL(u)
			if err != nil {
				t.Fatalf("ClassifyURL: %v", err)
			}
			if got != ClassDirect {
				t.Errorf("ClassifyURL(%q) = %v, esperado directo", u, got)
			}
		})
	}
}

func TestClassifyURLM3URemote(t *testing.T) {
	for _, u := range []string{"http://ejemplo.net/lista.m3u", "http://ejemplo.net/canal.m3u8"} {
		got, err := ClassifyURL(u)
		if err != nil {
			t.Fatalf("ClassifyURL: %v", err)
		}
		if got != ClassM3URemote {
			t.Errorf("ClassifyURL(%q) = %v, esperado m3u_remoto", u, got)
		}
	}
}

func TestClassifyURLDriveAndMega(t *testing.T) {
	cases := map[string]URLClass{
		"https://drive.google.com/file/d/1xK2/view": ClassDrive,
		"https://docs.google.com/file/d/1xK2/view":  ClassDrive,
		"https://mega.nz/file/abc123":               ClassMega,
		"https://mega.co.nz/file/abc123":            ClassMega,
	}
	for u, want := range cases {
		got, err := ClassifyURL(u)
		if err != nil {
			t.Fatalf("ClassifyURL(%q): %v", u, err)
		}
		if got != want {
			t.Errorf("ClassifyURL(%q) = %v, esperado %v", u, got, want)
		}
	}
}

// TestClassifyURLUnsupportedHosts cubre "host_no_soportado NO crea entrada
// Y SÍ se cuenta en el resumen" a nivel de clasificación (el pipeline_test.go
// verifica el efecto end-to-end).
func TestClassifyURLUnsupportedHosts(t *testing.T) {
	cases := []string{
		"https://streamtape.com/e/abc123",
		"https://www.streamtape.com/e/abc123", // con www.
		"https://filemoon.sx/e/abc123",
		"https://vidoza.net/abc123",
		"https://doodstream.com/e/abc123",
		"https://dood.to/e/abc123",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			got, err := ClassifyURL(u)
			if err != nil {
				t.Fatalf("ClassifyURL: %v", err)
			}
			if got != ClassUnsupported {
				t.Errorf("ClassifyURL(%q) = %v, esperado host_no_soportado", u, got)
			}
		})
	}
}

func TestClassifyURLUnknown(t *testing.T) {
	got, err := ClassifyURL("https://ejemplo.net/pagina.html")
	if err != nil {
		t.Fatalf("ClassifyURL: %v", err)
	}
	if got != ClassUnknown {
		t.Errorf("ClassifyURL = %v, esperado desconocido", got)
	}
}

func TestClassifyURLRejectsNonHTTP(t *testing.T) {
	for _, u := range []string{"ftp://ejemplo.net/a.mp4", "not a url at all", ""} {
		if _, err := ClassifyURL(u); err == nil {
			t.Errorf("ClassifyURL(%q) = nil error, esperado error", u)
		}
	}
}
