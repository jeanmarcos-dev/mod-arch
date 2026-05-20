package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MagicLinkRow is a single-use email-based login token. Once consumed, the
// row is kept (with used_at set) so the same token cannot be replayed; the
// cleanup pass prunes rows that are used or expired.
type MagicLinkRow struct {
	Token     string
	Email     string
	CreatedAt int64
	ExpiresAt int64
	UsedAt    int64
}

// CreateMagicLink stores a new pending login token tied to an email.
func (s *Store) CreateMagicLink(ctx context.Context, r MagicLinkRow) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO magic_links(token, email, created_at, expires_at, used_at)
        VALUES (?, ?, ?, ?, NULL)`,
		r.Token, r.Email, r.CreatedAt, r.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store: insert magic link: %w", err)
	}
	return nil
}

// ConsumeMagicLink atomically marks the token as used iff still pending and
// not expired. Returns the email it was issued to. ErrNotFound collapses the
// "unknown / used / expired" cases so callers cannot probe which one.
func (s *Store) ConsumeMagicLink(ctx context.Context, token string, now int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("store: begin consume magic link: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		email   string
		expires int64
	)
	err = tx.QueryRowContext(ctx,
		`SELECT email, expires_at FROM magic_links
         WHERE token = ? AND used_at IS NULL`, token).
		Scan(&email, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: select magic link: %w", err)
	}
	if now >= expires {
		return "", ErrNotFound
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE magic_links SET used_at = ? WHERE token = ? AND used_at IS NULL`,
		now, token); err != nil {
		return "", fmt.Errorf("store: mark magic link used: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("store: commit consume magic link: %w", err)
	}
	return email, nil
}

// DeleteExpiredMagicLinks prunes rows that are either consumed or past their
// expiration. Returns how many rows were removed.
func (s *Store) DeleteExpiredMagicLinks(ctx context.Context, now int64) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM magic_links WHERE used_at IS NOT NULL OR expires_at <= ?`, now)
	if err != nil {
		return 0, fmt.Errorf("store: prune magic links: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
