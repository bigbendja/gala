package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectInputTypeM3U(t *testing.T) {
	cases := []struct {
		name, blob, filename string
	}{
		{"prefijo EXTM3U", "#EXTM3U\n#EXTINF:-1,X\nhttp://x.com/a.mp4\n", ""},
		{"extensión .m3u del archivo", "http://x.com/a.mp4", "lista.m3u"},
		{"extensión .m3u8 del archivo", "http://x.com/a.mp4", "lista.M3U8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectInputType(tc.blob, tc.filename); got != InputM3U {
				t.Errorf("DetectInputType = %v, esperado InputM3U", got)
			}
		})
	}
}

func TestDetectInputTypeCSV(t *testing.T) {
	cases := []struct {
		name, blob, filename string
	}{
		{"extensión .csv", "title,url\nA,http://x.com/a.mp4", "listado.csv"},
		{"cabecera url,title,year con comas", "url,title,year\nhttp://x.com/a.mp4,Peli A,2020\nhttp://x.com/b.mp4,Peli B,2021", ""},
		{"cabecera con punto y coma", "url;title\nhttp://x.com/a.mp4;Peli A\nhttp://x.com/b.mp4;Peli B", ""},
		{"columna 'enlace' en español", "enlace,titulo\nhttp://x.com/a.mp4,Peli A\nhttp://x.com/b.mp4,Peli B", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectInputType(tc.blob, tc.filename); got != InputCSV {
				t.Errorf("DetectInputType(%q) = %v, esperado InputCSV", tc.blob, got)
			}
		})
	}
}

// TestDetectInputTypeDudosoCaeATexto cubre "si dudoso, tratar como texto
// suelto" (Brief §1): comas sueltas sin columna url reconocible, o solo una
// línea, no deben confundirse con CSV.
func TestDetectInputTypeDudosoCaeATexto(t *testing.T) {
	if got := DetectInputType("nombre,descripcion\nHola,Mundo", ""); got != InputText {
		t.Errorf("DetectInputType = %v, esperado InputText (sin columna url reconocible)", got)
	}
	if got := DetectInputType("mira, esto es un mensaje suelto, con comas", ""); got != InputText {
		t.Errorf("DetectInputType = %v, esperado InputText (una sola línea)", got)
	}
}

func TestDetectInputTypeTextoSuelto(t *testing.T) {
	blob := "🎬 mira lo q me pasaron!!\nhttps://drive.google.com/file/d/1xK2/view\nhttps://cdn.ejemplo.net/v/pelicula_hd.mp4 👌👌"
	if got := DetectInputType(blob, ""); got != InputText {
		t.Errorf("DetectInputType = %v, esperado InputText", got)
	}
}

func TestDetectInputTypeEmptyBlob(t *testing.T) {
	if got := DetectInputType("", ""); got != InputText {
		t.Errorf("DetectInputType(vacío) = %v, esperado InputText (no debe explotar ni clasificar como otra cosa)", got)
	}
}

func TestExtractURLsFromMessyText(t *testing.T) {
	blob := "🎬 mira lo q me pasaron!! (link: https://drive.google.com/file/d/1xK2/view) " +
		"y también http://ejemplo.net/pack_series.m3u, y esta: https://cdn.ejemplo.net/v/pelicula_hd.mp4 👌👌 " +
		"no es url: contacta@ejemplo.com"
	urls := ExtractURLs(blob)
	want := []string{
		"https://drive.google.com/file/d/1xK2/view",
		"http://ejemplo.net/pack_series.m3u",
		"https://cdn.ejemplo.net/v/pelicula_hd.mp4",
	}
	if len(urls) != len(want) {
		t.Fatalf("ExtractURLs = %v, esperado %v", urls, want)
	}
	for i, w := range want {
		if urls[i] != w {
			t.Errorf("urls[%d] = %q, esperado %q", i, urls[i], w)
		}
	}
}

func TestExtractLocalPathsOnlyExistingOnes(t *testing.T) {
	dir := t.TempDir()
	existingFile := filepath.Join(dir, "peli.mp4")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	blob := "🎬 mira esto\n" + existingFile + "\nC:\\Ruta\\Que\\No\\Existe\\peli.mp4\nhttps://x.com/a.mp4"
	got := ExtractLocalPaths(blob)
	if len(got) != 1 || got[0].Path != existingFile || got[0].IsDir {
		t.Fatalf("ExtractLocalPaths = %+v, esperado solo %q (archivo)", got, existingFile)
	}
}

func TestExtractLocalPathsDetectsDirectory(t *testing.T) {
	dir := t.TempDir()
	blob := dir
	got := ExtractLocalPaths(blob)
	if len(got) != 1 || !got[0].IsDir {
		t.Fatalf("ExtractLocalPaths = %+v, esperado 1 carpeta", got)
	}
}
