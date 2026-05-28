package i18n

import (
	"net/http"
	"strings"
)

// LangCookie is the cookie name the SPA writes when the user toggles the
// language in Preferences. Server templates and JSON errors read from it.
const LangCookie = "lang"

// ResolveLang inspects the request for the active locale, in this order:
//   1. lang cookie (set by the SPA when the user picks an idiom).
//   2. Accept-Language header (best-effort: only the first language tag).
//   3. DefaultLocale (es).
// Unknown values fall back to DefaultLocale rather than returning the raw
// header — we never want the templates to render with a bogus key.
func ResolveLang(r *http.Request) string {
	if c, err := r.Cookie(LangCookie); err == nil {
		if isSupported(c.Value) {
			return c.Value
		}
	}
	if h := r.Header.Get("Accept-Language"); h != "" {
		// Crude parse: first token before ';' or ','.
		parts := strings.FieldsFunc(h, func(r rune) bool { return r == ',' || r == ';' })
		for _, p := range parts {
			tag := strings.ToLower(strings.TrimSpace(p))
			// Trim quality factor if present (e.g. "en-US;q=0.9" → "en-us").
			tag = strings.SplitN(tag, "-", 2)[0]
			if isSupported(tag) {
				return tag
			}
		}
	}
	return DefaultLocale
}

func isSupported(s string) bool {
	for _, l := range SupportedLocales {
		if s == l {
			return true
		}
	}
	return false
}

// SetLangCookie writes the lang cookie with a one-year lifetime so the
// preference survives reloads and incognito-to-normal transitions.
func SetLangCookie(w http.ResponseWriter, lang string) {
	if !isSupported(lang) {
		lang = DefaultLocale
	}
	http.SetCookie(w, &http.Cookie{
		Name:     LangCookie,
		Value:    lang,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}
