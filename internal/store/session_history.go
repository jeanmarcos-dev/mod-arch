package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// End reasons stored in session_history.end_reason. Kept as constants so
// callers cannot drift from this set.
const (
	EndReasonIdle       = "idle"
	EndReasonAdminKill  = "admin_kill"
	EndReasonReplaced   = "replaced"
	EndReasonShutdown   = "shutdown"
	EndReasonUserLogout = "user_logout"
)

// SessionHistoryRow is the persistent record of a session that has ended. The
// row is written once, immediately before the live row in `sessions` is
// deleted, so it captures the final state of the session including aggregated
// counters.
type SessionHistoryRow struct {
	ID                int64
	OriginalSessionID string
	UserID            string
	Mode              string
	Label             string
	Public            bool
	ViewToken         string
	CreatedAt         int64
	EndedAt           int64
	DurationSeconds   int64
	EndReason         string
	ClientIPHash      string
	ClientIP          string
	UAShort           string
	GeoCountry        string
	ReferrerHost      string
	TotalCommands     int64
	TotalTutorQueries int64
}

// InsertSessionHistory appends a row to session_history.
func (s *Store) InsertSessionHistory(ctx context.Context, r SessionHistoryRow) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO session_history(
            original_session_id, user_id, mode, label, public, view_token,
            created_at, ended_at, duration_seconds, end_reason,
            client_ip_hash, client_ip, ua_short, geo_country, referrer_host,
            total_commands, total_tutor_queries)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.OriginalSessionID, nullableString(r.UserID), r.Mode, r.Label,
		boolToInt(r.Public), nullableString(r.ViewToken),
		r.CreatedAt, r.EndedAt, r.DurationSeconds, r.EndReason,
		nullableString(r.ClientIPHash), nullableString(r.ClientIP),
		nullableString(r.UAShort), nullableString(r.GeoCountry),
		nullableString(r.ReferrerHost),
		r.TotalCommands, r.TotalTutorQueries)
	if err != nil {
		return fmt.Errorf("store: insert session_history: %w", err)
	}
	return nil
}

// SessionHistoryFilter narrows a session_history listing. Zero-valued fields
// are ignored. Page is 1-based.
type SessionHistoryFilter struct {
	UserID    string
	EndReason string
	Country   string
	From      int64 // ended_at >= From when > 0
	To        int64 // ended_at <= To when > 0
	Page      int
	PageSize  int
}

const historyColumns = `id, original_session_id, user_id, mode, label, public, view_token,
created_at, ended_at, duration_seconds, end_reason,
client_ip_hash, client_ip, ua_short, geo_country, referrer_host,
total_commands, total_tutor_queries`

// ListSessionHistory returns matching rows newest first, plus the unfiltered
// total count for pagination UIs.
func (s *Store) ListSessionHistory(ctx context.Context, f SessionHistoryFilter) ([]SessionHistoryRow, int, error) {
	where, args := historyWhere(f)
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
	countQ := "SELECT COUNT(*) FROM session_history" + where
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count session_history: %w", err)
	}

	query := "SELECT " + historyColumns + " FROM session_history" + where +
		" ORDER BY ended_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, size, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list session_history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SessionHistoryRow, 0, size)
	for rows.Next() {
		r, err := scanSessionHistoryRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *r)
	}
	return out, total, rows.Err()
}

func historyWhere(f SessionHistoryFilter) (string, []any) {
	clauses := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if f.UserID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.EndReason != "" {
		clauses = append(clauses, "end_reason = ?")
		args = append(args, f.EndReason)
	}
	if f.Country != "" {
		clauses = append(clauses, "geo_country = ?")
		args = append(args, f.Country)
	}
	if f.From > 0 {
		clauses = append(clauses, "ended_at >= ?")
		args = append(args, f.From)
	}
	if f.To > 0 {
		clauses = append(clauses, "ended_at <= ?")
		args = append(args, f.To)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// UserActivity is the aggregated activity report for a user over a window.
type UserActivity struct {
	UserID             string
	From               int64
	To                 int64
	TotalSessions      int
	TotalActiveSeconds int64
	AvgSessionMinutes  float64
	LastSeenAt         int64
	LastSeenCountry    string
	TotalCommands      int64
	TotalTutorQueries  int64
	ByDay              []ActivityByDay
	ByCountry          []ActivityByCountry
}

type ActivityByDay struct {
	Day           string // YYYY-MM-DD UTC
	Sessions      int
	ActiveSeconds int64
	TutorQueries  int64
}

type ActivityByCountry struct {
	Country  string
	Sessions int
}

// GetUserActivity aggregates session_history for a given user inside [from, to]
// (unix seconds). Pass from=0 / to=0 for unbounded.
func (s *Store) GetUserActivity(ctx context.Context, userID string, from, to int64) (*UserActivity, error) {
	if userID == "" {
		return nil, ErrNotFound
	}
	act := &UserActivity{UserID: userID, From: from, To: to}

	// Headline aggregates in one round trip.
	var (
		totalSessions sql.NullInt64
		totalSeconds  sql.NullInt64
		totalCommands sql.NullInt64
		totalQueries  sql.NullInt64
		lastSeenAt    sql.NullInt64
		lastSeenGeo   sql.NullString
	)
	q := `SELECT COUNT(*), COALESCE(SUM(duration_seconds), 0),
                 COALESCE(SUM(total_commands), 0), COALESCE(SUM(total_tutor_queries), 0),
                 COALESCE(MAX(ended_at), 0),
                 (SELECT geo_country FROM session_history
                  WHERE user_id = ? ` + rangeClause("ended_at", from, to) + `
                  ORDER BY ended_at DESC LIMIT 1)
          FROM session_history
          WHERE user_id = ?` + rangeClause("ended_at", from, to)
	args := []any{userID}
	if from > 0 {
		args = append(args, from)
	}
	if to > 0 {
		args = append(args, to)
	}
	args = append(args, userID)
	if from > 0 {
		args = append(args, from)
	}
	if to > 0 {
		args = append(args, to)
	}
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(
		&totalSessions, &totalSeconds, &totalCommands, &totalQueries,
		&lastSeenAt, &lastSeenGeo); err != nil {
		return nil, fmt.Errorf("store: aggregate activity: %w", err)
	}
	act.TotalSessions = int(totalSessions.Int64)
	act.TotalActiveSeconds = totalSeconds.Int64
	act.TotalCommands = totalCommands.Int64
	act.TotalTutorQueries = totalQueries.Int64
	act.LastSeenAt = lastSeenAt.Int64
	act.LastSeenCountry = lastSeenGeo.String
	if act.TotalSessions > 0 {
		act.AvgSessionMinutes = float64(act.TotalActiveSeconds) / float64(act.TotalSessions) / 60.0
	}

	// By-day bucketing: SQLite strftime works on unix seconds via 'unixepoch'.
	dayQ := `SELECT strftime('%Y-%m-%d', ended_at, 'unixepoch') AS day,
                    COUNT(*), SUM(duration_seconds), SUM(total_tutor_queries)
             FROM session_history
             WHERE user_id = ?` + rangeClause("ended_at", from, to) + `
             GROUP BY day ORDER BY day`
	dayArgs := []any{userID}
	if from > 0 {
		dayArgs = append(dayArgs, from)
	}
	if to > 0 {
		dayArgs = append(dayArgs, to)
	}
	dayRows, err := s.db.QueryContext(ctx, dayQ, dayArgs...)
	if err != nil {
		return nil, fmt.Errorf("store: activity by day: %w", err)
	}
	defer func() { _ = dayRows.Close() }()
	for dayRows.Next() {
		var d ActivityByDay
		if err := dayRows.Scan(&d.Day, &d.Sessions, &d.ActiveSeconds, &d.TutorQueries); err != nil {
			return nil, err
		}
		act.ByDay = append(act.ByDay, d)
	}
	if err := dayRows.Err(); err != nil {
		return nil, err
	}

	// By-country breakdown (only rows with a non-empty country).
	countryQ := `SELECT geo_country, COUNT(*)
                 FROM session_history
                 WHERE user_id = ? AND geo_country IS NOT NULL AND geo_country <> ''` +
		rangeClause("ended_at", from, to) + `
                 GROUP BY geo_country ORDER BY COUNT(*) DESC`
	countryArgs := []any{userID}
	if from > 0 {
		countryArgs = append(countryArgs, from)
	}
	if to > 0 {
		countryArgs = append(countryArgs, to)
	}
	countryRows, err := s.db.QueryContext(ctx, countryQ, countryArgs...)
	if err != nil {
		return nil, fmt.Errorf("store: activity by country: %w", err)
	}
	defer func() { _ = countryRows.Close() }()
	for countryRows.Next() {
		var c ActivityByCountry
		if err := countryRows.Scan(&c.Country, &c.Sessions); err != nil {
			return nil, err
		}
		act.ByCountry = append(act.ByCountry, c)
	}
	return act, countryRows.Err()
}

// rangeClause builds the AND-prefixed fragment for an inclusive timestamp
// range. Returns "" when both bounds are zero so unbounded queries skip the
// clause entirely.
func rangeClause(col string, from, to int64) string {
	var sb strings.Builder
	if from > 0 {
		fmt.Fprintf(&sb, " AND %s >= ?", col)
	}
	if to > 0 {
		fmt.Fprintf(&sb, " AND %s <= ?", col)
	}
	return sb.String()
}

// PurgeSessionHistoryBefore removes rows whose ended_at is older than cutoff.
// Returns the row count removed.
func (s *Store) PurgeSessionHistoryBefore(ctx context.Context, cutoff int64) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM session_history WHERE ended_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: purge session_history: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanSessionHistoryRow(sc scanner) (*SessionHistoryRow, error) {
	var (
		r            SessionHistoryRow
		userID       sql.NullString
		viewToken    sql.NullString
		ipHash       sql.NullString
		ipPlain      sql.NullString
		uaShort      sql.NullString
		geoCountry   sql.NullString
		referrerHost sql.NullString
		publicInt    int
	)
	if err := sc.Scan(&r.ID, &r.OriginalSessionID, &userID, &r.Mode, &r.Label,
		&publicInt, &viewToken,
		&r.CreatedAt, &r.EndedAt, &r.DurationSeconds, &r.EndReason,
		&ipHash, &ipPlain, &uaShort, &geoCountry, &referrerHost,
		&r.TotalCommands, &r.TotalTutorQueries); err != nil {
		return nil, err
	}
	r.Public = publicInt != 0
	r.UserID = userID.String
	r.ViewToken = viewToken.String
	r.ClientIPHash = ipHash.String
	r.ClientIP = ipPlain.String
	r.UAShort = uaShort.String
	r.GeoCountry = geoCountry.String
	r.ReferrerHost = referrerHost.String
	return &r, nil
}
