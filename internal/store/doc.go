// Package store is the persistence layer for modos-cpu.
//
// It wraps a SQLite database (pure-Go via modernc.org/sqlite, no cgo) and
// exposes typed methods to manage users, sessions, invites, audit log entries
// and arbitrary settings.
//
// # Why SQLite
//
// modos-cpu runs as a single process and typically holds tens of sessions at
// any moment. A separate database server would be operational overhead with no
// concurrency benefit. SQLite gives durable storage, transactional semantics,
// crash recovery and zero-configuration deploys in a single file.
//
// # File location
//
// The default path is "$XDG_STATE_HOME/modos-cpu/state.db" (or
// "~/.local/state/modos-cpu/state.db" when XDG_STATE_HOME is unset). It can
// be overridden by setting MODOS_STATE_DB.
//
// # Schema versioning
//
// The schema is embedded from schema.sql and applied with IF NOT EXISTS
// statements so re-running it on an existing database is a no-op. Future
// non-additive migrations should bump PRAGMA user_version and run inside a
// transaction.
package store
