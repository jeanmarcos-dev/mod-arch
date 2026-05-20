package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/store"
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

// requireAdminAPI is the middleware for /admin/api/* routes. Unlike requests
// to /admin/* HTML, an unauthenticated API call returns a 401 JSON body so
// the SPA can react (push the user to /admin/login) without following a 302.
func (s *Server) requireAdminAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.admin.enabled {
			http.NotFound(w, r)
			return
		}
		c, err := r.Cookie(AdminCookie)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no autenticado"})
			return
		}
		if _, err := s.verifyCookie(c.Value); err != nil {
			clearCookie(w, AdminCookie)
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no autenticado"})
			return
		}
		next(w, r)
	}
}

// registerAdminRoutes wires the /admin* routes into the mux. When admin is
// disabled (env vars missing), the SPA catch-all returns 404 so the surface
// is not advertised.
func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/login", s.limitByIP(s.loginLimit, s.handleAdminLogin))
	mux.HandleFunc("POST /admin/logout", s.requireCSRF(s.handleAdminLogout))

	mux.HandleFunc("GET /admin/api/me", s.handleAdminMe)
	mux.HandleFunc("GET /admin/api/sessions", s.requireAdminAPI(s.handleAdminListSessions))
	mux.HandleFunc("POST /admin/api/sessions/{id}/kill", s.requireAdminAPI(s.requireCSRF(s.handleAdminKillSession)))
	mux.HandleFunc("POST /admin/api/sessions/{id}/label", s.requireAdminAPI(s.requireCSRF(s.handleAdminSetLabel)))
	mux.HandleFunc("POST /admin/api/sessions/{id}/public", s.requireAdminAPI(s.requireCSRF(s.handleAdminTogglePublic)))

	mux.HandleFunc("GET /admin/api/invites", s.requireAdminAPI(s.handleAdminListInvites))
	mux.HandleFunc("POST /admin/api/invites", s.requireAdminAPI(s.requireCSRF(s.handleAdminCreateInvite)))
	mux.HandleFunc("DELETE /admin/api/invites/{token}", s.requireAdminAPI(s.requireCSRF(s.handleAdminRevokeInvite)))

	mux.HandleFunc("GET /admin/api/settings", s.requireAdminAPI(s.handleAdminGetSettings))
	mux.HandleFunc("PUT /admin/api/settings", s.requireAdminAPI(s.requireCSRF(s.handleAdminSetSetting)))

	mux.HandleFunc("GET /admin/api/users", s.requireAdminAPI(s.handleAdminListUsers))
	mux.HandleFunc("POST /admin/api/users/{id}/kick", s.requireAdminAPI(s.requireCSRF(s.handleAdminKickUser)))

	mux.HandleFunc("GET /admin/api/audit", s.requireAdminAPI(s.handleAdminListAudit))

	mux.HandleFunc("GET /admin", s.handleAdminSPA)
	mux.HandleFunc("GET /admin/", s.handleAdminSPA)
}

// handleAdminSPA serves the SPA's index.html so the Vue Router can resolve
// /admin, /admin/login, /admin/anything on the client. Auth is enforced at
// the API layer; the HTML shell is identical for everyone, the SPA decides
// what to render after calling /admin/api/me. When admin is disabled at the
// server level, the same index.html is served with a 404 status so the
// generic NotFound view renders — no information about the admin panel's
// existence is leaked.
func (s *Server) handleAdminSPA(w http.ResponseWriter, r *http.Request) {
	if s.staticFS == nil {
		s.handlePlaceholder(w, r)
		return
	}
	if !s.admin.enabled {
		serveSPAStatus(w, s.staticFS, http.StatusNotFound)
		return
	}
	s.ensureCSRFCookie(w, r)
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r2)
}

// serveSPAStatus writes index.html with the given status code so the client
// shows the SPA NotFound view even when the response carries a real 404.
func serveSPAStatus(w http.ResponseWriter, files fs.FS, status int) {
	data, err := fs.ReadFile(files, "index.html")
	if err != nil {
		http.Error(w, "not found", status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// handleAdminMe reports the current admin identity to the SPA. Used by the
// router guard: 401 means "go to /admin/login"; 200 means "render dashboard".
func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	if !s.admin.enabled {
		http.NotFound(w, r)
		return
	}
	s.ensureCSRFCookie(w, r)
	c, err := r.Cookie(AdminCookie)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no autenticado"})
		return
	}
	user, err := s.verifyCookie(c.Value)
	if err != nil {
		clearCookie(w, AdminCookie)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no autenticado"})
		return
	}
	full, priv := s.manager.Count()
	total, peak := s.manager.Metrics()
	writeJSON(w, http.StatusOK, map[string]any{
		"user": user,
		"stats": map[string]any{
			"full":           full,
			"privileged":     priv,
			"max_public":     s.manager.MaxPublic(),
			"max_privileged": s.manager.MaxPrivileged(),
			"total_created":  total,
			"peak":           peak,
		},
	})
}

// handleAdminLogin validates credentials and sets the signed cookie. Accepts
// JSON ({user, pass}) for the SPA. Returns 200 with {ok, user} on success or
// 401 with {error} on failure. Body MUST be application/json.
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if !s.admin.enabled {
		http.NotFound(w, r)
		return
	}
	var creds struct {
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}

	// Constant-time username compare to prevent timing-based user enumeration.
	if subtle.ConstantTimeCompare([]byte(creds.User), []byte(s.admin.user)) != 1 {
		// Run a dummy bcrypt to keep total latency comparable to the success path.
		_ = bcrypt.CompareHashAndPassword(s.admin.passHash, []byte("x"))
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "credenciales inválidas"})
		return
	}
	if err := bcrypt.CompareHashAndPassword(s.admin.passHash, []byte(creds.Pass)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "credenciales inválidas"})
		return
	}

	signed, _ := s.signCookie(creds.User, AdminSessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     AdminCookie,
		Value:    signed,
		Path:     "/admin",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(AdminSessionTTL.Seconds()),
	})
	if st := s.manager.Store(); st != nil {
		_ = st.AppendAudit(r.Context(), store.AuditEntry{
			TS: time.Now().Unix(), Actor: "admin:" + creds.User, Action: "admin.login",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": creds.User})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	s.audit(r, "admin.logout", "", "")
	http.SetCookie(w, &http.Cookie{
		Name:     AdminCookie,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminActor extracts the admin username from a verified cookie. Returns the
// fallback "admin" when the cookie cannot be decoded (should not happen for
// requests that already passed requireAdminAPI).
func (s *Server) adminActor(r *http.Request) string {
	if c, err := r.Cookie(AdminCookie); err == nil {
		if user, err := s.verifyCookie(c.Value); err == nil {
			return "admin:" + user
		}
	}
	return "admin"
}

// audit safely appends an admin action to the audit log. metaJSON may be
// empty.
func (s *Server) audit(r *http.Request, action, target, metaJSON string) {
	s.auditFromContext(r, s.adminActor(r), action, target, metaJSON)
}

// auditFromContext is the same as audit but takes an explicit actor — used by
// student auth flows that need to record "user:<id>" instead of "admin:*".
func (s *Server) auditFromContext(r *http.Request, actor, action, target, metaJSON string) {
	st := s.manager.Store()
	if st == nil {
		return
	}
	err := st.AppendAudit(r.Context(), store.AuditEntry{
		TS:       time.Now().Unix(),
		Actor:    actor,
		Action:   action,
		Target:   target,
		MetaJSON: metaJSON,
	})
	if err != nil {
		s.logger.Warn("audit: append failed", "action", action, "err", err)
	}
}

func (s *Server) handleAdminKillSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.manager.Get(id) == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "sesión no encontrada"})
		return
	}
	s.manager.Kill(id)
	s.audit(r, "session.kill", id, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminSetLabel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.manager.Get(id)
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "sesión no encontrada"})
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body inválido"})
		return
	}
	label, err := cleanLabel(body.Label)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	sess.SetLabel(label)
	sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))
	meta, _ := json.Marshal(map[string]any{"label": label})
	s.audit(r, "session.label", id, string(meta))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "label": label})
}

func (s *Server) handleAdminTogglePublic(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.manager.Get(id)
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "sesión no encontrada"})
		return
	}
	var body struct {
		Public bool `json:"public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body inválido"})
		return
	}
	sess.SetPublic(body.Public)
	meta, _ := json.Marshal(map[string]any{"public": body.Public})
	s.audit(r, "session.public", id, string(meta))
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
	label, err := cleanLabel(body.Label)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	inv := s.manager.CreateInvite(label)
	meta, _ := json.Marshal(map[string]any{"label": label})
	s.audit(r, "invite.create", inv.Token, string(meta))

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
	s.audit(r, "invite.revoke", token, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminListSessions(w http.ResponseWriter, _ *http.Request) {
	type sessJSON struct {
		ID         string `json:"id"`
		Mode       string `json:"mode"`
		Label      string `json:"label"`
		Public     bool   `json:"public"`
		HasShare   bool   `json:"has_share"`
		ViewToken  string `json:"view_token,omitempty"`
		CreatedAt  int64  `json:"created_at"`
		LastActive int64  `json:"last_active"`
	}
	all := s.manager.All()
	out := make([]sessJSON, 0, len(all))
	for _, sess := range all {
		token := sess.ViewToken()
		out = append(out, sessJSON{
			ID:         sess.ID,
			Mode:       sess.Mode.String(),
			Label:      sess.Label(),
			Public:     sess.Public(),
			HasShare:   token != "",
			ViewToken:  token,
			CreatedAt:  sess.CreatedAt.Unix(),
			LastActive: sess.LastActiveAt().Unix(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
