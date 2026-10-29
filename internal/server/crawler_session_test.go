package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestCrawlersDoNotOccupySessionSlots(t *testing.T) {
	const (
		googlebot = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
		browser   = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	)
	tests := []struct {
		name        string
		path        string
		ua          string
		wantStatus  int
		wantSession bool
	}{
		{name: "crawler gets the html shell", path: "/procesos", ua: googlebot, wantStatus: http.StatusOK},
		{name: "crawler is refused by the session api", path: "/api/session", ua: googlebot, wantStatus: http.StatusForbidden},
		{name: "browser html creates a session", path: "/procesos", ua: browser, wantStatus: http.StatusOK, wantSession: true},
		{name: "browser session api creates a session", path: "/api/session", ua: browser, wantStatus: http.StatusOK, wantSession: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newLiveFixture(t)
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("User-Agent", tc.ua)
			rec := httptest.NewRecorder()
			f.mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d\n%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if full, _ := f.mgr.Count(); (full > 0) != tc.wantSession {
				t.Errorf("public sessions = %d, want session created = %v", full, tc.wantSession)
			}
			hasCookie := slices.ContainsFunc(rec.Result().Cookies(), func(c *http.Cookie) bool {
				return c.Name == SessionCookie
			})
			if hasCookie != tc.wantSession {
				t.Errorf("%s cookie set = %v, want %v", SessionCookie, hasCookie, tc.wantSession)
			}
		})
	}
}
