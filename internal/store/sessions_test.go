package store

import (
	"errors"
	"testing"
)

func TestSession_CreateGetUpdateDelete(t *testing.T) {
	s := openMem(t)

	row := SessionRow{
		ID:           "sess-1",
		Mode:         "full",
		Label:        "P1",
		Public:       true,
		CreatedAt:    1000,
		LastActiveAt: 1000,
	}
	if err := s.CreateSession(t.Context(), row); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetSession(t.Context(), "sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Label != "P1" || got.Mode != "full" || !got.Public {
		t.Errorf("unexpected row after insert: %+v", got)
	}
	if got.ViewToken != "" || got.UserID != "" {
		t.Errorf("nullable fields should be empty, got view=%q user=%q", got.ViewToken, got.UserID)
	}

	if err := s.UpdateSessionLabel(t.Context(), "sess-1", "renombrada"); err != nil {
		t.Fatalf("UpdateSessionLabel: %v", err)
	}
	if err := s.UpdateSessionPublic(t.Context(), "sess-1", false); err != nil {
		t.Fatalf("UpdateSessionPublic: %v", err)
	}
	if err := s.UpdateSessionViewToken(t.Context(), "sess-1", "tok-xyz"); err != nil {
		t.Fatalf("UpdateSessionViewToken: %v", err)
	}
	if err := s.UpdateSessionLastActive(t.Context(), "sess-1", 1234); err != nil {
		t.Fatalf("UpdateSessionLastActive: %v", err)
	}

	got, err = s.GetSession(t.Context(), "sess-1")
	if err != nil {
		t.Fatalf("GetSession after updates: %v", err)
	}
	if got.Label != "renombrada" || got.Public || got.ViewToken != "tok-xyz" || got.LastActiveAt != 1234 {
		t.Errorf("updates not reflected: %+v", got)
	}

	byTok, err := s.GetSessionByViewToken(t.Context(), "tok-xyz")
	if err != nil {
		t.Fatalf("GetSessionByViewToken: %v", err)
	}
	if byTok.ID != "sess-1" {
		t.Errorf("by view token resolved wrong session: %s", byTok.ID)
	}

	if err := s.DeleteSession(t.Context(), "sess-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.GetSession(t.Context(), "sess-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSession_ViewTokenUniqueAfterClear(t *testing.T) {
	// A NULL view_token must not collide with another NULL on the UNIQUE
	// index — two sessions without share enabled should coexist.
	s := openMem(t)
	mustInsertSession(t, s, "a", 100)
	mustInsertSession(t, s, "b", 100)
	if err := s.UpdateSessionViewToken(t.Context(), "a", "tok"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSessionViewToken(t.Context(), "a", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSessionViewToken(t.Context(), "b", "tok"); err != nil {
		t.Fatalf("reusing token freed by another session should be allowed: %v", err)
	}
}

func TestSession_ListAndCount(t *testing.T) {
	s := openMem(t)
	mustInsertSession(t, s, "a", 100)
	mustInsertSession(t, s, "b", 200)
	mustInsertSessionMode(t, s, "c", 300, "privileged")

	list, err := s.ListSessions(t.Context())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListSessions len=%d, want 3", len(list))
	}

	counts, err := s.CountSessionsByMode(t.Context())
	if err != nil {
		t.Fatalf("CountSessionsByMode: %v", err)
	}
	if counts["full"] != 2 || counts["privileged"] != 1 {
		t.Errorf("counts = %v, want {full:2 privileged:1}", counts)
	}
}

func TestSession_DeleteIdleSessions(t *testing.T) {
	s := openMem(t)
	mustInsertSession(t, s, "old", 100)
	mustInsertSession(t, s, "old2", 150)
	mustInsertSession(t, s, "fresh", 500)

	n, err := s.DeleteIdleSessions(t.Context(), 200)
	if err != nil {
		t.Fatalf("DeleteIdleSessions: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted %d, want 2", n)
	}
	if _, err := s.GetSession(t.Context(), "fresh"); err != nil {
		t.Errorf("fresh session should survive: %v", err)
	}
}

// openMem opens an in-memory database for the duration of the test.
func openMem(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mustInsertSession(t *testing.T, s *Store, id string, ts int64) {
	t.Helper()
	mustInsertSessionMode(t, s, id, ts, "full")
}

func mustInsertSessionMode(t *testing.T, s *Store, id string, ts int64, mode string) {
	t.Helper()
	err := s.CreateSession(t.Context(), SessionRow{
		ID:           id,
		Mode:         mode,
		CreatedAt:    ts,
		LastActiveAt: ts,
	})
	if err != nil {
		t.Fatalf("CreateSession(%s): %v", id, err)
	}
}
