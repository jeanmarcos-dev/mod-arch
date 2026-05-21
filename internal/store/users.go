package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// User role values stored in users.role. The enum is small enough that we
// repeat the validation inline at each callsite; declaring constants keeps the
// strings consistent across the codebase.
const (
	RoleStudent   = "student"
	RoleModerator = "moderator"
	RoleAdmin     = "admin"
)

// ValidRole reports whether r is one of the allowed user roles. Callers use
// this both to reject API input and to fall back to "student" when a legacy
// row carries an empty role.
func ValidRole(r string) bool {
	return r == RoleStudent || r == RoleModerator || r == RoleAdmin
}

// UserRow is the persistent identity of a student. Email and StudentID are
// optional but UNIQUE when present (NULL coexistence is enforced via
// nullableString). Role defaults to "student" when the column was just added
// via migration.
type UserRow struct {
	ID          string
	Name        string
	Email       string
	StudentID   string
	Role        string
	CreatedAt   int64
	LastLoginAt int64
}

// CreateUser inserts a new user row.
func (s *Store) CreateUser(ctx context.Context, r UserRow) error {
	role := r.Role
	if !ValidRole(role) {
		role = RoleStudent
	}
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO users(id, name, email, student_id, role, created_at, last_login_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, nullableString(r.Email), nullableString(r.StudentID),
		role, r.CreatedAt, r.LastLoginAt)
	if err != nil {
		return fmt.Errorf("store: insert user: %w", err)
	}
	return nil
}

// GetUser fetches a user by primary key. Returns ErrNotFound when absent.
func (s *Store) GetUser(ctx context.Context, id string) (*UserRow, error) {
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, role, created_at, last_login_at
         FROM users WHERE id = ?`, id)
}

// GetUserByEmail fetches a user by their unique email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*UserRow, error) {
	if email == "" {
		return nil, ErrNotFound
	}
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, role, created_at, last_login_at
         FROM users WHERE email = ?`, email)
}

// GetUserByStudentID fetches a user by their unique student identifier.
func (s *Store) GetUserByStudentID(ctx context.Context, sid string) (*UserRow, error) {
	if sid == "" {
		return nil, ErrNotFound
	}
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, role, created_at, last_login_at
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

// UpdateUserRole assigns a new role to an existing user. The value must be
// one of {student, moderator, admin}; otherwise the call returns an error
// without touching the row.
func (s *Store) UpdateUserRole(ctx context.Context, id, role string) error {
	if !ValidRole(role) {
		return fmt.Errorf("store: invalid role %q", role)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET role = ? WHERE id = ?`, role, id)
	if err != nil {
		return fmt.Errorf("store: update user role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUsers returns every user ordered by last_login_at descending.
func (s *Store) ListUsers(ctx context.Context) ([]UserRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, email, student_id, role, created_at, last_login_at
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
	if err := sc.Scan(&r.ID, &r.Name, &email, &sid, &r.Role, &r.CreatedAt, &r.LastLoginAt); err != nil {
		return nil, err
	}
	r.Email = email.String
	r.StudentID = sid.String
	if !ValidRole(r.Role) {
		// Defensive: should never happen with the column default, but a manual
		// edit or future migration error must not crash the runtime.
		r.Role = RoleStudent
	}
	return &r, nil
}
