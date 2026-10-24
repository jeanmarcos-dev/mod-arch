package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func countAllowed(s *Server, limiter *RateLimiter, attempts int) int {
	handler := s.limitByIP(limiter, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	allowed := 0
	for i := 0; i < attempts; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code == http.StatusOK {
			allowed++
		}
	}
	return allowed
}

func TestStudentLoginLimit(t *testing.T) {
	tests := []struct {
		name         string
		configured   int
		attempts     int
		wantAllowed  int
		wantRetryHdr bool
	}{
		{name: "default is 5", configured: 0, attempts: 6, wantAllowed: 5, wantRetryHdr: true},
		{name: "negative falls back to 5", configured: -3, attempts: 6, wantAllowed: 5, wantRetryHdr: true},
		{name: "300 allows 80 attempts", configured: 300, attempts: 80, wantAllowed: 80},
		{name: "300 blocks past the limit", configured: 300, attempts: 301, wantAllowed: 300, wantRetryHdr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Config{AuthLoginLimit: tc.configured, Logger: discardLogger()})
			if got := countAllowed(s, s.authLimit, tc.attempts); got != tc.wantAllowed {
				t.Fatalf("allowed = %d, want %d", got, tc.wantAllowed)
			}
			handler := s.limitByIP(s.authLimit, func(http.ResponseWriter, *http.Request) {})
			req := httptest.NewRequest(http.MethodPost, "/login", nil)
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			rec := httptest.NewRecorder()
			handler(rec, req)
			if gotHdr := rec.Header().Get("Retry-After") != ""; gotHdr != tc.wantRetryHdr {
				t.Fatalf("Retry-After present = %v, want %v", gotHdr, tc.wantRetryHdr)
			}
		})
	}
}

func TestAdminLoginLimitIgnoresStudentLimit(t *testing.T) {
	s := New(Config{AuthLoginLimit: 300, Logger: discardLogger()})
	if got := countAllowed(s, s.loginLimit, 6); got != 5 {
		t.Fatalf("admin login allowed = %d, want 5", got)
	}
}
