package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/bigbendja/gala/internal/auth"
	"github.com/bigbendja/gala/internal/lan"
)

// Autenticación de administrador (M-Servidor). Quién puede qué lo decide el
// guardián (internal/lan) con auth.Service; aquí solo están los endpoints.

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isJSONRequest(r *http.Request) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return mt == "application/json"
}

func (s *Server) authUnavailable(w http.ResponseWriter) bool {
	if s.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "La administración con clave no está disponible."})
		return true
	}
	return false
}

// requireAdmin deja pasar escrituras de administración que el guardián ya
// autorizó: desde el PC (con las defensas de requireOwner) o desde la red con
// una sesión de administrador válida (el guardián ya comprobó CSRF).
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request, needJSON bool) bool {
	if lan.IsLocal(r) {
		return requireOwner(w, r, needJSON)
	}
	if s.Auth == nil || s.Auth.SessionFromRequest(r) == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "Esto solo se puede hacer desde el PC donde corre GALA.", "code": "local_only"})
		return false
	}
	if needJSON && !isJSONRequest(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "se esperaba application/json"})
		return false
	}
	return true
}

type authStatusView struct {
	Configured    bool   `json:"configured"`
	Authenticated bool   `json:"authenticated"`
	IsLocalhost   bool   `json:"is_localhost"`
	NetworkAdmin  bool   `json:"network_admin"`
	CSRF          string `json:"csrf,omitempty"`
}

// handleAuthStatus: GET /api/v1/auth/status.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	local := lan.IsLocal(r)
	if s.Auth == nil { // sin servicio (tests): es el PC, sin clave
		writeJSON(w, http.StatusOK, authStatusView{IsLocalhost: local, Authenticated: local, NetworkAdmin: true})
		return
	}
	configured, err := s.Auth.Configured(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	v := authStatusView{
		Configured: configured, IsLocalhost: local,
		NetworkAdmin: !configured || s.Auth.NetworkAdmin(r.Context()),
	}
	if sess := s.Auth.SessionFromRequest(r); sess != nil {
		v.Authenticated = true
		v.CSRF = s.Auth.CSRFToken(sess.IDHash)
	} else if local && s.Auth.LocalBypass() {
		v.Authenticated = true
	}
	writeJSON(w, http.StatusOK, v)
}

// handleAuthCSRF: GET /api/v1/auth/csrf.
func (s *Server) handleAuthCSRF(w http.ResponseWriter, r *http.Request) {
	if s.authUnavailable(w) {
		return
	}
	sess := s.Auth.SessionFromRequest(r)
	if sess == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Inicia sesión para administrar GALA.", "code": "login_required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"csrf": s.Auth.CSRFToken(sess.IDHash)})
}

// handleAuthLogin: POST /api/v1/auth/login  body {"password": "..."}.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.authUnavailable(w) {
		return
	}
	if !isJSONRequest(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "se esperaba application/json"})
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		badRequest(w, "cuerpo no válido")
		return
	}
	tok, csrf, err := s.Auth.Login(r.Context(), req.Password, requestIP(r), r.UserAgent(), lan.IsLocal(r))
	var rl *auth.RateLimitedError
	switch {
	case errors.As(err, &rl):
		secs := int(rl.Wait.Seconds()) + 1
		w.Header().Set("Retry-After", fmt.Sprint(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": fmt.Sprintf("Demasiados intentos. Espera %d segundos.", secs), "code": "rate_limited"})
		return
	case errors.Is(err, auth.ErrNotConfigured):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "La administración desde la red no está configurada. Pon una contraseña desde el equipo o por SSH.",
			"code":  "auth_not_configured"})
		return
	case errors.Is(err, auth.ErrBadPassword):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Contraseña incorrecta. Inténtalo de nuevo.", "code": "bad_password"})
		return
	case err != nil:
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.SessionCookie, Value: tok, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil,
		MaxAge: s.Config.SessionDays() * 24 * 3600,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "csrf": csrf})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: auth.SessionCookie, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: -1, Expires: time.Unix(0, 0),
	})
}

// handleAuthLogout: POST /api/v1/auth/logout — cierra la sesión actual.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if s.authUnavailable(w) {
		return
	}
	if sess := s.Auth.SessionFromRequest(r); sess != nil {
		if err := s.Auth.Logout(r.Context(), sess.IDHash); err != nil {
			s.fail(w, err)
			return
		}
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAuthLogoutAll: POST /api/v1/auth/logout-all — administración.
func (s *Server) handleAuthLogoutAll(w http.ResponseWriter, r *http.Request) {
	if s.authUnavailable(w) {
		return
	}
	if err := s.Auth.LogoutAll(r.Context()); err != nil {
		s.fail(w, err)
		return
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAuthPassword: POST /api/v1/auth/password  body {"current","new"}.
// Con clave puesta exige la actual (también desde localhost). Sin clave solo
// se puede poner desde el PC (el guardián ya lo impide a la red).
func (s *Server) handleAuthPassword(w http.ResponseWriter, r *http.Request) {
	if s.authUnavailable(w) || !s.requireAdmin(w, r, true) {
		return
	}
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		badRequest(w, "cuerpo no válido")
		return
	}
	configured, err := s.Auth.Configured(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	local := lan.IsLocal(r)
	keep, ip := "", ""
	if sess := s.Auth.SessionFromRequest(r); sess != nil {
		keep = sess.IDHash
	}
	if !local {
		ip = requestIP(r)
	}
	if !configured {
		if !local {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "Esto solo se puede hacer desde el PC donde corre GALA.", "code": "local_only"})
			return
		}
		err = s.Auth.SetPassword(r.Context(), req.New)
	} else {
		err = s.Auth.ChangePassword(r.Context(), req.Current, req.New, keep, ip)
	}
	var rl *auth.RateLimitedError
	switch {
	case errors.As(err, &rl):
		secs := int(rl.Wait.Seconds()) + 1
		w.Header().Set("Retry-After", fmt.Sprint(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": fmt.Sprintf("Demasiados intentos. Espera %d segundos.", secs), "code": "rate_limited"})
	case errors.Is(err, auth.ErrBadPassword):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "La contraseña actual no es correcta.", "code": "bad_password"})
	case errors.Is(err, auth.ErrPasswordPolicy):
		badRequest(w, "La contraseña nueva debe tener entre 8 y 128 caracteres.")
	case err != nil:
		s.fail(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

type sessionRow struct {
	ID       string `json:"id"`
	Device   string `json:"device"`
	IP       string `json:"ip"`
	Created  string `json:"created_at"`
	LastSeen string `json:"last_seen"`
	Current  bool   `json:"current"`
}

// handleAuthSessions: GET /api/v1/auth/sessions.
func (s *Server) handleAuthSessions(w http.ResponseWriter, r *http.Request) {
	if s.authUnavailable(w) {
		return
	}
	list, err := s.Auth.List(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	cur := ""
	if sess := s.Auth.SessionFromRequest(r); sess != nil {
		cur = sess.IDHash
	}
	out := make([]sessionRow, 0, len(list))
	for _, x := range list {
		out = append(out, sessionRow{
			ID: x.IDHash[:16], Device: auth.DeviceName(x.UserAgent), IP: x.IP,
			Created: x.CreatedAt.Format(time.RFC3339), LastSeen: x.LastSeen.Format(time.RFC3339),
			Current: x.IDHash == cur,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAuthNetworkAdmin: PUT /api/v1/auth/network-admin body {"enabled": bool}.
// Solo desde el PC: no se puede cortar la rama en la que uno está sentado.
func (s *Server) handleAuthNetworkAdmin(w http.ResponseWriter, r *http.Request) {
	if s.authUnavailable(w) || !requireOwner(w, r, true) {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || req.Enabled == nil {
		badRequest(w, "se esperaba {\"enabled\": true|false}")
		return
	}
	if err := s.Auth.SetNetworkAdmin(r.Context(), *req.Enabled); err != nil {
		if errors.Is(err, auth.ErrNotConfigured) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Primero pon una contraseña de administrador."})
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"network_admin": *req.Enabled})
}
