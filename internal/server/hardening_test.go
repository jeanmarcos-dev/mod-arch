package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

func TestPublicHost(t *testing.T) {
	tests := []struct {
		name    string
		origins string
		host    string
		want    string
	}{
		{name: "allowed host is kept", origins: "https://good.example,https://alt.example", host: "alt.example", want: "alt.example"},
		{name: "spoofed host falls back to the first allowed origin", origins: "https://good.example", host: "evil.example", want: "good.example"},
		{name: "patterns without scheme", origins: "good.example", host: "GOOD.example", want: "GOOD.example"},
		{name: "dev defaults", origins: "", host: "localhost:9080", want: "localhost:9080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ALLOWED_ORIGINS", tc.origins)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Host = tc.host
			if got := publicHost(r); got != tc.want {
				t.Fatalf("publicHost = %q, want %q", got, tc.want)
			}
		})
	}
}

func enableViewerPassword(t *testing.T, st *store.Store, password string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	for k, v := range map[string]string{SettingViewerPasswordRequired: "true", SettingViewerPasswordHash: string(hash)} {
		if err := st.SetSetting(t.Context(), k, v, "test", 1); err != nil {
			t.Fatalf("SetSetting %s: %v", k, err)
		}
	}
}

func TestViewerCookieRequiresPasswordWhenGateIsOn(t *testing.T) {
	f := newLiveFixture(t)
	enableViewerPassword(t, f.st, "secret")
	sess, err := f.mgr.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token := f.mgr.ActivateShare(sess)

	bypass := f.decode(t, f.get("/api/session", &http.Cookie{Name: ViewCookie, Value: token}))
	if bypass["role"] == "viewer" || bypass["id"] == sess.ID {
		t.Fatalf("view token without the password cookie must not join as viewer, got %v", bypass)
	}

	ok := f.decode(t, f.get("/api/session",
		&http.Cookie{Name: ViewCookie, Value: token},
		&http.Cookie{Name: ViewerPasswordCookie, Value: f.s.viewerPasswordCookieValue()}))
	if ok["role"] != "viewer" || ok["id"] != sess.ID {
		t.Fatalf("valid password cookie must join as viewer, got %v", ok)
	}
}

func TestViewerPasswordAttemptsAreRateLimited(t *testing.T) {
	t.Setenv("ADMIN_USER", "boss")
	t.Setenv("ADMIN_PASS_HASH", "x")
	t.Setenv("SESSION_SECRET", "rate-limit-secret")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	enableViewerPassword(t, st, "secret")
	mgr := sessions.NewManager(sessions.Config{Store: st})
	s := New(Config{Manager: mgr, Logger: discardLogger(), AuthLoginLimit: 2})
	t.Cleanup(s.Close)
	mux := http.NewServeMux()
	s.Routes(mux)
	sess, err := mgr.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token := mgr.ActivateShare(sess)

	var last int
	for i := 0; i < 3; i++ {
		form := url.Values{"password": {"wrong"}}
		req := httptest.NewRequest(http.MethodPost, "/v/"+token+"/auth", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("third wrong attempt status = %d, want 429", last)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := withSecurityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	for _, tc := range []struct {
		path      string
		https     bool
		wantFrame string
		wantHSTS  bool
	}{
		{path: "/", wantFrame: ""},
		{path: "/admin/login", wantFrame: "DENY"},
		{path: "/admin/api/live", https: true, wantFrame: "DENY", wantHSTS: true},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.https {
			req.Header.Set("X-Forwarded-Proto", "https")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		hdr := rec.Header()
		if hdr.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing nosniff", tc.path)
		}
		if hdr.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
			t.Errorf("%s: Referrer-Policy = %q", tc.path, hdr.Get("Referrer-Policy"))
		}
		if got := hdr.Get("X-Frame-Options"); got != tc.wantFrame {
			t.Errorf("%s: X-Frame-Options = %q, want %q", tc.path, got, tc.wantFrame)
		}
		if got := hdr.Get("Strict-Transport-Security") != ""; got != tc.wantHSTS {
			t.Errorf("%s: HSTS present = %v, want %v", tc.path, got, tc.wantHSTS)
		}
	}
}

func TestRequestBodyIsCapped(t *testing.T) {
	var readErr error
	h := limitRequestBody(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(make([]byte, maxRequestBodyBytes+1)))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr == nil {
		t.Fatal("reading a body above the cap must fail")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(make([]byte, 1024)))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr != nil {
		t.Fatalf("small body must be readable, got %v", readErr)
	}
}
