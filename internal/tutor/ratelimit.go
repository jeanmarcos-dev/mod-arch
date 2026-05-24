package tutor

import (
	"sync"
	"time"
)

// RateLimiter is a simple sliding-window limiter keyed by session ID. The
// window is one rolling hour by default; configurable per instance.
//
// Memory grows with the number of distinct sessions that ask within the
// window. A janitor goroutine sweeps idle entries every 10 minutes so a
// long-lived process does not leak.
type RateLimiter struct {
	mu       sync.Mutex
	window   time.Duration
	max      int
	requests map[string][]time.Time
	stop     chan struct{}
}

// NewRateLimiter builds a limiter that allows max requests per window per
// session ID. window of 0 defaults to one hour; max of 0 defaults to 20.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	if max <= 0 {
		max = 20
	}
	if window <= 0 {
		window = time.Hour
	}
	r := &RateLimiter{
		window:   window,
		max:      max,
		requests: make(map[string][]time.Time),
		stop:     make(chan struct{}),
	}
	go r.sweep()
	return r
}

// Allow records an attempt for sessionID. Returns ok=false plus the duration
// until the oldest in-window request ages out when the cap is hit.
func (r *RateLimiter) Allow(sessionID string) (ok bool, retryAfter time.Duration) {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := now.Add(-r.window)
	hist := r.requests[sessionID]
	fresh := hist[:0]
	for _, t := range hist {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	if len(fresh) >= r.max {
		// Oldest in-window request determines when room opens up.
		return false, fresh[0].Add(r.window).Sub(now)
	}
	fresh = append(fresh, now)
	r.requests[sessionID] = fresh
	return true, 0
}

// Snapshot returns the current count for a session (debug / admin views).
func (r *RateLimiter) Snapshot(sessionID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().Add(-r.window)
	hist := r.requests[sessionID]
	n := 0
	for _, t := range hist {
		if t.After(now) {
			n++
		}
	}
	return n
}

// Close stops the janitor goroutine. Safe to call multiple times.
func (r *RateLimiter) Close() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}

// sweep purges entries whose entire history fell out of the window. Runs
// every 10 minutes to keep memory steady under churn.
func (r *RateLimiter) sweep() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case now := <-t.C:
			cutoff := now.Add(-r.window)
			r.mu.Lock()
			for id, hist := range r.requests {
				kept := hist[:0]
				for _, t := range hist {
					if t.After(cutoff) {
						kept = append(kept, t)
					}
				}
				if len(kept) == 0 {
					delete(r.requests, id)
				} else {
					r.requests[id] = kept
				}
			}
			r.mu.Unlock()
		}
	}
}
