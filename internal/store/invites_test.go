package store

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestInvite_CreateAndGet(t *testing.T) {
	s := openMem(t)
	err := s.CreateInvite(t.Context(), InviteRow{
		Token:     "tok",
		Label:     "Prof. Carlos",
		CreatedAt: 100,
		ExpiresAt: 200,
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	got, err := s.GetInvite(t.Context(), "tok")
	if err != nil {
		t.Fatalf("GetInvite: %v", err)
	}
	if got.Label != "Prof. Carlos" || got.ExpiresAt != 200 || got.UsedAt != 0 {
		t.Errorf("unexpected row: %+v", got)
	}
}

func TestInvite_ConsumeMarksUsed(t *testing.T) {
	s := openMem(t)
	mustCreateInvite(t, s, "tok", "L", 100, 200)

	label, err := s.ConsumeInvite(t.Context(), "tok", 150)
	if err != nil {
		t.Fatalf("ConsumeInvite: %v", err)
	}
	if label != "L" {
		t.Errorf("label=%q want %q", label, "L")
	}

	row, err := s.GetInvite(t.Context(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if row.UsedAt != 150 {
		t.Errorf("UsedAt=%d, want 150", row.UsedAt)
	}
}

func TestInvite_ConsumeRejects(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, s *Store)
		now   int64
		token string
	}{
		{
			name:  "unknown",
			setup: func(t *testing.T, s *Store) {},
			now:   150,
			token: "ghost",
		},
		{
			name: "expired",
			setup: func(t *testing.T, s *Store) {
				mustCreateInvite(t, s, "tok", "L", 100, 200)
			},
			now:   200,
			token: "tok",
		},
		{
			name: "already used",
			setup: func(t *testing.T, s *Store) {
				mustCreateInvite(t, s, "tok", "L", 100, 200)
				if _, err := s.ConsumeInvite(t.Context(), "tok", 150); err != nil {
					t.Fatal(err)
				}
			},
			now:   160,
			token: "tok",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openMem(t)
			tc.setup(t, s)
			if _, err := s.ConsumeInvite(t.Context(), tc.token, tc.now); !errors.Is(err, ErrNotFound) {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func TestInvite_ConsumeIsAtomic(t *testing.T) {
	// Many concurrent consumes for the same token must yield exactly one
	// success. This exercises the UPDATE ... WHERE used_at IS NULL guard.
	s := openMem(t)
	mustCreateInvite(t, s, "tok", "L", 0, 1_000_000)

	const goroutines = 32
	var successes atomic.Int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if _, err := s.ConsumeInvite(t.Context(), "tok", 100); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Errorf("successes=%d, want exactly 1", successes.Load())
	}
}

func TestInvite_ListPendingFiltersUsedAndExpired(t *testing.T) {
	s := openMem(t)
	mustCreateInvite(t, s, "pending", "p", 100, 1000)
	mustCreateInvite(t, s, "used", "u", 100, 1000)
	mustCreateInvite(t, s, "expired", "e", 50, 80)
	if _, err := s.ConsumeInvite(t.Context(), "used", 200); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListPendingInvites(t.Context(), 200)
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(list) != 1 || list[0].Token != "pending" {
		t.Errorf("pending=%+v want only 'pending'", list)
	}
}

func TestInvite_DeleteExpired(t *testing.T) {
	s := openMem(t)
	mustCreateInvite(t, s, "pending", "p", 100, 1000)
	mustCreateInvite(t, s, "used", "u", 100, 1000)
	mustCreateInvite(t, s, "expired", "e", 50, 80)
	if _, err := s.ConsumeInvite(t.Context(), "used", 200); err != nil {
		t.Fatal(err)
	}

	n, err := s.DeleteExpiredInvites(t.Context(), 200)
	if err != nil {
		t.Fatalf("DeleteExpiredInvites: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted %d, want 2 (used + expired)", n)
	}
	if _, err := s.GetInvite(t.Context(), "pending"); err != nil {
		t.Errorf("pending should survive: %v", err)
	}
}

func TestInvite_Revoke(t *testing.T) {
	s := openMem(t)
	mustCreateInvite(t, s, "tok", "L", 100, 1000)
	if err := s.RevokeInvite(t.Context(), "tok"); err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
	if _, err := s.GetInvite(t.Context(), "tok"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after revoke expected ErrNotFound, got %v", err)
	}
	// Revoking an unknown token is a no-op.
	if err := s.RevokeInvite(t.Context(), "ghost"); err != nil {
		t.Errorf("revoke unknown should be no-op, got %v", err)
	}
}

func mustCreateInvite(t *testing.T, s *Store, token, label string, created, expires int64) {
	t.Helper()
	if err := s.CreateInvite(t.Context(), InviteRow{
		Token:     token,
		Label:     label,
		CreatedAt: created,
		ExpiresAt: expires,
	}); err != nil {
		t.Fatalf("CreateInvite(%s): %v", token, err)
	}
}
