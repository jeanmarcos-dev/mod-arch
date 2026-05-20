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
	"strings"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
)

const (
	// SessionCookie identifies the owner of a session.
	SessionCookie = "mc_sid"
	// ViewCookie identifies a viewer subscribed via /v/<token>.
	ViewCookie = "mc_view"
)

// Server is the multi-tenant HTTP+WS frontend of the simulator.
type Server struct {
	manager  *sessions.Manager
	logger   *slog.Logger
	staticFS fs.FS

	fullTmpl *template.Template
	admin    adminConfig
}

// Config holds the Server construction parameters.
type Config struct {
	Manager  *sessions.Manager
	Logger   *slog.Logger
	StaticFS fs.FS
}

// New builds a Server around a *sessions.Manager.
func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{
		manager:  cfg.Manager,
		logger:   cfg.Logger,
		staticFS: cfg.StaticFS,
		fullTmpl: mustParseTemplate("full", "templates/full.html"),
		admin:    loadAdminConfig(),
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
	mux.HandleFunc("GET /api/programs", s.handlePrograms)
	mux.HandleFunc("GET /api/memory", s.handleMemory)
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.HandleFunc("GET /full", s.handleFull)
	mux.HandleFunc("GET /v/{token}", s.handleViewer)
	mux.HandleFunc("GET /_assets/{name}", s.handleAsset)
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
		if token := r.URL.Query().Get("t"); token != "" {
			if s.consumePrivInvite(w, r, token) {
				return
			}
			// Fall through to normal cookie/cap flow when consume fails.
		}
		if c, err := r.Cookie(ViewCookie); err == nil {
			if sess := s.manager.GetByViewToken(c.Value); sess != nil {
				sess.Touch()
			} else {
				clearCookie(w, ViewCookie)
			}
		} else {
			sess, err := s.getOrCreateSession(w, r)
			if errors.Is(err, sessions.ErrCapReached) {
				s.handleFull(w, r)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = sess
		}
	}
	if s.staticFS != nil {
		s.serveSPA(w, r, path)
		return
	}
	s.handlePlaceholder(w, r)
}

// serveSPA hands the request to the embedded FileServer. HTML-shaped paths
// other than "/" (e.g. /login) are rewritten to "/" so vue-router can take
// over on the client.
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request, path string) {
	if isHTMLPath(path) && path != "/" {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r2)
		return
	}
	http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r)
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
	sess, err := s.manager.Create(sessions.ModePrivileged)
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
// when the public cap is full.
func (s *Server) getOrCreateSession(w http.ResponseWriter, r *http.Request) (*sessions.Session, error) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		if sess := s.manager.Get(c.Value); sess != nil {
			sess.Touch()
			return sess, nil
		}
	}
	sess, err := s.manager.Create(sessions.ModeFull)
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

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	full, priv := s.manager.Count()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"sessions":   full,
		"privileged": priv,
		"max_public": s.manager.MaxPublic(),
	})
}

func (s *Server) handlePrograms(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.manager.Programs())
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

// handleViewer handles /v/{token}: sets the viewer cookie, drops the owner
// cookie if any (so the viewer cannot accidentally inherit owner powers), and
// serves the frontend index.html.
func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	sess := s.manager.GetByViewToken(token)
	if sess == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(readStaticAsset("templates/view_invalid.html"))
		return
	}
	sess.Touch()

	clearCookie(w, SessionCookie)
	http.SetCookie(w, &http.Cookie{
		Name:     ViewCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Serve index.html through a sub-request to "/" so the FileServer is not
	// confused by the /v/<token> path.
	if s.staticFS != nil {
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
func (s *Server) handleFull(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handlePlaceholder(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(readStaticAsset("templates/placeholder.html"))
}

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
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	s.logger.Info("server listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
