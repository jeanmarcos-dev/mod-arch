-- Schema for modos-cpu persistence layer.
--
-- All timestamps are unix epoch seconds stored as INTEGER. Strings are TEXT.
-- The schema version is tracked via PRAGMA user_version; bumping requires a
-- corresponding migration block in store.go.

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    email         TEXT,
    student_id    TEXT,
    role          TEXT NOT NULL DEFAULT 'student',
    created_at    INTEGER NOT NULL,
    last_login_at INTEGER NOT NULL,
    deleted_at    INTEGER
);

-- email and student_id uniqueness is enforced via partial indexes scoped to
-- active rows (deleted_at IS NULL). This lets a soft-deleted user release its
-- identifiers for reuse. Indexes are created programmatically in
-- store.go:migrate so the v2→v3→v4 upgrade path stays additive without
-- racing against columns that an older database may not yet have.

CREATE TABLE IF NOT EXISTS sessions (
    id             TEXT PRIMARY KEY,
    mode           TEXT NOT NULL,
    label          TEXT NOT NULL DEFAULT '',
    public         INTEGER NOT NULL DEFAULT 0,
    view_token     TEXT UNIQUE,
    user_id        TEXT,
    created_at     INTEGER NOT NULL,
    last_active_at INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_view_token ON sessions(view_token);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id    ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active_at);

CREATE TABLE IF NOT EXISTS invites (
    token      TEXT PRIMARY KEY,
    label      TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_invites_expires_at ON invites(expires_at);

CREATE TABLE IF NOT EXISTS magic_links (
    token      TEXT PRIMARY KEY,
    email      TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_magic_links_email   ON magic_links(email);
CREATE INDEX IF NOT EXISTS idx_magic_links_expires ON magic_links(expires_at);

CREATE TABLE IF NOT EXISTS audit_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        INTEGER NOT NULL,
    actor     TEXT    NOT NULL,
    action    TEXT    NOT NULL,
    target    TEXT,
    meta_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_log_ts ON audit_log(ts DESC);

CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    updated_by TEXT
);
