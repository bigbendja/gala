package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/identify"
	"github.com/bigbendja/gala/internal/providers/httpprovider"
)

// maxCandidates es el límite de candidatos por ingesta (Brief M5 §6): un
// pegado de WhatsApp con miles de líneas no debe colgar la petición ni
// crear un catálogo inmanejable de un solo golpe.
const maxCandidates = 5000

// maxM3UBodyBytes acota la descarga de una lista M3U remota (Brief M5 §6:
// SSRF y robustez — nunca confiar en el tamaño que declara un servidor externo).
const maxM3UBodyBytes = 10 << 20 // 10 MB

// ResumenIngesta es el resultado de una ingesta (Brief M5 §4, corregido por
// el addendum post-entrega tras detectar que los contadores no cuadraban).
//
// NOTA (addendum, "por_confirmar" eliminado): el Brief original pedía un
// campo PorConfirmar. Se quitó a propósito: el enriquecimiento TMDB es
// asíncrono (como en M3), así que en el momento de ESTA respuesta ningún
// recién-añadido puede estar confirmado todavía — un campo "por_confirmar"
// SIEMPRE valdría lo mismo que Anadidos, en cualquier ejecución posible
// mientras esta arquitectura siga siendo asíncrona. Un campo que solo puede
// medir lo mismo que otro, con un nombre que sugiere que podría no hacerlo,
// miente por omisión; se prefiere no tenerlo.
type ResumenIngesta struct {
	// Detectados es TODO lo que entró por la caja antes de truncar por
	// límite o expandir M3U remotas: una URL de texto suelto, una fila de
	// CSV, o una entrada de una M3U pegada/subida directamente cuentan aquí
	// 1 a 1. Puede diferir de Total: una M3U remota cuenta 1 vez aquí pero,
	// si se resuelve como lista, se expande en sus N entradas (Total >
	// Detectados); si el límite de candidatos corta algunos antes de
	// procesarlos, quedan en Detectados pero no en Total (Total <
	// Detectados). "Recibí X, esto pasó con cada una" (addendum M5).
	Detectados int `json:"detectados"`

	// Total es la suma EXACTA de los seis resultados de abajo — invariante
	// garantizada por construcción (se calcula como esa suma, nunca como un
	// conteo aparte): cada candidato que se llega a procesar (tras expandir
	// M3U remotas y aplicar el límite) termina en uno y solo uno de los seis.
	Total int `json:"total"`

	Anadidos             int `json:"anadidos"`               // items/sources nuevos creados
	Duplicados           int `json:"duplicados"`             // (provider, locator) ya existía
	PendientesConector   int `json:"pendientes_conector"`    // drive/mega: entrada creada, sin conector todavía
	OmitidosNoSoportados int `json:"omitidos_no_soportados"` // hosts de streaming gris: SIN entrada
	SinClasificar        int `json:"sin_clasificar"`         // no encajó en nada, falló su verificación/descarga, o error interno al darlo de alta
	Basura               int `json:"basura"`                 // el normalizador de M3 lo detectó como junk/extra

	PorTipo  map[string]int `json:"por_tipo"`
	Detalles []string       `json:"detalles"`

	// ListID es el id de la fila ingest_lists de ESTA ingesta (trazabilidad,
	// Brief §4/§5).
	ListID int64 `json:"list_id"`

	// hostVerdict cachea, durante UNA ingesta, el veredicto SSRF por host: una
	// lista con cientos de enlaces al mismo dominio resuelve su DNS una vez.
	hostVerdict map[string]error
}

// HealthNotifier despierta al worker de salud de M6 tras una ingesta con
// altas nuevas (Brief M6 §3, disparador "a": "al ingerir/escanear una
// fuente nueva"). Interfaz mínima (satisfecha por *health.Worker) en vez de
// depender directamente del paquete internal/health, para no acoplar M5 a
// M6 más de lo necesario.
type HealthNotifier interface{ Enqueue() }

// Pipeline orquesta la ingesta universal (Brief M5 §4).
type Pipeline struct {
	repo     *catalog.Repo
	identify *identify.Queue // se despierta al terminar; nil-safe (tests sin M3)
	log      *slog.Logger

	// HTTPClient descarga las listas M3U remotas referenciadas en una
	// ingesta; exportado (como identify.Client.BaseURL) para poder
	// sustituirlo por un Transport de prueba en tests, sin red real.
	HTTPClient *http.Client

	// Health, si no es nil, se despierta al terminar una ingesta con altas
	// nuevas (Brief M6 §3). Campo público asignable tras NewPipeline, como
	// HTTPClient: mantiene sin cambios todos los call sites existentes de
	// NewPipeline (tests de M5 incluidos).
	Health HealthNotifier
}

// NewPipeline crea el pipeline de ingesta. idQueue puede ser nil (tests que
// no ejercitan el enriquecimiento TMDB).
func NewPipeline(repo *catalog.Repo, idQueue *identify.Queue, log *slog.Logger) *Pipeline {
	return &Pipeline{
		repo: repo, identify: idQueue, log: log,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Run ejecuta la ingesta de blob (Brief M5 §4). filename es el nombre del
// archivo subido, si lo hay (ayuda a DetectInputType); vacío para el
// textarea. now, si vacío, es el momento actual (parámetro para tests
// deterministas).
func (p *Pipeline) Run(ctx context.Context, blob, filename, now string) (*ResumenIngesta, error) {
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	res := &ResumenIngesta{PorTipo: map[string]int{}}

	inputType := DetectInputType(blob, filename)

	var candidates []Candidate
	switch inputType {
	case InputM3U:
		for _, e := range ParseM3U(blob) {
			candidates = append(candidates, candidateFromURL(e.URL, e.Title, "m3u"))
		}
	case InputCSV:
		entries, err := ParseCSV(blob)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			candidates = append(candidates, candidateFromURL(e.URL, e.Title, "csv"))
		}
	default: // InputText
		for _, lp := range ExtractLocalPaths(blob) {
			kind := "archivo"
			if lp.IsDir {
				kind = "carpeta"
			}
			res.Detalles = append(res.Detalles, fmt.Sprintf(
				"ruta local detectada (%s): %s — usa \"Añadir carpeta\", M5 no la procesa aquí", kind, lp.Path))
		}
		for _, u := range ExtractURLs(blob) {
			candidates = append(candidates, candidateFromURL(u, "", "url"))
		}
	}

	// Detectados: TODO lo que entró, antes de truncar o expandir M3U
	// remotas (addendum M5 §2: "recibí X, esto pasó con cada una").
	res.Detectados = len(candidates)

	if len(candidates) > maxCandidates {
		res.Detalles = append(res.Detalles, fmt.Sprintf(
			"la ingesta traía %d candidatos; se procesan solo los primeros %d (límite de M5)",
			len(candidates), maxCandidates))
		candidates = candidates[:maxCandidates]
	}

	candidates = p.expandM3URemote(ctx, candidates, res, now)

	name := strings.TrimSpace(filename)
	if name == "" {
		name = "pegado " + now
	}
	listID, err := p.repo.InsertIngestList(ctx, &catalog.IngestList{
		Name: name, Type: string(inputType), ItemCount: len(candidates),
	}, now)
	if err != nil {
		return nil, fmt.Errorf("ingest: registrar ingest_list: %w", err)
	}
	res.ListID = listID

	for _, c := range candidates {
		effectiveListID := listID
		if c.nestedListID != 0 {
			effectiveListID = c.nestedListID
		}
		p.processCandidate(ctx, c, effectiveListID, now, res)
	}

	// Total se DERIVA de los seis resultados, nunca de un conteo
	// independiente: así la invariante se cumple por construcción (addendum
	// post-entrega de M5, tras detectar que un total contado aparte no
	// cuadraba con la suma de resultados).
	res.Total = res.Anadidos + res.Duplicados + res.PendientesConector +
		res.OmitidosNoSoportados + res.SinClasificar + res.Basura

	if res.Anadidos > 0 {
		if p.identify != nil {
			p.identify.Enqueue()
		}
		if p.Health != nil {
			p.Health.Enqueue()
		}
	}
	return res, nil
}

// candidateFromURL clasifica rawURL de inmediato: si no es una URL http(s)
// válida, Class queda vacía ("") y se cuenta como sin_clasificar más
// adelante — nunca se descarta aquí mismo sin dejar rastro.
func candidateFromURL(rawURL, titleHint, origin string) Candidate {
	class, err := ClassifyURL(rawURL)
	if err != nil {
		class = ""
	}
	return Candidate{Locator: rawURL, Class: class, TitleHint: titleHint, Origin: origin}
}

// expandM3URemote resuelve cada candidato m3u_remoto (Brief M5 §2/§3):
// descarga su contenido y decide si es un único stream HLS (se trata como
// "directo") o una lista de canales (se expande en N candidatos nuevos, con
// su propia fila ingest_lists para trazabilidad/re-sync futuro). Un fallo
// de descarga (red, SSRF bloqueado, tiempo agotado) se cuenta como
// sin_clasificar con el motivo en Detalles — nunca se pierde en silencio.
func (p *Pipeline) expandM3URemote(ctx context.Context, in []Candidate, res *ResumenIngesta, now string) []Candidate {
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if c.Class != ClassM3URemote {
			out = append(out, c)
			continue
		}
		content, err := p.fetchM3U(ctx, c.Locator)
		if err != nil {
			res.SinClasificar++
			res.PorTipo[c.Origin]++
			res.Detalles = append(res.Detalles, fmt.Sprintf("no se pudo descargar la lista M3U %s: %v", c.Locator, err))
			continue
		}

		if !classifyM3UContent(content) {
			c.Class = ClassDirect // playlist de medios HLS de un único vídeo
			out = append(out, c)
			continue
		}

		entries := ParseM3U(content)
		if len(entries) > maxCandidates {
			res.Detalles = append(res.Detalles, fmt.Sprintf(
				"la lista M3U %s traía %d entradas; se procesan solo las primeras %d",
				c.Locator, len(entries), maxCandidates))
			entries = entries[:maxCandidates]
		}
		nestedID, err := p.repo.InsertIngestList(ctx, &catalog.IngestList{
			Name: c.Locator, Type: "m3u_remoto", SourceURL: strPtr(c.Locator), ItemCount: len(entries),
		}, now)
		if err != nil {
			res.Detalles = append(res.Detalles, fmt.Sprintf("no se pudo registrar la lista M3U %s: %v", c.Locator, err))
			nestedID = 0
		}
		for _, e := range entries {
			nc := candidateFromURL(e.URL, e.Title, "m3u")
			nc.nestedListID = nestedID
			out = append(out, nc)
		}
	}
	return out
}

func (p *Pipeline) fetchM3U(ctx context.Context, rawURL string) (string, error) {
	if err := httpprovider.ValidateExternalURL(ctx, rawURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxM3UBodyBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// processCandidate aplica la regla de cada categoría (Brief M5 §2/§4) y
// actualiza el resumen. listID es la fila ingest_lists a la que se enlaza
// la fuente (la de la ingesta, o la de una M3U remota anidada).
func (p *Pipeline) processCandidate(ctx context.Context, c Candidate, listID int64, now string, res *ResumenIngesta) {
	res.PorTipo[c.Origin]++

	switch c.Class {
	case "":
		res.SinClasificar++
		res.Detalles = append(res.Detalles, fmt.Sprintf("URL no reconocida: %s", c.Locator))

	case ClassUnsupported:
		// Regla de hierro: NI se crea entrada NI se detalla más (sin
		// nombres de host, sin cómo obtener el vídeo).
		res.OmitidosNoSoportados++

	case ClassDrive, ClassMega:
		outcome := p.createProvisional(ctx, c, string(c.Class), catalog.HealthUnverified, listID, now, true)
		p.applyOutcome(res, outcome, func() { res.PendientesConector++ })

	case ClassDirect, ClassUnknown:
		// Mismo bloqueo SSRF que fetchM3U, los .strm y la salud: un enlace a
		// localhost o a una red privada no se cataloga (GALA no es un puente
		// hacia la red de casa).
		if err := p.checkSSRF(ctx, c.Locator, res); err != nil {
			res.SinClasificar++
			res.Detalles = append(res.Detalles, fmt.Sprintf(
				"enlace a una red privada o local, bloqueado por seguridad: %s", c.Locator))
			return
		}
		outcome := p.createProvisional(ctx, c, httpprovider.ProviderID, catalog.HealthUnverified, listID, now, false)
		p.applyOutcome(res, outcome, func() { res.Anadidos++ })

	default: // ClassM3URemote no debería sobrevivir a expandM3URemote
		res.SinClasificar++
		res.Detalles = append(res.Detalles, fmt.Sprintf("lista M3U sin resolver: %s", c.Locator))
	}
}

// ssrfDNSTimeout acota la comprobación de cada host nuevo: sin conexión no se
// debe quedar la ingesta esperando a un DNS que no responde.
const ssrfDNSTimeout = 3 * time.Second

// checkSSRF devuelve un error SOLO si la URL apunta a un destino interno
// (httpprovider.ErrBlockedTarget). Que el DNS no resuelva (p. ej. sin
// conexión) no la rechaza: es un enlace sintácticamente válido y la salud
// (M6) dirá si de verdad está caído — la ingesta funciona offline.
func (p *Pipeline) checkSSRF(ctx context.Context, rawURL string, res *ResumenIngesta) error {
	host := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		host = u.Hostname()
	}
	if verdict, ok := res.hostVerdict[host]; ok {
		return verdict
	}
	cctx, cancel := context.WithTimeout(ctx, ssrfDNSTimeout)
	defer cancel()
	var verdict error
	if err := httpprovider.ValidateExternalURL(cctx, rawURL); errors.Is(err, httpprovider.ErrBlockedTarget) {
		verdict = err
	}
	if res.hostVerdict == nil {
		res.hostVerdict = map[string]error{}
	}
	res.hostVerdict[host] = verdict
	return verdict
}

// candidateOutcome es el resultado de intentar dar de alta un candidato.
type candidateOutcome struct {
	created bool
	junk    bool
	dup     bool
	errMsg  string
}

// applyOutcome traduce un candidateOutcome a los contadores del resumen;
// onCreated añade el contador específico de la categoría que sí tuvo éxito
// (Anadidos, o PendientesConector). Un error interno real (fallo de BD al
// comprobar duplicados o al dar de alta) cae en SinClasificar: no hay una
// categoría propia para "fallo interno" en el contrato de ResumenIngesta, y
// dejarlo fuera de los seis resultados rompería la invariante Total = suma
// (addendum post-entrega de M5). El detalle ya explica la causa real.
func (p *Pipeline) applyOutcome(res *ResumenIngesta, o candidateOutcome, onCreated func()) {
	switch {
	case o.errMsg != "":
		res.SinClasificar++
		res.Detalles = append(res.Detalles, o.errMsg)
	case o.dup:
		res.Duplicados++
	case o.junk:
		res.Basura++
	case o.created:
		onCreated()
	}
}

// createProvisional da de alta item+source si no es un duplicado ni basura
// (Brief M5 §4: normalizar → dedupe → alta). skipJunkCheck se usa para
// drive/mega: su URL no tiene un "nombre de archivo" real que juzgar, y el
// Brief exige crear la entrada siempre para esas dos clases.
func (p *Pipeline) createProvisional(ctx context.Context, c Candidate, provider, health string, listID int64, now string, skipJunkCheck bool) candidateOutcome {
	if _, err := p.repo.SourceByLocator(ctx, provider, c.Locator); err == nil {
		return candidateOutcome{dup: true}
	} else if !errors.Is(err, catalog.ErrNotFound) {
		return candidateOutcome{errMsg: fmt.Sprintf("error comprobando duplicados de %s: %v", c.Locator, err)}
	}

	title := filenameHint(c)
	if !skipJunkCheck {
		parsed := identify.Parse(title, "")
		if parsed.Junk || parsed.Extra {
			return candidateOutcome{junk: true}
		}
	}
	displayTitle := strings.TrimSuffix(title, path.Ext(title))
	if displayTitle == "" {
		displayTitle = c.Locator
	}

	item := catalog.Item{Kind: catalog.KindOther, Title: displayTitle, Genres: []string{}, Confirmed: false}
	itemID, err := p.repo.InsertItem(ctx, &item, now)
	if err != nil {
		return candidateOutcome{errMsg: fmt.Sprintf("no se pudo dar de alta %s: %v", c.Locator, err)}
	}
	lid := listID
	src := catalog.Source{ItemID: itemID, Provider: provider, Locator: c.Locator, Health: health, OriginListID: &lid}
	if _, err := p.repo.InsertSource(ctx, &src, now); err != nil {
		return candidateOutcome{errMsg: fmt.Sprintf("no se pudo dar de alta la fuente %s: %v", c.Locator, err)}
	}
	return candidateOutcome{created: true}
}

// filenameHint deriva un "nombre de archivo" para alimentar
// identify.Parse: prioriza el título de EXTINF/CSV si lo hay; si no, el
// último segmento de la ruta de la URL (decodificado).
func filenameHint(c Candidate) string {
	if c.TitleHint != "" {
		return c.TitleHint
	}
	u, err := url.Parse(c.Locator)
	if err != nil {
		return c.Locator
	}
	base := path.Base(u.Path)
	if decoded, err := url.PathUnescape(base); err == nil {
		base = decoded
	}
	return base
}

func strPtr(s string) *string { return &s }
