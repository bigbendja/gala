package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/bigbendja/gala/internal/buildinfo"
	"github.com/bigbendja/gala/internal/update"
)

// Actualización (M-Servidor, módulo 3). Todas las rutas /update/* son de
// administración (el guardián exige sesión desde la red); /healthz es público.

// handleHealthz: GET /healthz — 200 si la BD responde (y por tanto el
// servidor está atendiendo). Sirve para el autochequeo y para monitorizar.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.DB.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) updateUnavailable(w http.ResponseWriter) bool {
	if s.UpdateChecker == nil || s.Updater == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "La actualización no está disponible."})
		return true
	}
	return false
}

type updateResultView struct {
	Status string `json:"status"`
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
	At     string `json:"at,omitempty"`
}

func (s *Server) updateResult() *updateResultView {
	r, ok, err := update.ReadResult(s.UpdateDir)
	if err != nil || !ok {
		return nil
	}
	v := &updateResultView{Status: r.Status, From: r.From, To: r.To, Reason: r.Reason}
	if !r.At.IsZero() {
		v.At = r.At.Format(time.RFC3339)
	}
	return v
}

type updateCheckView struct {
	Checked  bool         `json:"checked"`
	Error    string       `json:"error,omitempty"`
	Info     *update.Info `json:"info,omitempty"`
	Current  string       `json:"current"`
	CanApply bool         `json:"can_apply"`
	Reason   string       `json:"reason,omitempty"`
}

// handleUpdateCheck: GET /api/v1/update/check[?force=1]. Sin red o con GitHub
// caído responde 200 con checked=false: nada se rompe y la UI dice «No se pudo
// comprobar».
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.updateUnavailable(w) {
		return
	}
	can, reason := s.Updater.CanApply()
	v := updateCheckView{Current: buildinfo.Version, CanApply: can, Reason: reason}
	info, err := s.UpdateChecker.Check(r.Context(), r.URL.Query().Get("force") == "1")
	switch {
	case err == nil:
		v.Checked, v.Info = true, &info
	case errors.Is(err, update.ErrOffline):
		v.Error = "No se pudo comprobar las actualizaciones. Revisa la conexión a Internet e inténtalo más tarde."
		if cached, ok := s.UpdateChecker.Cached(); ok {
			v.Info = &cached
		}
	default:
		s.Log.Warn("actualización: comprobar", "err", err)
		v.Error = "No se pudo comprobar las actualizaciones."
	}
	writeJSON(w, http.StatusOK, v)
}

// handleUpdateApply: POST /api/v1/update/apply  body {"force": bool}. Lanza la
// actualización en segundo plano; el progreso se lee en /update/status.
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if s.updateUnavailable(w) {
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	if r.ContentLength != 0 {
		if !isJSONRequest(r) {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "se esperaba application/json"})
			return
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
			badRequest(w, "cuerpo no válido")
			return
		}
	}
	err := s.Updater.Start(req.Force)
	var pb *update.PlaybackActiveError
	switch {
	case err == nil:
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
	case errors.As(err, &pb):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "Hay una reproducción en curso. Si actualizas ahora, se cortará.",
			"code":  "playback_active", "count": pb.Count})
	case errors.Is(err, update.ErrBusy):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Ya hay una actualización en curso.", "code": "busy"})
	case errors.Is(err, update.ErrNotSupervised):
		_, reason := s.Updater.CanApply()
		writeJSON(w, http.StatusConflict, map[string]string{"error": reason, "code": "not_supervised"})
	case errors.Is(err, update.ErrNoUpdate):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "No hay una actualización disponible. Busca actualizaciones primero.", "code": "no_update"})
	default:
		s.fail(w, err)
	}
}

type updateStatusView struct {
	Status  update.Status     `json:"status"`
	Result  *updateResultView `json:"result,omitempty"`
	Current string            `json:"current"`
	// Available es lo último que se supo de GitHub (caché, sin consultar la
	// red): así la pantalla enseña la versión disponible al abrirse.
	Available *update.Info `json:"available,omitempty"`
	CanApply  bool         `json:"can_apply"`
	Reason    string       `json:"reason,omitempty"`
}

// handleUpdateStatus: GET /api/v1/update/status — progreso y resultado de la
// última actualización (ok o revertida), hasta que el administrador lo descarta.
func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if s.updateUnavailable(w) {
		return
	}
	v := updateStatusView{Status: s.Updater.Status(), Result: s.updateResult(), Current: buildinfo.Version}
	v.CanApply, v.Reason = s.Updater.CanApply()
	if info, ok := s.UpdateChecker.Cached(); ok && info.UpdateAvailable {
		v.Available = &info
	}
	writeJSON(w, http.StatusOK, v)
}

// handleUpdateResultDismiss: DELETE /api/v1/update/result.
func (s *Server) handleUpdateResultDismiss(w http.ResponseWriter, r *http.Request) {
	if s.updateUnavailable(w) {
		return
	}
	if err := update.ClearResult(s.UpdateDir); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
