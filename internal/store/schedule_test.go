package store

import (
	"errors"
	"testing"
)

func mustCreateRule(t *testing.T, s *Store, r ScheduleRule) {
	t.Helper()
	if err := s.CreateRule(t.Context(), r); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
}

func TestScheduleRule_CreateListRoundTrip(t *testing.T) {
	s := openMem(t)
	end := int64(2000)
	mustCreateRule(t, s, ScheduleRule{
		ID: "r1", Kind: "demo", Target: "deadlock", Action: "show",
		StartsAt: 1000, EndsAt: &end, Timezone: "America/Caracas", Enabled: true,
		CreatedAt: 100, CreatedBy: "admin:test", UpdatedAt: 100,
	})
	rules, err := s.ListRules(t.Context())
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("len=%d, want 1", len(rules))
	}
	got := rules[0]
	if got.Kind != "demo" || got.Target != "deadlock" || got.Action != "show" {
		t.Errorf("unexpected rule: %+v", got)
	}
	if got.EndsAt == nil || *got.EndsAt != 2000 {
		t.Errorf("EndsAt=%v, want 2000", got.EndsAt)
	}
	if got.Timezone != "America/Caracas" || !got.Enabled || got.CreatedBy != "admin:test" {
		t.Errorf("metadata lost: %+v", got)
	}
}

func TestScheduleRule_OpenEndedWindow(t *testing.T) {
	s := openMem(t)
	mustCreateRule(t, s, ScheduleRule{
		ID: "r1", Kind: "panel", Target: "ipc", Action: "hide",
		StartsAt: 500, EndsAt: nil, Timezone: "UTC", Enabled: true,
		CreatedAt: 1, UpdatedAt: 1,
	})
	got, err := s.GetRule(t.Context(), "r1")
	if err != nil {
		t.Fatalf("GetRule: %v", err)
	}
	if got.EndsAt != nil {
		t.Errorf("EndsAt=%v, want nil (open-ended)", *got.EndsAt)
	}
}

func TestScheduleRule_UpdateRoundTrip(t *testing.T) {
	s := openMem(t)
	mustCreateRule(t, s, ScheduleRule{
		ID: "r1", Kind: "route", Target: "concurrency", Action: "show",
		StartsAt: 100, Timezone: "UTC", Enabled: true, CreatedAt: 1, UpdatedAt: 1,
	})
	err := s.UpdateRule(t.Context(), ScheduleRule{
		ID: "r1", Kind: "route", Target: "concurrency", Action: "hide",
		StartsAt: 200, Timezone: "UTC", Enabled: false, UpdatedAt: 2,
	})
	if err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	got, _ := s.GetRule(t.Context(), "r1")
	if got.Action != "hide" || got.StartsAt != 200 || got.Enabled {
		t.Errorf("update not applied: %+v", got)
	}
}

func TestScheduleRule_UpdateMissing(t *testing.T) {
	s := openMem(t)
	err := s.UpdateRule(t.Context(), ScheduleRule{ID: "nope", Kind: "demo", Action: "show", Timezone: "UTC"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err=%v, want ErrNotFound", err)
	}
}

func TestScheduleRule_DeleteIdempotent(t *testing.T) {
	s := openMem(t)
	mustCreateRule(t, s, ScheduleRule{ID: "r1", Kind: "demo", Target: "x", Action: "show", Timezone: "UTC", CreatedAt: 1, UpdatedAt: 1})
	if err := s.DeleteRule(t.Context(), "r1"); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	// Second delete is a no-op, not an error.
	if err := s.DeleteRule(t.Context(), "r1"); err != nil {
		t.Errorf("second DeleteRule: %v", err)
	}
	if _, err := s.GetRule(t.Context(), "r1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete err=%v, want ErrNotFound", err)
	}
}

func TestScheduleRule_ListSortedByStart(t *testing.T) {
	s := openMem(t)
	mustCreateRule(t, s, ScheduleRule{ID: "late", Kind: "demo", Target: "a", Action: "show", StartsAt: 3000, Timezone: "UTC", CreatedAt: 1, UpdatedAt: 1})
	mustCreateRule(t, s, ScheduleRule{ID: "early", Kind: "demo", Target: "b", Action: "show", StartsAt: 1000, Timezone: "UTC", CreatedAt: 1, UpdatedAt: 1})
	rules, _ := s.ListRules(t.Context())
	if len(rules) != 2 || rules[0].ID != "early" || rules[1].ID != "late" {
		t.Errorf("not sorted by starts_at asc: %+v", rules)
	}
}
