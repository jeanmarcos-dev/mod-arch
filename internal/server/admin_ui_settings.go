package server

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jeanmarcos/modos-cpu/internal/programs"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

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

