package store

import (
	"errors"
	"testing"
)

func TestProfile_UpsertAndGet(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	mustCreateUser(t, s, "u1")

	// Missing profile surfaces ErrNotFound — caller renders empty form.
	if _, err := s.GetProfile(ctx, "u1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProfile expected ErrNotFound, got %v", err)
	}

	p := UserProfile{
		UserID:            "u1",
		Cohort:            "2026-A",
		Institution:       "UCV",
		Career:            "Sistemas",
		Bio:               "hola",
		GitHub:            "jeanmarcos",
		LinkedIn:          "jm",
		PreferredLanguage: "es",
		UpdatedAt:         100,
	}
	if err := s.UpsertProfile(ctx, p); err != nil {
		t.Fatalf("UpsertProfile insert: %v", err)
	}
	got, err := s.GetProfile(ctx, "u1")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if *got != p {
		t.Errorf("roundtrip mismatch:\nwant %+v\n got %+v", p, *got)
	}

	// Upsert overwrites, does not append.
	p.Cohort = "2026-B"
	p.UpdatedAt = 200
	if err := s.UpsertProfile(ctx, p); err != nil {
		t.Fatalf("UpsertProfile update: %v", err)
	}
	got, _ = s.GetProfile(ctx, "u1")
	if got.Cohort != "2026-B" || got.UpdatedAt != 200 {
		t.Errorf("update did not stick: %+v", got)
	}
}

func TestProfile_CascadeOnUserDelete(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	mustCreateUser(t, s, "u1")
	if err := s.UpsertProfile(ctx, UserProfile{UserID: "u1", UpdatedAt: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Hard delete via raw SQL to verify the FK CASCADE configuration. The
	// store layer uses soft deletes, which leave the profile intact by design.
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM users WHERE id = ?`, "u1"); err != nil {
		t.Fatalf("hard delete: %v", err)
	}
	if _, err := s.GetProfile(ctx, "u1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected profile to cascade-delete, got err=%v", err)
	}
}

func TestProfile_EmptyUserIDRejected(t *testing.T) {
	s := openMem(t)
	if err := s.UpsertProfile(t.Context(), UserProfile{}); err == nil {
		t.Error("expected error for empty user_id")
	}
	if _, err := s.GetProfile(t.Context(), ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for empty id, got %v", err)
	}
}
