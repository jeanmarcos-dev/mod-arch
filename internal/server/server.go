// Package server exposes one or more Simulators over HTTP + WebSocket.
//
// The Server multiplexes requests across the sessions held by the
// SessionManager: each HTTP/WS client carries a cookie (mc_sid for owners,
// mc_view for viewers) that identifies its Session. When the public cap is
// reached, GET / serves the /full page with the list of public sessions the
// client can join as a viewer.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/crypto"
	"github.com/jeanmarcos/modos-cpu/internal/programs"
	"github.com/jeanmarcos/modos-cpu/internal/server/i18n"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/store"
	"github.com/jeanmarcos/modos-cpu/internal/tutor"
)

const (
	// SessionCookie identifies the owner of a session.
	SessionCookie = "mc_sid"
	// ViewCookie identifies a viewer subscribed via /v/<token>.
	ViewCookie = "mc_view"
	// ViewerPasswordCookie remembers that the viewer has cleared the
	// per-instance password gate. Its value is a signed token.
	ViewerPasswordCookie = "mc_view_pw"
)

// Server is the multi-tenant HTTP+WS frontend of the simulator.
type Server struct {
	manager  *sessions.Manager
	logger   *slog.Logger
	staticFS fs.FS

	fullTmpl    *template.Template
	admin       adminConfig
	usage       *store.UsageRecorder
	live        liveCache
	loginLimit  *RateLimiter
	authLimit   *RateLimiter
	reportLimit *RateLimiter

	// tutorRegistry holds the active tutor service. Wrapping it in a
	// registry lets the admin /reload endpoint swap the service in place
	// without restarting the process.
	tutorRegistry *tutor.Registry
	tutorBuild    *tutor.BuildDeps // captured at startup so /reload can rebuild

	// encKey is the at-rest cipher key used to seal/open secrets stored in
	// the settings table (Turnstile secret, SMTP password, etc.). Derived
	// once from SESSION_SECRET at boot.
	encKey [32]byte
}

// Rate limit defaults for login surfaces. Tight enough to make brute force
// pointless while comfortable for legitimate retries.
const (
	loginLimitMax        = 5
	loginLimitWindow     = 15 * time.Minute
	rateLimitCleanupTick = 5 * time.Minute
	// Reports: 10/hour per IP is generous for legit feedback and tight enough
	// to keep a bored attacker from flooding the triage queue. Per the spec
	// the limit is intentionally not role-aware in v1.
	reportLimitMax    = 10
	reportLimitWindow = time.Hour
)

func studentLoginLimit(configured int) int {
	if configured > 0 {
		return configured
	}
	return loginLimitMax
}

// Config holds the Server construction parameters.
type Config struct {
	Manager        *sessions.Manager
	Logger         *slog.Logger
	StaticFS       fs.FS
	TutorRegistry  *tutor.Registry  // optional; empty registry disables the tutor surface
	TutorBuild     *tutor.BuildDeps // optional; required by /admin/api/tutor/reload to rebuild on config changes
	AuthLoginLimit int
}

// New builds a Server around a *sessions.Manager.
func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{
		manager:       cfg.Manager,
		logger:        cfg.Logger,
		staticFS:      cfg.StaticFS,
		fullTmpl:      mustParseTemplate("full", "templates/full.html"),
		admin:         loadAdminConfig(),
		loginLimit:    NewRateLimiter(context.Background(), loginLimitMax, loginLimitWindow, rateLimitCleanupTick),
		authLimit:     NewRateLimiter(context.Background(), studentLoginLimit(cfg.AuthLoginLimit), loginLimitWindow, rateLimitCleanupTick),
		reportLimit:   NewRateLimiter(context.Background(), reportLimitMax, reportLimitWindow, rateLimitCleanupTick),
		tutorRegistry: cfg.TutorRegistry,
		tutorBuild:    cfg.TutorBuild,
		encKey:        crypto.DeriveKey([]byte(os.Getenv("SESSION_SECRET"))),
	}
	if cfg.Manager != nil && cfg.Manager.Store() != nil {
		s.usage = store.NewUsageRecorder(cfg.Manager.Store(), cfg.Logger)
	}
	if s.admin.enabled {
		cfg.Logger.Info("admin enabled", "user", s.admin.user)
	} else {
		cfg.Logger.Info("admin disabled — set ADMIN_USER, ADMIN_PASS_HASH, SESSION_SECRET to enable")
	}
	return s
}

// Routes registers the HTTP endpoints on a mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /api/programs", s.handlePrograms)
	mux.HandleFunc("GET /api/categories", s.handleCategories)
	mux.HandleFunc("GET /api/memory", s.handleMemory)
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.HandleFunc("GET /full", s.handleFull)
	mux.HandleFunc("GET /v/{token}", s.handleViewer)
	mux.HandleFunc("POST /v/{token}/auth", s.handleViewerAuth)
	mux.HandleFunc("POST /v/leave", s.handleViewerLeave)
	mux.HandleFunc("GET /_assets/{name}", s.handleAsset)
	mux.HandleFunc("POST /api/i18n/lang", s.handleSetLang)
	s.registerAdminRoutes(mux)
	s.registerAuthRoutes(mux)
	mux.HandleFunc("/", s.handleRoot)
}

// handleRoot serves frontend static files. For HTML requests it first checks
// for `?t=<token>` (privileged invite) and consumes it; then it resolves or
// creates the client session. If the cap is full, /full is served instead.
// If the client carries an mc_view cookie (a viewer), no owner cookie is
// created — keeping the viewer role intact prevents accidental privilege.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if isHTMLPath(path) {
		if done := s.bootstrapHTMLRequest(w, r); done {
			return
		}
	}
	if s.staticFS != nil {
		if isHTMLPath(path) {
			s.ensureCSRFCookie(w, r)
		}
		s.serveSPA(w, r, path)
		return
	}
	s.handlePlaceholder(w, r)
}

// bootstrapHTMLRequest resolves the HTML-shaped request: consumes invite
// tokens, refreshes viewer cookies, and creates or attaches an owner session.
// Returns true when the response has already been written (cap-full, access
// denied, internal error, or invite redirect) and the caller should bail.
func (s *Server) bootstrapHTMLRequest(w http.ResponseWriter, r *http.Request) bool {
	if token := r.URL.Query().Get("t"); token != "" {
		if s.consumePrivInvite(w, r, token) {
			return true
		}
	}
	if c, err := r.Cookie(ViewCookie); err == nil {
		if sess := s.manager.GetByViewToken(c.Value); sess != nil {
			sess.Touch()
		} else {
			clearCookie(w, ViewCookie)
		}
		return false
	}
	if _, err := s.getOrCreateSession(w, r); err != nil {
		return s.handleSessionError(w, r, err)
	}
	return false
}

// handleSessionError translates the errors returned by getOrCreateSession
// into the appropriate HTTP response.
func (s *Server) handleSessionError(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, sessions.ErrCapReached) {
		s.handleFull(w, r)
		return true
	}
	if errors.Is(err, ErrAccessDenied) {
		_, reason := s.canCreateSession(r.Context())
		s.writeAccessDeniedHTML(w, r, reason)
		return true
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
	return true
}

// serveSPA hands the request to the embedded FileServer. HTML-shaped paths
// other than "/" (e.g. /login) are rewritten to "/" so vue-router can take
// over on the client.
//
// Cache headers are tuned for the hashed-bundle deploy model:
//   - The HTML shell is served with no-cache so the browser always revalidates
//     and picks up new asset hashes immediately after a deploy.
//   - 404 responses for /assets/* are wrapped with no-store so a CDN in front
//     (Cloudflare) cannot cache a stale "asset gone" answer for hours — that
//     was what made post-deploy navigation freeze on missing chunks.
//   - Successful asset responses keep the FileServer-provided headers; their
//     filename hash already provides cache busting.
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request, path string) {
	fs := http.FileServer(http.FS(s.staticFS))
	if isHTMLPath(path) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		if path != "/" {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fs.ServeHTTP(w, r2)
			return
		}
		fs.ServeHTTP(w, r)
		return
	}
	// Asset path. Wrap the writer so we can attach no-store to 404 responses
	// before the FileServer flushes them, blocking edge caches from holding
	// onto a "this hash is gone" answer between deploys.
	fs.ServeHTTP(&noStoreOn404Writer{ResponseWriter: w}, r)
}

// noStoreOn404Writer is a thin wrapper around http.ResponseWriter that, when
// the upstream handler writes a 4xx/5xx status, replaces Cache-Control with
// no-store. It does nothing for 2xx/3xx responses so cache-friendly assets
// keep their headers.
type noStoreOn404Writer struct {
	http.ResponseWriter
}

func (w *noStoreOn404Writer) WriteHeader(status int) {
	if status >= 400 {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.ResponseWriter.WriteHeader(status)
}

// consumePrivInvite tries to consume a privileged invite. On success it
// creates a ModePrivileged Session, sets the mc_sid cookie, redirects to "/"
// (stripping the token from the URL) and returns true. On failure it returns
// false so the caller continues with the normal flow.
func (s *Server) consumePrivInvite(w http.ResponseWriter, r *http.Request, token string) bool {
	label, err := s.manager.ConsumeInvite(token)
	if err != nil {
		s.logger.Info("invite consume failed", "token", token, "err", err)
		return false
	}
	sess, err := s.manager.CreateWithClient(sessions.ModePrivileged, clientInfoFrom(r))
	if err != nil {
		// MaxPrivileged reached despite a valid invite — rare; log and bail.
		s.logger.Warn("privileged create failed despite valid invite", "err", err)
		return false
	}
	if label != "" {
		sess.SetLabel(label)
	}
	// Drop any viewer cookie to avoid role confusion.
	clearCookie(w, ViewCookie)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
	return true
}

func isHTMLPath(path string) bool {
	if path == "/" {
		return true
	}
	lastSlash := strings.LastIndex(path, "/")
	last := path[lastSlash+1:]
	return !strings.Contains(last, ".")
}

// getOrCreateSession reads the mc_sid cookie and reuses the session if it
// exists; otherwise it creates a new ModeFull session. Returns ErrCapReached
// when the public cap is full, or ErrAccessDenied when access policy blocks
// new sessions for this caller. Existing sessions keep working even after
// access tightens — only new-creation is gated.
func (s *Server) getOrCreateSession(w http.ResponseWriter, r *http.Request) (*sessions.Session, error) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		if sess := s.manager.Get(c.Value); sess != nil {
			sess.Touch()
			return sess, nil
		}
	}
	if ok, _ := s.canCreateSession(r.Context()); !ok {
		return nil, ErrAccessDenied
	}
	sess, err := s.manager.CreateWithClient(sessions.ModeFull, clientInfoFrom(r))
	if err != nil {
		return nil, err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return sess, nil
}

// resolveSessionAndRole decides which Session the client belongs to and with
// which role. The viewer cookie (mc_view) takes precedence over the owner
// cookie (mc_sid) so that an owner who navigates to /v/<token> stays as a
// viewer until they return to /.
func (s *Server) resolveSessionAndRole(r *http.Request) (*sessions.Session, sessions.Role) {
	if c, err := r.Cookie(ViewCookie); err == nil {
		if sess := s.manager.GetByViewToken(c.Value); sess != nil {
			return sess, sessions.RoleViewer
		}
	}
	if c, err := r.Cookie(SessionCookie); err == nil {
		if sess := s.manager.Get(c.Value); sess != nil {
			return sess, sessions.RoleOwner
		}
	}
	return nil, sessions.RoleNone
}

// handleHealth is an unauthenticated liveness probe. It intentionally returns
// only {"ok": true} — session counts and capacity are operational state that
// belongs behind /api/admin, not in a public endpoint.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSession bootstraps the owner session for SPA clients that did not
// receive the initial HTML from Go — most notably the Vite dev server, which
// proxies /api and /ws to the backend but serves index.html itself, so
// handleRoot (and the mc_sid cookie it sets) never runs. Calling /api/session
// before opening the WebSocket guarantees the cookie exists; the endpoint is
// idempotent in production (the cookie is already set by handleRoot).
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sess, role := s.resolveSessionAndRole(r)
	if sess == nil {
		created, err := s.getOrCreateSession(w, r)
		if err != nil {
			switch {
			case errors.Is(err, sessions.ErrCapReached):
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "cap reached"})
			case errors.Is(err, ErrAccessDenied):
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "access denied"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			}
			return
		}
		sess = created
		role = sessions.RoleOwner
	}
	sess.Touch()
	writeJSON(w, http.StatusOK, s.sessionInfoMessage(sess, role))
}

func (s *Server) handlePrograms(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.manager.Programs())
}

// handleCategories returns the catalogue's category metadata so the frontend
// can render section headers (label + description + canonical order) without
// hardcoding them.
func (s *Server) handleCategories(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, programs.Categories())
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.resolveSessionAndRole(r)
	if sess == nil {
		http.Error(w, "no hay sesión — refresque la página", http.StatusUnauthorized)
		return
	}
	sess.Touch()

	addr, err := parseHexOrDec(r.URL.Query().Get("addr"))
	if err != nil {
		http.Error(w, "addr inválido", http.StatusBadRequest)
		return
	}
	lengthQ := r.URL.Query().Get("len")
	if lengthQ == "" {
		lengthQ = "64"
	}
	length, err := parseHexOrDec(lengthQ)
	if err != nil {
		http.Error(w, "len inválido", http.StatusBadRequest)
		return
	}
	if length > 1024 {
		length = 1024
	}
	bytesOut, err := sess.Sim.MemorySlice(addr, length)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out := make([]int, len(bytesOut))
	for i, b := range bytesOut {
		out[i] = int(b)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"addr":  addr,
		"bytes": out,
	})
}

// handleViewerLeave clears the viewer cookie so the spectator returns to the
// landing as a fresh client. The HttpOnly cookie cannot be cleared from JS,
// hence this endpoint. Idempotent: works even when no cookie is set.
func (s *Server) handleViewerLeave(w http.ResponseWriter, _ *http.Request) {
	clearCookie(w, ViewCookie)
	w.WriteHeader(http.StatusNoContent)
}

// handleSetLang persists the locale selected by the SPA in a cookie so the
// server-side templates (full.html, viewer_password.html, etc.) render in the
// same idiom on the next paint. The frontend keeps its own copy in
// localStorage; this endpoint is fire-and-forget from the client point of
// view — failures fall back to the previous locale silently.
func (s *Server) handleSetLang(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Locale string `json:"locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		i18n.WriteError(w, r, http.StatusBadRequest, i18n.CodeAuthInvalidJSON, nil)
		return
	}
	body.Locale = strings.ToLower(strings.TrimSpace(body.Locale))
	supported := false
	for _, l := range i18n.SupportedLocales {
		if body.Locale == l {
			supported = true
			break
		}
	}
	if !supported {
		i18n.WriteError(w, r, http.StatusBadRequest, i18n.CodeAdminI18nUnsupportedLocale, nil)
		return
	}
	i18n.SetLangCookie(w, body.Locale)
	w.WriteHeader(http.StatusNoContent)
}

// handleViewer handles /v/{token}: sets the viewer cookie, drops the owner
// cookie if any (so the viewer cannot accidentally inherit owner powers), and
// serves the frontend index.html. When a viewer password is configured, the
// caller must clear the gate first via POST /v/{token}/auth.
func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	sess := s.manager.GetByViewToken(token)
	if sess == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		s.renderViewInvalid(w, r)
		return
	}
	sess.Touch()

	if required, _ := s.viewerPasswordRequired(r.Context()); required {
		if !s.hasValidViewerPasswordCookie(r) {
			s.writeViewerPasswordForm(w, r, token, "")
			return
		}
	}

	clearCookie(w, SessionCookie)
	http.SetCookie(w, &http.Cookie{
		Name:     ViewCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	if s.staticFS != nil {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r2)
		return
	}
	s.handlePlaceholder(w, r)
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// handleFull serves the cap-full page with the list of public sessions.
func (s *Server) handleFull(w http.ResponseWriter, r *http.Request) {
	type sessItem struct {
		ViewToken    string
		Label        string
		AgeMinutes   int
		ViewersCount int
	}
	full, _ := s.manager.Count()
	shared := s.manager.PublicShared()
	items := make([]sessItem, 0, len(shared))
	now := time.Now()
	for _, sess := range shared {
		label := sess.Label()
		if label == "" {
			label = "Sesión " + sess.ID[:6]
		}
		items = append(items, sessItem{
			ViewToken:    sess.ViewToken(),
			Label:        label,
			AgeMinutes:   int(now.Sub(sess.CreatedAt).Minutes()),
			ViewersCount: 0,
		})
	}
	data := map[string]any{
		"Full":      full,
		"MaxPublic": s.manager.MaxPublic(),
		"Sessions":  items,
		"Lang":      i18n.ResolveLang(r),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.fullTmpl.Execute(w, data); err != nil {
		s.logger.Warn("full template execute failed", "err", err)
	}
}

func parseHexOrDec(s string) (uint32, error) {
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		var v uint32
		_, err := fmt.Sscanf(s[2:], "%x", &v)
		return v, err
	}
	var v uint32
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func (s *Server) handlePlaceholder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{"Lang": i18n.ResolveLang(r)}
	if err := placeholderTmpl.Execute(w, data); err != nil {
		s.logger.Warn("placeholder template execute failed", "err", err)
	}
}

// renderViewInvalid serves the 404 HTML for unknown viewer tokens. Lives on
// the Server so callers can hand the request through and the template
// resolves the locale from the cookie/headers.
func (s *Server) renderViewInvalid(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Lang": i18n.ResolveLang(r)}
	if err := viewInvalidTmpl.Execute(w, data); err != nil {
		s.logger.Warn("view_invalid template execute failed", "err", err)
	}
}

var (
	placeholderTmpl = mustParseTemplate("placeholder", "templates/placeholder.html")
	viewInvalidTmpl = mustParseTemplate("view_invalid", "templates/view_invalid.html")
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ListenAndServe brings the HTTP server up on addr and blocks until it
// shuts down.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := &http.Server{
		Addr:              addr,
		Handler:           WithClientCtx([]byte(os.Getenv("SESSION_SECRET")), CloudflareGeoResolver{}, RevealIPEnabled())(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	s.logger.Info("server listening", "url", "http://"+addr)
	err := srv.ListenAndServe()
	s.Close()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Close() {
	if s.usage != nil {
		s.usage.Close()
	}
}
