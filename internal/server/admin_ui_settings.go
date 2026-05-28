package server

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/jeanmarcos/modos-cpu/internal/programs"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// demoCatalogEntry is the slim shape the admin SPA needs to render the
// "disable demos" toggle table — name, description and optional category.
// The runtime payload (bytes, source maps) is intentionally omitted so the
// transfer stays under a couple of KB.
type demoCatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
}

// handleAdminListDemos returns the static catalog of demos so the admin can
// pick which ones to disable. The endpoint is read-only and lives on the
// moderator surface; admin still owns the writes to disabled_demos.
func (s *Server) handleAdminListDemos(w http.ResponseWriter, r *http.Request) {
	all := programs.Demos()
	out := make([]demoCatalogEntry, len(all))
	for i, d := range all {
		out[i] = demoCatalogEntry{Name: d.Name, Description: d.Description, Category: d.Category}
	}
	writeJSON(w, http.StatusOK, out)
}

// Setting keys for the admin-curated UI surface.
const (
	SettingUIDisabledRoutes = "ui.disabled_routes"
	SettingUIDisabledPanels = "ui.disabled_panels"
	SettingUIDisabledDemos  = "ui.disabled_demos"
	SettingUIDefaultRoute   = "ui.default_route"
)

// DefaultRouteName is the post-login landing route when the admin has not
// chosen one explicitly.
const DefaultRouteName = "overview"

// uiValidRouteNames mirrors the Vue Router `name` attribute of every
// student-facing route in web/src/router/index.ts. Keeping the list here as
// the source of truth lets the admin endpoint reject typos before they reach
// SQLite.
var uiValidRouteNames = []string{
	"overview",
	"processes",
	"memory",
	"interrupts",
	"concurrency",
	"x86-real",
	"glossary",
	"preferences",
	"profile",
}

// uiValidPanelIDs lists the panel slots inside Simulator.vue's layout. They
// match the keys of the `visible` computed in the SPA so admin toggles map
// 1:1 onto what the user sees.
var uiValidPanelIDs = []string{
	"cpu",
	"code",
	"events",
	"memory",
	"procs",
	"ipc",
	"console",
}

// isValidRouteName returns true if name is one of the known SPA routes.
func isValidRouteName(name string) bool {
	return slices.Contains(uiValidRouteNames, name)
}

// isValidPanelID returns true if id matches one of the simulator panel slots.
func isValidPanelID(id string) bool {
	return slices.Contains(uiValidPanelIDs, id)
}

// isValidDemoName returns true if name matches a demo registered in the
// programs package. Built on every call because the demo catalog is static
// and small (~48 entries).
func isValidDemoName(name string) bool {
	return slices.Contains(programs.DemoNames(), name)
}

// parseStringArray decodes a JSON array of strings, returning an empty slice
// for empty input. Anything else (object, number, mixed types) is an error.
func parseStringArray(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// readStringArraySetting loads a setting that holds a JSON array of strings.
// Missing keys and decode failures both yield an empty slice so the SPA gets
// a stable shape even after a bad write.
func readStringArraySetting(ctx context.Context, st *store.Store, key string) []string {
	if st == nil {
		return []string{}
	}
	raw, err := st.GetSetting(ctx, key)
	if err != nil || raw == "" {
		return []string{}
	}
	out, err := parseStringArray(raw)
	if err != nil {
		return []string{}
	}
	return out
}

// readDefaultRoute loads the configured default route, falling back to the
// canonical landing route when the setting is missing or invalid.
func readDefaultRoute(ctx context.Context, st *store.Store) string {
	if st == nil {
		return DefaultRouteName
	}
	v, err := st.GetSetting(ctx, SettingUIDefaultRoute)
	if err != nil || v == "" || !isValidRouteName(v) {
		return DefaultRouteName
	}
	return v
}

// demoBlockedForSession returns true when the demo identified by name is in
// the disabled-demos list AND the user behind the session is a student (or
// unauthenticated). Admins and moderators bypass the gate.
func (s *Server) demoBlockedForSession(sess *sessions.Session, name string) bool {
	if name == "" {
		return false
	}
	st := s.manager.Store()
	if st == nil {
		return false
	}
	disabled := readStringArraySetting(context.Background(), st, SettingUIDisabledDemos)
	if !slices.Contains(disabled, name) {
		return false
	}
	uid := sess.UserID()
	if uid == "" {
		return true
	}
	u, err := st.GetUser(context.Background(), uid)
	if err != nil {
		return true
	}
	role := u.Role
	if role == store.RoleAdmin || role == store.RoleModerator {
		return false
	}
	return true
}

// validateStringArrayAgainst checks every element of raw against the given
// validator. Returns (parsed slice, ok, offendingValue). Duplicates are
// allowed but collapsed.
func validateStringArrayAgainst(raw string, valid func(string) bool) ([]string, bool, string) {
	arr, err := parseStringArray(raw)
	if err != nil {
		return nil, false, ""
	}
	seen := make(map[string]struct{}, len(arr))
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if !valid(v) {
			return nil, false, v
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, true, ""
}

