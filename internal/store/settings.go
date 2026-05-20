package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetSetting fetches a key/value entry. Returns ErrNotFound when absent.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: get setting %q: %w", key, err)
	}
	return value, nil
}

// SetSetting upserts a settings row.
func (s *Store) SetSetting(ctx context.Context, key, value, updatedBy string, ts int64) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO settings(key, value, updated_at, updated_by)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(key) DO UPDATE SET
            value = excluded.value,
            updated_at = excluded.updated_at,
            updated_by = excluded.updated_by`,
		key, value, ts, nullableString(updatedBy))
	if err != nil {
		return fmt.Errorf("store: set setting %q: %w", key, err)
	}
	return nil
}

// ListSettings returns every key/value pair indexed by key.
func (s *Store) ListSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("store: list settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// DeleteSetting removes a row. Idempotent.
func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("store: delete setting %q: %w", key, err)
	}
	return nil
}
