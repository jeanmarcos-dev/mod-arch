package llm

import (
	"context"
	"sync"
	"time"
)

// TokenBucket is a process-wide rate limiter for LLM calls. Every request
// must Acquire a token before going out; tokens refill at a fixed interval
// so the long-run rate matches the provider's RPM ceiling. Bursts up to
// `burst` tokens are allowed (useful so the first few questions of a class
// session do not all queue serially).
//
// Goroutine-safe. Multiple callers Acquire concurrently and the bucket
// guarantees FIFO ordering of waiters.
type TokenBucket struct {
	mu        sync.Mutex
	available int
	capacity  int
	interval  time.Duration
	lastFill  time.Time
	waiters   []chan struct{} // FIFO of goroutines blocked on Acquire
	stop      chan struct{}
}

// NewTokenBucket returns a bucket that holds up to `burst` tokens and adds
// one token every `interval`. burst<=0 or interval<=0 makes the bucket a
// no-op (Acquire returns immediately) so callers can disable it cleanly by
// passing zeros.
func NewTokenBucket(burst int, interval time.Duration) *TokenBucket {
	if burst <= 0 || interval <= 0 {
		return &TokenBucket{}
	}
	b := &TokenBucket{
		available: burst,
		capacity:  burst,
		interval:  interval,
		lastFill:  time.Now(),
		stop:      make(chan struct{}),
	}
	go b.run()
	return b
}

// Acquire blocks until a token is available or ctx is cancelled. Returns
// nil on success, ctx.Err() on cancellation. A nil receiver is a no-op so
// callers can pass an optional bucket without nil-checking.
func (b *TokenBucket) Acquire(ctx context.Context) error {
	if b == nil || b.interval == 0 {
		return nil
	}
	b.mu.Lock()
	if b.available > 0 {
		b.available--
		b.mu.Unlock()
		return nil
	}
	// No room: queue up.
	ch := make(chan struct{})
	b.waiters = append(b.waiters, ch)
	b.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		// Remove ourselves from the queue so the next refill does not hand
		// the token to a cancelled waiter.
		b.mu.Lock()
		for i, w := range b.waiters {
			if w == ch {
				b.waiters = append(b.waiters[:i], b.waiters[i+1:]...)
				break
			}
		}
		b.mu.Unlock()
		return ctx.Err()
	}
}

// TryAcquire is non-blocking. Returns true if a token was claimed.
func (b *TokenBucket) TryAcquire() bool {
	if b == nil || b.interval == 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.available > 0 {
		b.available--
		return true
	}
	return false
}

// NextAvailableIn returns how long the caller would wait if they called
// Acquire right now (counting any waiters already ahead).
func (b *TokenBucket) NextAvailableIn() time.Duration {
	if b == nil || b.interval == 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.available > 0 {
		return 0
	}
	queueAhead := len(b.waiters)
	elapsed := time.Since(b.lastFill)
	nextRefill := max(b.interval-elapsed, 0)
	return nextRefill + time.Duration(queueAhead)*b.interval
}

// QueueDepth reports how many goroutines are currently blocked on Acquire.
// Used by the orchestrator to tell the UI "you are #N in line".
func (b *TokenBucket) QueueDepth() int {
	if b == nil || b.interval == 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.waiters)
}

// Close stops the refill goroutine.
func (b *TokenBucket) Close() {
	if b == nil || b.stop == nil {
		return
	}
	select {
	case <-b.stop:
	default:
		close(b.stop)
	}
}

// run periodically adds one token, handing it directly to the next waiter
// in line if any. This collapses the refill + wake into a single critical
// section so two near-simultaneous waiters never race for the same token.
func (b *TokenBucket) run() {
	t := time.NewTicker(b.interval)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.mu.Lock()
			b.lastFill = time.Now()
			if len(b.waiters) > 0 {
				// Hand the freshly-minted token to the head of the queue.
				next := b.waiters[0]
				b.waiters = b.waiters[1:]
				b.mu.Unlock()
				close(next) // wakes the waiter
				continue
			}
			if b.available < b.capacity {
				b.available++
			}
			b.mu.Unlock()
		}
	}
}
