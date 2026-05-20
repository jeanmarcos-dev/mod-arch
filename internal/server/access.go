package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/jeanmarcos/modos-cpu/internal/store"
)

var (
	accessDeniedTmpl   = mustParseTemplate("access_denied", "templates/access_denied.html")
	viewerPasswordTmpl = mustParseTemplate("viewer_password", "templates/viewer_password.html")
)

// Settings keys persisted in the settings table.
const (
	SettingAccessMode             = "access_mode"
	SettingViewerPasswordRequired = "viewer_password_required"
	SettingViewerPasswordHash     = "viewer_password_hash"
)

// Access mode values.
const (
	AccessOpen        = "open"
	AccessInviteOnly  = "invite_only"
	AccessClosed      = "closed"
	DefaultAccessMode = AccessOpen
)

// ErrAccessDenied signals that a new session cannot be created under the
// current access policy. Callers should respond with 503 + an explanation.
var ErrAccessDenied = errors.New("server: new sessions blocked by access policy")

// accessMode reads the current access_mode setting (defaults to "open").
func (s *Server) accessMode(ctx context.Context) string {
	st := s.manager.Store()
	if st == nil {
		return DefaultAccessMode
	}
	v, err := st.GetSetting(ctx, SettingAccessMode)
	if errors.Is(err, store.ErrNotFound) {
		return DefaultAccessMode
	}
	if err != nil {
		s.logger.Warn("settings: read access_mode failed", "err", err)
		return DefaultAccessMode
	}
	switch v {
	case AccessOpen, AccessInviteOnly, AccessClosed:
		return v
	default:
		return DefaultAccessMode
	}
}

// viewerPasswordRequired returns true iff a password gate is configured for
// /v/<token>. The password itself is stored as a bcrypt hash; required=true
// with an empty hash is treated as "not required" to avoid locking everyone
// out due to a half-applied setting.
func (s *Server) viewerPasswordRequired(ctx context.Context) (required bool, hash string) {
	st := s.manager.Store()
	if st == nil {
		return false, ""
	}
	v, err := st.GetSetting(ctx, SettingViewerPasswordRequired)
	if err != nil || v != "true" {
		return false, ""
	}
	hash, err = st.GetSetting(ctx, SettingViewerPasswordHash)
	if err != nil || hash == "" {
		return false, ""
	}
	return true, hash
}

// canCreateSession returns whether a brand-new public session may be created
// for the caller right now. invite-only requests succeed only after the
// caller has consumed an invite — that path bypasses this check via
// consumePrivInvite, so here invite_only collapses to "deny".
func (s *Server) canCreateSession(ctx context.Context) (bool, string) {
	switch s.accessMode(ctx) {
	case AccessOpen:
		return true, ""
	case AccessInviteOnly:
		return false, "se requiere una invitación válida para acceder a esta instancia"
	case AccessClosed:
		return false, "el acceso a esta instancia está cerrado"
	}
	return true, ""
}

// writeAccessDeniedHTML serves a friendly 503 page for blocked HTML requests.
func (s *Server) writeAccessDeniedHTML(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := accessDeniedTmpl.Execute(w, map[string]any{"Reason": reason}); err != nil {
		s.logger.Warn("access_denied template execute failed", "err", err)
	}
}

// hasValidViewerPasswordCookie verifies the HMAC signature on the cookie. The
// secret is the same one used for admin cookies; password gates only make
// sense when admin is enabled.
func (s *Server) hasValidViewerPasswordCookie(r *http.Request) bool {
	if !s.admin.enabled {
		return false
	}
	c, err := r.Cookie(ViewerPasswordCookie)
	if err != nil {
		return false
	}
	expected := s.viewerPasswordCookieValue()
	if expected == "" {
		return false
	}
	return hmac.Equal([]byte(c.Value), []byte(expected))
}

// viewerPasswordCookieValue derives a deterministic signed token from the
// current hash. Rotating the password invalidates every previously-issued
// cookie automatically because the input changes.
func (s *Server) viewerPasswordCookieValue() string {
	st := s.manager.Store()
	if st == nil {
		return ""
	}
	hash, err := st.GetSetting(context.Background(), SettingViewerPasswordHash)
	if err != nil || hash == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.admin.secret)
	mac.Write([]byte("viewer-pw:"))
	mac.Write([]byte(hash))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// writeViewerPasswordForm renders the gate page that POSTs the password to
// /v/{token}/auth. errMsg, when non-empty, surfaces the previous failure.
func (s *Server) writeViewerPasswordForm(w http.ResponseWriter, token, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	data := map[string]any{"Token": token, "Error": errMsg}
	if err := viewerPasswordTmpl.Execute(w, data); err != nil {
		s.logger.Warn("viewer_password template execute failed", "err", err)
	}
}

// handleViewerAuth is the POST endpoint for the viewer password gate. On
// success it sets the signed cookie and redirects to /v/{token}.
func (s *Server) handleViewerAuth(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if s.manager.GetByViewToken(token) == nil {
		http.Error(w, "vista no encontrada", http.StatusNotFound)
		return
	}
	required, hash := s.viewerPasswordRequired(r.Context())
	if !required {
		http.Redirect(w, r, "/v/"+token, http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form inválido", http.StatusBadRequest)
		return
	}
	pass := r.PostFormValue("password")
	if pass == "" {
		s.writeViewerPasswordForm(w, token, "ingrese una contraseña")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass)); err != nil {
		s.auditFromContext(r, "anon", "viewer_pw.fail", token, "")
		s.writeViewerPasswordForm(w, token, "contraseña incorrecta")
		return
	}
	value := s.viewerPasswordCookieValue()
	http.SetCookie(w, &http.Cookie{
		Name:     ViewerPasswordCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((24 * 7 * 3600)),
	})
	s.auditFromContext(r, "anon", "viewer_pw.ok", token, "")
	http.Redirect(w, r, "/v/"+token, http.StatusFound)
}
