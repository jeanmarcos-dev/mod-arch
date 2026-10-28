package server

import (
	"crypto/md5" //nolint:gosec // md5 is required by the Gravatar spec
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jeanmarcos-dev/mod-arch/internal/server/i18n"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

// Field length budgets. Long enough for real values, short enough that a
// pasted novel does not bloat the row or the response payload.
const (
	maxCohortRunes   = 80
	maxInstRunes     = 120
	maxCareerRunes   = 120
	maxBioRunes      = 500
	maxHandleRunes   = 80
	maxLanguageRunes = 16
)

// profileJSON is the wire shape for the profile endpoints. AvatarURL is
// computed server-side from the user's email (Gravatar) so the SPA does not
// need to hash anything client-side.
type profileJSON struct {
	UserID            string `json:"user_id"`
	Name              string `json:"name"`
	Email             string `json:"email,omitempty"`
	AvatarURL         string `json:"avatar_url,omitempty"`
	Cohort            string `json:"cohort"`
	Institution       string `json:"institution"`
	Career            string `json:"career"`
	Bio               string `json:"bio"`
	GitHub            string `json:"github"`
	LinkedIn          string `json:"linkedin"`
	PreferredLanguage string `json:"preferred_language"`
	UpdatedAt         int64  `json:"updated_at,omitempty"`
}

// handleProfileGet returns the caller's own profile, merging the stored
// fields with the canonical name/email from the users table. Missing
// profiles return zero-valued fields rather than 404 so the SPA renders an
// empty form on first visit.
func (s *Server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		i18n.WriteError(w, r, http.StatusUnauthorized, i18n.CodeAuthUnauthenticated, nil)
		return
	}
	st := s.manager.Store()
	if st == nil {
		i18n.WriteError(w, r, http.StatusServiceUnavailable, i18n.CodeAuthPersistenceDisabled, nil)
		return
	}
	resp := profileFromUser(user)
	if p, err := st.GetProfile(r.Context(), user.ID); err == nil {
		mergeProfileFields(&resp, p)
	} else if !errors.Is(err, store.ErrNotFound) {
		s.logger.Warn("profile: get failed", "user", user.ID, "err", err)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleProfilePut validates and persists the caller's own profile fields.
// Identity fields (name, email, internal id) are not editable here; that
// lives in the admin Users panel to keep the upsert semantics simple.
func (s *Server) handleProfilePut(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		i18n.WriteError(w, r, http.StatusUnauthorized, i18n.CodeAuthUnauthenticated, nil)
		return
	}
	st := s.manager.Store()
	if st == nil {
		i18n.WriteError(w, r, http.StatusServiceUnavailable, i18n.CodeAuthPersistenceDisabled, nil)
		return
	}
	var body struct {
		Cohort            string `json:"cohort"`
		Institution       string `json:"institution"`
		Career            string `json:"career"`
		Bio               string `json:"bio"`
		GitHub            string `json:"github"`
		LinkedIn          string `json:"linkedin"`
		PreferredLanguage string `json:"preferred_language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		i18n.WriteError(w, r, http.StatusBadRequest, i18n.CodeAuthInvalidJSON, nil)
		return
	}
	p := store.UserProfile{
		UserID:            user.ID,
		Cohort:            trimTo(body.Cohort, maxCohortRunes),
		Institution:       trimTo(body.Institution, maxInstRunes),
		Career:            trimTo(body.Career, maxCareerRunes),
		Bio:               trimTo(body.Bio, maxBioRunes),
		GitHub:            trimSocialHandle(body.GitHub, maxHandleRunes),
		LinkedIn:          trimSocialHandle(body.LinkedIn, maxHandleRunes),
		PreferredLanguage: trimTo(body.PreferredLanguage, maxLanguageRunes),
		UpdatedAt:         time.Now().Unix(),
	}
	if err := st.UpsertProfile(r.Context(), p); err != nil {
		s.logger.Warn("profile: upsert failed", "user", user.ID, "err", err)
		i18n.WriteError(w, r, http.StatusInternalServerError, i18n.CodeAuthInternalError, nil)
		return
	}
	resp := profileFromUser(user)
	mergeProfileFields(&resp, &p)
	writeJSON(w, http.StatusOK, resp)
}

// handleAdminProfileGet exposes any user's profile to moderators+. Used by
// the admin user-activity view to enrich the report with cohort, institution,
// etc. The endpoint never reveals an email that the user did not provide.
func (s *Server) handleAdminProfileGet(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistencia deshabilitada"})
		return
	}
	id := r.PathValue("id")
	user, err := st.GetUser(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "usuario no encontrado"})
		return
	}
	resp := profileFromUser(user)
	if p, err := st.GetProfile(r.Context(), id); err == nil {
		mergeProfileFields(&resp, p)
	}
	writeJSON(w, http.StatusOK, resp)
}

func profileFromUser(u *store.UserRow) profileJSON {
	return profileJSON{
		UserID:    u.ID,
		Name:      u.Name,
		Email:     u.Email,
		AvatarURL: gravatarURL(u.Email),
	}
}

func mergeProfileFields(dst *profileJSON, p *store.UserProfile) {
	dst.Cohort = p.Cohort
	dst.Institution = p.Institution
	dst.Career = p.Career
	dst.Bio = p.Bio
	dst.GitHub = p.GitHub
	dst.LinkedIn = p.LinkedIn
	dst.PreferredLanguage = p.PreferredLanguage
	dst.UpdatedAt = p.UpdatedAt
}

// gravatarURL returns the canonical Gravatar URL for an email, or "" when no
// email is on file. We default to the "identicon" fallback so users without a
// Gravatar account still get a stable visual.
func gravatarURL(email string) string {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" {
		return ""
	}
	sum := md5.Sum([]byte(e)) //nolint:gosec
	return "https://www.gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?d=identicon&s=160"
}

// trimTo enforces a UTF-8 rune cap while preserving leading/trailing
// whitespace removal in one pass.
func trimTo(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:maxRunes]))
}

// trimSocialHandle removes a leading '@', protocol prefixes and whitespace so
// the stored value is always the bare handle/path. github.com URLs paste
// fine: "https://github.com/foo" → "foo".
func trimSocialHandle(s string, maxRunes int) string {
	v := strings.TrimSpace(s)
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	v = strings.TrimPrefix(v, "www.")
	for _, prefix := range []string{"github.com/", "linkedin.com/in/", "linkedin.com/"} {
		v = strings.TrimPrefix(v, prefix)
	}
	v = strings.TrimPrefix(v, "@")
	v = strings.TrimRight(v, "/")
	return trimTo(v, maxRunes)
}
