package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHashIP(t *testing.T) {
	salt := []byte("test-salt-for-hashing")
	a := hashIP(salt, "192.0.2.1")
	b := hashIP(salt, "192.0.2.1")
	c := hashIP(salt, "192.0.2.2")
	d := hashIP([]byte("different-salt"), "192.0.2.1")

	if a != b {
		t.Fatalf("hashIP not deterministic: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("different IPs collided: %q", a)
	}
	if a == d {
		t.Fatalf("different salts collided: %q", a)
	}
	if got := hashIP(salt, ""); got != "" {
		t.Fatalf("hashIP(\"\") = %q, want empty", got)
	}
	if len(a) != 32 {
		t.Fatalf("hashIP length = %d, want 32", len(a))
	}
}

func TestParseUAShort(t *testing.T) {
	cases := []struct {
		ua       string
		contains string
	}{
		{"", "unknown"},
		{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36", "Chrome 130 / Linux"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.2849.46", "Edge 130 / Windows"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "Safari 17 / macOS"},
		{"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:122.0) Gecko/20100101 Firefox/122.0", "Firefox 122 / Linux"},
		{"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "bot/Googlebot"},
		{"curl/8.5.0", "bot/curl"},
		{"go-http-client/1.1", "bot/go-http-client"},
	}
	for _, c := range cases {
		got := parseUAShort(c.ua)
		if !strings.Contains(got, c.contains) {
			t.Errorf("parseUAShort(%q) = %q, want to contain %q", c.ua, got, c.contains)
		}
	}
}

func TestExtractReferrerHost(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"", ""},
		{"https://example.com/path?q=1", "example.com"},
		{"http://example.com", "example.com"},
		{"https://sub.domain.co.uk:8080/x", "sub.domain.co.uk"},
	}
	for _, c := range cases {
		if got := extractReferrerHost(c.in); got != c.out {
			t.Errorf("extractReferrerHost(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestWithClientCtx_HashOnly(t *testing.T) {
	salt := []byte("salt")
	var captured *ClientCtx
	handler := WithClientCtx(salt, NoopGeoResolver{}, false, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = ClientCtxFrom(r)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux) Firefox/122.0")
	req.Header.Set("Referer", "https://example.com/")
	req.Header.Set("X-Forwarded-For", "203.0.113.42")
	req.RemoteAddr = "10.0.0.1:1234"

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured == nil {
		t.Fatal("ClientCtx not captured")
	}
	if captured.IPHash == "" {
		t.Error("IPHash empty")
	}
	if captured.IPPlain != "" {
		t.Errorf("IPPlain should be empty when reveal off, got %q", captured.IPPlain)
	}
	if !strings.Contains(captured.UAShort, "Firefox") {
		t.Errorf("UAShort missing browser family, got %q", captured.UAShort)
	}
	if captured.ReferrerHost != "example.com" {
		t.Errorf("ReferrerHost = %q, want example.com", captured.ReferrerHost)
	}
}

func TestWithClientCtx_RevealOn(t *testing.T) {
	var captured *ClientCtx
	handler := WithClientCtx([]byte("salt"), nil, true, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = ClientCtxFrom(r)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured.IPPlain == "" {
		t.Error("IPPlain empty when reveal on")
	}
	if captured.IPHash == "" {
		t.Error("IPHash still expected when reveal on")
	}
}

func TestClientCtxFrom_NoMiddleware(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	cc := ClientCtxFrom(req)
	if cc == nil {
		t.Fatal("ClientCtxFrom returned nil")
	}
	if cc.IPHash != "" || cc.UAShort != "" {
		t.Error("expected zero-valued ClientCtx when middleware did not run")
	}
}

func TestCloudflareGeoResolver(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "venezuela", header: "VE", want: "VE"},
		{name: "lowercase is normalized", header: "ve", want: "VE"},
		{name: "unknown country XX is ignored", header: "XX", want: ""},
		{name: "tor T1 is ignored", header: "T1", want: ""},
		{name: "missing header", header: "", want: ""},
		{name: "three letters rejected", header: "VEN", want: ""},
		{name: "digits rejected", header: "1A", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tc.header != "" {
				req.Header.Set("CF-IPCountry", tc.header)
			}
			if got := (CloudflareGeoResolver{}).Country(req); got != tc.want {
				t.Fatalf("Country = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWithClientCtx_CapturesCloudflareCountry(t *testing.T) {
	var captured *ClientCtx
	handler := WithClientCtx([]byte("salt"), CloudflareGeoResolver{}, false, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = ClientCtxFrom(r)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("CF-IPCountry", "VE")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if captured == nil || captured.GeoCountry != "VE" {
		t.Fatalf("GeoCountry = %+v, want VE", captured)
	}
	if got := clientInfoFrom(req.WithContext(context.WithValue(req.Context(), clientCtxKey{}, captured))).GeoCountry; got != "VE" {
		t.Fatalf("clientInfoFrom country = %q, want VE", got)
	}
}
