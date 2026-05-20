package store

import (
	"path/filepath"
	"testing"
)

func TestOpen_InMemory_Migrates(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("user_version = %d, want %d", version, SchemaVersion)
	}
}

func TestOpen_FileBacked_PersistsAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	if _, err := s1.db.Exec(
		`INSERT INTO settings(key, value, updated_at) VALUES (?, ?, ?)`,
		"access_mode", "open", 1,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close #1: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("open #2: %v", err)
	}
	defer s2.Close()

	var value string
	if err := s2.db.QueryRow(
		`SELECT value FROM settings WHERE key = ?`, "access_mode",
	).Scan(&value); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if value != "open" {
		t.Errorf("value = %q, want %q", value, "open")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// Calling migrate again on a current schema must be a no-op.
	if err := s.migrate(t.Context()); err != nil {
		t.Errorf("second migrate: %v", err)
	}
}

func TestSchemaTablesPresent(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	want := []string{"users", "sessions", "invites", "magic_links", "audit_log", "settings"}
	for _, table := range want {
		t.Run(table, func(t *testing.T) {
			var n int
			err := s.db.QueryRow(
				`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`,
				table,
			).Scan(&n)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if n != 1 {
				t.Errorf("table %s missing", table)
			}
		})
	}
}
