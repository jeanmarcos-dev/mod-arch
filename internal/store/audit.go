package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
		e, err := scanAuditEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// AuditFilter narrows ListAuditPaginated. Zero-valued fields are ignored.
// Page is 1-based; PageSize is clamped to [1, 200].
type AuditFilter struct {
	Actor    string // substring match (LIKE %?%)
	Action   string // substring match
	From     int64  // ts >= From when > 0
	To       int64  // ts <= To when > 0
	Page     int
	PageSize int
}

// ListAuditPaginated returns matching audit rows newest first, together with
// the unfiltered total. Used by the admin UI's paginated audit table.
func (s *Store) ListAuditPaginated(ctx context.Context, f AuditFilter) ([]AuditEntry, int, error) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if f.Actor != "" {
		clauses = append(clauses, "actor LIKE ?")
		args = append(args, "%"+f.Actor+"%")
	}
	if f.Action != "" {
		clauses = append(clauses, "action LIKE ?")
		args = append(args, "%"+f.Action+"%")
	}
	if f.From > 0 {
		clauses = append(clauses, "ts >= ?")
		args = append(args, f.From)
	}
	if f.To > 0 {
		clauses = append(clauses, "ts <= ?")
		args = append(args, f.To)
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
		"SELECT COUNT(*) FROM audit_log"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count audit: %w", err)
	}

	query := "SELECT id, ts, actor, action, target, meta_json FROM audit_log" + where +
		" ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, query, append(args, size, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list audit paginated: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]AuditEntry, 0, size)
	for rows.Next() {
		e, err := scanAuditEntry(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *e)
	}
	return out, total, rows.Err()
}

// PurgeAuditLogBefore removes rows older than cutoff. Returns the count removed.
func (s *Store) PurgeAuditLogBefore(ctx context.Context, cutoff int64) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_log WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: purge audit: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanAuditEntry(sc scanner) (*AuditEntry, error) {
	var (
		e      AuditEntry
		target sql.NullString
		meta   sql.NullString
	)
	if err := sc.Scan(&e.ID, &e.TS, &e.Actor, &e.Action, &target, &meta); err != nil {
		return nil, err
	}
	e.Target = target.String
	e.MetaJSON = meta.String
	return &e, nil
}
