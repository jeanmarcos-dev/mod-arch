package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowWithinLimit(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 3, time.Minute, 0)
	for i := 0; i < 3; i++ {
		ok, _ := rl.Allow("1.2.3.4")
		if !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	ok, retry := rl.Allow("1.2.3.4")
	if ok {
		t.Fatal("fourth attempt should be blocked")
	}
	if retry <= 0 {
		t.Fatal("retry must be positive on block")
	}
}

func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 1, time.Minute, 0)
	if ok, _ := rl.Allow("a"); !ok {
		t.Fatal("first attempt for a")
	}
	if ok, _ := rl.Allow("a"); ok {
		t.Fatal("second attempt for a should block")
	}
	if ok, _ := rl.Allow("b"); !ok {
		t.Fatal("first attempt for b should pass")
	}
}

func TestRateLimiter_WindowResets(t *testing.T) {
	base := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	clock := base
	rl := NewRateLimiter(context.Background(), 2, time.Minute, 0)
	rl.now = func() time.Time { return clock }

	rl.Allow("ip")
	rl.Allow("ip")
	if ok, _ := rl.Allow("ip"); ok {
		t.Fatal("third attempt should block while in window")
	}

	clock = base.Add(2 * time.Minute)
	if ok, _ := rl.Allow("ip"); !ok {
		t.Fatal("attempt after window expiry should be allowed")
	}
}

func TestRateLimiter_PurgeExpired(t *testing.T) {
	base := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	clock := base
	rl := NewRateLimiter(context.Background(), 1, time.Minute, 0)
	rl.now = func() time.Time { return clock }

	rl.Allow("ip")
	clock = base.Add(2 * time.Minute)
	rl.purgeExpired()

	purged := 0
	rl.buckets.Range(func(_, _ any) bool { purged++; return true })
	if purged != 0 {
		t.Fatalf("expected all buckets purged, found %d", purged)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*httptest.ResponseRecorder)
		remote   string
		xff      string
		xreal    string
		expected string
	}{
		{name: "remote only", remote: "1.2.3.4:5678", expected: "1.2.3.4"},
		{name: "xff single", remote: "10.0.0.1:80", xff: "5.6.7.8", expected: "5.6.7.8"},
		{name: "xff list takes first", remote: "10.0.0.1:80", xff: "9.9.9.9, 5.6.7.8", expected: "9.9.9.9"},
		{name: "xreal-ip fallback", remote: "10.0.0.1:80", xreal: "7.7.7.7", expected: "7.7.7.7"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.xreal != "" {
				r.Header.Set("X-Real-IP", tc.xreal)
			}
			got := clientIP(r)
			if got != tc.expected {
				t.Fatalf("got %q, want %q", got, tc.expected)
			}
		})
	}
}
