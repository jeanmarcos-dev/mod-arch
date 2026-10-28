package server

import (
	"context"
	"net/http"
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

func TestResolveClientIP(t *testing.T) {
	tests := []struct {
		name    string
		trusted string
		remote  string
		headers map[string]string
		want    string
	}{
		{name: "remote only", remote: "1.2.3.4:5678", want: "1.2.3.4"},
		{name: "forwarded headers ignored without trusted proxy", remote: "10.0.0.1:80", headers: map[string]string{"X-Forwarded-For": "5.6.7.8", "X-Real-IP": "7.7.7.7", "CF-Connecting-IP": "8.8.8.8"}, want: "10.0.0.1"},
		{name: "cloudflare header", trusted: "CF-Connecting-IP", remote: "10.0.0.1:80", headers: map[string]string{"CF-Connecting-IP": "203.0.113.9", "X-Forwarded-For": "6.6.6.6"}, want: "203.0.113.9"},
		{name: "x-forwarded-for keeps the hop added by the trusted proxy", trusted: "X-Forwarded-For", remote: "10.0.0.1:80", headers: map[string]string{"X-Forwarded-For": "6.6.6.6, 203.0.113.9"}, want: "203.0.113.9"},
		{name: "missing trusted header falls back to remote", trusted: "X-Real-IP", remote: "10.0.0.1:80", want: "10.0.0.1"},
		{name: "invalid trusted value falls back to remote", trusted: "X-Real-IP", remote: "10.0.0.1:80", headers: map[string]string{"X-Real-IP": "not-an-ip"}, want: "10.0.0.1"},
		{name: "ipv6", trusted: "CF-Connecting-IP", remote: "[::1]:80", headers: map[string]string{"CF-Connecting-IP": "2001:db8::1"}, want: "2001:db8::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := resolveClientIP(r, tc.trusted); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClientIPUsesValueResolvedByMiddleware(t *testing.T) {
	var got string
	h := WithClientCtx([]byte("salt"), nil, false, "CF-Connecting-IP")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = clientIP(r)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:80"
	r.Header.Set("CF-Connecting-IP", "203.0.113.9")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got != "203.0.113.9" {
		t.Fatalf("clientIP = %q, want the middleware-resolved address", got)
	}
}
