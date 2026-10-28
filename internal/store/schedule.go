package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ScheduleRule is a time-windowed override of the admin-curated UI surface. A
// rule shows or hides a demo / panel / route — or applies a defaults blob —
// while the wall clock sits inside [StartsAt, EndsAt). EndsAt is nil for an
// open-ended window. StartsAt/EndsAt are epoch UTC (the source of truth);
// Timezone records the IANA zone the admin authored the window in, so the UI
// can render it back in the same frame.
type ScheduleRule struct {
	ID        string
	Kind      string // demo | panel | route | defaults
	Target    string // demo/panel/route id, or a UIDefaults JSON blob for kind=defaults
	Action    string // show | hide | apply
	StartsAt  int64  // epoch UTC
	EndsAt    *int64 // epoch UTC, nil = open-ended
	Timezone  string // IANA name, for display only
	Enabled   bool
	CreatedAt int64
	CreatedBy string
	UpdatedAt int64
}

// ListRules returns every rule ordered by StartsAt ascending so the caller's
// defaults merge is deterministic (later windows win).
func (s *Store) ListRules(ctx context.Context) ([]ScheduleRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, target, action, starts_at, ends_at, timezone, enabled,
		       created_at, created_by, updated_at
		FROM schedule_rules
		ORDER BY starts_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list schedule rules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]ScheduleRule, 0)
	for rows.Next() {
		r, err := scanScheduleRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRule fetches one rule by id. Returns ErrNotFound when absent.
func (s *Store) GetRule(ctx context.Context, id string) (*ScheduleRule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, kind, target, action, starts_at, ends_at, timezone, enabled,
		       created_at, created_by, updated_at
		FROM schedule_rules WHERE id = ?`, id)
	r, err := scanScheduleRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get schedule rule: %w", err)
	}
	return &r, nil
}

// CreateRule inserts a new rule.
func (s *Store) CreateRule(ctx context.Context, r ScheduleRule) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO schedule_rules(id, kind, target, action, starts_at, ends_at,
		                           timezone, enabled, created_at, created_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Kind, r.Target, r.Action, r.StartsAt, nullableInt64(r.EndsAt),
		r.Timezone, boolToInt(r.Enabled), r.CreatedAt, nullableString(r.CreatedBy), r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: insert schedule rule: %w", err)
	}
	return nil
}

// UpdateRule overwrites the mutable fields of an existing rule. Returns
// ErrNotFound when the id does not exist. created_at / created_by are left
// untouched.
func (s *Store) UpdateRule(ctx context.Context, r ScheduleRule) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE schedule_rules
		SET kind = ?, target = ?, action = ?, starts_at = ?, ends_at = ?,
		    timezone = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		r.Kind, r.Target, r.Action, r.StartsAt, nullableInt64(r.EndsAt),
		r.Timezone, boolToInt(r.Enabled), r.UpdatedAt, r.ID)
	if err != nil {
		return fmt.Errorf("store: update schedule rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRule removes a rule by id. Idempotent.
func (s *Store) DeleteRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM schedule_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete schedule rule: %w", err)
	}
	return nil
}

// rowScanner abstracts *sql.Row and *sql.Rows so one scan helper serves both
// the single-row and list paths.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanScheduleRule(sc rowScanner) (ScheduleRule, error) {
	var (
		r         ScheduleRule
		endsAt    sql.NullInt64
		createdBy sql.NullString
		enabled   int64
	)
	if err := sc.Scan(&r.ID, &r.Kind, &r.Target, &r.Action, &r.StartsAt, &endsAt,
		&r.Timezone, &enabled, &r.CreatedAt, &createdBy, &r.UpdatedAt); err != nil {
		return ScheduleRule{}, err
	}
	if endsAt.Valid {
		v := endsAt.Int64
		r.EndsAt = &v
	}
	r.CreatedBy = createdBy.String
	r.Enabled = enabled != 0
	return r, nil
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
