package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

// CSRFCookie is read by the SPA and echoed back in the X-CSRF-Token header on
// every mutating request — the classic double-submit pattern. The cookie is
// intentionally NOT HttpOnly so client code can read it.
const CSRFCookie = "mc_csrf"

// CSRFHeader carries the cookie value back to the server. It must be present
// on POST/PUT/DELETE for routes wrapped in requireCSRF.
const CSRFHeader = "X-CSRF-Token"

const csrfTokenLen = 32 // 32 bytes → 64 hex chars

// ensureCSRFCookie writes a fresh token cookie when the caller does not
// already have one. Existing tokens are reused so concurrent tabs share the
// same value (otherwise a second tab would overwrite the first tab's cookie
// and break the original tab's pending requests).
func (s *Server) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CSRFCookie); err == nil && len(c.Value) == csrfTokenLen*2 {
		return
	}
	var buf [csrfTokenLen]byte
	if _, err := rand.Read(buf[:]); err != nil {
		s.logger.Warn("csrf: rand.Read failed", "err", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookie,
		Value:    hex.EncodeToString(buf[:]),
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

// verifyCSRF checks that the cookie and header match in constant time.
// Missing cookie OR missing header counts as a failure.
func (s *Server) verifyCSRF(r *http.Request) bool {
	c, err := r.Cookie(CSRFCookie)
	if err != nil || len(c.Value) != csrfTokenLen*2 {
		return false
	}
	header := r.Header.Get(CSRFHeader)
	if len(header) != csrfTokenLen*2 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(header)) == 1
}

// requireCSRF rejects requests whose CSRF cookie and header do not match.
// Returns 403 with a JSON body the SPA can display.
func (s *Server) requireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.verifyCSRF(r) {
			s.logger.Info("csrf check failed", "ip", clientIP(r), "path", r.URL.Path)
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error": "verificación CSRF fallida, recargue la página",
			})
			return
		}
		next(w, r)
	}
}
