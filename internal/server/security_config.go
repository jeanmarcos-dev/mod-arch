package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jeanmarcos-dev/mod-arch/internal/crypto"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

// Settings keys for the security namespace. The sitekey is public (lands in
// the SPA bootstrap config) while the secret is sealed at rest using the
// session secret as the encryption key.
const (
	settingTurnstileSitekey = "security.turnstile_sitekey"
	settingTurnstileSecret  = "security.turnstile_secret"
)

// SecurityConfig is the decrypted view of the at-rest security settings. An
// empty TurnstileSecret means the verification step is bypassed (dev/local).
type SecurityConfig struct {
	TurnstileSitekey string
	TurnstileSecret  string
}

// loadSecurityConfig reads both settings and decrypts the secret. Empty
// fields fall back to TURNSTILE_SITEKEY / TURNSTILE_SECRET so a fresh deploy
// configured via `.env` works without first touching the admin panel.
func loadSecurityConfig(ctx context.Context, st *store.Store, encKey [32]byte) (SecurityConfig, error) {
	var cfg SecurityConfig
	if v, err := st.GetSetting(ctx, settingTurnstileSitekey); err == nil {
		cfg.TurnstileSitekey = v
	} else if !errors.Is(err, store.ErrNotFound) {
		return cfg, fmt.Errorf("security: read sitekey: %w", err)
	}

	sealed, err := st.GetSetting(ctx, settingTurnstileSecret)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return cfg, fmt.Errorf("security: read secret: %w", err)
	}
	if sealed != "" {
		plain, err := crypto.Open(sealed, encKey)
		if err != nil {
			return cfg, fmt.Errorf("security: decrypt secret: %w", err)
		}
		cfg.TurnstileSecret = plain
	}

	if cfg.TurnstileSitekey == "" {
		cfg.TurnstileSitekey = strings.TrimSpace(os.Getenv("TURNSTILE_SITEKEY"))
	}
	if cfg.TurnstileSecret == "" {
		cfg.TurnstileSecret = strings.TrimSpace(os.Getenv("TURNSTILE_SECRET"))
	}
	return cfg, nil
}

// SecurityUpdate is a partial patch. Nil fields mean "leave alone". Setting
// a pointer to an empty string clears that field.
type SecurityUpdate struct {
	TurnstileSitekey *string `json:"turnstile_sitekey,omitempty"`
	TurnstileSecret  *string `json:"turnstile_secret,omitempty"`
}

// saveSecurityConfig writes the patched fields, encrypting the secret.
func saveSecurityConfig(ctx context.Context, st *store.Store, encKey [32]byte, patch SecurityUpdate, actor string, nowUnix int64) error {
	if patch.TurnstileSitekey != nil {
		if err := st.SetSetting(ctx, settingTurnstileSitekey, *patch.TurnstileSitekey, actor, nowUnix); err != nil {
			return fmt.Errorf("security: write sitekey: %w", err)
		}
	}
	if patch.TurnstileSecret != nil {
		sealed, err := crypto.Seal(*patch.TurnstileSecret, encKey)
		if err != nil {
			return fmt.Errorf("security: seal secret: %w", err)
		}
		if err := st.SetSetting(ctx, settingTurnstileSecret, sealed, actor, nowUnix); err != nil {
			return fmt.Errorf("security: write secret: %w", err)
		}
	}
	return nil
}
