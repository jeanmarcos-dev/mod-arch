package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// UserProfile is the extended identity associated with a UserRow. Created
// lazily on the first PUT to /api/profile/me; absent profiles return
// ErrNotFound from GetProfile.
type UserProfile struct {
	UserID            string
	Cohort            string
	Institution       string
	Career            string
	Bio               string
	GitHub            string
	LinkedIn          string
	PreferredLanguage string
	UpdatedAt         int64
}

// GetProfile fetches the profile for a user. Returns ErrNotFound when no row
// exists; callers should treat that as "empty profile, render empty form".
func (s *Store) GetProfile(ctx context.Context, userID string) (*UserProfile, error) {
	if userID == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx, `
        SELECT user_id, cohort, institution, career, bio, github, linkedin,
               preferred_language, updated_at
        FROM user_profiles WHERE user_id = ?`, userID)
	var p UserProfile
	if err := row.Scan(&p.UserID, &p.Cohort, &p.Institution, &p.Career, &p.Bio,
		&p.GitHub, &p.LinkedIn, &p.PreferredLanguage, &p.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get profile: %w", err)
	}
	return &p, nil
}

// UpsertProfile inserts or fully replaces the profile row for a user. The
// UpdatedAt field on the input is honored so tests can pin a deterministic
// timestamp. Callers are responsible for trimming/validating field contents.
func (s *Store) UpsertProfile(ctx context.Context, p UserProfile) error {
	if p.UserID == "" {
		return errors.New("store: upsert profile: user_id required")
	}
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO user_profiles(user_id, cohort, institution, career, bio,
                                  github, linkedin, preferred_language, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(user_id) DO UPDATE SET
            cohort = excluded.cohort,
            institution = excluded.institution,
            career = excluded.career,
            bio = excluded.bio,
            github = excluded.github,
            linkedin = excluded.linkedin,
            preferred_language = excluded.preferred_language,
            updated_at = excluded.updated_at`,
		p.UserID, p.Cohort, p.Institution, p.Career, p.Bio,
		p.GitHub, p.LinkedIn, p.PreferredLanguage, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: upsert profile: %w", err)
	}
	return nil
}
