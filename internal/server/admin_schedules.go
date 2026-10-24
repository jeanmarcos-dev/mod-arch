package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// EffectiveUI is the UI surface after the admin base settings are combined with
// every schedule rule active at a given instant. It is computed on-read so
// there is no materialized state to drift out of sync with the clock.
type EffectiveUI struct {
	DisabledRoutes []string
	DisabledPanels []string
	DisabledDemos  []string
	DefaultRoute   string
	Defaults       UIDefaults
}

// effectiveUIConfig folds the active schedule rules onto the base settings:
//   - hide  → add the target to the matching disabled set
//   - show  → remove it (a window can re-enable something disabled by default)
//   - apply → replace the defaults blob (rules are sorted by starts_at, so the
//     latest active window wins)
//
// Windows are half-open [starts_at, ends_at) evaluated against epoch UTC.
func (s *Server) effectiveUIConfig(ctx context.Context, now time.Time) EffectiveUI {
	st := s.manager.Store()
	routes := sliceToSet(readStringArraySetting(ctx, st, SettingUIDisabledRoutes))
	panels := sliceToSet(readStringArraySetting(ctx, st, SettingUIDisabledPanels))
	demos := sliceToSet(readStringArraySetting(ctx, st, SettingUIDisabledDemos))
	defaults := readUIDefaults(ctx, st)
	defRoute := readDefaultRoute(ctx, st)

	if st != nil {
		if rules, err := st.ListRules(ctx); err == nil {
			nowU := now.Unix()
			for _, ru := range rules {
				if !ru.Enabled || nowU < ru.StartsAt {
					continue
				}
				if ru.EndsAt != nil && nowU >= *ru.EndsAt {
					continue
				}
				switch ru.Kind {
				case "route":
					applyShowHide(routes, ru.Action, ru.Target)
				case "panel":
					applyShowHide(panels, ru.Action, ru.Target)
				case "demo":
					applyShowHide(demos, ru.Action, ru.Target)
				case "defaults":
					if d, ok, _ := validateUIDefaults(ru.Target); ok {
						defaults = d
					}
				}
			}
		}
	}

	return EffectiveUI{
		DisabledRoutes: setToSortedSlice(routes),
		DisabledPanels: setToSortedSlice(panels),
		DisabledDemos:  setToSortedSlice(demos),
		DefaultRoute:   defRoute,
		Defaults:       defaults,
	}
}

func applyShowHide(set map[string]struct{}, action, target string) {
	switch action {
	case "hide":
		set[target] = struct{}{}
	case "show":
		delete(set, target)
	}
}

func sliceToSet(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, v := range in {
		out[v] = struct{}{}
	}
	return out
}

func setToSortedSlice(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// scheduleRuleJSON is the wire shape for a rule. Times travel as both epoch UTC
// (the canonical value, used for sorting/status) and a pre-formatted wall-clock
// string in the rule's timezone, so the SPA never has to do IANA math itself.
type scheduleRuleJSON struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	Action      string `json:"action"`
	StartsAt    int64  `json:"starts_at"`
	EndsAt      *int64 `json:"ends_at"`
	StartsLocal string `json:"starts_local"`
	EndsLocal   string `json:"ends_local"`
	Timezone    string `json:"timezone"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// scheduleRuleInput is the create/update body. The window is sent as wall-clock
// strings in Timezone; the server converts to epoch UTC via ParseInLocation.
type scheduleRuleInput struct {
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	Action      string `json:"action"`
	StartsLocal string `json:"starts_local"`
	EndsLocal   string `json:"ends_local"`
	Timezone    string `json:"timezone"`
	Enabled     bool   `json:"enabled"`
}

// localLayouts are the accepted wall-clock formats. The HTML datetime-local
// control emits minute precision, but a second-precision value is tolerated.
var localLayouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05"}

func parseLocalTime(s string, loc *time.Location) (int64, bool) {
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.Unix(), true
		}
	}
	return 0, false
}

func formatLocalTime(epoch int64, loc *time.Location) string {
	return time.Unix(epoch, 0).In(loc).Format("2006-01-02T15:04")
}

func ruleToJSON(r store.ScheduleRule) scheduleRuleJSON {
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		loc = time.UTC
	}
	out := scheduleRuleJSON{
		ID:          r.ID,
		Kind:        r.Kind,
		Target:      r.Target,
		Action:      r.Action,
		StartsAt:    r.StartsAt,
		EndsAt:      r.EndsAt,
		StartsLocal: formatLocalTime(r.StartsAt, loc),
		Timezone:    r.Timezone,
		Enabled:     r.Enabled,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	if r.EndsAt != nil {
		out.EndsLocal = formatLocalTime(*r.EndsAt, loc)
	}
	return out
}

// validateScheduleRule parses and validates an input body into a store-ready
// rule (without id / timestamps). Returns an HTTP status and message on
// failure. Dispatches per kind, reusing the existing identifier validators.
func validateScheduleRule(in scheduleRuleInput) (store.ScheduleRule, int, string) {
	var out store.ScheduleRule
	if !slices.Contains([]string{"demo", "panel", "route", "defaults"}, in.Kind) {
		return out, http.StatusBadRequest, "tipo de regla desconocido: " + in.Kind
	}
	tz := in.Timezone
	if tz == "" {
		tz = uiDefaultTZ
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return out, http.StatusBadRequest, "zona horaria inválida: " + tz
	}
	startsAt, ok := parseLocalTime(in.StartsLocal, loc)
	if !ok {
		return out, http.StatusBadRequest, "fecha de inicio inválida"
	}
	var endsAt *int64
	if in.EndsLocal != "" {
		e, ok := parseLocalTime(in.EndsLocal, loc)
		if !ok {
			return out, http.StatusBadRequest, "fecha de fin inválida"
		}
		if e <= startsAt {
			return out, http.StatusBadRequest, "la fecha de fin debe ser posterior al inicio"
		}
		endsAt = &e
	}

	target := in.Target
	switch in.Kind {
	case "demo", "panel", "route":
		if in.Action != "show" && in.Action != "hide" {
			return out, http.StatusBadRequest, "acción inválida (show|hide)"
		}
		valid := map[string]func(string) bool{
			"demo": isValidDemoName, "panel": isValidPanelID, "route": isValidRouteName,
		}[in.Kind]
		if !valid(target) {
			return out, http.StatusBadRequest, "identificador desconocido: " + target
		}
	case "defaults":
		if in.Action != "apply" {
			return out, http.StatusBadRequest, "acción inválida para defaults (apply)"
		}
		parsed, okD, offender := validateUIDefaults(target)
		if !okD {
			if offender != "" {
				return out, http.StatusBadRequest, "valor inválido en defaults: " + offender
			}
			return out, http.StatusBadRequest, "defaults inválidos"
		}
		normalized, _ := json.Marshal(parsed)
		target = string(normalized)
	}

	out = store.ScheduleRule{
		Kind:     in.Kind,
		Target:   target,
		Action:   in.Action,
		StartsAt: startsAt,
		EndsAt:   endsAt,
		Timezone: tz,
		Enabled:  in.Enabled,
	}
	return out, http.StatusOK, ""
}

// handleAdminListSchedules returns every rule, newest window first is handled
// client-side; the store returns them sorted by starts_at ascending.
func (s *Server) handleAdminListSchedules(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	rules, err := st.ListRules(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudieron listar las reglas"})
		return
	}
	out := make([]scheduleRuleJSON, 0, len(rules))
	for _, ru := range rules {
		out = append(out, ruleToJSON(ru))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rules":    out,
		"timezone": readScheduleTZ(r.Context(), st),
	})
}

func (s *Server) handleAdminCreateSchedule(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	var in scheduleRuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}
	rule, status, msg := validateScheduleRule(in)
	if status != http.StatusOK {
		writeJSON(w, status, map[string]any{"error": msg})
		return
	}
	now := time.Now().Unix()
	rule.ID = newAuthID()
	rule.CreatedAt = now
	rule.CreatedBy = s.adminActor(r)
	rule.UpdatedAt = now
	if err := st.CreateRule(r.Context(), rule); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo crear la regla"})
		return
	}
	meta, _ := json.Marshal(map[string]any{"kind": rule.Kind, "action": rule.Action, "target": rule.Target})
	s.audit(r, "schedule.create", rule.ID, string(meta))
	writeJSON(w, http.StatusCreated, ruleToJSON(rule))
}

func (s *Server) handleAdminUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	existing, err := st.GetRule(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "regla no encontrada"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo leer la regla"})
		return
	}
	var in scheduleRuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON inválido"})
		return
	}
	rule, status, msg := validateScheduleRule(in)
	if status != http.StatusOK {
		writeJSON(w, status, map[string]any{"error": msg})
		return
	}
	rule.ID = id
	rule.CreatedAt = existing.CreatedAt
	rule.CreatedBy = existing.CreatedBy
	rule.UpdatedAt = time.Now().Unix()
	if err := st.UpdateRule(r.Context(), rule); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "regla no encontrada"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo actualizar la regla"})
		return
	}
	meta, _ := json.Marshal(map[string]any{"kind": rule.Kind, "action": rule.Action, "target": rule.Target})
	s.audit(r, "schedule.update", id, string(meta))
	writeJSON(w, http.StatusOK, ruleToJSON(rule))
}

func (s *Server) handleAdminDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	if err := st.DeleteRule(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "no se pudo eliminar la regla"})
		return
	}
	s.audit(r, "schedule.delete", id, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
