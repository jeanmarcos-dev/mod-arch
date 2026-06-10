package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
)

// ClientCtx is the per-request snapshot of the client identity that gets
// folded into each Session at creation. Captured by middleware so handlers
// and the session manager pull it from context.Context instead of re-parsing.
type ClientCtx struct {
	IPHash       string // SHA-256(salt || ip), truncated hex
	IPPlain      string // raw IP; only populated when ADMIN_REVEAL_IP=true
	UAShort      string // "Firefox 122 / Linux", "bot/Googlebot", etc.
	GeoCountry   string // ISO 3166-1 alpha-2 (or "")
	ReferrerHost string // host portion of Referer header (or "")
}

type clientCtxKey struct{}

// WithClientCtx is an HTTP middleware that captures IP, UA, referrer and an
// optional geo lookup, stuffing them into request context.Context. Wire once
// at the outermost layer of the mux.
func WithClientCtx(salt []byte, geo GeoResolver, revealIP bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			cc := &ClientCtx{
				IPHash:       hashIP(salt, ip),
				UAShort:      parseUAShort(r.Header.Get("User-Agent")),
				ReferrerHost: extractReferrerHost(r.Header.Get("Referer")),
			}
			if revealIP {
				cc.IPPlain = ip
			}
			if geo != nil {
				cc.GeoCountry = geo.Country(ip)
			}
			r = r.WithContext(context.WithValue(r.Context(), clientCtxKey{}, cc))
			next.ServeHTTP(w, r)
		})
	}
}

// ClientCtxFrom retrieves the captured client context from a request. Returns
// an empty (non-nil) struct when the middleware did not run, so callers can
// always safely read fields.
func ClientCtxFrom(r *http.Request) *ClientCtx {
	if v, ok := r.Context().Value(clientCtxKey{}).(*ClientCtx); ok && v != nil {
		return v
	}
	return &ClientCtx{}
}

// hashIP returns a deterministic, salted, truncated SHA-256 of the IP.
// Same input + salt → same hash, so admins can see "this hash created N
// sessions" without seeing the IP. Truncation to 16 bytes (32 hex chars)
// keeps storage compact; collisions are not a privacy concern here.
func hashIP(salt []byte, ip string) string {
	if ip == "" {
		return ""
	}
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(ip))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// parseUAShort returns a stable, low-cardinality summary of the User-Agent
// suitable for analytics tables. The format is "<Browser> <Major> / <OS>";
// known bots get "bot/<name>"; empty/unparseable input returns "unknown".
//
// This is deliberately tiny and approximate. UA detection is a rabbit hole and
// MODARCH only needs the rough shape for analytics, not pixel-perfect
// fingerprinting.
func parseUAShort(ua string) string {
	if ua == "" {
		return "unknown"
	}
	lower := strings.ToLower(ua)
	if name := matchBot(lower); name != "" {
		return "bot/" + name
	}
	os := detectOS(lower)
	browser, version := detectBrowser(ua, lower)
	if browser == "" {
		return "unknown"
	}
	if version != "" {
		return browser + " " + version + " / " + os
	}
	return browser + " / " + os
}

func matchBot(lower string) string {
	for _, b := range []struct{ needle, name string }{
		{"googlebot", "Googlebot"},
		{"bingbot", "Bingbot"},
		{"duckduckbot", "DuckDuckBot"},
		{"baiduspider", "Baiduspider"},
		{"yandexbot", "YandexBot"},
		{"applebot", "Applebot"},
		{"facebookexternalhit", "FacebookBot"},
		{"twitterbot", "Twitterbot"},
		{"slackbot", "Slackbot"},
		{"discordbot", "Discordbot"},
		{"telegrambot", "Telegrambot"},
		{"curl/", "curl"},
		{"wget/", "wget"},
		{"python-requests", "python-requests"},
		{"go-http-client", "go-http-client"},
	} {
		if strings.Contains(lower, b.needle) {
			return b.name
		}
	}
	return ""
}

func detectOS(lower string) string {
	switch {
	case strings.Contains(lower, "android"):
		return "Android"
	case strings.Contains(lower, "iphone"), strings.Contains(lower, "ipad"), strings.Contains(lower, "ipod"):
		return "iOS"
	case strings.Contains(lower, "mac os x"), strings.Contains(lower, "macintosh"):
		return "macOS"
	case strings.Contains(lower, "windows"):
		return "Windows"
	case strings.Contains(lower, "linux"):
		return "Linux"
	case strings.Contains(lower, "cros"):
		return "ChromeOS"
	case strings.Contains(lower, "freebsd"):
		return "FreeBSD"
	}
	return "other"
}

// detectBrowser extracts a coarse browser family + major version. Order
// matters: Edge/OPR/Brave must be checked before Chrome (they include "Chrome"
// in their UA); Chrome before Safari (Safari is the catch-all). Returns
// ("", "") for unrecognized agents.
func detectBrowser(ua, lower string) (string, string) {
	type rule struct {
		family string
		token  string // token preceding the version number
	}
	rules := []rule{
		{"Edge", "edg/"},
		{"Opera", "opr/"},
		{"Brave", "brave/"},
		{"Vivaldi", "vivaldi/"},
		{"Firefox", "firefox/"},
		{"Chrome", "chrome/"},
		{"Safari", "version/"},
	}
	for _, r := range rules {
		if idx := strings.Index(lower, r.token); idx >= 0 {
			return r.family, extractMajor(ua[idx+len(r.token):])
		}
	}
	return "", ""
}

func extractMajor(rest string) string {
	end := 0
	for end < len(rest) && (rest[end] >= '0' && rest[end] <= '9') {
		end++
	}
	if end == 0 {
		return ""
	}
	if n, err := strconv.Atoi(rest[:end]); err == nil {
		return strconv.Itoa(n)
	}
	return ""
}

// extractReferrerHost returns the host of a Referer header, or "" when absent
// or unparseable. Same-origin referrers (host equals the server's own host)
// are not filtered here; callers can compare with their own host if needed.
func extractReferrerHost(referer string) string {
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// GeoResolver returns an ISO 3166-1 alpha-2 country code for an IP, or "" when
// the resolver cannot determine one. A nil resolver is treated as "off".
type GeoResolver interface {
	Country(ip string) string
}

// NoopGeoResolver is the zero-cost default when geo lookup is disabled.
type NoopGeoResolver struct{}

func (NoopGeoResolver) Country(string) string { return "" }

// RevealIPEnabled reports whether plain IPs should be captured into the
// store. The toggle is intentionally env-only: a privacy decision the
// operator owns at deploy time, not a setting an admin can flip later.
func RevealIPEnabled() bool {
	return strings.EqualFold(os.Getenv("ADMIN_REVEAL_IP"), "true")
}

// clientInfoFrom adapts the per-request ClientCtx into the sessions.ClientInfo
// the manager understands. Lifted to a helper so every session-create call
// site stays a single line.
func clientInfoFrom(r *http.Request) sessions.ClientInfo {
	cc := ClientCtxFrom(r)
	return sessions.ClientInfo{
		IPHash:       cc.IPHash,
		IPPlain:      cc.IPPlain,
		UAShort:      cc.UAShort,
		GeoCountry:   cc.GeoCountry,
		ReferrerHost: cc.ReferrerHost,
	}
}
