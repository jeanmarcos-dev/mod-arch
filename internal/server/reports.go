package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// Field budgets for inbound reports. Generous for real feedback, tight enough
// that pasted novels do not balloon the row.
const (
	maxReportTitleRunes   = 120
	maxReportBodyRunes    = 4000
	maxReportContextBytes = 4096
)

var validReportKinds = []string{
	store.ReportKindFeedback, store.ReportKindBug, store.ReportKindIdea,
}

type reportRequest struct {
	Kind           string          `json:"kind"`
	Title          string          `json:"title"`
	Body           string          `json:"body"`
	ContextJSON    json.RawMessage `json:"context"`
	TurnstileToken string          `json:"turnstile_token"`
}

type reportResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	TS     int64  `json:"ts"`
}

// handlePostReport receives a public report submission. Anonymous is allowed
// (no user attached). Rate-limited by IP via reportLimit.
func (s *Server) handlePostReport(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	var req reportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}

	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if !slices.Contains(validReportKinds, kind) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "tipo de reporte inválido"})
		return
	}
	title := trimTo(req.Title, maxReportTitleRunes)
	body := trimTo(req.Body, maxReportBodyRunes)
	if title == "" || body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "título y cuerpo son obligatorios"})
		return
	}
	ctxJSON := sanitizeContextJSON(req.ContextJSON)

	cc := ClientCtxFrom(r)
	row := store.UserReport{
		TS:           time.Now().Unix(),
		Kind:         kind,
		Title:        title,
		Body:         body,
		ContextJSON:  ctxJSON,
		Status:       store.ReportStatusOpen,
		ClientIPHash: cc.IPHash,
	}
	if sess, _ := s.resolveSessionAndRole(r); sess != nil {
		row.SessionID = sess.ID
		if uid := sess.UserID(); uid != "" {
			row.UserID = uid
		}
	}

	// Turnstile is only enforced for anonymous reports: logged-in users
	// already proved they were human via the login Turnstile + magic-link
	// flow, and an authenticated submission is already attributable. When
	// no Turnstile secret is configured (dev mode), this is a no-op.
	if row.UserID == "" {
		secCfg, _ := loadSecurityConfig(r.Context(), st, s.encKey)
		if secCfg.TurnstileSecret != "" {
			if err := verifyTurnstile(r.Context(), secCfg.TurnstileSecret, req.TurnstileToken, r.RemoteAddr); err != nil {
				if errors.Is(err, errTurnstileFailed) {
					writeJSON(w, http.StatusForbidden, map[string]any{"error": "verificación antibot falló", "code": "antibot_failed"})
					return
				}
				s.logger.Warn("reports: turnstile verify error", "err", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "no fue posible verificar el desafío antibot", "code": "antibot_unavailable"})
				return
			}
		}
	}

	id, err := st.InsertReport(r.Context(), row)
	if err != nil {
		s.logger.Warn("reports: insert failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo guardar el reporte"})
		return
	}
	actor := "anon"
	if row.UserID != "" {
		actor = "user:" + row.UserID
	}
	s.auditFromContext(r, actor, "report.create", strings.ToLower(kind), `{"id":`+itoa(id)+`}`)
	writeJSON(w, http.StatusCreated, reportResponse{ID: id, Status: row.Status, TS: row.TS})
}

// sanitizeContextJSON keeps a small client-supplied JSON blob attached to the
// report, but rejects anything that is not a JSON object or is suspiciously
// large. We never parse the inner shape — the admin reads it as-is for
// debugging.
func sanitizeContextJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	if len(raw) > maxReportContextBytes {
		return "{}"
	}
	trimmed := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return "{}"
	}
	// Make sure the bytes parse as JSON before storing.
	var probe map[string]any
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return "{}"
	}
	return trimmed
}

// itoa is a tiny dependency-free formatter for the audit meta payload.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

// handleAdminListReports paginates and filters reports for the admin triage.
func (s *Server) handleAdminListReports(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	q := r.URL.Query()
	filter := store.ReportFilter{
		Kind:     q.Get("kind"),
		Status:   q.Get("status"),
		UserID:   q.Get("user_id"),
		Page:     parseIntParam(q.Get("page"), 1),
		PageSize: parseIntParam(q.Get("page_size"), 50),
	}
	rows, total, err := st.ListReports(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo leer los reportes"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":      rows,
		"page":      filter.Page,
		"page_size": filter.PageSize,
		"total":     total,
		"filters": map[string]any{
			"kind":    filter.Kind,
			"status":  filter.Status,
			"user_id": filter.UserID,
		},
	})
}

// handleAdminUpdateReportStatus changes status. Body: {"status":"resolved"}.
// Records the actor and timestamp on the row when transitioning to a
// terminal state.
func (s *Server) handleAdminUpdateReportStatus(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	idStr := r.PathValue("id")
	id := parseInt64Param(idStr)
	if id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id inválido"})
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}
	next := strings.ToLower(strings.TrimSpace(body.Status))
	allowed := []string{
		store.ReportStatusOpen, store.ReportStatusReviewing,
		store.ReportStatusResolved, store.ReportStatusDismissed,
	}
	if !slices.Contains(allowed, next) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "estado inválido"})
		return
	}

	var (
		resolvedAt int64
		actor      = s.adminActor(r)
		who        string
	)
	if next == store.ReportStatusResolved || next == store.ReportStatusDismissed {
		resolvedAt = time.Now().Unix()
		who = actor
	}
	if _, err := st.GetReport(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "reporte no encontrado"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "error interno"})
		return
	}
	if err := st.UpdateReportStatus(r.Context(), id, next, who, resolvedAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo actualizar"})
		return
	}
	s.audit(r, "report.status", itoa(id), `{"status":"`+next+`"}`)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": next})
}
