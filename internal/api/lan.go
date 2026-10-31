package api

import (
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/lan"
	"github.com/bigbendja/gala/internal/providers/localfs"
)

var pinRe = regexp.MustCompile(`^[0-9]{4}$`)

// lanStatusView es GET /api/v1/lan/status. Nunca lleva el PIN, solo pin_set.
type lanStatusView struct {
	Enabled    bool            `json:"enabled"`
	Bind       string          `json:"bind"`
	Port       int             `json:"port"`
	LanIP      string          `json:"lan_ip"`
	URL        string          `json:"url"`
	PinSet     bool            `json:"pin_set"`
	QRPayload  string          `json:"qr_payload"`
	Candidates []lan.Candidate `json:"candidates"`
}

func (s *Server) lanCandidates() []lan.Candidate {
	if s.LanCandidates != nil {
		return s.LanCandidates()
	}
	return lan.Candidates()
}

func (s *Server) lanStatus() lanStatusView {
	bind, pin := s.Config.LanState()
	v := lanStatusView{
		Enabled: bind == config.BindAllInterfaces, Bind: bind, Port: s.Config.Port,
		PinSet: pin != "", Candidates: s.lanCandidates(),
	}
	if v.Candidates == nil {
		v.Candidates = []lan.Candidate{}
	}
	if best, ok := bestCandidate(v.Candidates); ok {
		v.LanIP = best.IP
		// El QR/URL NUNCA llevan el PIN: quien vea o fotografíe la pantalla no
		// se lo lleva; el PIN se teclea en el móvil (Brief M7 §2/§5).
		v.URL = fmt.Sprintf("http://%s", net.JoinHostPort(best.IP, fmt.Sprint(v.Port)))
		v.QRPayload = v.URL
	}
	return v
}

// bestCandidate: la primera no virtual; si todas lo son, la primera.
func bestCandidate(cs []lan.Candidate) (lan.Candidate, bool) {
	for _, c := range cs {
		if !c.Virtual {
			return c, true
		}
	}
	if len(cs) > 0 {
		return cs[0], true
	}
	return lan.Candidate{}, false
}

// requireOwner deja pasar solo al PC que ejecuta GALA. Defensa en
// profundidad: el guardián ya lo impide, pero estos endpoints cambian la
// exposición de la red, así que lo comprueban también ellos. Además exige que
// la petición no pueda venir de otra web abierta en el navegador del dueño
// (CSRF / DNS rebinding): Host de loopback y cuerpo application/json (que un
// formulario de otra web no puede enviar sin preflight).
func requireOwner(w http.ResponseWriter, r *http.Request, needJSON bool) bool {
	if !lan.IsLocal(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "Esto solo se puede hacer desde el PC donde corre GALA.", "code": "local_only"})
		return false
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if !(host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Origen no permitido."})
		return false
	}
	if needJSON {
		mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mt != "application/json" {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "se esperaba application/json"})
			return false
		}
	}
	return true
}

// handleLanStatus: GET /api/v1/lan/status.
func (s *Server) handleLanStatus(w http.ResponseWriter, r *http.Request) {
	if !requireOwner(w, r, false) {
		return
	}
	writeJSON(w, http.StatusOK, s.lanStatus())
}

// handleLanToggle: POST /api/v1/lan/toggle  body {"enabled": true}. Reabre el
// listener en 0.0.0.0 (activar) o 127.0.0.1 (desactivar), persiste y devuelve
// el estado ya confirmado.
func (s *Server) handleLanToggle(w http.ResponseWriter, r *http.Request) {
	if !requireOwner(w, r, true) {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || req.Enabled == nil {
		badRequest(w, "se esperaba {\"enabled\": true|false}")
		return
	}
	target := config.BindLoopback
	if *req.Enabled {
		target = config.BindAllInterfaces
	}
	current, _ := s.Config.LanState()
	if current != target {
		if s.LanListen != nil {
			if err := s.LanListen(target); err != nil {
				s.Log.Error("modo LAN: reabrir listener", "bind", target, "err", err)
				writeJSON(w, http.StatusConflict, map[string]string{
					"error": "No se pudo cambiar el Modo LAN (¿el puerto está ocupado o el cortafuegos lo impide?)."})
				return
			}
		}
		if err := s.Config.SetBind(target); err != nil {
			// El listener ya cambió pero no se pudo guardar: volver atrás
			// para que lo que se ve y lo que quedará tras reiniciar coincidan.
			if s.LanListen != nil {
				_ = s.LanListen(current)
			}
			s.fail(w, err)
			return
		}
		s.Log.Info("modo LAN", "bind", target)
	}
	writeJSON(w, http.StatusOK, s.lanStatus())
}

// handleLanPIN: PUT /api/v1/lan/pin  body {"pin":"1234"} o {"pin":""} (quitar).
func (s *Server) handleLanPIN(w http.ResponseWriter, r *http.Request) {
	// Desde el PC, o desde la red con sesión de administrador (el PIN es lo
	// que protege el acceso de los móviles; el admin autenticado puede
	// cambiarlo, a diferencia de apagar el Modo LAN).
	if !s.requireAdmin(w, r, true) {
		return
	}
	var req struct {
		PIN *string `json:"pin"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || req.PIN == nil {
		badRequest(w, "se esperaba {\"pin\": \"1234\"} (o \"\" para quitarlo)")
		return
	}
	pin := *req.PIN
	if pin != "" && !pinRe.MatchString(pin) {
		badRequest(w, "el PIN debe tener exactamente 4 dígitos")
		return
	}
	if err := s.Config.SetLanPIN(pin); err != nil {
		s.fail(w, err)
		return
	}
	// Cambiar o quitar el PIN expulsa a los dispositivos ya autenticados:
	// el PIN anterior ya no vale para seguir dentro.
	if s.LanGuard != nil {
		s.LanGuard.ResetSessions()
		// Un administrador de red que cambia el PIN no se expulsa a sí mismo:
		// recibe una sesión LAN nueva con el PIN nuevo.
		if !lan.IsLocal(r) && pin != "" {
			s.LanGuard.IssueSession(w)
		}
	}
	writeJSON(w, http.StatusOK, s.lanStatus())
}

// redactForRemote quita de la ficha lo que no debe ver otro dispositivo de la
// red: rutas del disco del PC, ids de volumen y detalles de salud (que pueden
// llevar rutas). Del archivo solo queda su nombre. Los enlaces http son datos
// del propio usuario y se conservan.
func redactForRemote(it *catalog.Item) {
	for i := range it.Sources {
		src := &it.Sources[i]
		src.VolumeID = nil
		if src.Provider == localfs.ProviderID {
			src.Locator = filepath.Base(strings.ReplaceAll(src.Locator, `\`, `/`))
			src.HealthDetail = nil
		}
	}
}
