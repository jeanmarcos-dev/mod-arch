package server

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/server/i18n"
)

// RateLimiter is a fixed-window per-IP limiter intended for low-frequency
// authentication endpoints. The bucket is reset when the window expires; we
// do not amortize across windows on purpose — the goal is to make brute-force
// attempts trivially detectable, not to shape regular traffic.
type RateLimiter struct {
	limit   int
	window  time.Duration
	buckets sync.Map // map[string]*rlBucket
	now     func() time.Time
}

type rlBucket struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

// NewRateLimiter builds a limiter that allows at most `limit` events per IP
// within each `window`. The cleanup goroutine purges idle buckets every
// `cleanup` interval until ctx is canceled.
func NewRateLimiter(ctx context.Context, limit int, window, cleanup time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limit:  limit,
		window: window,
		now:    time.Now,
	}
	if cleanup > 0 {
		go rl.cleanupLoop(ctx, cleanup)
	}
	return rl
}

// Allow records an attempt for ip and reports whether it is within the limit.
// retryAfter is the time the caller should wait before the window resets;
// callers should expose it via the Retry-After HTTP header on 429 responses.
func (rl *RateLimiter) Allow(ip string) (allowed bool, retryAfter time.Duration) {
	if ip == "" {
		return true, 0
	}
	now := rl.now()
	v, _ := rl.buckets.LoadOrStore(ip, &rlBucket{resetAt: now.Add(rl.window)})
	b := v.(*rlBucket)
	b.mu.Lock()
	defer b.mu.Unlock()
	if now.After(b.resetAt) {
		b.count = 0
		b.resetAt = now.Add(rl.window)
	}
	b.count++
	if b.count > rl.limit {
		return false, b.resetAt.Sub(now)
	}
	return true, 0
}

func (rl *RateLimiter) cleanupLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rl.purgeExpired()
		}
	}
}

func (rl *RateLimiter) purgeExpired() {
	now := rl.now()
	rl.buckets.Range(func(k, v any) bool {
		b := v.(*rlBucket)
		b.mu.Lock()
		expired := now.After(b.resetAt)
		b.mu.Unlock()
		if expired {
			rl.buckets.Delete(k)
		}
		return true
	})
}

// limitByIP wraps next so each call increments the IP bucket before reaching
// the handler. When the limit is exceeded the response is 429 with a JSON
// body and a Retry-After header in seconds.
func (s *Server) limitByIP(rl *RateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		ok, retry := rl.Allow(ip)
		if !ok {
			secs := int(retry.Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			s.logger.Info("rate limit hit", "ip", ip, "path", r.URL.Path, "retry_after_s", secs)
			i18n.WriteErrorAlong(w, r, http.StatusTooManyRequests, i18n.CodeRateLimitTooMany, nil, map[string]any{
				"retry_after": secs,
			})
			return
		}
		next(w, r)
	}
}

// clientIP extracts the caller's IP, honoring X-Forwarded-For when a trusted
// reverse proxy sets it. The first value in the list is taken — operators
// behind multiple proxies should configure their edge to rewrite the header.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.IndexByte(xff, ','); comma >= 0 {
			return strings.TrimSpace(xff[:comma])
		}
		return strings.TrimSpace(xff)
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
