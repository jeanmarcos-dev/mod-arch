package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// InviteRow is the persistent state of a single-use admin invite.
type InviteRow struct {
	Token     string
	Label     string
	CreatedAt int64 // unix seconds
	ExpiresAt int64 // unix seconds
	UsedAt    int64 // 0 when pending
}

// CreateInvite inserts a new pending invite.
func (s *Store) CreateInvite(ctx context.Context, r InviteRow) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO invites(token, label, created_at, expires_at, used_at)
        VALUES (?, ?, ?, ?, NULL)`,
		r.Token, r.Label, r.CreatedAt, r.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store: insert invite: %w", err)
	}
	return nil
}

// GetInvite looks up an invite by token (used or not, expired or not).
func (s *Store) GetInvite(ctx context.Context, token string) (*InviteRow, error) {
	var (
		r      InviteRow
		usedAt sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT token, label, created_at, expires_at, used_at
         FROM invites WHERE token = ?`, token).
		Scan(&r.Token, &r.Label, &r.CreatedAt, &r.ExpiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan invite: %w", err)
	}
	r.UsedAt = usedAt.Int64
	return &r, nil
}

// ConsumeInvite atomically marks the invite as used iff it is still pending
// AND not expired. Returns the associated label on success. Returns
// ErrNotFound when no eligible row matched (unknown / already used /
// expired) — the three causes are collapsed so brute-force callers cannot
// distinguish them.
func (s *Store) ConsumeInvite(ctx context.Context, token string, now int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("store: begin consume invite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		label   string
		expires int64
	)
	err = tx.QueryRowContext(ctx,
		`SELECT label, expires_at FROM invites
         WHERE token = ? AND used_at IS NULL`, token).
		Scan(&label, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: select invite: %w", err)
	}
	if now >= expires {
		return "", ErrNotFound
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE invites SET used_at = ? WHERE token = ? AND used_at IS NULL`,
		now, token); err != nil {
		return "", fmt.Errorf("store: mark invite used: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("store: commit consume invite: %w", err)
	}
	return label, nil
}

// RevokeInvite removes an invite by token. Idempotent.
func (s *Store) RevokeInvite(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM invites WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("store: delete invite: %w", err)
	}
	return nil
}

// ListPendingInvites returns the invites that are still consumable: not used
// and not expired. Ordered by created_at descending.
func (s *Store) ListPendingInvites(ctx context.Context, now int64) ([]InviteRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT token, label, created_at, expires_at, used_at
         FROM invites
         WHERE used_at IS NULL AND expires_at > ?
         ORDER BY created_at DESC`, now)
	if err != nil {
		return nil, fmt.Errorf("store: list invites: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]InviteRow, 0)
	for rows.Next() {
		var (
			r      InviteRow
			usedAt sql.NullInt64
		)
		if err := rows.Scan(&r.Token, &r.Label, &r.CreatedAt, &r.ExpiresAt, &usedAt); err != nil {
			return nil, err
		}
		r.UsedAt = usedAt.Int64
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteExpiredInvites prunes rows that are either consumed or past their
// expiration. Returns how many rows were removed.
func (s *Store) DeleteExpiredInvites(ctx context.Context, now int64) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM invites WHERE used_at IS NOT NULL OR expires_at <= ?`, now)
	if err != nil {
		return 0, fmt.Errorf("store: prune invites: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
