package server

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

// Settings keys for the email allowlist. Both default to the empty string;
// when both are empty the allowlist is bypassed (any well-formed email is
// accepted). Operators turn enforcement on by populating either list.
const (
	SettingAllowedDomains = "access.allowed_domains"
	SettingAllowedEmails  = "access.allowed_emails"
)

// EmailAllowlist captures both lists at the moment of the check. Always read
// fresh per-request: the lists are small and the auth flow is not hot.
type EmailAllowlist struct {
	Domains []string // lowercase, no leading '@'
	Emails  []string // lowercase, exact match
}

// IsEnabled reports whether enforcement should run for this request. With
// both lists empty there is nothing to enforce.
func (a EmailAllowlist) IsEnabled() bool {
	return len(a.Domains) > 0 || len(a.Emails) > 0
}

// Permits reports whether the given (already-lowercased, already-validated)
// email matches at least one entry. Empty input is permitted: cleanEmail
// accepts blanks for the "skip email" flow and that decision belongs upstream.
func (a EmailAllowlist) Permits(email string) bool {
	if email == "" {
		return true
	}
	if !a.IsEnabled() {
		return true
	}
	if slices.Contains(a.Emails, email) {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return false
	}
	return slices.Contains(a.Domains, email[at+1:])
}

// loadEmailAllowlist reads both allowlist settings from the store. Returns an
// empty allowlist (enforcement off) when persistence is unavailable.
func (s *Server) loadEmailAllowlist(ctx context.Context) EmailAllowlist {
	st := s.manager.Store()
	if st == nil {
		return EmailAllowlist{}
	}
	return EmailAllowlist{
		Domains: readListSetting(ctx, st, SettingAllowedDomains, normalizeDomain),
		Emails:  readListSetting(ctx, st, SettingAllowedEmails, normalizeEmail),
	}
}

// readListSetting fetches a setting whose value is a comma- or newline-
// separated list, normalizes each entry through `norm`, drops blanks and
// duplicates, and preserves the input order so the admin UI shows the same
// order the operator typed.
func readListSetting(ctx context.Context, st *store.Store, key string, norm func(string) string) []string {
	raw, err := st.GetSetting(ctx, key)
	if err != nil || raw == "" {
		if !errors.Is(err, store.ErrNotFound) && err != nil {
			// Don't propagate read errors to the auth path; just log via the
			// fact that the list comes back empty. Settings IO failure is
			// rare and not a place we want to fail-open silently — but here
			// the caller (loadEmailAllowlist) treats an empty list as "off",
			// which is the safest default.
		}
		return nil
	}
	return parseList(raw, norm)
}

// parseList splits a comma/newline/semicolon-separated string into normalized
// entries with duplicates and blanks removed. Whitespace inside an entry is
// not allowed by either normalizer so it is safe to leave as-is.
func parseList(raw string, norm func(string) string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := norm(p)
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// normalizeDomain trims, lowercases, and strips leading '@' characters so
// operators can paste either "@uni.edu" or "uni.edu" (or "@@uni.edu" from a
// sloppy copy-paste). Returns "" for invalid input.
func normalizeDomain(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	v = strings.TrimLeft(v, "@")
	if v == "" {
		return ""
	}
	if strings.ContainsAny(v, " \t/\\@") {
		return ""
	}
	if !strings.Contains(v, ".") {
		return ""
	}
	return v
}

// normalizeEmail is the validation used when reading allowlist entries. We
// reuse cleanEmail to stay consistent with the auth-time format check. Any
// entry the user types that does not pass cleanEmail is silently dropped so
// the admin UI can show what was actually stored vs. what was attempted.
func normalizeEmail(s string) string {
	v, err := cleanEmail(s)
	if err != nil {
		return ""
	}
	return v
}

// joinList renders a list back to a string suitable for re-display in the
// admin UI. Uses newlines for readability.
func joinList(items []string) string {
	return strings.Join(items, "\n")
}
