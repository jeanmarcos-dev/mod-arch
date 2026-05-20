package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// UserRow is the persistent identity of a student. Email and StudentID are
// optional but UNIQUE when present (NULL coexistence is enforced via
// nullableString).
type UserRow struct {
	ID          string
	Name        string
	Email       string
	StudentID   string
	CreatedAt   int64
	LastLoginAt int64
}

// CreateUser inserts a new user row.
func (s *Store) CreateUser(ctx context.Context, r UserRow) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO users(id, name, email, student_id, created_at, last_login_at)
        VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, nullableString(r.Email), nullableString(r.StudentID),
		r.CreatedAt, r.LastLoginAt)
	if err != nil {
		return fmt.Errorf("store: insert user: %w", err)
	}
	return nil
}

// GetUser fetches a user by primary key. Returns ErrNotFound when absent.
func (s *Store) GetUser(ctx context.Context, id string) (*UserRow, error) {
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, created_at, last_login_at
         FROM users WHERE id = ?`, id)
}

// GetUserByEmail fetches a user by their unique email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*UserRow, error) {
	if email == "" {
		return nil, ErrNotFound
	}
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, created_at, last_login_at
         FROM users WHERE email = ?`, email)
}

// GetUserByStudentID fetches a user by their unique student identifier.
func (s *Store) GetUserByStudentID(ctx context.Context, sid string) (*UserRow, error) {
	if sid == "" {
		return nil, ErrNotFound
	}
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, created_at, last_login_at
         FROM users WHERE student_id = ?`, sid)
}

// UpdateUserName overwrites the display name of an existing user.
func (s *Store) UpdateUserName(ctx context.Context, id, name string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("store: update user name: %w", err)
	}
	return nil
}

// UpdateUserLastLogin stamps the most recent login time on a user row.
func (s *Store) UpdateUserLastLogin(ctx context.Context, id string, ts int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = ? WHERE id = ?`, ts, id)
	if err != nil {
		return fmt.Errorf("store: update user last_login: %w", err)
	}
	return nil
}

// ListUsers returns every user ordered by last_login_at descending.
func (s *Store) ListUsers(ctx context.Context) ([]UserRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, email, student_id, created_at, last_login_at
         FROM users ORDER BY last_login_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]UserRow, 0)
	for rows.Next() {
		r, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *Store) scanUser(ctx context.Context, query string, args ...any) (*UserRow, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	r, err := scanUserRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func scanUserRow(sc scanner) (*UserRow, error) {
	var (
		r     UserRow
		email sql.NullString
		sid   sql.NullString
	)
	if err := sc.Scan(&r.ID, &r.Name, &email, &sid, &r.CreatedAt, &r.LastLoginAt); err != nil {
		return nil, err
	}
	r.Email = email.String
	r.StudentID = sid.String
	return &r, nil
}
