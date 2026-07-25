package api

import (
	"net/http"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/health"
)

// --- /api/v1/health/... (Brief M6 §5) -----------------------------------

// healthSummaryView es el agregado global de salud de fuentes (Brief M6
// §5). Total se DERIVA de la suma de los cinco estados — mismo principio de
// invariante-por-construcción que ResumenIngesta (M5, addendum): nunca se
// cuenta aparte, así nunca puede desincronizarse de la suma.
type healthSummaryView struct {
	Total        int     `json:"total"`
	Viva         int     `json:"viva"`
	Lenta        int     `json:"lenta"`
	Caida        int     `json:"caida"`
	Desconectada int     `json:"desconectada"`
	SinVerificar int     `json:"sin_verificar"`
	LastRun      *string `json:"last_run,omitempty"`
	Running      bool    `json:"running"`
}

// handleHealthSummary: GET /api/v1/health/summary — agregado global de
// salud (Brief M6 §5); base de los puntos de color del mockup y de un
// futuro panel de estado.
func (s *Server) handleHealthSummary(w http.ResponseWriter, r *http.Request) {
	counts, err := s.Catalog.HealthCounts(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	view := healthSummaryView{
		Viva:         counts[catalog.HealthAlive],
		Lenta:        counts[catalog.HealthSlow],
		Caida:        counts[catalog.HealthDead],
		Desconectada: counts[catalog.HealthDisconnected],
		SinVerificar: counts[catalog.HealthUnverified],
	}
	for _, n := range counts {
		view.Total += n
	}
	if s.Health != nil {
		st := s.Health.Status()
		view.Running = st.Running
		view.LastRun = st.LastFinished
	}
	writeJSON(w, http.StatusOK, view)
}

// handleHealthCheck: POST /api/v1/health/check — dispara una verificación
// completa bajo demanda (Brief M6 §5); 409 si ya hay una en curso
// (simétrico a POST /api/v1/scan de M2).
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if s.Health == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "verificador de salud no disponible"})
		return
	}
	if err := s.Health.TriggerFull(); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ya hay una verificación de salud en curso"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "verificando"})
}

// handleHealthStatus: GET /api/v1/health/status — estado del worker (Brief
// M6 §5).
func (s *Server) handleHealthStatus(w http.ResponseWriter, r *http.Request) {
	if s.Health == nil {
		writeJSON(w, http.StatusOK, health.Status{})
		return
	}
	writeJSON(w, http.StatusOK, s.Health.Status())
}
