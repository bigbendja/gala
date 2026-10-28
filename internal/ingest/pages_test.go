package ingest

import (
	"context"
	"strings"
	"testing"
)

// Una página (por extensión o porque el servidor responde HTML) NO crea item
// ni fuente: se cuenta como omitida y el motivo sale en los detalles.
func TestPipelineWebPagesAreRejectedNotCatalogued(t *testing.T) {
	p, repo := newPipelineForTest(t, map[string]fakeM3UResponse{
		"paginas.example/pelicula/12345": {contentType: "text/html; charset=utf-8", body: "<html>"},
		"video.example/watch":            {contentType: "text/html"},
		"cdn.example/stream/Dune.2021":   {contentType: "video/mp4"},
	})
	blob := strings.Join([]string{
		"https://paginas.example/pelicula/12345", // sin extensión, el servidor dice HTML
		"https://video.example/watch",            // idem
		"https://web.example/ver/peli.html",      // por extensión, sin preguntar
		"https://cdn.example/v/pelicula.mp4",     // vídeo directo por extensión
		"https://cdn.example/stream/Dune.2021",   // sin extensión, el servidor dice vídeo
	}, "\n")

	res, err := p.Run(context.Background(), blob, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.OmitidosNoDirectos != 3 || res.Anadidos != 2 {
		t.Fatalf("OmitidosNoDirectos=%d Anadidos=%d, esperado 3/2: %+v", res.OmitidosNoDirectos, res.Anadidos, res)
	}
	assertResumenInvariant(t, res)
	if n := countItems(t, repo); n != 2 {
		t.Errorf("items en catálogo = %d, esperado 2 (las páginas no crean nada)", n)
	}
	if len(res.Detalles) == 0 || res.Detalles[0] != MensajePaginaWeb {
		t.Fatalf("el primer detalle debe ser el mensaje claro, hay %q", res.Detalles)
	}
	if !strings.Contains(strings.Join(res.Detalles, "\n"), "paginas.example/pelicula/12345") {
		t.Errorf("los detalles deben nombrar el enlace rechazado: %q", res.Detalles)
	}
}

// Sin conexión o ante cualquier duda no se rechaza: la ingesta funciona
// offline y la salud (M6) ya dirá si el enlace sirve.
func TestPipelineUnreachableUnknownLinkStillCatalogued(t *testing.T) {
	// El transporte falso no conoce el host: la petición falla (como sin red).
	p, _ := newPipelineForTest(t, map[string]fakeM3UResponse{})
	res, err := p.Run(context.Background(), "https://sin-red.example/algo/Dune.2021", "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Anadidos != 1 || res.OmitidosNoDirectos != 0 {
		t.Fatalf("Anadidos=%d OmitidosNoDirectos=%d, esperado 1/0", res.Anadidos, res.OmitidosNoDirectos)
	}
	assertResumenInvariant(t, res)
}

// Un destino interno no se pregunta (SSRF) y sigue yendo por su camino de siempre.
func TestPipelineWebPageProbeSkipsBlockedTargets(t *testing.T) {
	p, _ := newPipelineForTest(t, map[string]fakeM3UResponse{})
	res, err := p.Run(context.Background(), "http://192.168.1.10/pagina/3", "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.SinClasificar != 1 || res.OmitidosNoDirectos != 0 {
		t.Fatalf("SinClasificar=%d OmitidosNoDirectos=%d, esperado 1/0", res.SinClasificar, res.OmitidosNoDirectos)
	}
	assertResumenInvariant(t, res)
}

// El mensaje al usuario no nombra herramientas ni hosts ni cómo conseguir el vídeo.
func TestMensajePaginaWebKeepsTheWall(t *testing.T) {
	for _, bad := range []string{"yt-dlp", "youtube-dl", "descarg", "extrae", "http"} {
		if strings.Contains(strings.ToLower(MensajePaginaWeb), bad) {
			t.Errorf("el mensaje contiene %q", bad)
		}
	}
	for _, want := range []string{"página web", "enlace directo a un vídeo", "archivos locales", "listas M3U"} {
		if !strings.Contains(MensajePaginaWeb, want) {
			t.Errorf("el mensaje debe decir %q", want)
		}
	}
}
