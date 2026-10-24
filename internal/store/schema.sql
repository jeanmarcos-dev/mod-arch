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
    client_ip_hash TEXT,
    client_ip      TEXT,
    ua_short       TEXT,
    geo_country    TEXT,
    referrer_host  TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_view_token  ON sessions(view_token);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id     ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active_at);
-- idx_sessions_country is created programmatically in store.go:migrate after
-- the geo_country ALTER, so v6 databases do not race the index against a
-- column that has not been added yet.

-- Append-only audit of every session that has ended. A row is inserted right
-- before the live row is deleted from `sessions`, capturing aggregated
-- counters and an end_reason. Used by the admin history view, per-user
-- activity reports and retention purges.
CREATE TABLE IF NOT EXISTS session_history (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    original_session_id TEXT    NOT NULL,
    user_id             TEXT,
    mode                TEXT    NOT NULL,
    label               TEXT    NOT NULL DEFAULT '',
    public              INTEGER NOT NULL DEFAULT 0,
    view_token          TEXT,
    created_at          INTEGER NOT NULL,
    ended_at            INTEGER NOT NULL,
    duration_seconds    INTEGER NOT NULL,
    end_reason          TEXT    NOT NULL,
    client_ip_hash      TEXT,
    client_ip           TEXT,
    ua_short            TEXT,
    geo_country         TEXT,
    referrer_host       TEXT,
    total_commands      INTEGER NOT NULL DEFAULT 0,
    total_tutor_queries INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_session_history_ended  ON session_history(ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_history_user   ON session_history(user_id, ended_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_history_reason ON session_history(end_reason);
CREATE INDEX IF NOT EXISTS idx_session_history_geo    ON session_history(geo_country);

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

-- Time-windowed overrides of the admin-curated UI surface. Each rule shows or
-- hides a demo/panel/route (or applies a defaults blob) while the wall clock
-- is inside [starts_at, ends_at). ends_at NULL = open-ended. Times are epoch
-- UTC; timezone records the IANA zone the window was authored in, for display.
CREATE TABLE IF NOT EXISTS schedule_rules (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,            -- demo | panel | route | defaults
    target     TEXT NOT NULL DEFAULT '',
    action     TEXT NOT NULL,            -- show | hide | apply
    starts_at  INTEGER NOT NULL,         -- epoch UTC
    ends_at    INTEGER,                  -- epoch UTC, NULL = open-ended
    timezone   TEXT NOT NULL DEFAULT 'UTC',
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    created_by TEXT,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_schedule_rules_window  ON schedule_rules(starts_at, ends_at);
CREATE INDEX IF NOT EXISTS idx_schedule_rules_enabled ON schedule_rules(enabled);

-- User-submitted reports (feedback, bug, idea). Created via POST /api/reports.
-- session_id and user_id are nullable so anonymous reports are accepted. The
-- admin panel triages by status.
CREATE TABLE IF NOT EXISTS user_reports (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    ts            INTEGER NOT NULL,
    session_id    TEXT,
    user_id       TEXT,
    client_ip_hash TEXT,
    kind          TEXT    NOT NULL,
    title         TEXT    NOT NULL,
    body          TEXT    NOT NULL,
    context_json  TEXT    NOT NULL DEFAULT '{}',
    status        TEXT    NOT NULL DEFAULT 'open',
    resolved_at   INTEGER,
    resolved_by   TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_user_reports_status ON user_reports(status, ts DESC);
CREATE INDEX IF NOT EXISTS idx_user_reports_kind   ON user_reports(kind, ts DESC);
CREATE INDEX IF NOT EXISTS idx_user_reports_user   ON user_reports(user_id, ts DESC);

-- Extended student profile, one row per user. Created lazily on the first
-- PUT /api/profile/me; nullable fields default to empty strings so the SPA
-- can render the form without null guards on every binding.
CREATE TABLE IF NOT EXISTS user_profiles (
    user_id            TEXT PRIMARY KEY,
    cohort             TEXT NOT NULL DEFAULT '',
    institution        TEXT NOT NULL DEFAULT '',
    career             TEXT NOT NULL DEFAULT '',
    bio                TEXT NOT NULL DEFAULT '',
    github             TEXT NOT NULL DEFAULT '',
    linkedin           TEXT NOT NULL DEFAULT '',
    preferred_language TEXT NOT NULL DEFAULT '',
    updated_at         INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_profiles_cohort ON user_profiles(cohort);

-- RAG: corpus indexado (glosario, libro). Embeddings se guardan como BLOB
-- little-endian []float32. corpus_version permite invalidar cachés sin
-- borrar la tabla: el indexador inserta filas nuevas con un version mayor
-- y la app filtra por la versión activa (settings: rag.corpus_version).
CREATE TABLE IF NOT EXISTS rag_chunks (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    source         TEXT NOT NULL,
    source_id      TEXT NOT NULL,
    text           TEXT NOT NULL,
    metadata       TEXT NOT NULL DEFAULT '{}',
    embedding      BLOB NOT NULL,
    corpus_version INTEGER NOT NULL,
    created_at     INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rag_chunks_version ON rag_chunks(corpus_version);
CREATE INDEX IF NOT EXISTS idx_rag_chunks_source  ON rag_chunks(source, corpus_version);

-- Caché determinístico de embeddings de queries. Reduce llamadas al embedder
-- cuando varios estudiantes preguntan lo mismo. Sin TTL: los embeddings son
-- estables para un texto dado.
CREATE TABLE IF NOT EXISTS rag_query_cache (
    query_hash TEXT PRIMARY KEY,
    embedding  BLOB NOT NULL,
    hits_count INTEGER NOT NULL DEFAULT 1,
    last_used  INTEGER NOT NULL
);

-- Caché de respuestas exact-match. La clave incluye el snapshot del simulador
-- para que la misma pregunta en estados distintos no choque. corpus_version
-- en la clave invalida toda la caché cuando se reindexa.
CREATE TABLE IF NOT EXISTS rag_answer_cache (
    cache_key   TEXT PRIMARY KEY,
    answer      TEXT NOT NULL,
    citations   TEXT NOT NULL DEFAULT '[]',
    created_at  INTEGER NOT NULL,
    last_used   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rag_answer_cache_last_used ON rag_answer_cache(last_used);

-- Per-question metrics persisted by the tutor. One row per Ask() call. The
-- admin dashboard aggregates over these to show query volume, latency,
-- cache hit-rate and tool usage. We deliberately keep `question` itself out
-- of the row to avoid storing potentially personal text; the digest lets us
-- correlate repeated questions without keeping the content.
CREATE TABLE IF NOT EXISTS tutor_metrics (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    ts              INTEGER NOT NULL,
    session_id      TEXT,
    user_id         TEXT,
    question_digest TEXT NOT NULL,
    question_chars  INTEGER NOT NULL,
    latency_ms      INTEGER NOT NULL,
    cache_hit       INTEGER NOT NULL DEFAULT 0,
    degraded        INTEGER NOT NULL DEFAULT 0,
    tools_used      TEXT NOT NULL DEFAULT '[]',
    tokens_total    INTEGER NOT NULL DEFAULT 0,
    citations_count INTEGER NOT NULL DEFAULT 0,
    error           TEXT
);

CREATE INDEX IF NOT EXISTS idx_tutor_metrics_ts ON tutor_metrics(ts DESC);
CREATE INDEX IF NOT EXISTS idx_tutor_metrics_session ON tutor_metrics(session_id, ts DESC);

CREATE TABLE IF NOT EXISTS usage_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          INTEGER NOT NULL,
    session_id  TEXT NOT NULL,
    user_id     TEXT,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER,
    role        TEXT NOT NULL DEFAULT 'owner'
);

CREATE INDEX IF NOT EXISTS idx_usage_events_ts   ON usage_events(ts);
CREATE INDEX IF NOT EXISTS idx_usage_events_kind ON usage_events(kind, ts);
CREATE INDEX IF NOT EXISTS idx_usage_events_user ON usage_events(user_id, ts);

CREATE TABLE IF NOT EXISTS tutor_qa (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    ts             INTEGER NOT NULL,
    session_id     TEXT NOT NULL,
    user_id        TEXT,
    request_id     TEXT NOT NULL,
    question       TEXT NOT NULL,
    answer         TEXT NOT NULL DEFAULT '',
    citations_json TEXT NOT NULL DEFAULT '[]',
    degraded       INTEGER NOT NULL DEFAULT 0,
    cache_hit      INTEGER NOT NULL DEFAULT 0,
    retrieved      INTEGER NOT NULL DEFAULT 0,
    latency_ms     INTEGER NOT NULL DEFAULT 0,
    provider       TEXT NOT NULL DEFAULT '',
    model          TEXT NOT NULL DEFAULT '',
    error          TEXT NOT NULL DEFAULT '',
    rating         INTEGER,
    rated_at       INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tutor_qa_req ON tutor_qa(session_id, request_id);
CREATE INDEX IF NOT EXISTS idx_tutor_qa_ts ON tutor_qa(ts);
