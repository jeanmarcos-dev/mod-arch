package server

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func serveSEO(t *testing.T, path, host string, https bool) *httptest.ResponseRecorder {
	t.Helper()
	t.Setenv("ALLOWED_ORIGINS", "https://mod-arch.example")
	mux := http.NewServeMux()
	New(Config{Logger: discardLogger()}).Routes(mux)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	if https {
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d, want %d", path, rec.Code, http.StatusOK)
	}
	return rec
}

func TestRobotsTxt(t *testing.T) {
	rec := serveSEO(t, "/robots.txt", "mod-arch.example", true)

	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if lines[0] != "User-agent: *" {
		t.Errorf("first line = %q, want %q", lines[0], "User-agent: *")
	}
	for _, p := range []string{"/admin", "/login", "/perfil", "/preferencias", "/sesiones", "/full", "/v/"} {
		if !slices.Contains(lines, "Disallow: "+p) {
			t.Errorf("missing Disallow: %s", p)
		}
	}
	if !slices.Contains(lines, "Sitemap: https://mod-arch.example/sitemap.xml") {
		t.Errorf("missing absolute Sitemap line in:\n%s", rec.Body.String())
	}
}

func TestSitemapXML(t *testing.T) {
	tests := []struct {
		name  string
		host  string
		https bool
		base  string
	}{
		{name: "https behind proxy", host: "mod-arch.example", https: true, base: "https://mod-arch.example"},
		{name: "plain http", host: "mod-arch.example", base: "http://mod-arch.example"},
		{name: "spoofed host falls back to the allowed origin", host: "evil.example", https: true, base: "https://mod-arch.example"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveSEO(t, "/sitemap.xml", tc.host, tc.https)

			if got := rec.Header().Get("Content-Type"); got != "application/xml; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			body := rec.Body.String()
			if !strings.HasPrefix(body, xml.Header) {
				t.Errorf("body does not start with the XML declaration:\n%s", body)
			}
			var doc struct {
				XMLName xml.Name
				URLs    []struct {
					Loc string `xml:"loc"`
				} `xml:"url"`
			}
			if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
				t.Fatalf("invalid XML: %v\n%s", err, body)
			}
			if doc.XMLName.Space != "http://www.sitemaps.org/schemas/sitemap/0.9" || doc.XMLName.Local != "urlset" {
				t.Errorf("root = %+v, want sitemap urlset", doc.XMLName)
			}
			var got []string
			for _, u := range doc.URLs {
				got = append(got, u.Loc)
			}
			var want []string
			for _, p := range []string{"/", "/procesos", "/memoria", "/interrupciones", "/concurrencia", "/x86", "/glosario"} {
				want = append(want, tc.base+p)
			}
			if !slices.Equal(got, want) {
				t.Errorf("locs = %v\nwant %v", got, want)
			}
		})
	}
}
