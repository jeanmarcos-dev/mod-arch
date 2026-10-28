package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jeanmarcos-dev/mod-arch/internal/server/i18n"
	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

// magicLinkTTL is how long an emailed login token remains usable.
const magicLinkTTL = 15 * time.Minute

func (s *Server) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", s.limitByIP(s.authLimit, s.requireCSRF(s.handleAuthLogin)))
	mux.HandleFunc("GET /api/auth/me", s.handleAuthMe)
	mux.HandleFunc("GET /api/auth/sessions", s.handleAuthMySessions)
	mux.HandleFunc("POST /api/auth/logout", s.requireCSRF(s.handleAuthLogout))
	mux.HandleFunc("GET /api/profile/me", s.handleProfileGet)
	mux.HandleFunc("PUT /api/profile/me", s.requireCSRF(s.handleProfilePut))
	mux.HandleFunc("POST /api/reports", s.limitByIP(s.reportLimit, s.requireCSRF(s.handlePostReport)))
	mux.HandleFunc("GET /auth/magic", s.handleAuthMagic)
	// Public bootstrap config (only fields safe for anonymous callers).
	mux.HandleFunc("GET /api/config", s.handlePublicConfig)
}

type loginRequest struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	StudentID      string `json:"student_id"`
	TurnstileToken string `json:"turnstile_token"`
}

type userPublic struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Role  string `json:"role,omitempty"`
}

// handleAuthLogin reacts in one of three ways depending on SMTP availability:
//
//   - SMTP off (no host/from configured): trust-on-first-use. The session is
//     attached to the user immediately so the student can keep working in
//     dev or in an aula where mail is not set up.
//   - SMTP on and the magic link was delivered: the session stays anonymous;
//     the caller has to click the link in their inbox to actually log in.
//     This is the path that prevents impersonation when SMTP works.
//   - SMTP on but the send failed: 503. We do NOT fall back to direct login,
//     since that would let a deliberate SMTP outage downgrade the security
//     model.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		i18n.WriteError(w, r, http.StatusBadRequest, i18n.CodeAuthInvalidJSON, nil)
		return
	}
	name, err := cleanName(req.Name)
	if err != nil {
		i18n.WriteError(w, r, http.StatusBadRequest, i18n.CodeAuthValidationFailed, map[string]string{"reason": err.Error()})
		return
	}
	email, err := cleanEmail(req.Email)
	if err != nil {
		i18n.WriteError(w, r, http.StatusBadRequest, i18n.CodeAuthValidationFailed, map[string]string{"reason": err.Error()})
		return
	}
	if email != "" {
		allow := s.loadEmailAllowlist(r.Context())
		if !allow.Permits(email) {
			i18n.WriteError(w, r, http.StatusForbidden, i18n.CodeAuthEmailNotAllowed, nil)
			return
		}
	}
	sid, err := cleanStudentID(req.StudentID)
	if err != nil {
		i18n.WriteError(w, r, http.StatusBadRequest, i18n.CodeAuthValidationFailed, map[string]string{"reason": err.Error()})
		return
	}

	st := s.manager.Store()
	if st == nil {
		i18n.WriteError(w, r, http.StatusServiceUnavailable, i18n.CodeAuthPersistenceDisabled, nil)
		return
	}

	// Turnstile is enforced only when a secret is persisted. Cloudflare
	// network errors fail closed (503) so misconfiguration cannot bypass
	// the check by accident.
	secCfg, secErr := loadSecurityConfig(r.Context(), st, s.encKey)
	if secErr != nil {
		s.logger.Warn("auth: load security config failed", "err", secErr)
	}
	if err := verifyTurnstile(r.Context(), secCfg.TurnstileSecret, req.TurnstileToken, r.RemoteAddr); err != nil {
		if errors.Is(err, errTurnstileFailed) {
			i18n.WriteError(w, r, http.StatusForbidden, i18n.CodeAuthAntibotFailed, nil)
			return
		}
		s.logger.Warn("auth: turnstile verify error", "err", err)
		i18n.WriteError(w, r, http.StatusServiceUnavailable, i18n.CodeAuthAntibotUnavailable, nil)
		return
	}

	user, err := s.upsertUser(r.Context(), st, name, email, sid)
	if err != nil {
		s.logger.Warn("auth: upsert user failed", "err", err)
		i18n.WriteError(w, r, http.StatusInternalServerError, i18n.CodeAuthUserCreateFailed, nil)
		return
	}

	sess, err := s.getOrCreateSession(w, r)
	if err != nil {
		i18n.WriteError(w, r, http.StatusServiceUnavailable, i18n.CodeAuthNoSessionQuota, nil)
		return
	}

	// Decide the flow based on SMTP availability. With no email the only
	// path possible is direct attach (there is nowhere to send a link to).
	if email == "" {
		s.attachLoggedInSession(sess, user)
		s.auditFromContext(r, "user:"+user.ID, "auth.login", sess.ID, "")
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":   true,
			"user": publicUser(user),
			"smtp": "off",
		})
		return
	}

	_, err = s.issueMagicLink(r, email)
	switch {
	case errors.Is(err, errSMTPNotConfigured):
		// Graceful degradation: no mail backend means no way to verify, so
		// the caller is trusted as before. This keeps the simulator usable
		// in environments without SMTP (local dev, classroom without mail).
		s.attachLoggedInSession(sess, user)
		s.auditFromContext(r, "user:"+user.ID, "auth.login", sess.ID, "")
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":   true,
			"user": publicUser(user),
			"smtp": "off",
		})

	case err != nil:
		// SMTP is configured but the send failed (transient network issue,
		// bad credentials, etc.). Refuse the login outright — falling back
		// would let an attacker DoS the mailer to bypass verification.
		s.logger.Warn("auth: send magic link failed", "email", email, "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "no se pudo enviar el correo de verificación. Intente de nuevo en unos minutos o contacte al administrador.",
			"code":  "smtp_send_failed",
		})

	default:
		// Mail accepted. The session stays anonymous until the user clicks
		// the link, which lands on handleAuthMagic and runs SetUserID there.
		s.auditFromContext(r, "user:"+user.ID, "auth.login_pending", sess.ID, "")
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"smtp":    "on",
			"pending": true,
			"email":   email,
		})
	}
}

// attachLoggedInSession runs the side effects that complete a login: bind the
// session to the user, set the visible alias, kill any other session the
// user might have left running elsewhere, and broadcast the updated
// session-info packet so any connected WebSocket sees the new identity.
//
// The "one session per user" policy is enforced here so it kicks in from
// both entry points (direct login when SMTP is off, and magic-link consume).
func (s *Server) attachLoggedInSession(sess *sessions.Session, user *store.UserRow) {
	sess.SetUserID(user.ID)
	sess.SetLabel(user.Name)
	s.manager.KillOtherUserSessions(user.ID, sess.ID)
	sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))
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
// resulting URL by email. The link is always logged to stderr so an operator
// can recover it from journalctl even when SMTP, the UI and the network all
// fail at once.
//
// Returns:
//   - url: the magic-link URL (always populated when the token was persisted)
//   - err: nil when the link was delivered by mail; errSMTPNotConfigured when
//     no SMTP backend is set up; any other error means the link was minted
//     but the send failed (configured-but-broken — caller should treat as a
//     hard error rather than degrade silently).
func (s *Server) issueMagicLink(r *http.Request, email string) (string, error) {
	st := s.manager.Store()
	if st == nil {
		return "", errors.New("store unavailable")
	}
	token := newAuthID() + newAuthID()
	now := time.Now()
	if err := st.CreateMagicLink(r.Context(), store.MagicLinkRow{
		Token:     token,
		Email:     email,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(magicLinkTTL).Unix(),
	}); err != nil {
		return "", err
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/auth/magic?t=%s", scheme, r.Host, token)
	s.logger.Info("magic link", "email", maskEmail(email), "expires_in", magicLinkTTL.String())

	if err := s.sendMagicLinkEmail(r.Context(), email, url); err != nil {
		return url, err
	}
	return url, nil
}

func maskEmail(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at <= 0 {
		return "***"
	}
	first, _ := utf8.DecodeRuneInString(email)
	return string(first) + "***" + email[at:]
}

// handleAuthMe reports the user attached to the caller's session, if any.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	s.ensureCSRFCookie(w, r)
	user, ok := s.currentUser(r)
	if !ok {
		i18n.WriteError(w, r, http.StatusUnauthorized, i18n.CodeAuthUnauthenticated, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

// handleAuthMySessions lists every session attached to the caller's user. The
// student-facing "Mis sesiones" page uses this to show a history.
func (s *Server) handleAuthMySessions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		i18n.WriteError(w, r, http.StatusUnauthorized, i18n.CodeAuthUnauthenticated, nil)
		return
	}
	st := s.manager.Store()
	if st == nil {
		i18n.WriteError(w, r, http.StatusServiceUnavailable, i18n.CodeAuthPersistenceDisabled, nil)
		return
	}
	rows, err := st.ListSessionsByUserID(r.Context(), user.ID)
	if err != nil {
		s.logger.Warn("auth: list user sessions failed", "user", user.ID, "err", err)
		i18n.WriteError(w, r, http.StatusInternalServerError, i18n.CodeAuthInternalError, nil)
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
	// Re-check the allowlist on consume so a setting tightened between
	// issuance and click does not let an old link bypass current policy.
	if allow := s.loadEmailAllowlist(r.Context()); !allow.Permits(email) {
		http.Error(w, "este correo ya no está autorizado", http.StatusForbidden)
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
	s.attachLoggedInSession(sess, user)
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
