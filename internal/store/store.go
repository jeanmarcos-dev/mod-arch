package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// SchemaVersion is the current schema version stored via PRAGMA user_version.
// Bump only when an incompatible migration is added.
//
// v3: users.role column ('student' | 'moderator' | 'admin').
const SchemaVersion = 3

// Store is the database handle. Methods on Store are safe for concurrent use:
// SQLite serializes writes internally and the connection pool handles reads.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
// Pass ":memory:" for an ephemeral database (useful in tests).
//
// Foreign keys are enabled per-connection via a connection initializer; the
// busy timeout is set to 5 seconds so concurrent writers wait briefly instead
// of returning SQLITE_BUSY immediately.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("store: mkdir parent: %w", err)
		}
	}
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// Single connection limit for :memory: keeps state visible across queries.
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB returns the underlying *sql.DB. Reserved for tests and rare cases where
// callers need to run ad-hoc queries; production code should prefer the typed
// methods on Store.
func (s *Store) DB() *sql.DB { return s.db }

// migrate applies the embedded schema and bumps user_version when needed.
func (s *Store) migrate(ctx context.Context) error {
	var current int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("store: read user_version: %w", err)
	}
	if current == SchemaVersion {
		return nil
	}
	if current > SchemaVersion {
		return fmt.Errorf("store: database schema version %d is newer than binary (%d)", current, SchemaVersion)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store: apply schema: %w", err)
	}

	// Additive column migrations for upgrade paths. CREATE TABLE IF NOT EXISTS
	// above is a no-op for pre-existing tables, so we add missing columns
	// explicitly. Each step checks `columnExists` to stay idempotent across
	// repeated upgrades and re-runs.
	if has, err := columnExists(ctx, tx, "users", "role"); err != nil {
		return fmt.Errorf("store: probe users.role: %w", err)
	} else if !has {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'student'`); err != nil {
			return fmt.Errorf("store: add users.role: %w", err)
		}
	}
	// Index creation comes AFTER the column-add so old databases bumping from
	// v2 to v3 do not try to index a column that does not exist yet.
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_users_role ON users(role)`); err != nil {
		return fmt.Errorf("store: create idx_users_role: %w", err)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return fmt.Errorf("store: bump user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration: %w", err)
	}
	return nil
}

// columnExists reports whether `table` already has a column named `column`.
// Uses PRAGMA table_info which returns one row per column; iterating is the
// canonical SQLite-portable way to introspect schema during migrations.
func columnExists(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// DefaultPath returns the canonical on-disk location for the database. It
// honors MODOS_STATE_DB, falling back to "$XDG_STATE_HOME/modos-cpu/state.db"
// and then to "~/.local/state/modos-cpu/state.db".
func DefaultPath() (string, error) {
	if p := os.Getenv("MODOS_STATE_DB"); p != "" {
		return p, nil
	}
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "modos-cpu", "state.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("store: cannot determine home directory; set MODOS_STATE_DB")
	}
	return filepath.Join(home, ".local", "state", "modos-cpu", "state.db"), nil
}
