package scan_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/providers/httpprovider"
	"github.com/bigbendja/gala/internal/scan"
)

// fakeDNS sustituye la resolución de nombres: los tests no dependen de la red.
func fakeDNS(t *testing.T, ip string, err error) {
	t.Helper()
	prev := httpprovider.LookupIPAddr
	httpprovider.LookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if err != nil {
			return nil, err
		}
		return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
	}
	t.Cleanup(func() { httpprovider.LookupIPAddr = prev })
}

func publicDNS(t *testing.T) { fakeDNS(t, "93.184.216.34", nil) }

func writeText(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanOnce(t *testing.T, ctx context.Context, repo *catalog.Repo, root string) *scan.Scanner {
	t.Helper()
	cfg := &config.Config{MediaFolders: []string{root}}
	s := scan.New(ctx, cfg, repo, fakeEnum{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := s.Start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitDone(t, s)
	return s
}

func listAll(t *testing.T, ctx context.Context, repo *catalog.Repo) []catalog.Item {
	t.Helper()
	res, err := repo.List(ctx, catalog.ListFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return res.Items
}

func TestStrmCreatesItemAndHTTPSource(t *testing.T) {
	publicDNS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	// Muy por debajo de MinFileSize: un .strm es un puntero, no un vídeo.
	writeText(t, filepath.Join(root, "Inception (2010).strm"), "https://cdn.example.com/inception.mp4\n")

	repo := newRepo(t)
	s := scanOnce(t, ctx, repo, root)

	items := listAll(t, ctx, repo)
	if len(items) != 1 {
		t.Fatalf("items=%d, esperado 1", len(items))
	}
	if items[0].Title != "Inception (2010)" {
		t.Fatalf("título=%q: debe salir del nombre del .strm sin extensión", items[0].Title)
	}
	if s.Status().ItemsNew != 1 {
		t.Fatalf("items_new=%d", s.Status().ItemsNew)
	}
	full, err := repo.Get(ctx, items[0].ID)
	if err != nil || len(full.Sources) != 1 {
		t.Fatalf("get: %v fuentes=%d", err, len(full.Sources))
	}
	src := full.Sources[0]
	if src.Provider != httpprovider.ProviderID || src.Locator != "https://cdn.example.com/inception.mp4" {
		t.Fatalf("fuente = %s %q", src.Provider, src.Locator)
	}
	if src.OriginPath == nil || filepath.Base(*src.OriginPath) != "Inception (2010).strm" {
		t.Fatalf("origin_path = %v", src.OriginPath)
	}

	// Idempotente: reescanear no duplica.
	scanOnce(t, ctx, repo, root)
	if got := len(listAll(t, ctx, repo)); got != 1 {
		t.Fatalf("tras reescanear items=%d, esperado 1", got)
	}
}

func TestStrmIgnoresCommentsAndTakesFirstValidURL(t *testing.T) {
	publicDNS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	writeText(t, filepath.Join(root, "Peli.2020.strm"),
		"# generado por una herramienta\r\n\r\n# http://comentario.example.com/no.mp4\r\nfile:///etc/passwd\r\n  https://real.example.com/a.mp4  \r\nhttps://otra.example.com/b.mp4\r\n")

	repo := newRepo(t)
	scanOnce(t, ctx, repo, root)

	items := listAll(t, ctx, repo)
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	full, _ := repo.Get(ctx, items[0].ID)
	if got := full.Sources[0].Locator; got != "https://real.example.com/a.mp4" {
		t.Fatalf("URL extraída = %q", got)
	}
}

func TestStrmEmptyOrWithoutURLCreatesNothing(t *testing.T) {
	publicDNS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	writeText(t, filepath.Join(root, "vacio.strm"), "")
	writeText(t, filepath.Join(root, "solo-comentarios.strm"), "# nada\n# aquí\n")
	writeText(t, filepath.Join(root, "ruta.strm"), "C:/videos/peli.mkv\n")
	writeText(t, filepath.Join(root, "ftp.strm"), "ftp://example.com/peli.mp4\n")

	repo := newRepo(t)
	s := scanOnce(t, ctx, repo, root) // no debe romper ni colgarse

	if got := len(listAll(t, ctx, repo)); got != 0 {
		t.Fatalf("items=%d: un .strm inválido no debe crear item ni fuente", got)
	}
	if s.Status().FilesSeen != 0 {
		t.Fatalf("files_seen=%d", s.Status().FilesSeen)
	}
}

func TestStrmToPrivateIPIsRejectedBySSRF(t *testing.T) {
	// Un nombre que resuelve a una IP interna también se rechaza.
	fakeDNS(t, "10.0.0.7", nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	writeText(t, filepath.Join(root, "a.strm"), "http://192.168.1.50/peli.mp4\n")
	writeText(t, filepath.Join(root, "b.strm"), "http://127.0.0.1:7420/api/v1/settings/tmdb\n")
	writeText(t, filepath.Join(root, "c.strm"), "http://169.254.169.254/latest/meta-data\n")
	writeText(t, filepath.Join(root, "d.strm"), "http://[::1]/x\n")
	writeText(t, filepath.Join(root, "e.strm"), "http://interno.example.com/x.mp4\n")

	repo := newRepo(t)
	scanOnce(t, ctx, repo, root)

	if got := len(listAll(t, ctx, repo)); got != 0 {
		t.Fatalf("items=%d: ninguna URL interna debe catalogarse", got)
	}
}

func TestStrmUnresolvableHostIsKeptForHealthToJudge(t *testing.T) {
	// Sin conexión el DNS falla: no es SSRF, es un enlace que M6 marcará.
	fakeDNS(t, "", &net.DNSError{Err: "sin red", Name: "cdn.example.com"})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	writeText(t, filepath.Join(root, "Offline.strm"), "https://cdn.example.com/x.mp4\n")

	repo := newRepo(t)
	scanOnce(t, ctx, repo, root)
	if got := len(listAll(t, ctx, repo)); got != 1 {
		t.Fatalf("items=%d, esperado 1", got)
	}
}

func TestStrmURLChangeFollowsTheFile(t *testing.T) {
	publicDNS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	p := filepath.Join(root, "Peli.strm")
	writeText(t, p, "https://cdn.example.com/v1.mp4\n")

	repo := newRepo(t)
	scanOnce(t, ctx, repo, root)
	writeText(t, p, "https://cdn.example.com/v2.mp4\n")
	scanOnce(t, ctx, repo, root)

	items := listAll(t, ctx, repo)
	if len(items) != 1 {
		t.Fatalf("items=%d: cambiar la URL no debe crear otro item", len(items))
	}
	full, _ := repo.Get(ctx, items[0].ID)
	if len(full.Sources) != 1 || full.Sources[0].Locator != "https://cdn.example.com/v2.mp4" {
		t.Fatalf("fuentes = %+v", full.Sources)
	}
}

func TestStrmRemovedAndReturningReusesItem(t *testing.T) {
	publicDNS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	p := filepath.Join(root, "Peli.strm")
	writeText(t, p, "https://cdn.example.com/v1.mp4\n")

	repo := newRepo(t)
	scanOnce(t, ctx, repo, root)
	first := listAll(t, ctx, repo)[0].ID

	// Se borra el .strm: la fuente se retira.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	scanOnce(t, ctx, repo, root)
	if full, _ := repo.Get(ctx, first); len(full.Sources) != 0 {
		t.Fatalf("fuentes tras borrar el .strm = %d", len(full.Sources))
	}

	// Vuelve: mismo item, no un duplicado.
	writeText(t, p, "https://cdn.example.com/v1.mp4\n")
	scanOnce(t, ctx, repo, root)
	items := listAll(t, ctx, repo)
	if len(items) != 1 || items[0].ID != first {
		t.Fatalf("items=%d, esperado reutilizar el item %d", len(items), first)
	}
	if full, _ := repo.Get(ctx, first); len(full.Sources) != 1 {
		t.Fatalf("fuentes tras volver = %d", len(full.Sources))
	}
}

func TestStrmTurnedInvalidDropsItsSource(t *testing.T) {
	publicDNS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	p := filepath.Join(root, "Peli.strm")
	writeText(t, p, "https://cdn.example.com/v1.mp4\n")

	repo := newRepo(t)
	scanOnce(t, ctx, repo, root)
	id := listAll(t, ctx, repo)[0].ID
	writeText(t, p, "")
	scanOnce(t, ctx, repo, root)

	if full, _ := repo.Get(ctx, id); len(full.Sources) != 0 {
		t.Fatalf("un .strm que se vació no debe conservar una fuente rota (%d)", len(full.Sources))
	}
}

func TestRemoveFolderSourcesAlsoCoversStrm(t *testing.T) {
	publicDNS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	writeText(t, filepath.Join(root, "Peli.strm"), "https://cdn.example.com/v1.mp4\n")

	repo := newRepo(t)
	scanOnce(t, ctx, repo, root)
	n, err := scan.RemoveFolderSources(ctx, repo, root)
	if err != nil || n != 1 {
		t.Fatalf("removidas=%d err=%v", n, err)
	}
	// Re-añadir la carpeta reconcilia en lugar de duplicar.
	scanOnce(t, ctx, repo, root)
	if got := len(listAll(t, ctx, repo)); got != 1 {
		t.Fatalf("items=%d tras re-añadir la carpeta", got)
	}
}
