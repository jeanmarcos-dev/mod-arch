package store

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMagicLinks_ConsumeOnce(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	row := MagicLinkRow{
		Token:     "tok-1",
		Email:     "ada@ucv.ve",
		CreatedAt: 100,
		ExpiresAt: 200,
	}
	if err := s.CreateMagicLink(ctx, row); err != nil {
		t.Fatalf("create: %v", err)
	}

	email, err := s.ConsumeMagicLink(ctx, "tok-1", 150)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if email != "ada@ucv.ve" {
		t.Errorf("email = %q", email)
	}

	if _, err := s.ConsumeMagicLink(ctx, "tok-1", 160); !errors.Is(err, ErrNotFound) {
		t.Errorf("second consume err = %v, want ErrNotFound", err)
	}
}

func TestMagicLinks_Expired(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	row := MagicLinkRow{
		Token:     "tok-old",
		Email:     "x@y.z",
		CreatedAt: 100,
		ExpiresAt: 200,
	}
	_ = s.CreateMagicLink(ctx, row)

	if _, err := s.ConsumeMagicLink(ctx, "tok-old", 300); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired consume err = %v", err)
	}
}

func TestMagicLinks_ConcurrentConsumeIsAtomic(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	row := MagicLinkRow{
		Token:     "tok-race",
		Email:     "x@y.z",
		CreatedAt: 0,
		ExpiresAt: 1_000_000,
	}
	_ = s.CreateMagicLink(ctx, row)

	const goroutines = 16
	var wg sync.WaitGroup
	var ok int32
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if _, err := s.ConsumeMagicLink(ctx, "tok-race", 1); err == nil {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Errorf("ok = %d, want 1 (atomic)", ok)
	}
}

func TestMagicLinks_DeleteExpired(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	_ = s.CreateMagicLink(ctx, MagicLinkRow{Token: "a", Email: "a@a", CreatedAt: 0, ExpiresAt: 10})
	_ = s.CreateMagicLink(ctx, MagicLinkRow{Token: "b", Email: "b@b", CreatedAt: 0, ExpiresAt: 100})
	_, _ = s.ConsumeMagicLink(ctx, "b", 50)
	_ = s.CreateMagicLink(ctx, MagicLinkRow{Token: "c", Email: "c@c", CreatedAt: 0, ExpiresAt: 100_000})

	n, err := s.DeleteExpiredMagicLinks(ctx, 50)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 2 {
		t.Errorf("pruned = %d, want 2 (expired + used)", n)
	}
}
