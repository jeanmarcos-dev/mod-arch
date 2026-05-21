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

// TestMigrate_UpgradesFromV2 simulates the production scenario the user hit:
// an existing database created at schema v2 (no users.role column) must
// transparently grow the column + index when the binary boots at v3.
//
// Building the v2 baseline by hand avoids depending on archived snapshots:
// we open a fresh file, hand-write the v2 users table without the role
// column, stamp PRAGMA user_version=2, and then reopen the file using the
// real Open() so the migrate() path runs in full.
func TestMigrate_UpgradesFromV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	// Hand-build a v2 baseline: same shape the binary used to ship, no role
	// column, no idx_users_role index.
	{
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		_, _ = s.db.Exec(`DROP TABLE IF EXISTS users`)
		if _, err := s.db.Exec(`
            CREATE TABLE users (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                email TEXT UNIQUE,
                student_id TEXT UNIQUE,
                created_at INTEGER NOT NULL,
                last_login_at INTEGER NOT NULL
            )
        `); err != nil {
			t.Fatalf("seed v2 schema: %v", err)
		}
		if _, err := s.db.Exec(`PRAGMA user_version = 2`); err != nil {
			t.Fatalf("stamp v2: %v", err)
		}
		// Seed a row so we can check the default backfill survived.
		if _, err := s.db.Exec(
			`INSERT INTO users(id, name, email, student_id, created_at, last_login_at) VALUES (?, ?, NULL, NULL, ?, ?)`,
			"u-legacy", "Pre-v3 user", 1, 1,
		); err != nil {
			t.Fatalf("seed row: %v", err)
		}
		_ = s.Close()
	}

	// Reopen — this is what failed in production until the migration order
	// was fixed (CREATE INDEX raced against the missing column).
	s, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade open: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("user_version after upgrade = %d, want %d", version, SchemaVersion)
	}

	row, err := s.GetUser(t.Context(), "u-legacy")
	if err != nil {
		t.Fatalf("get legacy user: %v", err)
	}
	if row.Role != RoleStudent {
		t.Errorf("legacy row role = %q, want %q (the column default)", row.Role, RoleStudent)
	}

	// Confirm the index was created so query plans can use it.
	var idxCount int
	if err := s.db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_users_role'`,
	).Scan(&idxCount); err != nil {
		t.Fatalf("probe index: %v", err)
	}
	if idxCount != 1 {
		t.Errorf("idx_users_role missing after upgrade")
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
