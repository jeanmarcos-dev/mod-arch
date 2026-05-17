package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
)

// AdminCookie is the name of the signed cookie that authenticates the admin.
const AdminCookie = "mc_admin"

// AdminSessionTTL is how long an admin session lasts before requiring re-login.
const AdminSessionTTL = 8 * time.Hour

// adminConfig groups the parameters the admin reads from env vars at startup.
// If user/passHash/secret are missing, admin is DISABLED and /admin/* routes
// return 404 — secure default posture.
type adminConfig struct {
	user     string
	passHash []byte
	secret   []byte // HMAC key for signing cookies
	enabled  bool
}

func loadAdminConfig() adminConfig {
	user := os.Getenv("ADMIN_USER")
	hash := os.Getenv("ADMIN_PASS_HASH")
	secret := os.Getenv("SESSION_SECRET")
	if user == "" || hash == "" || secret == "" {
		return adminConfig{}
	}
	return adminConfig{
		user:     user,
		passHash: []byte(hash),
		secret:   []byte(secret),
		enabled:  true,
	}
}

// HashAdminPassword is a CLI helper that produces the value for ADMIN_PASS_HASH.
// It is invoked from main.go when the binary runs as `modos-cpu admin-hash -pass=...`.
func HashAdminPassword(pass string) (string, error) {
	if pass == "" {
		return "", fmt.Errorf("empty password")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type adminCookiePayload struct {
	User string `json:"u"`
	Exp  int64  `json:"e"`
}

// signCookie produces a `<base64-payload>.<base64-hmac>` string valid for ttl.
func (s *Server) signCookie(user string, ttl time.Duration) (string, error) {
	p := adminCookiePayload{User: user, Exp: time.Now().Add(ttl).Unix()}
	raw, _ := json.Marshal(p)
	pb := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.admin.secret)
	mac.Write([]byte(pb))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return pb + "." + sig, nil
}

// verifyCookie validates the signature and expiration. Returns the user when OK.
func (s *Server) verifyCookie(value string) (string, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid format")
	}
	mac := hmac.New(sha256.New, s.admin.secret)
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	// Constant-time compare to avoid timing side channels on the HMAC.
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) != 1 {
		return "", fmt.Errorf("invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	var p adminCookiePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	if time.Now().Unix() > p.Exp {
		return "", fmt.Errorf("expired")
	}
	return p.User, nil
}

// requireAdmin redirects to login if the request lacks a valid admin cookie.
// When admin is disabled (env vars missing) it returns 404 so the surface is
// not advertised.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.admin.enabled {
			http.NotFound(w, r)
			return
		}
		c, err := r.Cookie(AdminCookie)
		if err != nil {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		if _, err := s.verifyCookie(c.Value); err != nil {
			clearCookie(w, AdminCookie)
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// registerAdminRoutes wires the /admin* routes into the mux. They are mounted
// even when admin is disabled (handlers return 404) so they don't dilute the
// catch-all "/" route.
func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin", s.requireAdmin(s.handleAdminDashboard))
	mux.HandleFunc("GET /admin/login", s.handleAdminLoginGet)
	mux.HandleFunc("POST /admin/login", s.handleAdminLoginPost)
	mux.HandleFunc("POST /admin/logout", s.handleAdminLogout)

	mux.HandleFunc("GET /admin/api/sessions", s.requireAdmin(s.handleAdminListSessions))
	mux.HandleFunc("POST /admin/api/sessions/{id}/kill", s.requireAdmin(s.handleAdminKillSession))
	mux.HandleFunc("POST /admin/api/sessions/{id}/label", s.requireAdmin(s.handleAdminSetLabel))
	mux.HandleFunc("POST /admin/api/sessions/{id}/public", s.requireAdmin(s.handleAdminTogglePublic))

	mux.HandleFunc("GET /admin/api/invites", s.requireAdmin(s.handleAdminListInvites))
	mux.HandleFunc("POST /admin/api/invites", s.requireAdmin(s.handleAdminCreateInvite))
	mux.HandleFunc("DELETE /admin/api/invites/{token}", s.requireAdmin(s.handleAdminRevokeInvite))
}

func (s *Server) handleAdminKillSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.manager.Get(id) == nil {
		http.Error(w, "sesión no encontrada", http.StatusNotFound)
		return
	}
	s.manager.Kill(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminSetLabel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.manager.Get(id)
	if sess == nil {
		http.Error(w, "sesión no encontrada", http.StatusNotFound)
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "body inválido", http.StatusBadRequest)
		return
	}
	label := strings.TrimSpace(body.Label)
	if len(label) > 32 {
		label = label[:32]
	}
	sess.SetLabel(label)
	sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "label": label})
}

func (s *Server) handleAdminTogglePublic(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.manager.Get(id)
	if sess == nil {
		http.Error(w, "sesión no encontrada", http.StatusNotFound)
		return
	}
	var body struct {
		Public bool `json:"public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "body inválido", http.StatusBadRequest)
		return
	}
	sess.SetPublic(body.Public)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "public": body.Public})
}

func (s *Server) handleAdminCreateInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Body is optional; an empty label is acceptable.
		body.Label = ""
	}
	label := strings.TrimSpace(body.Label)
	if len(label) > 32 {
		label = label[:32]
	}
	inv := s.manager.CreateInvite(label)

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	url := fmt.Sprintf("%s://%s/?t=%s", scheme, host, inv.Token)

	writeJSON(w, http.StatusOK, map[string]any{
		"token": inv.Token,
		"label": inv.Label,
		"url":   url,
	})
}

func (s *Server) handleAdminListInvites(w http.ResponseWriter, r *http.Request) {
	type inviteJSON struct {
		Token     string `json:"token"`
		Label     string `json:"label"`
		CreatedAt int64  `json:"created_at"`
		URL       string `json:"url"`
	}
	list := s.manager.ListInvites()
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	out := make([]inviteJSON, 0, len(list))
	for _, inv := range list {
		out = append(out, inviteJSON{
			Token:     inv.Token,
			Label:     inv.Label,
			CreatedAt: inv.CreatedAt.Unix(),
			URL:       fmt.Sprintf("%s://%s/?t=%s", scheme, host, inv.Token),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminRevokeInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	s.manager.RevokeInvite(token)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminLoginGet(w http.ResponseWriter, r *http.Request) {
	if !s.admin.enabled {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{"Error": r.URL.Query().Get("err")}
	if err := s.adminLoginTmpl.Execute(w, data); err != nil {
		s.logger.Warn("admin login template", "err", err)
	}
}

func (s *Server) handleAdminLoginPost(w http.ResponseWriter, r *http.Request) {
	if !s.admin.enabled {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulario inválido", http.StatusBadRequest)
		return
	}
	user := r.FormValue("user")
	pass := r.FormValue("pass")

	// Constant-time username compare to prevent timing-based user enumeration.
	if subtle.ConstantTimeCompare([]byte(user), []byte(s.admin.user)) != 1 {
		// Run a dummy bcrypt to keep total latency comparable to the success path.
		_ = bcrypt.CompareHashAndPassword(s.admin.passHash, []byte("x"))
		http.Redirect(w, r, "/admin/login?err=credenciales", http.StatusFound)
		return
	}
	if err := bcrypt.CompareHashAndPassword(s.admin.passHash, []byte(pass)); err != nil {
		http.Redirect(w, r, "/admin/login?err=credenciales", http.StatusFound)
		return
	}

	signed, _ := s.signCookie(user, AdminSessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     AdminCookie,
		Value:    signed,
		Path:     "/admin",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(AdminSessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     AdminCookie,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

// handleAdminDashboard serves the dashboard HTML. Live data is fetched by
// the client via /admin/api/sessions (AJAX), so the HTML itself is static.
func (s *Server) handleAdminDashboard(w http.ResponseWriter, _ *http.Request) {
	full, priv := s.manager.Count()
	total, peak := s.manager.Metrics()
	data := map[string]any{
		"User":          s.admin.user,
		"Full":          full,
		"Privileged":    priv,
		"MaxPublic":     s.manager.MaxPublic(),
		"MaxPrivileged": s.manager.MaxPrivileged(),
		"TotalCreated":  total,
		"Peak":          peak,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.adminDashTmpl.Execute(w, data); err != nil {
		s.logger.Warn("admin dashboard template", "err", err)
	}
}

func (s *Server) handleAdminListSessions(w http.ResponseWriter, _ *http.Request) {
	type sessJSON struct {
		ID         string `json:"id"`
		Mode       string `json:"mode"`
		Label      string `json:"label"`
		Public     bool   `json:"public"`
		HasShare   bool   `json:"has_share"`
		CreatedAt  int64  `json:"created_at"`
		LastActive int64  `json:"last_active"`
	}
	all := s.manager.All()
	out := make([]sessJSON, 0, len(all))
	for _, sess := range all {
		out = append(out, sessJSON{
			ID:         sess.ID,
			Mode:       sess.Mode.String(),
			Label:      sess.Label(),
			Public:     sess.Public(),
			HasShare:   sess.ViewToken() != "",
			CreatedAt:  sess.CreatedAt.Unix(),
			LastActive: sess.LastActiveAt().Unix(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// initAdminTemplates parses the admin HTML templates. Called from New().
func (s *Server) initAdminTemplates() {
	s.adminLoginTmpl = mustParseTemplate("login", "templates/admin_login.html")
	s.adminDashTmpl = mustParseTemplate("dash", "templates/admin_dashboard.html")
}
