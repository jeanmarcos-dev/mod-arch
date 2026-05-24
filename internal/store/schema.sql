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
