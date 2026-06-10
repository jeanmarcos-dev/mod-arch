package server

import (
	"strings"
	"testing"
)

func TestGravatarURL(t *testing.T) {
	cases := []struct {
		in     string
		expect string
	}{
		{"", ""},
		// Canonical Gravatar example: md5("johndoe@example.com").
		{"johndoe@example.com", "https://www.gravatar.com/avatar/"},
		{"  JOHNDOE@example.com  ", "https://www.gravatar.com/avatar/"},
	}
	for _, c := range cases {
		got := gravatarURL(c.in)
		if c.expect == "" {
			if got != "" {
				t.Errorf("gravatarURL(%q) = %q, want empty", c.in, got)
			}
			continue
		}
		if !strings.HasPrefix(got, c.expect) {
			t.Errorf("gravatarURL(%q) = %q, want prefix %q", c.in, got, c.expect)
		}
		if !strings.Contains(got, "identicon") {
			t.Errorf("gravatarURL(%q) missing identicon fallback: %q", c.in, got)
		}
	}
	// Case-insensitive: the same email with different casing must produce the
	// same URL (Gravatar specification).
	a := gravatarURL("Test@Example.com")
	b := gravatarURL("test@example.com")
	if a != b {
		t.Errorf("case-insensitive: %q != %q", a, b)
	}
}

func TestTrimTo(t *testing.T) {
	if got := trimTo("  hola  ", 10); got != "hola" {
		t.Errorf("trim+space: got %q", got)
	}
	long := strings.Repeat("a", 1000)
	if got := trimTo(long, 5); got != "aaaaa" {
		t.Errorf("truncation: got %q (len %d)", got, len(got))
	}
	// UTF-8 rune-aware truncation (not byte-aware).
	utf := strings.Repeat("ñ", 8)
	if got := trimTo(utf, 4); got != "ññññ" {
		t.Errorf("UTF-8 truncation: got %q", got)
	}
}

func TestTrimSocialHandle(t *testing.T) {
	cases := []struct{ in, out string }{
		{"foo", "foo"},
		{"@foo", "foo"},
		{"https://github.com/foo", "foo"},
		{"http://www.github.com/foo/", "foo"},
		{"linkedin.com/in/jm", "jm"},
		{"https://linkedin.com/in/jm/", "jm"},
	}
	for _, c := range cases {
		if got := trimSocialHandle(c.in, 80); got != c.out {
			t.Errorf("trimSocialHandle(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}
