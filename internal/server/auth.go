package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// magicLinkTTL is how long an emailed login token remains usable.
const magicLinkTTL = 15 * time.Minute

func (s *Server) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", s.limitByIP(s.authLimit, s.requireCSRF(s.handleAuthLogin)))
	mux.HandleFunc("GET /api/auth/me", s.handleAuthMe)
	mux.HandleFunc("GET /api/auth/sessions", s.handleAuthMySessions)
	mux.HandleFunc("POST /api/auth/logout", s.requireCSRF(s.handleAuthLogout))
	mux.HandleFunc("GET /auth/magic", s.handleAuthMagic)
}

type loginRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	StudentID string `json:"student_id"`
}

type userPublic struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Role  string `json:"role,omitempty"`
}

// handleAuthLogin creates or reuses a user identity and attaches it to the
// caller's session. When an email is supplied it also issues a magic link
// (logged to stderr) so the student can re-attach from another browser.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}
	name, err := cleanName(req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	email, err := cleanEmail(req.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	sid, err := cleanStudentID(req.StudentID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}

	user, err := s.upsertUser(r.Context(), st, name, email, sid)
	if err != nil {
		s.logger.Warn("auth: upsert user failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no fue posible crear el usuario"})
		return
	}

	sess, err := s.getOrCreateSession(w, r)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "no hay cupo para sesiones nuevas"})
		return
	}
	sess.SetUserID(user.ID)
	sess.SetLabel(user.Name)
	sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))

	magicSent := false
	magicFallback := ""
	if email != "" {
		url, sent, err := s.issueMagicLink(r, email)
		if err != nil {
			s.logger.Warn("auth: issue magic link failed", "email", email, "err", err)
		} else {
			magicSent = sent
			if !sent {
				// SMTP was unavailable or off — expose the link so the
				// frontend can show a toast with a copyable URL instead of
				// leaving the user stuck.
				magicFallback = url
			}
		}
	}

	s.auditFromContext(r, "user:"+user.ID, "auth.login", sess.ID, "")

	resp := map[string]any{
		"ok":         true,
		"user":       publicUser(user),
		"magic_sent": magicSent,
	}
	if magicFallback != "" {
		resp["magic_link"] = magicFallback
	}
	writeJSON(w, http.StatusOK, resp)
}

// upsertUser dedupes by email, then by student_id, otherwise creates a fresh
// row. Always refreshes name and last_login_at.
func (s *Server) upsertUser(ctx context.Context, st *store.Store, name, email, sid string) (*store.UserRow, error) {
	now := time.Now().Unix()
	var existing *store.UserRow
	var err error
	if email != "" {
		existing, err = st.GetUserByEmail(ctx, email)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	if existing == nil && sid != "" {
		existing, err = st.GetUserByStudentID(ctx, sid)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	if existing != nil {
		if existing.Name != name {
			if err := st.UpdateUserName(ctx, existing.ID, name); err != nil {
				return nil, err
			}
			existing.Name = name
		}
		if err := st.UpdateUserLastLogin(ctx, existing.ID, now); err != nil {
			return nil, err
		}
		existing.LastLoginAt = now
		return existing, nil
	}
	row := store.UserRow{
		ID:          newAuthID(),
		Name:        name,
		Email:       email,
		StudentID:   sid,
		CreatedAt:   now,
		LastLoginAt: now,
	}
	if err := st.CreateUser(ctx, row); err != nil {
		return nil, err
	}
	return &row, nil
}

// issueMagicLink mints a token, persists it, and attempts to deliver the
// resulting URL by email. Returns:
//
//   - url:  the magic-link URL (always populated when err is nil)
//   - sent: true when SMTP accepted the message; false when SMTP is not
//     configured or the send failed (caller surfaces the URL as fallback)
//   - err:  only set when the token could not be persisted at all
//
// The link is always logged to stderr so a desperate operator can recover it
// from journalctl even when SMTP, the UI and the network all fail at once.
func (s *Server) issueMagicLink(r *http.Request, email string) (string, bool, error) {
	st := s.manager.Store()
	if st == nil {
		return "", false, errors.New("store unavailable")
	}
	token := newAuthID() + newAuthID()
	now := time.Now()
	if err := st.CreateMagicLink(r.Context(), store.MagicLinkRow{
		Token:     token,
		Email:     email,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(magicLinkTTL).Unix(),
	}); err != nil {
		return "", false, err
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/auth/magic?t=%s", scheme, r.Host, token)
	s.logger.Info("magic link", "email", email, "url", url, "expires_in", magicLinkTTL.String())

	if err := s.sendMagicLinkEmail(r.Context(), email, url); err != nil {
		if !errors.Is(err, errSMTPNotConfigured) {
			s.logger.Warn("smtp: send magic link failed", "email", email, "err", err)
		}
		return url, false, nil
	}
	return url, true, nil
}

// handleAuthMe reports the user attached to the caller's session, if any.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	s.ensureCSRFCookie(w, r)
	user, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no autenticado"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

// handleAuthMySessions lists every session attached to the caller's user. The
// student-facing "Mis sesiones" page uses this to show a history.
func (s *Server) handleAuthMySessions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no autenticado"})
		return
	}
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	rows, err := st.ListSessionsByUserID(r.Context(), user.ID)
	if err != nil {
		s.logger.Warn("auth: list user sessions failed", "user", user.ID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "error interno"})
		return
	}
	type item struct {
		ID         string `json:"id"`
		Mode       string `json:"mode"`
		Label      string `json:"label"`
		Public     bool   `json:"public"`
		ViewToken  string `json:"view_token,omitempty"`
		CreatedAt  int64  `json:"created_at"`
		LastActive int64  `json:"last_active"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		out = append(out, item{
			ID:         row.ID,
			Mode:       row.Mode,
			Label:      row.Label,
			Public:     row.Public,
			ViewToken:  row.ViewToken,
			CreatedAt:  row.CreatedAt,
			LastActive: row.LastActiveAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAuthLogout detaches the user from the current session while keeping
// the session itself alive, so the student can continue anonymously.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	sess, role := s.resolveSessionAndRole(r)
	if sess != nil && role == sessions.RoleOwner {
		prevUser := sess.UserID()
		sess.SetUserID("")
		sess.SetLabel("")
		sess.Sim.Broadcast(s.sessionInfoMessage(sess, role))
		if prevUser != "" {
			s.auditFromContext(r, "user:"+prevUser, "auth.logout", sess.ID, "")
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAuthMagic consumes a magic-link token, resolves the user by the
// associated email, attaches it to the caller's session and redirects to /.
func (s *Server) handleAuthMagic(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("t")
	if token == "" {
		http.Error(w, "token requerido", http.StatusBadRequest)
		return
	}
	st := s.manager.Store()
	if st == nil {
		http.Error(w, "persistencia deshabilitada", http.StatusServiceUnavailable)
		return
	}
	email, err := st.ConsumeMagicLink(r.Context(), token, time.Now().Unix())
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "el enlace es inválido o expiró", http.StatusGone)
		return
	}
	if err != nil {
		s.logger.Warn("auth: consume magic link failed", "err", err)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	user, err := st.GetUserByEmail(r.Context(), email)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "el usuario asociado ya no existe", http.StatusGone)
		return
	}
	if err != nil {
		s.logger.Warn("auth: lookup user by email failed", "err", err)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	if err := st.UpdateUserLastLogin(r.Context(), user.ID, time.Now().Unix()); err != nil {
		s.logger.Warn("auth: refresh last_login failed", "err", err)
	}

	sess, err := s.getOrCreateSession(w, r)
	if err != nil {
		http.Error(w, "no hay cupo para sesiones nuevas", http.StatusServiceUnavailable)
		return
	}
	sess.SetUserID(user.ID)
	sess.SetLabel(user.Name)
	sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))
	s.auditFromContext(r, "user:"+user.ID, "auth.magic", sess.ID, "")

	http.Redirect(w, r, "/", http.StatusFound)
}

// currentUser resolves the user attached to the caller's session ("" => false).
func (s *Server) currentUser(r *http.Request) (*store.UserRow, bool) {
	sess, _ := s.resolveSessionAndRole(r)
	if sess == nil {
		return nil, false
	}
	uid := sess.UserID()
	if uid == "" {
		return nil, false
	}
	st := s.manager.Store()
	if st == nil {
		return nil, false
	}
	user, err := st.GetUser(r.Context(), uid)
	if err != nil {
		return nil, false
	}
	return user, true
}

// EnvAdminUserID is the synthetic user id reported by userRoleForRequest when
// the caller is the env-configured ADMIN_USER. It is NOT a real row in the
// users table — promote/demote refuses to touch this id so the env admin is
// always reachable for recovery.
const EnvAdminUserID = "env-admin"

// userRoleForRequest reports the authenticated identity behind a request and
// its role for admin-side gating. It is the single source of truth used by
// requireAdminAPI / requireModRole.
//
// Resolution order:
//  1. If the mc_admin signed cookie is valid, return ("env-admin", "admin").
//     The env admin is irrevocable and exists outside the users table.
//  2. Otherwise, if the user session cookie (mc_sid) is attached to a known
//     user row, return (user.id, user.role).
//  3. Otherwise return ("", "").
func (s *Server) userRoleForRequest(r *http.Request) (userID string, role string) {
	if s.admin.enabled {
		if c, err := r.Cookie(AdminCookie); err == nil {
			if _, err := s.verifyCookie(c.Value); err == nil {
				return EnvAdminUserID, store.RoleAdmin
			}
		}
	}
	if user, ok := s.currentUser(r); ok {
		role := user.Role
		if !store.ValidRole(role) {
			role = store.RoleStudent
		}
		return user.ID, role
	}
	return "", ""
}

// roleActor returns the audit_log "actor" string for the current request,
// matching the existing conventions: "admin:<user>" for env admin (carries
// the configured ADMIN_USER name) and "user:<id>" for DB-backed identities.
// Empty when the request is unauthenticated.
func (s *Server) roleActor(r *http.Request) string {
	if s.admin.enabled {
		if c, err := r.Cookie(AdminCookie); err == nil {
			if user, err := s.verifyCookie(c.Value); err == nil {
				return "admin:" + user
			}
		}
	}
	if user, ok := s.currentUser(r); ok {
		return "user:" + user.ID
	}
	return ""
}

func publicUser(u *store.UserRow) userPublic {
	role := u.Role
	if !store.ValidRole(role) {
		role = store.RoleStudent
	}
	return userPublic{ID: u.ID, Name: u.Name, Email: u.Email, Role: role}
}

func newAuthID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}
