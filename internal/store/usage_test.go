package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openUsageTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func countUsage(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM usage_events`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestMigrate_UpgradesFromV10(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	{
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		for _, stmt := range []string{
			`DROP TABLE usage_events`,
			`DROP TABLE tutor_qa`,
			`PRAGMA user_version = 10`,
		} {
			if _, err := s.db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		if _, err := s.db.Exec(
			`INSERT INTO settings(key, value, updated_at) VALUES ('keep', 'me', 1)`); err != nil {
			t.Fatalf("seed: %v", err)
		}
		_ = s.Close()
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade open: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != 11 {
		t.Errorf("user_version = %d, want 11", version)
	}
	var value string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'keep'`).Scan(&value); err != nil || value != "me" {
		t.Errorf("settings row lost: value=%q err=%v", value, err)
	}
	for _, table := range []string{"usage_events", "tutor_qa"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s missing after upgrade (n=%d err=%v)", table, n, err)
		}
	}
}

func TestUsageRecorder_WritesInBatches(t *testing.T) {
	tests := []struct {
		name     string
		events   int
		interval time.Duration
		batch    int
	}{
		{name: "flushes on batch size", events: 10, interval: time.Hour, batch: 5},
		{name: "flushes on interval", events: 3, interval: 10 * time.Millisecond, batch: 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := openUsageTestStore(t)
			r := newUsageRecorder(s, nil, 64, tc.batch, tc.interval)
			dur := int64(1500)
			for i := 0; i < tc.events; i++ {
				r.Record(UsageEvent{SessionID: "s1", UserID: "u1", Kind: UsageKindPageView, Name: "/procesos", DurationMS: &dur})
			}
			deadline := time.Now().Add(2 * time.Second)
			for countUsage(t, s) < tc.events && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			r.Close()
			if got := countUsage(t, s); got != tc.events {
				t.Fatalf("rows = %d, want %d", got, tc.events)
			}
			if r.Dropped() != 0 {
				t.Fatalf("dropped = %d, want 0", r.Dropped())
			}
		})
	}
}

func TestUsageRecorder_CloseFlushesPending(t *testing.T) {
	s := openUsageTestStore(t)
	r := newUsageRecorder(s, nil, 64, 100, time.Hour)
	for i := 0; i < 7; i++ {
		r.Record(UsageEvent{SessionID: "s1", Kind: UsageKindDemoLoad, Name: "counter"})
	}
	r.Close()
	if got := countUsage(t, s); got != 7 {
		t.Fatalf("rows = %d, want 7", got)
	}
	var userID *string
	var role string
	if err := s.db.QueryRow(`SELECT user_id, role FROM usage_events LIMIT 1`).Scan(&userID, &role); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if userID != nil || role != UsageRoleOwner {
		t.Fatalf("user_id = %v role = %q, want NULL and owner", userID, role)
	}
}

func TestUsageRecorder_DropsWhenBufferFull(t *testing.T) {
	s := openUsageTestStore(t)
	r := &UsageRecorder{
		store:  s,
		events: make(chan UsageEvent, 2),
	}
	for i := 0; i < 5; i++ {
		r.Record(UsageEvent{SessionID: "s1", Kind: UsageKindDemoRun})
	}
	if got := r.Dropped(); got != 3 {
		t.Fatalf("dropped = %d, want 3", got)
	}
	if len(r.events) != 2 {
		t.Fatalf("buffered = %d, want 2", len(r.events))
	}
}

func TestTutorQA_RateIsScopedToSession(t *testing.T) {
	s := openUsageTestStore(t)
	ctx := context.Background()
	qa := TutorQA{
		TS: 100, SessionID: "s1", UserID: "u1", RequestID: "r1",
		Question: "¿Qué es un page fault?", Answer: "Es una excepción.",
		CitationsJSON: `["[Glosario: Page fault]"]`, Provider: "gemini", Model: "flash",
	}
	if err := s.InsertTutorQA(ctx, qa); err != nil {
		t.Fatalf("insert: %v", err)
	}

	tests := []struct {
		name       string
		session    string
		request    string
		rating     int
		wantOK     bool
		wantRating *int
	}{
		{name: "other session is ignored", session: "s2", request: "r1", rating: 1, wantOK: false},
		{name: "unknown request is ignored", session: "s1", request: "nope", rating: 1, wantOK: false},
		{name: "owner thumbs up", session: "s1", request: "r1", rating: 1, wantOK: true, wantRating: intPtr(1)},
		{name: "owner switches to thumbs down", session: "s1", request: "r1", rating: -1, wantOK: true, wantRating: intPtr(-1)},
		{name: "owner clears rating", session: "s1", request: "r1", rating: 0, wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := s.RateTutorQA(ctx, tc.session, tc.request, tc.rating, 200)
			if err != nil {
				t.Fatalf("rate: %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			got, err := s.GetTutorQA(ctx, "s1", "r1")
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			switch {
			case tc.wantRating == nil && got.Rating != nil && tc.wantOK:
				t.Fatalf("rating = %d, want NULL", *got.Rating)
			case tc.wantRating != nil && (got.Rating == nil || *got.Rating != *tc.wantRating):
				t.Fatalf("rating = %v, want %d", got.Rating, *tc.wantRating)
			}
		})
	}

	t.Run("untouched row keeps NULL rating", func(t *testing.T) {
		if err := s.InsertTutorQA(ctx, TutorQA{TS: 1, SessionID: "s9", RequestID: "r9", Question: "q"}); err != nil {
			t.Fatalf("insert: %v", err)
		}
		if _, err := s.RateTutorQA(ctx, "other", "r9", 1, 2); err != nil {
			t.Fatalf("rate: %v", err)
		}
		got, err := s.GetTutorQA(ctx, "s9", "r9")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Rating != nil || got.RatedAt != nil {
			t.Fatalf("rating = %v rated_at = %v, want NULL", got.Rating, got.RatedAt)
		}
	})
}

func intPtr(v int) *int { return &v }
