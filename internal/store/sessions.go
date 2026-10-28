package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound is returned by queries that match no row.
var ErrNotFound = errors.New("store: not found")

// SessionRow is the persistent state of a Session. The runtime *sim.Simulator
// is intentionally NOT persisted: its memory and register state are volatile
// by design. What survives a restart is identity (id, mode), display state
// (label, public flag, view_token), timestamps and client tracking metadata
// captured at session creation.
type SessionRow struct {
	ID             string
	Mode           string
	Label          string
	Public         bool
	ViewToken      string // empty when share is disabled
	UserID         string // empty when no user is attached
	CreatedAt      int64  // unix seconds
	LastActiveAt   int64  // unix seconds
	ClientIPHash   string // SHA-256(ip + SESSION_SECRET), truncated hex
	ClientIP       string // raw IP, only populated when ADMIN_REVEAL_IP=true at session create
	UAShort        string // parsed "Browser N / OS" form
	GeoCountry     string // ISO 3166-1 alpha-2 code; empty if geo lookup disabled
	ReferrerHost   string // host portion of HTTP Referer at session create
	OwnerTokenHash string
}

// sessionColumns lists the columns selected by every session read path. Kept
// as a constant so SELECT statements and the scanner stay in lockstep.
const sessionColumns = `id, mode, label, public, view_token, user_id, created_at, last_active_at,
client_ip_hash, client_ip, ua_short, geo_country, referrer_host, owner_token_hash`

// CreateSession inserts a new row. Caller is responsible for setting
// CreatedAt and LastActiveAt to the same value.
func (s *Store) CreateSession(ctx context.Context, r SessionRow) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO sessions(id, mode, label, public, view_token, user_id, created_at, last_active_at,
                             client_ip_hash, client_ip, ua_short, geo_country, referrer_host, owner_token_hash)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Mode, r.Label, boolToInt(r.Public),
		nullableString(r.ViewToken), nullableString(r.UserID),
		r.CreatedAt, r.LastActiveAt,
		nullableString(r.ClientIPHash), nullableString(r.ClientIP),
		nullableString(r.UAShort), nullableString(r.GeoCountry),
		nullableString(r.ReferrerHost), nullableString(r.OwnerTokenHash))
	if err != nil {
		return fmt.Errorf("store: insert session: %w", err)
	}
	return nil
}

// GetSession looks up a row by primary key.
func (s *Store) GetSession(ctx context.Context, id string) (*SessionRow, error) {
	return s.scanSession(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
}

// GetSessionByViewToken resolves a row by its share token (UNIQUE indexed).
func (s *Store) GetSessionByViewToken(ctx context.Context, token string) (*SessionRow, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	return s.scanSession(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE view_token = ?`, token)
}

// ListSessions returns every persisted session. Used at Manager startup to
// rehydrate the in-memory map.
func (s *Store) ListSessions(ctx context.Context) ([]SessionRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions`)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]SessionRow, 0)
	for rows.Next() {
		r, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// ListSessionsByUserID returns every session attached to a given user, newest
// activity first. Used by the student-facing "Mis sesiones" page.
func (s *Store) ListSessionsByUserID(ctx context.Context, userID string) ([]SessionRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE user_id = ? ORDER BY last_active_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions by user: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]SessionRow, 0)
	for rows.Next() {
		r, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// UpdateSessionLabel persists a label change.
func (s *Store) UpdateSessionLabel(ctx context.Context, id, label string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET label = ? WHERE id = ?`, label, id)
	if err != nil {
		return fmt.Errorf("store: update label: %w", err)
	}
	return nil
}

// UpdateSessionPublic persists a public-visibility change.
func (s *Store) UpdateSessionPublic(ctx context.Context, id string, public bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET public = ? WHERE id = ?`, boolToInt(public), id)
	if err != nil {
		return fmt.Errorf("store: update public: %w", err)
	}
	return nil
}

// UpdateSessionViewToken sets or clears the share token. Pass "" to clear.
func (s *Store) UpdateSessionViewToken(ctx context.Context, id, token string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET view_token = ? WHERE id = ?`,
		nullableString(token), id)
	if err != nil {
		return fmt.Errorf("store: update view_token: %w", err)
	}
	return nil
}

// UpdateSessionUserID attaches (or detaches with "") a user to a session.
func (s *Store) UpdateSessionUserID(ctx context.Context, id, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET user_id = ? WHERE id = ?`,
		nullableString(userID), id)
	if err != nil {
		return fmt.Errorf("store: update user_id: %w", err)
	}
	return nil
}

// UpdateSessionLastActive bumps last_active_at. Used by the cleanup pass to
// flush in-memory activity to disk; not called on every Touch to avoid I/O.
func (s *Store) UpdateSessionLastActive(ctx context.Context, id string, ts int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET last_active_at = ? WHERE id = ?`, ts, id)
	if err != nil {
		return fmt.Errorf("store: update last_active: %w", err)
	}
	return nil
}

// DeleteSession removes a row by ID. Idempotent.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

// DeleteIdleSessions removes rows whose last_active_at is older than
// thresholdTs. Returns the count removed.
func (s *Store) DeleteIdleSessions(ctx context.Context, thresholdTs int64) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE last_active_at < ?`, thresholdTs)
	if err != nil {
		return 0, fmt.Errorf("store: delete idle sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CountSessionsByMode returns how many sessions exist per mode.
func (s *Store) CountSessionsByMode(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT mode, COUNT(*) FROM sessions GROUP BY mode`)
	if err != nil {
		return nil, fmt.Errorf("store: count sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int)
	for rows.Next() {
		var mode string
		var n int
		if err := rows.Scan(&mode, &n); err != nil {
			return nil, err
		}
		out[mode] = n
	}
	return out, rows.Err()
}

func (s *Store) scanSession(ctx context.Context, query string, args ...any) (*SessionRow, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	r, err := scanSessionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanSessionRow(sc scanner) (*SessionRow, error) {
	var (
		r            SessionRow
		publicInt    int
		viewToken    sql.NullString
		userID       sql.NullString
		ipHash       sql.NullString
		ipPlain      sql.NullString
		uaShort      sql.NullString
		geoCountry   sql.NullString
		referrerHost sql.NullString
		ownerHash    sql.NullString
	)
	if err := sc.Scan(&r.ID, &r.Mode, &r.Label, &publicInt, &viewToken, &userID,
		&r.CreatedAt, &r.LastActiveAt,
		&ipHash, &ipPlain, &uaShort, &geoCountry, &referrerHost, &ownerHash); err != nil {
		return nil, err
	}
	r.Public = publicInt != 0
	r.ViewToken = viewToken.String
	r.UserID = userID.String
	r.ClientIPHash = ipHash.String
	r.ClientIP = ipPlain.String
	r.UAShort = uaShort.String
	r.GeoCountry = geoCountry.String
	r.ReferrerHost = referrerHost.String
	r.OwnerTokenHash = ownerHash.String
	return &r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullableString returns nil for "" so SQLite stores NULL instead of an empty
// string. This matters for UNIQUE columns (view_token) so multiple sessions
// can coexist without share enabled.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
