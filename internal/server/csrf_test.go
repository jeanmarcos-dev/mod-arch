package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCSRFServer(t *testing.T) *Server {
	t.Helper()
	return &Server{logger: discardLogger()}
}

func TestEnsureCSRFCookie_SetsWhenAbsent(t *testing.T) {
	s := newCSRFServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	s.ensureCSRFCookie(w, r)
	res := w.Result()
	defer res.Body.Close()
	cookies := res.Cookies()
	var token string
	for _, c := range cookies {
		if c.Name == CSRFCookie {
			token = c.Value
		}
	}
	if len(token) != csrfTokenLen*2 {
		t.Fatalf("expected %d-byte hex token, got %q", csrfTokenLen*2, token)
	}
}

func TestEnsureCSRFCookie_ReusesExisting(t *testing.T) {
	s := newCSRFServer(t)
	w := httptest.NewRecorder()
	existing := strings.Repeat("a", csrfTokenLen*2)
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: existing})
	s.ensureCSRFCookie(w, r)
	for _, c := range w.Result().Cookies() {
		if c.Name == CSRFCookie {
			t.Fatalf("should not overwrite existing csrf cookie")
		}
	}
}

func TestVerifyCSRF_OK(t *testing.T) {
	s := newCSRFServer(t)
	token := strings.Repeat("b", csrfTokenLen*2)
	r := httptest.NewRequest("POST", "/admin/api/sessions/x/kill", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: token})
	r.Header.Set(CSRFHeader, token)
	if !s.verifyCSRF(r) {
		t.Fatal("matching cookie+header must verify")
	}
}

func TestVerifyCSRF_MissingHeader(t *testing.T) {
	s := newCSRFServer(t)
	token := strings.Repeat("c", csrfTokenLen*2)
	r := httptest.NewRequest("POST", "/admin/api/sessions/x/kill", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: token})
	if s.verifyCSRF(r) {
		t.Fatal("must reject when header missing")
	}
}

func TestVerifyCSRF_Mismatch(t *testing.T) {
	s := newCSRFServer(t)
	r := httptest.NewRequest("POST", "/admin/api/sessions/x/kill", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: strings.Repeat("d", csrfTokenLen*2)})
	r.Header.Set(CSRFHeader, strings.Repeat("e", csrfTokenLen*2))
	if s.verifyCSRF(r) {
		t.Fatal("must reject mismatched cookie/header")
	}
}

func TestRequireCSRF_Rejects403(t *testing.T) {
	s := newCSRFServer(t)
	called := false
	wrapped := s.requireCSRF(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/admin/api/sessions/x/kill", nil)
	wrapped(w, r)
	if called {
		t.Fatal("downstream handler must not run")
	}
	if w.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Result().StatusCode)
	}
}
