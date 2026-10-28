package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/tutor"
)

// handleAdminTutorConfigGet returns the persisted tutor config with API
// keys masked. Visible to admins only (not moderators) because it exposes
// which providers/keys are present.
func (s *Server) handleAdminTutorConfigGet(w http.ResponseWriter, r *http.Request) {
	if s.tutorBuild == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tutor build deps no registradas"})
		return
	}
	cfg, err := tutor.LoadConfig(r.Context(), s.tutorBuild.Store, s.tutorBuild.EncKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	merged := tutor.MergeEnv(cfg, os.Getenv)

	resp := map[string]any{
		"config":         merged.Mask(),
		"tutor_running":  s.tutorRegistry != nil && s.tutorRegistry.Get() != nil,
		"corpus_version": readActiveCorpus(r.Context(), s),
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAdminTutorConfigPut applies a partial update to the persisted tutor
// config. Optional `reload=true` query param triggers an in-place rebuild
// so the change takes effect immediately. CSRF-protected via the route
// registration.
func (s *Server) handleAdminTutorConfigPut(w http.ResponseWriter, r *http.Request) {
	if s.tutorBuild == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tutor build deps no registradas"})
		return
	}
	var patch tutor.Update
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "json: " + err.Error()})
		return
	}

	base, err := tutor.LoadConfig(r.Context(), s.tutorBuild.Store, s.tutorBuild.EncKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	updated := base.Apply(patch)

	if err := tutor.SaveConfig(r.Context(), s.tutorBuild.Store, s.tutorBuild.EncKey, updated, "admin"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Default: reload immediately so the admin sees the change live. Pass
	// ?reload=false to skip (useful when chaining several PUTs).
	reload := r.URL.Query().Get("reload") != "false"
	var reloadErr string
	if reload {
		if err := s.reloadTutorFromConfig(r.Context()); err != nil {
			reloadErr = err.Error()
			s.logger.Warn("tutor reload after config PUT failed", "err", err)
		}
	}

	final := tutor.MergeEnv(updated, os.Getenv)
	resp := map[string]any{
		"config":        final.Mask(),
		"tutor_running": s.tutorRegistry != nil && s.tutorRegistry.Get() != nil,
		"reload_error":  reloadErr,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAdminTutorReload rebuilds the tutor service from the currently
// persisted config. Exposed as POST so it shows up in audit log distinct
// from the GET above; CSRF-protected at the route layer.
func (s *Server) handleAdminTutorReload(w http.ResponseWriter, r *http.Request) {
	if err := s.reloadTutorFromConfig(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tutor_running":  s.tutorRegistry != nil && s.tutorRegistry.Get() != nil,
		"corpus_version": readActiveCorpus(r.Context(), s),
	})
}

// reloadTutorFromConfig rebuilds the service using the same loader the
// boot path uses. Centralized here so PUT and explicit /reload share the
// exact same code path.
func (s *Server) reloadTutorFromConfig(ctx context.Context) error {
	if s.tutorBuild == nil || s.tutorRegistry == nil {
		return nil
	}
	cfg, err := tutor.LoadConfig(ctx, s.tutorBuild.Store, s.tutorBuild.EncKey)
	if err != nil {
		return err
	}
	cfg = tutor.MergeEnv(cfg, os.Getenv)
	svc, err := tutor.Build(ctx, *s.tutorBuild, cfg)
	if err != nil {
		s.tutorRegistry.Set(nil)
		return err
	}
	s.tutorRegistry.Set(svc)
	return nil
}

// readActiveCorpus returns the rag.corpus_version currently active.
func readActiveCorpus(ctx context.Context, s *Server) int {
	if s.tutorBuild == nil || s.tutorBuild.Store == nil {
		return 0
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	row := s.tutorBuild.Store.DB().QueryRowContext(cctx, `SELECT value FROM settings WHERE key='rag.corpus_version'`)
	var val string
	if err := row.Scan(&val); err != nil {
		return 0
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return n
}
