package ingest

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/providers/httpprovider"
)

func countItems(t *testing.T, repo *catalog.Repo) int {
	t.Helper()
	n, err := repo.CountItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Un enlace directo a localhost o a una red privada no se cataloga: mismo
// bloqueo SSRF (M5) que ya aplicaban fetchM3U, los .strm y la salud.
func TestPipelineRunBlocksPrivateAndLoopbackLinks(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	blob := strings.Join([]string{
		"http://127.0.0.1:7420/api/v1/settings/tmdb",
		"http://localhost/x.mp4",
		"http://192.168.1.50/privado.mp4",
		"http://10.0.0.5/a.mp4",
		"http://172.16.4.4/b.mp4",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/c.mp4",
		"https://cdn.example.com/publico.mp4",
	}, "\n")

	res, err := p.Run(context.Background(), blob, "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertResumenInvariant(t, res)

	if res.Anadidos != 1 {
		t.Errorf("Anadidos = %d, esperado 1 (solo el enlace público): %+v", res.Anadidos, res)
	}
	if res.SinClasificar != 7 {
		t.Errorf("SinClasificar = %d, esperado 7 bloqueados: %+v", res.SinClasificar, res)
	}
	if got := countItems(t, repo); got != 1 {
		t.Errorf("items = %d, esperado 1: ningún enlace interno debe catalogarse", got)
	}
	blocked := 0
	for _, d := range res.Detalles {
		if strings.Contains(d, "bloqueado por seguridad") {
			blocked++
		}
	}
	if blocked != 7 {
		t.Errorf("detalles con el motivo = %d, esperado 7: %v", blocked, res.Detalles)
	}
}

// Un nombre de host que RESUELVE a una IP interna también se bloquea.
func TestPipelineRunBlocksHostnameResolvingToPrivateIP(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	httpprovider.LookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("192.168.0.9")}}, nil
	}

	res, err := p.Run(context.Background(), "http://nas.casa.example/peli.mp4", "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertResumenInvariant(t, res)
	if res.SinClasificar != 1 || res.Anadidos != 0 || countItems(t, repo) != 0 {
		t.Errorf("resumen = %+v, items = %d", res, countItems(t, repo))
	}
}

// Las entradas de una M3U pegada pasan por el mismo filtro.
func TestPipelineRunBlocksPrivateEntriesInPastedM3U(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	m3u := "#EXTM3U\n#EXTINF:-1,Canal Interno\nhttp://192.168.1.77:8080/stream.ts\n#EXTINF:-1,Canal Publico\nhttps://cdn.example.com/canal.mp4\n"

	res, err := p.Run(context.Background(), m3u, "lista.m3u", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertResumenInvariant(t, res)
	if res.Anadidos != 1 || res.SinClasificar != 1 || countItems(t, repo) != 1 {
		t.Errorf("resumen = %+v, items = %d", res, countItems(t, repo))
	}
}

// Sin conexión (DNS que falla) un enlace válido SÍ se cataloga: la ingesta
// funciona offline y es la salud (M6) la que dirá si está caído.
func TestPipelineRunKeepsLinkWhenDNSFails(t *testing.T) {
	p, repo := newPipelineForTest(t, nil)
	httpprovider.LookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return nil, &net.DNSError{Err: "sin red", Name: host}
	}

	res, err := p.Run(context.Background(), "https://cdn.example.com/offline.mp4", "", testNow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertResumenInvariant(t, res)
	if res.Anadidos != 1 || countItems(t, repo) != 1 {
		t.Errorf("resumen = %+v, items = %d", res, countItems(t, repo))
	}
}
