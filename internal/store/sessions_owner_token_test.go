package store

import (
	"path/filepath"
	"testing"
)

func TestMigrate_AddsOwnerTokenHashFromV11(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	{
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if err := s.CreateSession(t.Context(), SessionRow{ID: "old", Mode: "full", CreatedAt: 1, LastActiveAt: 1}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		for _, stmt := range []string{
			`ALTER TABLE sessions DROP COLUMN owner_token_hash`,
			`PRAGMA user_version = 11`,
		} {
			if _, err := s.db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		_ = s.Close()
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade open: %v", err)
	}
	defer s.Close()

	row, err := s.GetSession(t.Context(), "old")
	if err != nil {
		t.Fatalf("existing session lost after upgrade: %v", err)
	}
	if row.OwnerTokenHash != "" {
		t.Errorf("OwnerTokenHash = %q, want empty for pre-v12 rows", row.OwnerTokenHash)
	}
	if err := s.CreateSession(t.Context(), SessionRow{ID: "new", Mode: "full", CreatedAt: 2, LastActiveAt: 2, OwnerTokenHash: "abc"}); err != nil {
		t.Fatalf("insert after upgrade: %v", err)
	}
	got, err := s.GetSession(t.Context(), "new")
	if err != nil || got.OwnerTokenHash != "abc" {
		t.Errorf("round trip OwnerTokenHash = %+v err=%v", got, err)
	}
}
