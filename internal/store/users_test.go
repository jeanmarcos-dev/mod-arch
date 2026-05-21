package store

import (
	"errors"
	"testing"
	"time"
)

func TestUsers_CRUD(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	now := time.Now().Unix()
	row := UserRow{
		ID:          "u1",
		Name:        "Ada",
		Email:       "ada@ucv.ve",
		StudentID:   "UCV-001",
		CreatedAt:   now,
		LastLoginAt: now,
	}
	if err := s.CreateUser(ctx, row); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetUser(ctx, "u1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Ada" || got.Email != "ada@ucv.ve" || got.StudentID != "UCV-001" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	byMail, err := s.GetUserByEmail(ctx, "ada@ucv.ve")
	if err != nil {
		t.Fatalf("by email: %v", err)
	}
	if byMail.ID != "u1" {
		t.Errorf("by email id = %q", byMail.ID)
	}

	bySID, err := s.GetUserByStudentID(ctx, "UCV-001")
	if err != nil {
		t.Fatalf("by sid: %v", err)
	}
	if bySID.ID != "u1" {
		t.Errorf("by sid id = %q", bySID.ID)
	}

	if _, err := s.GetUserByEmail(ctx, "missing@ucv.ve"); !errors.Is(err, ErrNotFound) {
		t.Errorf("by missing email err = %v", err)
	}

	if err := s.UpdateUserName(ctx, "u1", "Ada Lovelace"); err != nil {
		t.Fatalf("update name: %v", err)
	}
	if err := s.UpdateUserLastLogin(ctx, "u1", now+10); err != nil {
		t.Fatalf("update last login: %v", err)
	}
	got, _ = s.GetUser(ctx, "u1")
	if got.Name != "Ada Lovelace" || got.LastLoginAt != now+10 {
		t.Errorf("post-update mismatch: %+v", got)
	}
}

func TestUsers_NullEmailCoexistence(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	for i, id := range []string{"u1", "u2"} {
		err := s.CreateUser(ctx, UserRow{
			ID:          id,
			Name:        "anon",
			CreatedAt:   int64(i),
			LastLoginAt: int64(i),
		})
		if err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("list len = %d, want 2", len(users))
	}
}

// TestUsers_Role covers the default + explicit role workflow plus the
// validation gate on UpdateUserRole. The default-on-insert path matters
// because legacy callers create users with Role == "".
func TestUsers_Role(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	// Default role on creation when the caller omits the field.
	if err := s.CreateUser(ctx, UserRow{
		ID: "u1", Name: "Ada", CreatedAt: 1, LastLoginAt: 1,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, _ := s.GetUser(ctx, "u1")
	if got.Role != RoleStudent {
		t.Errorf("default role = %q, want %q", got.Role, RoleStudent)
	}

	// Promote then read back.
	if err := s.UpdateUserRole(ctx, "u1", RoleModerator); err != nil {
		t.Fatalf("promote: %v", err)
	}
	got, _ = s.GetUser(ctx, "u1")
	if got.Role != RoleModerator {
		t.Errorf("post-promote role = %q, want %q", got.Role, RoleModerator)
	}

	// Invalid role must be rejected without changing the row.
	if err := s.UpdateUserRole(ctx, "u1", "owner"); err == nil {
		t.Error("expected error for invalid role, got nil")
	}
	got, _ = s.GetUser(ctx, "u1")
	if got.Role != RoleModerator {
		t.Errorf("role after invalid update = %q, want unchanged %q", got.Role, RoleModerator)
	}

	// Unknown target must surface as ErrNotFound.
	if err := s.UpdateUserRole(ctx, "u-missing", RoleAdmin); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing user err = %v, want ErrNotFound", err)
	}

	// Explicit role at creation time is honored.
	if err := s.CreateUser(ctx, UserRow{
		ID: "u2", Name: "Bob", Role: RoleAdmin, CreatedAt: 2, LastLoginAt: 2,
	}); err != nil {
		t.Fatalf("create with role: %v", err)
	}
	got, _ = s.GetUser(ctx, "u2")
	if got.Role != RoleAdmin {
		t.Errorf("create-with-admin role = %q, want %q", got.Role, RoleAdmin)
	}
}

func mustOpen(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
