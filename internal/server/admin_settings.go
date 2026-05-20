package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jeanmarcos/modos-cpu/internal/store"
)

type settingsResponse struct {
	AccessMode             string `json:"access_mode"`
	ViewerPasswordRequired bool   `json:"viewer_password_required"`
	ViewerPasswordSet      bool   `json:"viewer_password_set"`
}

// handleAdminGetSettings returns the access-related settings as a flat object
// the SPA can bind to. The bcrypt hash itself is never returned — only the
// "is one configured?" boolean.
func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	mode := s.accessMode(r.Context())

	required := false
	if v, err := st.GetSetting(r.Context(), SettingViewerPasswordRequired); err == nil && v == "true" {
		required = true
	}
	hash, _ := st.GetSetting(r.Context(), SettingViewerPasswordHash)
	writeJSON(w, http.StatusOK, settingsResponse{
		AccessMode:             mode,
		ViewerPasswordRequired: required && hash != "",
		ViewerPasswordSet:      hash != "",
	})
}

// handleAdminSetSetting accepts {key, value} and applies one of the known
// settings. Unknown keys are rejected to keep the surface tight.
func (s *Server) handleAdminSetSetting(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}

	if status, msg := s.applySetting(r, st, body.Key, body.Value); status != http.StatusOK {
		writeJSON(w, status, map[string]any{"error": msg})
		return
	}
	meta := body.Value
	if body.Key == "viewer_password" {
		meta = "updated"
		if body.Value == "" {
			meta = "cleared"
		}
	}
	metaJSON, _ := json.Marshal(map[string]any{"value": meta})
	s.audit(r, "settings.set", body.Key, string(metaJSON))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// applySetting validates and persists one settings change. Returns
// (200, "") on success; otherwise an HTTP status and an error message
// suitable for the client.
func (s *Server) applySetting(r *http.Request, st *store.Store, key, value string) (int, string) {
	now := time.Now().Unix()
	actor := s.adminActor(r)
	switch key {
	case SettingAccessMode:
		if value != AccessOpen && value != AccessInviteOnly && value != AccessClosed {
			return http.StatusBadRequest, "valor inválido para access_mode"
		}
		return persistSetting(r, st, key, value, actor, now)
	case SettingViewerPasswordRequired:
		if value != "true" && value != "false" {
			return http.StatusBadRequest, "valor inválido (true|false)"
		}
		return persistSetting(r, st, key, value, actor, now)
	case "viewer_password":
		return s.applyViewerPassword(r, st, value, actor, now)
	default:
		return http.StatusBadRequest, "clave desconocida"
	}
}

func persistSetting(r *http.Request, st *store.Store, key, value, actor string, now int64) (int, string) {
	if err := st.SetSetting(r.Context(), key, value, actor, now); err != nil {
		return http.StatusInternalServerError, "no se pudo guardar"
	}
	return http.StatusOK, ""
}

// applyViewerPassword bcrypts the plaintext value before storing it, or
// clears the hash AND the required flag when the value is empty.
func (s *Server) applyViewerPassword(r *http.Request, st *store.Store, value, actor string, now int64) (int, string) {
	if value == "" {
		_ = st.DeleteSetting(r.Context(), SettingViewerPasswordHash)
		_ = st.SetSetting(r.Context(), SettingViewerPasswordRequired, "false", actor, now)
		return http.StatusOK, ""
	}
	h, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)
	if err != nil {
		return http.StatusInternalServerError, "no se pudo hashear"
	}
	if err := st.SetSetting(r.Context(), SettingViewerPasswordHash, string(h), actor, now); err != nil {
		return http.StatusInternalServerError, "no se pudo guardar"
	}
	return http.StatusOK, ""
}

type userResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Email       string             `json:"email,omitempty"`
	StudentID   string             `json:"student_id,omitempty"`
	LastLoginAt int64              `json:"last_login_at"`
	Sessions    []userSessionBrief `json:"sessions"`
}

type userSessionBrief struct {
	ID         string `json:"id"`
	Mode       string `json:"mode"`
	Label      string `json:"label"`
	LastActive int64  `json:"last_active"`
}

// handleAdminListUsers joins every persisted user with their currently live
// sessions (in-memory). Users with no active session still appear with an
// empty sessions array.
func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	users, err := st.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudieron listar usuarios"})
		return
	}
	sessionsByUser := make(map[string][]userSessionBrief)
	for _, sess := range s.manager.All() {
		uid := sess.UserID()
		if uid == "" {
			continue
		}
		sessionsByUser[uid] = append(sessionsByUser[uid], userSessionBrief{
			ID:         sess.ID,
			Mode:       sess.Mode.String(),
			Label:      sess.Label(),
			LastActive: sess.LastActiveAt().Unix(),
		})
	}
	out := make([]userResponse, 0, len(users))
	for _, u := range users {
		sess := sessionsByUser[u.ID]
		if sess == nil {
			sess = []userSessionBrief{}
		}
		out = append(out, userResponse{
			ID:          u.ID,
			Name:        u.Name,
			Email:       u.Email,
			StudentID:   u.StudentID,
			LastLoginAt: u.LastLoginAt,
			Sessions:    sess,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAdminKickUser kills every live session attached to the user. The
// users row is kept so audit history and dedupe by email/student_id survive.
func (s *Server) handleAdminKickUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	if _, err := st.GetUser(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "usuario no encontrado"})
		return
	}
	killed := 0
	for _, sess := range s.manager.All() {
		if sess.UserID() == id {
			s.manager.Kill(sess.ID)
			killed++
		}
	}
	meta, _ := json.Marshal(map[string]any{"sessions_killed": killed})
	s.audit(r, "user.kick", id, string(meta))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sessions_killed": killed})
}

type auditResponse struct {
	ID     int64  `json:"id"`
	TS     int64  `json:"ts"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Meta   string `json:"meta,omitempty"`
}

// handleAdminListAudit returns the most recent entries, newest first.
func (s *Server) handleAdminListAudit(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	rows, err := st.ListAudit(r.Context(), 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo leer el audit log"})
		return
	}
	out := make([]auditResponse, 0, len(rows))
	for _, e := range rows {
		out = append(out, auditResponse{
			ID: e.ID, TS: e.TS, Actor: e.Actor,
			Action: e.Action, Target: e.Target, Meta: e.MetaJSON,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
