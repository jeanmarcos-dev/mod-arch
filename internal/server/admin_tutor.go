package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/tutor"
)

// handleAdminTutorMetrics returns the aggregated tutor metrics for the
// admin dashboard. The window is controlled by `?hours=N` (default 24).
//
// The endpoint is open to moderators since they are the ones supervising
// classroom usage; full admins inherit access via requireModRole.
func (s *Server) handleAdminTutorMetrics(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store no disponible"})
		return
	}
	hours := 24
	if v := r.URL.Query().Get("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 24*30 {
			hours = n
		}
	}

	store := tutor.NewMetricsStore(st.DB())
	summary, err := store.Summarize(r.Context(), tutor.SummaryWindow{
		Since: time.Now().Add(-time.Duration(hours) * time.Hour),
		Until: time.Now(),
	})
	if err != nil {
		s.logger.Warn("admin tutor metrics failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
