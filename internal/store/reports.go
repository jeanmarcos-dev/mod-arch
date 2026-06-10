package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Report kinds and statuses. Kept as constants so callers cannot drift from
// the validated set. The schema does not enforce a CHECK; validation lives in
// the server layer to keep error reporting consistent with the i18n catalog.
const (
	ReportKindFeedback = "feedback"
	ReportKindBug      = "bug"
	ReportKindIdea     = "idea"

	ReportStatusOpen      = "open"
	ReportStatusReviewing = "reviewing"
	ReportStatusResolved  = "resolved"
	ReportStatusDismissed = "dismissed"
)

// UserReport is one row in user_reports. ContextJSON carries the SPA-captured
// state (route, panel, build sha, UA) verbatim so the admin sees exactly what
// the user saw without us having to schematize it.
type UserReport struct {
	ID           int64
	TS           int64
	SessionID    string
	UserID       string
	ClientIPHash string
	Kind         string
	Title        string
	Body         string
	ContextJSON  string
	Status       string
	ResolvedAt   int64
	ResolvedBy   string
}

// InsertReport persists a new report. Returns the assigned ID via the SQLite
// last-insert-rowid mechanism.
func (s *Store) InsertReport(ctx context.Context, r UserReport) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
        INSERT INTO user_reports(ts, session_id, user_id, client_ip_hash,
                                 kind, title, body, context_json, status)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.TS, nullableString(r.SessionID), nullableString(r.UserID),
		nullableString(r.ClientIPHash),
		r.Kind, r.Title, r.Body,
		emptyToObject(r.ContextJSON), r.Status)
	if err != nil {
		return 0, fmt.Errorf("store: insert report: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// emptyToObject ensures an empty context payload is stored as "{}", matching
// the column default. This avoids two flavors of empty (NULL vs ""/{}) in
// downstream code paths.
func emptyToObject(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
}

// ReportFilter narrows ListReports. Zero-valued fields are ignored. Page is
// 1-based; PageSize clamps to [1, 200].
type ReportFilter struct {
	Kind     string
	Status   string
	UserID   string
	Page     int
	PageSize int
}

const reportColumns = `id, ts, session_id, user_id, client_ip_hash,
kind, title, body, context_json, status, resolved_at, resolved_by`

// ListReports returns matching rows newest first plus the unfiltered total.
func (s *Store) ListReports(ctx context.Context, f ReportFilter) ([]UserReport, int, error) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if f.Kind != "" {
		clauses = append(clauses, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, f.Status)
	}
	if f.UserID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, f.UserID)
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	page := max(f.Page, 1)
	size := f.PageSize
	switch {
	case size <= 0:
		size = 50
	case size > 200:
		size = 200
	}
	offset := (page - 1) * size

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_reports"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count reports: %w", err)
	}

	query := "SELECT " + reportColumns + " FROM user_reports" + where +
		" ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, query, append(args, size, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list reports: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]UserReport, 0, size)
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *r)
	}
	return out, total, rows.Err()
}

// GetReport fetches one row by ID. Returns ErrNotFound when missing.
func (s *Store) GetReport(ctx context.Context, id int64) (*UserReport, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+reportColumns+" FROM user_reports WHERE id = ?", id)
	return scanReport(row)
}

// UpdateReportStatus changes status + optionally records who resolved it and
// when. Pass status "resolved" or "dismissed" with a non-zero resolvedAt; for
// "open" or "reviewing" pass 0 to clear those fields.
func (s *Store) UpdateReportStatus(ctx context.Context, id int64, status, resolvedBy string, resolvedAt int64) error {
	var (
		raArg sql.NullInt64
		rbArg sql.NullString
	)
	if resolvedAt > 0 {
		raArg.Valid = true
		raArg.Int64 = resolvedAt
	}
	if resolvedBy != "" {
		rbArg.Valid = true
		rbArg.String = resolvedBy
	}
	_, err := s.db.ExecContext(ctx, `
        UPDATE user_reports SET status = ?, resolved_at = ?, resolved_by = ?
        WHERE id = ?`, status, raArg, rbArg, id)
	if err != nil {
		return fmt.Errorf("store: update report status: %w", err)
	}
	return nil
}

// CountReportsSince returns how many reports a given IP hash submitted after
// `since` (unix seconds). Used by the public POST handler for rate limiting.
// An empty hash returns 0 — anon hashed-IP-less environments cannot be
// rate-limited per-IP, so this is callers' cue to fall back to per-session.
func (s *Store) CountReportsSince(ctx context.Context, ipHash string, since int64) (int, error) {
	if ipHash == "" {
		return 0, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_reports WHERE client_ip_hash = ? AND ts >= ?`,
		ipHash, since).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count reports since: %w", err)
	}
	return n, nil
}

func scanReport(sc scanner) (*UserReport, error) {
	var (
		r          UserReport
		sessionID  sql.NullString
		userID     sql.NullString
		ipHash     sql.NullString
		resolvedAt sql.NullInt64
		resolvedBy sql.NullString
	)
	if err := sc.Scan(&r.ID, &r.TS, &sessionID, &userID, &ipHash,
		&r.Kind, &r.Title, &r.Body, &r.ContextJSON, &r.Status,
		&resolvedAt, &resolvedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	r.SessionID = sessionID.String
	r.UserID = userID.String
	r.ClientIPHash = ipHash.String
	r.ResolvedAt = resolvedAt.Int64
	r.ResolvedBy = resolvedBy.String
	return &r, nil
}
