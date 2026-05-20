package store

import (
	"context"
	"database/sql"
	"fmt"
)

// AuditEntry is a single row in the admin audit log.
type AuditEntry struct {
	ID       int64
	TS       int64
	Actor    string
	Action   string
	Target   string
	MetaJSON string
}

// AppendAudit inserts a new audit row.
func (s *Store) AppendAudit(ctx context.Context, e AuditEntry) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO audit_log(ts, actor, action, target, meta_json)
        VALUES (?, ?, ?, ?, ?)`,
		e.TS, e.Actor, e.Action, nullableString(e.Target), nullableString(e.MetaJSON))
	if err != nil {
		return fmt.Errorf("store: append audit: %w", err)
	}
	return nil
}

// ListAudit returns the most recent entries (newest first), up to limit.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ts, actor, action, target, meta_json
         FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list audit: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]AuditEntry, 0, limit)
	for rows.Next() {
		var (
			e      AuditEntry
			target sql.NullString
			meta   sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.Action, &target, &meta); err != nil {
			return nil, err
		}
		e.Target = target.String
		e.MetaJSON = meta.String
		out = append(out, e)
	}
	return out, rows.Err()
}
