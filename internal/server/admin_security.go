package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// maskedSecurityConfig is the safe representation surfaced to the admin UI:
// the secret never travels back over the wire — only a boolean and the last
// few characters of the persisted value (when present) so the admin can tell
// which key is currently active.
type maskedSecurityConfig struct {
	TurnstileSitekey   string `json:"turnstile_sitekey"`
	HasTurnstileSecret bool   `json:"has_turnstile_secret"`
	TurnstileSecretTag string `json:"turnstile_secret_tag,omitempty"`
}

func maskSecurity(cfg SecurityConfig) maskedSecurityConfig {
	out := maskedSecurityConfig{
		TurnstileSitekey:   cfg.TurnstileSitekey,
		HasTurnstileSecret: cfg.TurnstileSecret != "",
	}
	if cfg.TurnstileSecret != "" {
		tag := cfg.TurnstileSecret
		if len(tag) > 4 {
			tag = tag[len(tag)-4:]
		}
		out.TurnstileSecretTag = "••••" + tag
	}
	return out
}

func (s *Server) handleAdminSecurityGet(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	cfg, err := loadSecurityConfig(r.Context(), st, s.encKey)
	if err != nil {
		s.logger.Warn("admin: load security config", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no fue posible leer la configuración"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": maskSecurity(cfg)})
}

func (s *Server) handleAdminSecurityPut(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	var patch SecurityUpdate
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}
	// Trim sitekey/secret to avoid storing accidental whitespace. We keep
	// nil-vs-empty-string distinction so the admin can explicitly clear a
	// value by sending "".
	if patch.TurnstileSitekey != nil {
		trimmed := strings.TrimSpace(*patch.TurnstileSitekey)
		patch.TurnstileSitekey = &trimmed
	}
	if patch.TurnstileSecret != nil {
		trimmed := strings.TrimSpace(*patch.TurnstileSecret)
		patch.TurnstileSecret = &trimmed
	}
	if err := saveSecurityConfig(r.Context(), st, s.encKey, patch, "admin", time.Now().Unix()); err != nil {
		s.logger.Warn("admin: save security config", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no fue posible guardar la configuración"})
		return
	}
	cfg, err := loadSecurityConfig(r.Context(), st, s.encKey)
	if err != nil {
		s.logger.Warn("admin: reload security config", "err", err)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": maskSecurity(cfg)})
}

// handlePublicConfig exposes only the bootstrap settings the SPA needs to
// render its login screen. Anything secret stays server-side. The Turnstile
// sitekey is reported only when the secret is also configured: a half-set
// pair (sitekey without secret, or vice versa) means the verifier is not
// going to enforce anything, so rendering a non-functional widget on the
// login page is worse than showing nothing.
func (s *Server) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	out := map[string]any{
		"turnstile_sitekey": "",
		"disabled_routes":   []string{},
		"disabled_panels":   []string{},
		"disabled_demos":    []string{},
		"default_route":     DefaultRouteName,
	}
	if st != nil {
		if cfg, err := loadSecurityConfig(r.Context(), st, s.encKey); err == nil {
			if cfg.TurnstileSitekey != "" && cfg.TurnstileSecret != "" {
				out["turnstile_sitekey"] = cfg.TurnstileSitekey
			}
		}
		out["disabled_routes"] = readStringArraySetting(r.Context(), st, SettingUIDisabledRoutes)
		out["disabled_panels"] = readStringArraySetting(r.Context(), st, SettingUIDisabledPanels)
		out["disabled_demos"] = readStringArraySetting(r.Context(), st, SettingUIDisabledDemos)
		out["default_route"] = readDefaultRoute(r.Context(), st)
	}
	writeJSON(w, http.StatusOK, out)
}
