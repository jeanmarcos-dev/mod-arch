package llm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenBucketBurst(t *testing.T) {
	b := NewTokenBucket(3, 100*time.Millisecond)
	defer b.Close()
	ctx := context.Background()
	// First three Acquires must return immediately from the burst pool.
	for i := range 3 {
		start := time.Now()
		if err := b.Acquire(ctx); err != nil {
			t.Fatalf("burst Acquire %d: %v", i, err)
		}
		if d := time.Since(start); d > 20*time.Millisecond {
			t.Errorf("burst Acquire %d took %v, expected near-zero", i, d)
		}
	}
}

func TestTokenBucketRefill(t *testing.T) {
	b := NewTokenBucket(1, 60*time.Millisecond)
	defer b.Close()
	ctx := context.Background()
	_ = b.Acquire(ctx) // drain
	start := time.Now()
	if err := b.Acquire(ctx); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	waited := time.Since(start)
	if waited < 50*time.Millisecond {
		t.Errorf("waited %v, expected at least one interval (~60ms)", waited)
	}
}

func TestTokenBucketFIFO(t *testing.T) {
	b := NewTokenBucket(0, 30*time.Millisecond) // burst=0 forces every caller to wait
	// burst=0 actually disables the bucket per NewTokenBucket; use 1 instead and drain it.
	b.Close()
	b = NewTokenBucket(1, 30*time.Millisecond)
	defer b.Close()
	_ = b.Acquire(context.Background()) // drain the single burst slot
	var order []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range 3 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Stagger starts so the FIFO order is deterministic.
			time.Sleep(time.Duration(i) * 5 * time.Millisecond)
			_ = b.Acquire(context.Background())
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	for i, v := range order {
		if v != i {
			t.Errorf("FIFO violated: position %d had goroutine %d", i, v)
		}
	}
}

func TestTokenBucketContextCancel(t *testing.T) {
	b := NewTokenBucket(1, time.Hour) // ~never refills
	defer b.Close()
	_ = b.Acquire(context.Background()) // drain
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := b.Acquire(ctx)
	if err == nil {
		t.Fatal("expected cancel error")
	}
}

func TestTokenBucketDisabled(t *testing.T) {
	// burst<=0 or interval<=0 should make Acquire a no-op.
	b := NewTokenBucket(0, time.Second)
	defer b.Close()
	var n atomic.Int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Acquire(context.Background())
			n.Add(1)
		}()
	}
	wg.Wait()
	if n.Load() != 50 {
		t.Errorf("only %d/50 Acquires completed; disabled bucket should let all through", n.Load())
	}
}
