package lan

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// SessionCookie es la cookie de sesión LAN que se planta al acertar el PIN.
const SessionCookie = "gala_lan"

const sessionTTL = 30 * 24 * time.Hour

// Settings es lo que el guardián necesita de la configuración.
type Settings interface {
	// LanState devuelve el bind y el PIN vigentes ("" = sin PIN).
	LanState() (bind, pin string)
}

// Guard decide, para cada petición, si pasa. Las del propio PC (loopback)
// pasan siempre; las de otros dispositivos solo si la ruta está en la lista
// permitida (RemoteAllowed) y, si hay PIN, traen una sesión válida.
type Guard struct {
	cfg      Settings
	sessions *sessions
	limiter  *limiter
}

// NewGuard crea el guardián.
func NewGuard(cfg Settings) *Guard {
	return &Guard{cfg: cfg, sessions: newSessions(time.Now), limiter: newLimiter(time.Now)}
}

// ResetSessions invalida todas las sesiones LAN (al cambiar o quitar el PIN).
func (g *Guard) ResetSessions() { g.sessions.reset() }

// IsLocal indica si la petición viene del propio PC. Usa RemoteAddr, nunca
// cabeceras (X-Forwarded-For, Host...) que un cliente remoto controla.
func IsLocal(r *http.Request) bool { return IsLoopbackHostPort(r.RemoteAddr) }

// Middleware envuelve al mux entero.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsLocal(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")

		// Una ruta que cambia al normalizarla (".." etc.) es un intento de
		// colarse por la lista permitida: fuera.
		p := r.URL.Path
		if len(p) > 1 {
			p = strings.TrimSuffix(p, "/")
		}
		if p == "" || path.Clean(p) != p {
			forbidden(w)
			return
		}
		if !RemoteAllowed(r.Method, r.URL.Path) {
			forbidden(w)
			return
		}
		if _, pin := g.cfg.LanState(); pin != "" && needsPIN(r.URL.Path) && !g.authenticated(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Hace falta el PIN para entrar.", "code": "pin_required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Guard) authenticated(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie)
	return err == nil && g.sessions.valid(c.Value)
}

func forbidden(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "Esto solo se puede hacer desde el PC donde corre GALA.", "code": "local_only"})
}

// RemoteAllowed es LA lista de lo que puede hacer un cliente LAN: ver la
// galería, reproducir, y guardar su propio progreso y su lista de deseos.
// Todo lo demás (carpetas, escaneo, ingesta, ajustes, clave TMDB, borrar,
// confirmar, salud, Modo LAN) queda reservado al PC. Lista cerrada: una ruta
// nueva de la API es solo-local hasta que se añada aquí a propósito.
func RemoteAllowed(method, urlPath string) bool {
	get := method == http.MethodGet || method == http.MethodHead
	if !strings.HasPrefix(urlPath, "/api/") {
		return get // la SPA embebida (HTML/JS/CSS/fuentes)
	}
	seg := strings.Split(strings.Trim(urlPath, "/"), "/") // api v1 <recurso> ...
	if len(seg) < 3 || seg[1] != "v1" {
		return false
	}
	switch seg[2] {
	case "ping":
		return get && len(seg) == 3
	case "items":
		switch len(seg) {
		case 3, 4:
			return get
		case 5:
			if seg[4] == "details" {
				return get // reparto, equipo, similares y ficha técnica (sin rutas)
			}
			return seg[4] == "wish" && (method == http.MethodPost || method == http.MethodDelete)
		}
	case "genres", "facets":
		return get && len(seg) == 3
	case "settings":
		// Solo la preferencia de la Home (la leen también los dispositivos de
		// la red); la clave TMDB y el resto de ajustes siguen siendo del PC.
		return get && len(seg) == 4 && seg[3] == "home"
	case "img":
		return get && len(seg) == 4
	case "play":
		return get && len(seg) == 4
	case "stream":
		// Un cliente LAN puede liberar SU sesión de remux al salir del
		// reproductor (DELETE .../hls/{session}); sin esto agotaría el límite.
		if method == http.MethodDelete {
			return len(seg) == 6 && seg[4] == "hls"
		}
		return get && len(seg) >= 4
	case "sources":
		return len(seg) == 5 && seg[4] == "failed" && method == http.MethodPost
	case "playback":
		return len(seg) == 4 && (get || method == http.MethodPut)
	case "lan":
		if len(seg) == 4 {
			return (seg[3] == "session" && get) || (seg[3] == "auth" && method == http.MethodPost)
		}
	}
	return false
}

// needsPIN: la API de datos exige sesión; la SPA, el ping y los endpoints
// del propio PIN no (hay que poder cargar la pantalla que pide el PIN).
func needsPIN(urlPath string) bool {
	if !strings.HasPrefix(urlPath, "/api/") {
		return false
	}
	switch urlPath {
	case "/api/v1/ping", "/api/v1/lan/session", "/api/v1/lan/auth":
		return false
	}
	return true
}

// --- endpoints públicos del PIN -----------------------------------------

type sessionView struct {
	Local         bool `json:"local"`
	PinRequired   bool `json:"pin_required"`
	Authenticated bool `json:"authenticated"`
}

// HandleSession: GET /api/v1/lan/session — la SPA lo consulta al cargar para
// saber si está en el PC (todo permitido) o en un dispositivo LAN (y si le
// falta el PIN).
func (g *Guard) HandleSession(w http.ResponseWriter, r *http.Request) {
	if IsLocal(r) {
		writeJSON(w, http.StatusOK, sessionView{Local: true, Authenticated: true})
		return
	}
	_, pin := g.cfg.LanState()
	required := pin != ""
	writeJSON(w, http.StatusOK, sessionView{
		PinRequired: required, Authenticated: !required || g.authenticated(r)})
}

// HandleAuth: POST /api/v1/lan/auth  body {"pin":"1234"}. Con rate-limit
// progresivo por IP y global. El PIN nunca se registra en logs.
func (g *Guard) HandleAuth(w http.ResponseWriter, r *http.Request) {
	if IsLocal(r) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	_, pin := g.cfg.LanState()
	if pin == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	ip := clientIP(r)
	if ok, wait := g.limiter.allow(ip); !ok {
		secs := int(wait.Seconds()) + 1
		w.Header().Set("Retry-After", fmt.Sprint(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": fmt.Sprintf("Demasiados intentos. Espera %d segundos.", secs), "code": "rate_limited"})
		return
	}
	var req struct {
		PIN string `json:"pin"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cuerpo no válido"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.PIN), []byte(pin)) != 1 {
		g.limiter.fail(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "PIN incorrecto", "code": "bad_pin"})
		return
	}
	g.limiter.succeed(ip)
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: g.sessions.issue(), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// --- sesiones -----------------------------------------------------------

// sessions guarda tokens aleatorios en memoria (no un "pin correcto" en la
// cookie). Reiniciar GALA los invalida: el móvil vuelve a pedir el PIN.
type sessions struct {
	now func() time.Time
	mu  sync.Mutex
	m   map[string]time.Time
}

const maxSessions = 1000

func newSessions(now func() time.Time) *sessions {
	return &sessions{now: now, m: map[string]time.Time{}}
}

func (s *sessions) issue() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("lan: sin entropía: " + err.Error()) // no se puede emitir un token seguro
	}
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.m) >= maxSessions {
		for k, exp := range s.m {
			if now.After(exp) {
				delete(s.m, k)
			}
		}
		for k := range s.m { // sigue lleno: sacar una cualquiera
			if len(s.m) < maxSessions {
				break
			}
			delete(s.m, k)
		}
	}
	s.m[tok] = now.Add(sessionTTL)
	return tok
}

func (s *sessions) valid(tok string) bool {
	if tok == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.m[tok]
	if !ok {
		return false
	}
	if s.now().After(exp) {
		delete(s.m, tok)
		return false
	}
	return true
}

func (s *sessions) reset() {
	s.mu.Lock()
	s.m = map[string]time.Time{}
	s.mu.Unlock()
}

// --- rate limit ---------------------------------------------------------

const (
	freeAttempts   = 5                // fallos por IP antes del primer bloqueo
	baseLock       = 30 * time.Second // bloqueo tras el 5.º fallo; se duplica en cada fallo posterior
	maxLock        = 15 * time.Minute
	globalWindow   = 10 * time.Minute
	globalMaxFails = 30 // fallos totales (todas las IPs) por ventana antes de bloquear a todos
	globalLock     = 5 * time.Minute
)

type ipState struct {
	fails  int
	locked time.Time
	last   time.Time
}

// limiter frena la fuerza bruta de un PIN de 4 dígitos: bloqueo progresivo
// por IP y un tope global (varias IPs/dispositivos atacando a la vez).
type limiter struct {
	now func() time.Time
	mu  sync.Mutex
	ips map[string]*ipState

	winStart   time.Time
	winFails   int
	globalLock time.Time
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, ips: map[string]*ipState{}}
}

func (l *limiter) allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Before(l.globalLock) {
		return false, l.globalLock.Sub(now)
	}
	if st, ok := l.ips[ip]; ok && now.Before(st.locked) {
		return false, st.locked.Sub(now)
	}
	return true, 0
}

func (l *limiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	if len(l.ips) > 1000 { // memoria acotada: olvidar IPs viejas
		for k, st := range l.ips {
			if now.Sub(st.last) > time.Hour {
				delete(l.ips, k)
			}
		}
	}
	st := l.ips[ip]
	if st == nil {
		st = &ipState{}
		l.ips[ip] = st
	}
	st.fails++
	st.last = now
	if st.fails >= freeAttempts {
		d := baseLock << uint(min(st.fails-freeAttempts, 10))
		if d > maxLock || d <= 0 {
			d = maxLock
		}
		st.locked = now.Add(d)
	}

	if now.Sub(l.winStart) > globalWindow {
		l.winStart, l.winFails = now, 0
	}
	l.winFails++
	if l.winFails >= globalMaxFails {
		l.globalLock = now.Add(globalLock)
		l.winStart, l.winFails = now, 0
	}
}

func (l *limiter) succeed(ip string) {
	l.mu.Lock()
	delete(l.ips, ip)
	l.mu.Unlock()
}
