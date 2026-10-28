package server

import (
	"slices"
	"testing"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/programs"
	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

func TestValidateScheduleRule_DemoShowHide(t *testing.T) {
	demo := programs.DemoNames()[0]
	rule, status, msg := validateScheduleRule(scheduleRuleInput{
		Kind: "demo", Target: demo, Action: "show",
		StartsLocal: "2026-06-12T08:00", EndsLocal: "2026-06-12T10:00",
		Timezone: "America/Caracas", Enabled: true,
	})
	if status != 200 {
		t.Fatalf("status=%d msg=%q", status, msg)
	}
	if rule.StartsAt >= *rule.EndsAt {
		t.Errorf("window not ordered: %d..%d", rule.StartsAt, *rule.EndsAt)
	}
	// America/Caracas is UTC-4: 08:00 local = 12:00 UTC.
	wantStart := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC).Unix()
	if rule.StartsAt != wantStart {
		t.Errorf("StartsAt=%d, want %d (tz applied)", rule.StartsAt, wantStart)
	}
}

func TestValidateScheduleRule_BadEndBeforeStart(t *testing.T) {
	demo := programs.DemoNames()[0]
	_, status, _ := validateScheduleRule(scheduleRuleInput{
		Kind: "demo", Target: demo, Action: "show",
		StartsLocal: "2026-06-12T10:00", EndsLocal: "2026-06-12T09:00",
		Timezone: "UTC", Enabled: true,
	})
	if status != 400 {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestValidateScheduleRule_BadTimezone(t *testing.T) {
	_, status, _ := validateScheduleRule(scheduleRuleInput{
		Kind: "panel", Target: "ipc", Action: "hide",
		StartsLocal: "2026-06-12T10:00", Timezone: "Nowhere/Void",
	})
	if status != 400 {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestValidateScheduleRule_DefaultsApply(t *testing.T) {
	rule, status, msg := validateScheduleRule(scheduleRuleInput{
		Kind: "defaults", Action: "apply",
		Target:      `{"mode":"projector","narratorsEnabled":true}`,
		StartsLocal: "2026-06-12T10:00", Timezone: "UTC", Enabled: true,
	})
	if status != 200 {
		t.Fatalf("status=%d msg=%q", status, msg)
	}
	// Target is normalized to a full UIDefaults blob.
	d, ok, _ := validateUIDefaults(rule.Target)
	if !ok || d.Mode != "projector" {
		t.Errorf("defaults target not normalized: %q", rule.Target)
	}
}

func TestValidateScheduleRule_DefaultsWrongAction(t *testing.T) {
	_, status, _ := validateScheduleRule(scheduleRuleInput{
		Kind: "defaults", Action: "hide", Target: `{}`,
		StartsLocal: "2026-06-12T10:00", Timezone: "UTC",
	})
	if status != 400 {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestEffectiveUIConfig_HideWithinWindow(t *testing.T) {
	s, st := newUITestServer(t)
	start := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC).Unix()
	end := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC).Unix()
	mustCreateServerRule(t, st, store.ScheduleRule{
		ID: "r1", Kind: "panel", Target: "ipc", Action: "hide",
		StartsAt: start, EndsAt: &end, Timezone: "UTC", Enabled: true,
	})
	// Inside the window: ipc is hidden.
	in := s.effectiveUIConfig(t.Context(), time.Unix(start+3600, 0))
	if !slices.Contains(in.DisabledPanels, "ipc") {
		t.Errorf("ipc should be hidden inside window: %v", in.DisabledPanels)
	}
	// Before the window: ipc is visible.
	before := s.effectiveUIConfig(t.Context(), time.Unix(start-3600, 0))
	if slices.Contains(before.DisabledPanels, "ipc") {
		t.Errorf("ipc should be visible before window: %v", before.DisabledPanels)
	}
	// After the window: ipc is visible again.
	after := s.effectiveUIConfig(t.Context(), time.Unix(end+1, 0))
	if slices.Contains(after.DisabledPanels, "ipc") {
		t.Errorf("ipc should be visible after window: %v", after.DisabledPanels)
	}
}

func TestEffectiveUIConfig_ShowOverridesBase(t *testing.T) {
	s, st := newUITestServer(t)
	// Base setting disables ipc permanently.
	_ = st.SetSetting(t.Context(), SettingUIDisabledPanels, `["ipc"]`, "test", 0)
	start := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC).Unix()
	mustCreateServerRule(t, st, store.ScheduleRule{
		ID: "r1", Kind: "panel", Target: "ipc", Action: "show",
		StartsAt: start, EndsAt: nil, Timezone: "UTC", Enabled: true,
	})
	in := s.effectiveUIConfig(t.Context(), time.Unix(start+10, 0))
	if slices.Contains(in.DisabledPanels, "ipc") {
		t.Errorf("active show-rule should re-enable ipc: %v", in.DisabledPanels)
	}
}

func TestEffectiveUIConfig_DefaultsMerge(t *testing.T) {
	s, st := newUITestServer(t)
	start := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC).Unix()
	mustCreateServerRule(t, st, store.ScheduleRule{
		ID: "r1", Kind: "defaults", Action: "apply",
		Target:   `{"mode":"projector","theme":"dark","narratorsEnabled":true,"density":"comfortable","speed":4,"narratorMinTickMs":250}`,
		StartsAt: start, Timezone: "UTC", Enabled: true,
	})
	in := s.effectiveUIConfig(t.Context(), time.Unix(start+10, 0))
	if in.Defaults.Mode != "projector" || in.Defaults.Theme != "dark" {
		t.Errorf("active defaults rule not merged: %+v", in.Defaults)
	}
	// Disabled rule has no effect.
	out := s.effectiveUIConfig(t.Context(), time.Unix(start-10, 0))
	if out.Defaults.Mode == "projector" {
		t.Errorf("defaults applied outside window: %+v", out.Defaults)
	}
}

func TestDemoBlockedForSession_RespectsSchedule(t *testing.T) {
	s, st := newUITestServer(t)
	demo := programs.DemoNames()[0]
	start := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC).Unix()
	// Hide the demo via an open-ended window that is already active (start in
	// the past relative to the wall clock during the test run).
	mustCreateServerRule(t, st, store.ScheduleRule{
		ID: "r1", Kind: "demo", Target: demo, Action: "hide",
		StartsAt: start, EndsAt: nil, Timezone: "UTC", Enabled: true,
	})
	sess, err := s.manager.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !s.demoBlockedForSession(sess, demo) {
		t.Errorf("demo hidden by active schedule must be blocked for an anonymous session")
	}
}

func mustCreateServerRule(t *testing.T, st *store.Store, r store.ScheduleRule) {
	t.Helper()
	if r.CreatedAt == 0 {
		r.CreatedAt = 1
	}
	if r.UpdatedAt == 0 {
		r.UpdatedAt = 1
	}
	if err := st.CreateRule(t.Context(), r); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
}
