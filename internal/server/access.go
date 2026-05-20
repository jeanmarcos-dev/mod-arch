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
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="es"><head><meta charset="utf-8"><title>Acceso restringido</title>
<style>
body{background:#0a0a0a;color:#e0e0e0;font-family:'JetBrains Mono',monospace;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;padding:24px}
.card{max-width:520px;text-align:center;border:1px solid #3a3a3a;background:rgba(0,0,0,.55);padding:32px}
h1{font-family:'Major Mono Display',monospace;color:#ffb347;letter-spacing:6px;font-size:18px;margin:0 0 12px}
p{color:#aaa;font-size:13px;line-height:1.6}
</style></head><body><div class="card">
<h1>ACCESO RESTRINGIDO</h1>
<p>` + htmlEscape(reason) + `</p>
<p>Si dispone de una invitación, abra el enlace que le facilitó el docente.</p>
</div></body></html>`))
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

// writeViewerPasswordForm renders a self-contained HTML form that POSTs to
// /v/{token}/auth. The optional errMsg is shown when the previous attempt
// was rejected.
func (s *Server) writeViewerPasswordForm(w http.ResponseWriter, token, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	page := `<!doctype html>
<html lang="es"><head><meta charset="utf-8"><title>Acceso · MODOS-CPU</title>
<style>
body{background:#0a0a0a;color:#e0e0e0;font-family:'JetBrains Mono',monospace;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;padding:24px}
.card{width:min(100%,380px);border:1px solid #3a3a3a;background:rgba(0,0,0,.55);padding:28px 26px}
h1{font-family:'Major Mono Display',monospace;color:#ffb347;letter-spacing:6px;font-size:16px;margin:0 0 18px;text-align:center}
p{color:#888;font-size:12px;line-height:1.5}
label{display:flex;flex-direction:column;gap:6px;font-size:11px;letter-spacing:1px;color:#888;text-transform:uppercase}
input{background:rgba(0,0,0,.55);border:1px solid #3a3a3a;color:#e0e0e0;padding:9px 10px;font-size:14px;font-family:inherit;outline:none}
input:focus{border-color:#ffb347;box-shadow:0 0 6px rgba(255,179,71,.25)}
button{margin-top:14px;width:100%;background:rgba(255,179,71,.12);border:1px solid #ffb347;color:#ffb347;padding:10px 12px;font-size:13px;letter-spacing:2px;text-transform:uppercase;cursor:pointer;font-family:inherit}
button:hover{background:rgba(255,179,71,.2)}
.err{color:#ff7a7a;font-size:12px;border-left:2px solid #ff7a7a;padding-left:8px;margin:0 0 14px}
</style></head><body><div class="card">
<h1>VISTA RESTRINGIDA</h1>
<p>Esta sesión compartida requiere una contraseña para ser observada.</p>`
	if errMsg != "" {
		page += `<p class="err">` + htmlEscape(errMsg) + `</p>`
	}
	page += `<form method="post" action="/v/` + htmlEscape(token) + `/auth">
<label>Contraseña<input name="password" type="password" autofocus required></label>
<button type="submit">INGRESAR</button>
</form></div></body></html>`
	_, _ = w.Write([]byte(page))
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
	http.Redirect(w, r, "/v/"+token, http.StatusFound)
}

func htmlEscape(s string) string {
	// Minimal escaper for the few characters that could appear in admin text.
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch r {
		case '&':
			out = append(out, "&amp;"...)
		case '<':
			out = append(out, "&lt;"...)
		case '>':
			out = append(out, "&gt;"...)
		case '"':
			out = append(out, "&quot;"...)
		default:
			out = append(out, string(r)...)
		}
	}
	return string(out)
}
