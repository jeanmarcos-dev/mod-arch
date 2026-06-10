package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// historyRowJSON is the wire shape for one session_history entry. Plain IP is
// only populated when the operator opted in via ADMIN_REVEAL_IP.
type historyRowJSON struct {
	ID                int64  `json:"id"`
	OriginalSessionID string `json:"original_session_id"`
	UserID            string `json:"user_id,omitempty"`
	UserLabel         string `json:"user_label,omitempty"`
	Mode              string `json:"mode"`
	Label             string `json:"label,omitempty"`
	Public            bool   `json:"public"`
	ViewToken         string `json:"view_token,omitempty"`
	CreatedAt         int64  `json:"created_at"`
	EndedAt           int64  `json:"ended_at"`
	DurationSeconds   int64  `json:"duration_seconds"`
	EndReason         string `json:"end_reason"`
	ClientIPHash      string `json:"client_ip_hash,omitempty"`
	ClientIP          string `json:"client_ip,omitempty"`
	UAShort           string `json:"ua_short,omitempty"`
	GeoCountry        string `json:"geo_country,omitempty"`
	ReferrerHost      string `json:"referrer_host,omitempty"`
	TotalCommands     int64  `json:"total_commands"`
	TotalTutorQueries int64  `json:"total_tutor_queries"`
}

// handleAdminListSessionHistory returns paginated session_history rows with
// optional filters. Plain IPs are stripped unless ADMIN_REVEAL_IP is set.
func (s *Server) handleAdminListSessionHistory(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	q := r.URL.Query()
	filter := store.SessionHistoryFilter{
		UserID:    q.Get("user_id"),
		EndReason: q.Get("end_reason"),
		Country:   q.Get("country"),
		From:      parseInt64Param(q.Get("from")),
		To:        parseInt64Param(q.Get("to")),
		Page:      parseIntParam(q.Get("page"), 1),
		PageSize:  parseIntParam(q.Get("page_size"), 50),
	}
	rows, total, err := st.ListSessionHistory(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo leer el historial"})
		return
	}

	users := loadUserLabels(r.Context(), st, rows)
	reveal := RevealIPEnabled()
	out := make([]historyRowJSON, 0, len(rows))
	for _, h := range rows {
		j := historyRowJSON{
			ID:                h.ID,
			OriginalSessionID: h.OriginalSessionID,
			UserID:            h.UserID,
			UserLabel:         users[h.UserID],
			Mode:              h.Mode,
			Label:             h.Label,
			Public:            h.Public,
			ViewToken:         h.ViewToken,
			CreatedAt:         h.CreatedAt,
			EndedAt:           h.EndedAt,
			DurationSeconds:   h.DurationSeconds,
			EndReason:         h.EndReason,
			ClientIPHash:      h.ClientIPHash,
			UAShort:           h.UAShort,
			GeoCountry:        h.GeoCountry,
			ReferrerHost:      h.ReferrerHost,
			TotalCommands:     h.TotalCommands,
			TotalTutorQueries: h.TotalTutorQueries,
		}
		if reveal {
			j.ClientIP = h.ClientIP
		}
		out = append(out, j)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":      out,
		"page":      filter.Page,
		"page_size": filter.PageSize,
		"total":     total,
		"filters": map[string]any{
			"user_id":    filter.UserID,
			"end_reason": filter.EndReason,
			"country":    filter.Country,
			"from":       filter.From,
			"to":         filter.To,
		},
	})
}

// userActivityJSON is the wire shape of /admin/api/users/{id}/activity.
type userActivityJSON struct {
	UserID             string                          `json:"user_id"`
	UserLabel          string                          `json:"user_label,omitempty"`
	Window             map[string]int64                `json:"window"`
	TotalSessions      int                             `json:"total_sessions"`
	TotalActiveSeconds int64                           `json:"total_active_seconds"`
	AvgSessionMinutes  float64                         `json:"avg_session_minutes"`
	LastSeenAt         int64                           `json:"last_seen_at"`
	LastSeenCountry    string                          `json:"last_seen_country,omitempty"`
	TotalCommands      int64                           `json:"total_commands"`
	TotalTutorQueries  int64                           `json:"total_tutor_queries"`
	ByDay              []store.ActivityByDay           `json:"by_day"`
	ByCountry          []store.ActivityByCountry       `json:"by_country"`
}

// handleAdminUserActivity returns the activity report for a single user over
// an optional time window. Defaults to the last 30 days when no window is
// provided. Returns zero-values (not 404) for users with no history.
func (s *Server) handleAdminUserActivity(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	userID := r.PathValue("id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id requerido"})
		return
	}
	q := r.URL.Query()
	from := parseInt64Param(q.Get("from"))
	to := parseInt64Param(q.Get("to"))
	if from == 0 && to == 0 {
		// Default window: last 30 days.
		now := time.Now().Unix()
		from = now - 30*86400
		to = now
	}
	act, err := st.GetUserActivity(r.Context(), userID, from, to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo calcular actividad"})
		return
	}

	label := ""
	if u, err := st.GetUser(r.Context(), userID); err == nil {
		label = u.Name
	}
	writeJSON(w, http.StatusOK, userActivityJSON{
		UserID:             act.UserID,
		UserLabel:          label,
		Window:             map[string]int64{"from": act.From, "to": act.To},
		TotalSessions:      act.TotalSessions,
		TotalActiveSeconds: act.TotalActiveSeconds,
		AvgSessionMinutes:  act.AvgSessionMinutes,
		LastSeenAt:         act.LastSeenAt,
		LastSeenCountry:    act.LastSeenCountry,
		TotalCommands:      act.TotalCommands,
		TotalTutorQueries:  act.TotalTutorQueries,
		ByDay:              nilToEmptyDay(act.ByDay),
		ByCountry:          nilToEmptyCountry(act.ByCountry),
	})
}

// loadUserLabels resolves user IDs found in rows to their display names in
// one GetUser call per distinct id. Page size is small, so the N+1 cost is
// acceptable. Missing/deleted users surface as an empty label.
func loadUserLabels(ctx context.Context, st *store.Store, rows []store.SessionHistoryRow) map[string]string {
	out := make(map[string]string)
	for _, r := range rows {
		if r.UserID == "" {
			continue
		}
		if _, seen := out[r.UserID]; seen {
			continue
		}
		out[r.UserID] = ""
		if u, err := st.GetUser(ctx, r.UserID); err == nil {
			out[r.UserID] = u.Name
		}
	}
	return out
}

func nilToEmptyDay(in []store.ActivityByDay) []store.ActivityByDay {
	if in == nil {
		return []store.ActivityByDay{}
	}
	return in
}

func nilToEmptyCountry(in []store.ActivityByCountry) []store.ActivityByCountry {
	if in == nil {
		return []store.ActivityByCountry{}
	}
	return in
}

func parseIntParam(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func parseInt64Param(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
