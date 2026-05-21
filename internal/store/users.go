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

// Conflict sentinels for unique columns. Returned by CreateUser /
// UpdateUserFields when the requested email or student_id is already used by
// an active (non-soft-deleted) row. Callers translate these to HTTP 409.
var (
	ErrEmailTaken     = errors.New("store: email already in use")
	ErrStudentIDTaken = errors.New("store: student_id already in use")
)

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

// CreateUser inserts a new user row. Returns ErrEmailTaken / ErrStudentIDTaken
// when the identifier collides with an active row; soft-deleted rows do not
// reserve their identifiers thanks to the partial unique indexes.
func (s *Store) CreateUser(ctx context.Context, r UserRow) error {
	role := r.Role
	if !ValidRole(role) {
		role = RoleStudent
	}
	if r.Email != "" {
		if _, err := s.GetUserByEmail(ctx, r.Email); err == nil {
			return ErrEmailTaken
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	if r.StudentID != "" {
		if _, err := s.GetUserByStudentID(ctx, r.StudentID); err == nil {
			return ErrStudentIDTaken
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
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

// GetUser fetches a user by primary key. Returns ErrNotFound when absent or
// when the row was soft-deleted — callers that need to inspect deleted rows
// must query the table directly.
func (s *Store) GetUser(ctx context.Context, id string) (*UserRow, error) {
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, role, created_at, last_login_at
         FROM users WHERE id = ? AND deleted_at IS NULL`, id)
}

// GetUserByEmail fetches an active (non-deleted) user by email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*UserRow, error) {
	if email == "" {
		return nil, ErrNotFound
	}
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, role, created_at, last_login_at
         FROM users WHERE email = ? AND deleted_at IS NULL`, email)
}

// GetUserByStudentID fetches an active (non-deleted) user by student ID.
func (s *Store) GetUserByStudentID(ctx context.Context, sid string) (*UserRow, error) {
	if sid == "" {
		return nil, ErrNotFound
	}
	return s.scanUser(ctx,
		`SELECT id, name, email, student_id, role, created_at, last_login_at
         FROM users WHERE student_id = ? AND deleted_at IS NULL`, sid)
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

// UpdateUserFields struct collects the patchable identity fields. Nil pointers
// leave the corresponding column untouched.
type UpdateUserFieldsArgs struct {
	Name      *string
	Email     *string
	StudentID *string
}

// UpdateUserFields patches name/email/student_id on an existing user. When the
// email changes, any pending magic_links for the previous address are deleted
// in the same transaction so an attacker who intercepted the old link cannot
// use it to log in as the edited user. Returns ErrNotFound if id is unknown
// (or already soft-deleted) and ErrEmailTaken / ErrStudentIDTaken on conflict.
func (s *Store) UpdateUserFields(ctx context.Context, id string, args UpdateUserFieldsArgs) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin update user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current struct {
		Name      string
		Email     sql.NullString
		StudentID sql.NullString
	}
	err = tx.QueryRowContext(ctx,
		`SELECT name, email, student_id FROM users
         WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&current.Name, &current.Email, &current.StudentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: load user for update: %w", err)
	}

	if args.Email != nil && *args.Email != current.Email.String && *args.Email != "" {
		var exists string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM users
             WHERE email = ? AND deleted_at IS NULL AND id != ?`,
			*args.Email, id).Scan(&exists)
		if err == nil {
			return ErrEmailTaken
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: probe email uniqueness: %w", err)
		}
	}
	if args.StudentID != nil && *args.StudentID != current.StudentID.String && *args.StudentID != "" {
		var exists string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM users
             WHERE student_id = ? AND deleted_at IS NULL AND id != ?`,
			*args.StudentID, id).Scan(&exists)
		if err == nil {
			return ErrStudentIDTaken
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: probe student_id uniqueness: %w", err)
		}
	}

	if args.Name != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET name = ? WHERE id = ?`, *args.Name, id); err != nil {
			return fmt.Errorf("store: update name: %w", err)
		}
	}
	if args.Email != nil && *args.Email != current.Email.String {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET email = ? WHERE id = ?`,
			nullableString(*args.Email), id); err != nil {
			return fmt.Errorf("store: update email: %w", err)
		}
		if current.Email.Valid && current.Email.String != "" {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM magic_links WHERE email = ? AND used_at IS NULL`,
				current.Email.String); err != nil {
				return fmt.Errorf("store: invalidate magic links: %w", err)
			}
		}
	}
	if args.StudentID != nil && *args.StudentID != current.StudentID.String {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET student_id = ? WHERE id = ?`,
			nullableString(*args.StudentID), id); err != nil {
			return fmt.Errorf("store: update student_id: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit update user: %w", err)
	}
	return nil
}

// DeleteUser marks a user row as soft-deleted (deleted_at = now). Idempotent:
// repeated calls on an already-deleted row succeed without overwriting the
// original timestamp. Returns ErrNotFound only when no row with that id
// exists at all.
func (s *Store) DeleteUser(ctx context.Context, id string, now int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET deleted_at = ?
         WHERE id = ? AND deleted_at IS NULL`, now, id)
	if err != nil {
		return fmt.Errorf("store: soft-delete user: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	// Zero rows: either the user is already deleted (idempotent OK) or the id
	// does not exist (NotFound). Disambiguate with a raw lookup.
	var exists string
	err = s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: probe user exists: %w", err)
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

// ListUsers returns every active (non-soft-deleted) user ordered by
// last_login_at descending.
func (s *Store) ListUsers(ctx context.Context) ([]UserRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, email, student_id, role, created_at, last_login_at
         FROM users WHERE deleted_at IS NULL
         ORDER BY last_login_at DESC`)
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
