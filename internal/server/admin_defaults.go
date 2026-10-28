package server

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

// Setting keys for the admin-curated default experience and scheduling frame.
const (
	// SettingUIDefaults holds the JSON-encoded UIDefaults a fresh visitor
	// inherits (applied client-side per-field, sticky).
	SettingUIDefaults = "ui.defaults"
	// SettingUIScheduleTZ holds the IANA timezone the admin authors schedule
	// windows in. Display-only; stored windows are epoch UTC.
	SettingUIScheduleTZ = "ui.schedule_tz"
)

// UIDefaults is the admin-curated baseline the SPA seeds onto untouched prefs.
// `Speed` is the simulated clock in Hz. The field names match the wire shape
// the client expects in GET /api/config → defaults.
type UIDefaults struct {
	NarratorsEnabled  bool   `json:"narratorsEnabled"`
	Mode              string `json:"mode"`
	Theme             string `json:"theme"`
	Density           string `json:"density"`
	Speed             int    `json:"speed"`
	NarratorMinTickMs int    `json:"narratorMinTickMs"`
}

// Allowed values mirror the unions in web/src/stores/preferences.ts. Kept here
// as the server-side source of truth so a typo is rejected before it reaches
// SQLite or a student's browser.
var (
	uiValidModes     = []string{"compact", "pedagogic", "projector"}
	uiValidThemes    = []string{"amber", "green", "dark", "accesible"}
	uiValidDensities = []string{"comfortable", "compact"}
)

const (
	uiSpeedMin     = 1
	uiSpeedMax     = 60
	uiTickMsMax    = 5000
	uiDefaultTZ    = "UTC"
)

// defaultUIDefaults mirrors the code defaults in the preferences store so a
// site that never configured ui.defaults still ships the same first-run
// experience the SPA would have used on its own.
func defaultUIDefaults() UIDefaults {
	return UIDefaults{
		NarratorsEnabled:  true,
		Mode:              "pedagogic",
		Theme:             "amber",
		Density:           "comfortable",
		Speed:             2,
		NarratorMinTickMs: 200,
	}
}

// validateUIDefaults parses and validates a defaults blob. Missing fields fall
// back to defaultUIDefaults, so a partial payload still yields a complete,
// valid object. Returns (parsed, ok, offendingValue) — the same contract as
// validateStringArrayAgainst.
func validateUIDefaults(raw string) (UIDefaults, bool, string) {
	base := defaultUIDefaults()
	if raw == "" {
		return base, true, ""
	}
	var in struct {
		NarratorsEnabled  *bool   `json:"narratorsEnabled"`
		Mode              *string `json:"mode"`
		Theme             *string `json:"theme"`
		Density           *string `json:"density"`
		Speed             *int    `json:"speed"`
		NarratorMinTickMs *int    `json:"narratorMinTickMs"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return UIDefaults{}, false, ""
	}
	if in.NarratorsEnabled != nil {
		base.NarratorsEnabled = *in.NarratorsEnabled
	}
	if in.Mode != nil {
		if !slices.Contains(uiValidModes, *in.Mode) {
			return UIDefaults{}, false, *in.Mode
		}
		base.Mode = *in.Mode
	}
	if in.Theme != nil {
		if !slices.Contains(uiValidThemes, *in.Theme) {
			return UIDefaults{}, false, *in.Theme
		}
		base.Theme = *in.Theme
	}
	if in.Density != nil {
		if !slices.Contains(uiValidDensities, *in.Density) {
			return UIDefaults{}, false, *in.Density
		}
		base.Density = *in.Density
	}
	if in.Speed != nil {
		if *in.Speed < uiSpeedMin || *in.Speed > uiSpeedMax {
			return UIDefaults{}, false, strconv.Itoa(*in.Speed)
		}
		base.Speed = *in.Speed
	}
	if in.NarratorMinTickMs != nil {
		if *in.NarratorMinTickMs < 0 || *in.NarratorMinTickMs > uiTickMsMax {
			return UIDefaults{}, false, strconv.Itoa(*in.NarratorMinTickMs)
		}
		base.NarratorMinTickMs = *in.NarratorMinTickMs
	}
	return base, true, ""
}

// readUIDefaults loads the stored defaults, falling back to defaultUIDefaults
// when missing or invalid.
func readUIDefaults(ctx context.Context, st *store.Store) UIDefaults {
	if st == nil {
		return defaultUIDefaults()
	}
	raw, err := st.GetSetting(ctx, SettingUIDefaults)
	if err != nil || raw == "" {
		return defaultUIDefaults()
	}
	d, ok, _ := validateUIDefaults(raw)
	if !ok {
		return defaultUIDefaults()
	}
	return d
}

// readScheduleTZ loads the configured authoring timezone, falling back to UTC
// when missing or invalid.
func readScheduleTZ(ctx context.Context, st *store.Store) string {
	if st == nil {
		return uiDefaultTZ
	}
	v, err := st.GetSetting(ctx, SettingUIScheduleTZ)
	if err != nil || v == "" {
		return uiDefaultTZ
	}
	if _, err := time.LoadLocation(v); err != nil {
		return uiDefaultTZ
	}
	return v
}

// applyUIDefaults validates and persists the ui.defaults blob. An empty value
// resets to the built-in defaults by deleting the row.
func applyUIDefaults(r *http.Request, st *store.Store, value, actor string, now int64) (int, string) {
	if value == "" {
		if err := st.DeleteSetting(r.Context(), SettingUIDefaults); err != nil {
			return http.StatusInternalServerError, "no se pudo guardar"
		}
		return http.StatusOK, ""
	}
	parsed, ok, offender := validateUIDefaults(value)
	if !ok {
		if offender != "" {
			return http.StatusBadRequest, "valor inválido en defaults: " + offender
		}
		return http.StatusBadRequest, "defaults inválidos: se esperaba un objeto JSON"
	}
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return http.StatusInternalServerError, "no se pudo serializar"
	}
	return persistSetting(r, st, SettingUIDefaults, string(normalized), actor, now)
}

// applyScheduleTZ validates an IANA timezone and persists it. Empty resets to
// UTC by deleting the row.
func applyScheduleTZ(r *http.Request, st *store.Store, value, actor string, now int64) (int, string) {
	if value == "" {
		if err := st.DeleteSetting(r.Context(), SettingUIScheduleTZ); err != nil {
			return http.StatusInternalServerError, "no se pudo guardar"
		}
		return http.StatusOK, ""
	}
	if _, err := time.LoadLocation(value); err != nil {
		return http.StatusBadRequest, "zona horaria inválida: " + value
	}
	return persistSetting(r, st, SettingUIScheduleTZ, value, actor, now)
}
