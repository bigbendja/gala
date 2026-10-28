package ingest

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bigbendja/gala/internal/providers/httpprovider"
)

// MensajePaginaWeb es lo que se le dice al usuario cuando un enlace es una
// página web y no un vídeo. Sin nombres de hosts ni de herramientas: GALA no
// sugiere cómo conseguir el vídeo.
const MensajePaginaWeb = "Este enlace es una página web, no un enlace directo a un vídeo. " +
	"GALA reproduce archivos locales, enlaces directos a vídeo y listas M3U."

const (
	// pageProbeTimeout acota la comprobación de CADA enlace; pageProbeBudget,
	// la de TODA la ingesta (lo que no dé tiempo se queda como estaba: se
	// cataloga y la salud de M6 dirá si funciona).
	pageProbeTimeout = 5 * time.Second
	pageProbeBudget  = 45 * time.Second
	pageProbeWorkers = 8
	// maxPageDetails: cuántos enlaces rechazados se nombran en los detalles.
	maxPageDetails = 5
)

// markWebPages pregunta al servidor de cada candidato de clase "desconocido"
// (http(s) sin extensión de vídeo) qué tipo de contenido sirve: si es HTML es
// una página web y se marca ClassWebPage. Todo lo demás (vídeo, binario, sin
// respuesta, sin conexión, destino interno) queda como estaba: la ingesta
// funciona offline y nunca descarta por no poder comprobar. Las llamadas
// van en paralelo y con tiempo acotado.
func (p *Pipeline) markWebPages(ctx context.Context, cands []Candidate) {
	var idx []int
	for i, c := range cands {
		if c.Class == ClassUnknown {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return
	}
	bctx, cancel := context.WithTimeout(ctx, pageProbeBudget)
	defer cancel()

	var mu sync.Mutex
	hostBlocked := map[string]bool{} // veredicto SSRF por host, una vez por ingesta
	blocked := func(raw string) bool {
		host := raw
		if u, err := url.Parse(raw); err == nil {
			host = u.Hostname()
		}
		mu.Lock()
		v, ok := hostBlocked[host]
		mu.Unlock()
		if ok {
			return v
		}
		cctx, c := context.WithTimeout(bctx, ssrfDNSTimeout)
		defer c()
		err := httpprovider.ValidateExternalURL(cctx, raw)
		v = err != nil // si no se puede validar, tampoco se pregunta
		mu.Lock()
		hostBlocked[host] = v
		mu.Unlock()
		return v
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < pageProbeWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if bctx.Err() != nil || blocked(cands[i].Locator) {
					continue
				}
				if p.serverSaysHTML(bctx, cands[i].Locator) {
					cands[i].Class = ClassWebPage
				}
			}
		}()
	}
	for _, i := range idx {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

// serverSaysHTML hace una petición mínima (solo cabeceras, con Range por si
// el servidor ignora que no se quiere el cuerpo) y dice si el Content-Type
// es HTML. Cualquier error o duda es false.
func (p *Pipeline) serverSaysHTML(ctx context.Context, rawURL string) bool {
	cctx, cancel := context.WithTimeout(ctx, pageProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-1023")
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	return strings.HasPrefix(ct, "text/html") || strings.HasPrefix(ct, "application/xhtml")
}

// rejectWebPageDetails añade a los detalles el motivo de los enlaces que eran
// páginas web (el mensaje una vez y los primeros enlaces afectados).
func rejectWebPageDetails(res *ResumenIngesta, links []string) {
	if len(links) == 0 {
		return
	}
	res.Detalles = append(res.Detalles, MensajePaginaWeb)
	for i, l := range links {
		if i == maxPageDetails {
			res.Detalles = append(res.Detalles, fmt.Sprintf("…y %d enlaces más de este tipo.", len(links)-maxPageDetails))
			break
		}
		res.Detalles = append(res.Detalles, "· "+l)
	}
}
