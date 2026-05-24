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
// v4: users.deleted_at column + email/student_id uniqueness via partial
//     indexes (so a soft-deleted row releases its identifiers).
// v5: rag_chunks, rag_query_cache, rag_answer_cache for the tutor's RAG
//     pipeline. All three are additive; no programmatic step needed beyond
//     bumping user_version.
// v6: tutor_metrics — one row per Ask() with latency, tools used, cache
//     hits. Drives the admin dashboard.
const SchemaVersion = 6

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

	// v3 → v4: rebuild the users table to drop the inline UNIQUE constraints on
	// email/student_id (so soft-deleted rows can release their identifiers via
	// partial indexes) and add the deleted_at column. Detection key is the
	// deleted_at column: present means v4 already applied.
	if has, err := columnExists(ctx, tx, "users", "deleted_at"); err != nil {
		return fmt.Errorf("store: probe users.deleted_at: %w", err)
	} else if !has {
		// defer_foreign_keys postpones FK validation to COMMIT so we can DROP
		// the users table while sessions.user_id still references it; the
		// rename below restores the reference before the transaction ends.
		if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
			return fmt.Errorf("store: defer foreign keys: %w", err)
		}
		rebuild := `
			CREATE TABLE users_v4 (
				id            TEXT PRIMARY KEY,
				name          TEXT NOT NULL,
				email         TEXT,
				student_id    TEXT,
				role          TEXT NOT NULL DEFAULT 'student',
				created_at    INTEGER NOT NULL,
				last_login_at INTEGER NOT NULL,
				deleted_at    INTEGER
			);
			INSERT INTO users_v4(id, name, email, student_id, role, created_at, last_login_at)
				SELECT id, name, email, student_id, COALESCE(NULLIF(role, ''), 'student'),
				       created_at, last_login_at FROM users;
			DROP TABLE users;
			ALTER TABLE users_v4 RENAME TO users;
		`
		if _, err := tx.ExecContext(ctx, rebuild); err != nil {
			return fmt.Errorf("store: rebuild users v3→v4: %w", err)
		}
	}

	// Indexes come AFTER the column-add / table rebuild so old databases
	// bumping from v2 or v3 do not try to index columns that do not exist yet.
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_users_role ON users(role)`); err != nil {
		return fmt.Errorf("store: create idx_users_role: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_active
			ON users(email) WHERE email IS NOT NULL AND deleted_at IS NULL`); err != nil {
		return fmt.Errorf("store: create idx_users_email_active: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_student_id_active
			ON users(student_id) WHERE student_id IS NOT NULL AND deleted_at IS NULL`); err != nil {
		return fmt.Errorf("store: create idx_users_student_id_active: %w", err)
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
