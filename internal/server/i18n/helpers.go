package i18n

import (
	"encoding/json"
	"net/http"
)

// WriteError emits a JSON response of the form
//   {"error_code": "<code>", "error": "<translated>"}.
// The frontend prefers error_code (so it can re-translate against its own
// catalog) and uses error only as a fallback for legacy clients or codes
// missing from the SPA catalog.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code string, params map[string]string) {
	lang := ResolveLang(r)
	translated := T(lang, code, params)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error_code": code,
		"error":      translated,
	})
}

// WriteErrorAlong is the variant for handlers that need to merge additional
// fields next to the standard error_code/error pair. Callers pass the extra
// fields as a map; the helper makes sure they do not clobber the canonical
// keys.
func WriteErrorAlong(w http.ResponseWriter, r *http.Request, status int, code string, params map[string]string, extra map[string]any) {
	lang := ResolveLang(r)
	translated := T(lang, code, params)
	out := map[string]any{
		"error_code": code,
		"error":      translated,
	}
	for k, v := range extra {
		if k == "error" || k == "error_code" {
			continue
		}
		out[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(out)
}

// HTTPError writes a plain-text response for endpoints that historically used
// http.Error. Same locale resolution as WriteError.
func HTTPError(w http.ResponseWriter, r *http.Request, status int, code string, params map[string]string) {
	lang := ResolveLang(r)
	http.Error(w, T(lang, code, params), status)
}
