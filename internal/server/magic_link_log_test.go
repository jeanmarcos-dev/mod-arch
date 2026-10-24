package server

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

func TestIssueMagicLink_DoesNotLogTokenOrURL(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	s := New(Config{Manager: sessions.NewManager(sessions.Config{Store: st, Logger: logger}), Logger: logger})
	defer s.Close()

	req := httptest.NewRequest("POST", "https://modarch.example/api/auth/login", nil)
	url, _ := s.issueMagicLink(req, "maria@example.com")
	if url == "" {
		t.Fatal("issueMagicLink returned no URL")
	}
	token := url[strings.Index(url, "t=")+2:]

	out := logs.String()
	for _, leaked := range []string{token, "auth/magic", "url=", "maria@example.com"} {
		if strings.Contains(out, leaked) {
			t.Errorf("log leaks %q:\n%s", leaked, out)
		}
	}
	for _, kept := range []string{"magic link", "m***@example.com", "expires_in=15m0s"} {
		if !strings.Contains(out, kept) {
			t.Errorf("log lost %q:\n%s", kept, out)
		}
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct{ in, want string }{
		{"maria@example.com", "m***@example.com"},
		{"ñandú@uni.edu.ve", "ñ***@uni.edu.ve"},
		{"a@b.co", "a***@b.co"},
		{"sin-arroba", "***"},
		{"@dominio.com", "***"},
		{"", "***"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := maskEmail(tc.in); got != tc.want {
				t.Fatalf("maskEmail(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
