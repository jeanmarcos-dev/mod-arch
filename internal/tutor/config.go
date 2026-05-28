package tutor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/crypto"
)

// Settings keys persisted in the `settings` table. Prefix keeps tutor entries
// from clashing with other subsystems.
const (
	cfgEnabled            = "tutor.enabled"              // "true" | "false"; false desactiva todo el tutor
	cfgLLMProvider        = "tutor.llm_provider"         // gemini | deepseek | groq | openai_compat
	cfgLLMModel           = "tutor.llm_model"            // override del default del provider
	cfgLLMBaseURL         = "tutor.llm_base_url"         // solo para openai_compat
	cfgLLMRPM             = "tutor.llm_rpm"              // rate limit por minuto, 0 desactiva
	cfgMode               = "tutor.mode"                 // direct | socratic (global, controlado por admin)
	cfgEmbedderProvider   = "tutor.embedder_provider"    // ollama | jina
	cfgOllamaHost         = "tutor.ollama_host"
	cfgOllamaEmbedModel   = "tutor.ollama_embed_model"
	cfgHistoryWindowTurns = "tutor.history_window_turns" // 0..10; 0 desactiva la memoria multi-turno

	// API keys (encrypted at rest with crypto.Seal).
	cfgKeyGemini       = "tutor.api_key.gemini"
	cfgKeyDeepSeek     = "tutor.api_key.deepseek"
	cfgKeyGroq         = "tutor.api_key.groq"
	cfgKeyOpenAICompat = "tutor.api_key.openai_compat"
	cfgKeyJina         = "tutor.api_key.jina"
)

// Config carries the full runtime configuration the tutor needs. Empty
// strings mean "use env var / default" so the admin can persist only the
// fields they care about. API keys are stored decrypted in this struct;
// callers must not log or serialize Config directly — use Mask() for that.
type Config struct {
	Enabled          bool   // master switch; false desactiva el tutor entero
	LLMProvider      string
	LLMModel         string
	LLMBaseURL       string
	LLMRPM           int
	Mode             string // direct | socratic
	EmbedderProvider string
	OllamaHost       string
	OllamaEmbedModel string
	// HistoryWindowTurns is the number of prior student/tutor turns the
	// service replays to the LLM. -1 means "unset" so MergeEnv knows to
	// apply the default (3); 0 is a valid kill-switch.
	HistoryWindowTurns int

	GeminiAPIKey    string
	DeepSeekAPIKey  string
	GroqAPIKey      string
	OpenAICompatKey string
	JinaAPIKey      string
}

// MaskedConfig is the safe-to-render version of Config: every API key is
// replaced by either "" (when absent) or "••••••XYZW" where XYZW are the
// last four characters. The boolean Has* flags let the UI show "set" vs
// "unset" without leaking the actual value.
type MaskedConfig struct {
	Enabled            bool   `json:"enabled"`
	LLMProvider        string `json:"llm_provider"`
	LLMModel           string `json:"llm_model"`
	LLMBaseURL         string `json:"llm_base_url"`
	LLMRPM             int    `json:"llm_rpm"`
	Mode               string `json:"mode"`
	EmbedderProvider   string `json:"embedder_provider"`
	OllamaHost         string `json:"ollama_host"`
	OllamaEmbedModel   string `json:"ollama_embed_model"`
	HistoryWindowTurns int    `json:"history_window_turns"`

	GeminiAPIKey      string `json:"gemini_api_key"`
	HasGeminiKey      bool   `json:"has_gemini_key"`
	DeepSeekAPIKey    string `json:"deepseek_api_key"`
	HasDeepSeekKey    bool   `json:"has_deepseek_key"`
	GroqAPIKey        string `json:"groq_api_key"`
	HasGroqKey        bool   `json:"has_groq_key"`
	OpenAICompatKey   string `json:"openai_compat_api_key"`
	HasOpenAICompatKey bool  `json:"has_openai_compat_key"`
	JinaAPIKey        string `json:"jina_api_key"`
	HasJinaKey        bool   `json:"has_jina_key"`
}

// Mask returns the masked view safe to send through the admin API.
func (c Config) Mask() MaskedConfig {
	mask := func(k string) (masked string, has bool) {
		if k == "" {
			return "", false
		}
		if len(k) <= 4 {
			return "••••", true
		}
		return "••••••" + k[len(k)-4:], true
	}
	m := MaskedConfig{
		Enabled:            c.Enabled,
		LLMProvider:        c.LLMProvider,
		LLMModel:           c.LLMModel,
		LLMBaseURL:         c.LLMBaseURL,
		LLMRPM:             c.LLMRPM,
		Mode:               c.Mode,
		EmbedderProvider:   c.EmbedderProvider,
		OllamaHost:         c.OllamaHost,
		OllamaEmbedModel:   c.OllamaEmbedModel,
		HistoryWindowTurns: c.HistoryWindowTurns,
	}
	m.GeminiAPIKey, m.HasGeminiKey = mask(c.GeminiAPIKey)
	m.DeepSeekAPIKey, m.HasDeepSeekKey = mask(c.DeepSeekAPIKey)
	m.GroqAPIKey, m.HasGroqKey = mask(c.GroqAPIKey)
	m.OpenAICompatKey, m.HasOpenAICompatKey = mask(c.OpenAICompatKey)
	m.JinaAPIKey, m.HasJinaKey = mask(c.JinaAPIKey)
	return m
}

// settingsReader is the narrow contract LoadConfig needs from the store.
// Defined as an interface so tests can supply a stub.
type settingsReader interface {
	GetSetting(ctx context.Context, key string) (string, error)
}

// settingsWriter is the narrow contract SaveConfig needs.
type settingsWriter interface {
	SetSetting(ctx context.Context, key, value, updatedBy string, ts int64) error
}

// LoadConfig reads the tutor config from the settings table, decrypting
// each API key. Missing rows are not errors: they leave the corresponding
// field empty so the caller can fall back to env vars.
func LoadConfig(ctx context.Context, r settingsReader, encKey [32]byte) (Config, error) {
	if r == nil {
		return Config{}, nil
	}
	get := func(key string) string {
		v, err := r.GetSetting(ctx, key)
		if err != nil {
			return ""
		}
		return v
	}
	openKey := func(stored string) string {
		if stored == "" {
			return ""
		}
		plain, err := crypto.Open(stored, encKey)
		if err != nil {
			return "" // treat unreadable rows as absent rather than failing the whole load
		}
		return plain
	}

	c := Config{
		Enabled:            parseBoolDefault(get(cfgEnabled), true),
		LLMProvider:        get(cfgLLMProvider),
		LLMModel:           get(cfgLLMModel),
		LLMBaseURL:         get(cfgLLMBaseURL),
		Mode:               get(cfgMode),
		EmbedderProvider:   get(cfgEmbedderProvider),
		OllamaHost:         get(cfgOllamaHost),
		OllamaEmbedModel:   get(cfgOllamaEmbedModel),
		HistoryWindowTurns: -1, // "unset" sentinel; MergeEnv fills in the default

		GeminiAPIKey:    openKey(get(cfgKeyGemini)),
		DeepSeekAPIKey:  openKey(get(cfgKeyDeepSeek)),
		GroqAPIKey:      openKey(get(cfgKeyGroq)),
		OpenAICompatKey: openKey(get(cfgKeyOpenAICompat)),
		JinaAPIKey:      openKey(get(cfgKeyJina)),
	}
	if v := get(cfgLLMRPM); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.LLMRPM = n
		}
	}
	if v := get(cfgHistoryWindowTurns); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.HistoryWindowTurns = n
		}
	}
	return c, nil
}

// SaveConfig writes every field of cfg to settings. Empty strings are
// persisted as empty rows (caller-side overwrite of a previous value).
// Pass updatedBy for the audit trail (the actor's user ID or "admin").
func SaveConfig(ctx context.Context, w settingsWriter, encKey [32]byte, cfg Config, updatedBy string) error {
	if w == nil {
		return errors.New("tutor/config: no writer")
	}
	now := time.Now().Unix()
	put := func(key, value string) error {
		return w.SetSetting(ctx, key, value, updatedBy, now)
	}
	putKey := func(key, raw string) error {
		sealed, err := crypto.Seal(raw, encKey)
		if err != nil {
			return fmt.Errorf("seal %s: %w", key, err)
		}
		return put(key, sealed)
	}
	enabled := "false"
	if cfg.Enabled {
		enabled = "true"
	}
	steps := []func() error{
		func() error { return put(cfgEnabled, enabled) },
		func() error { return put(cfgLLMProvider, cfg.LLMProvider) },
		func() error { return put(cfgLLMModel, cfg.LLMModel) },
		func() error { return put(cfgLLMBaseURL, cfg.LLMBaseURL) },
		func() error { return put(cfgLLMRPM, strconv.Itoa(cfg.LLMRPM)) },
		func() error { return put(cfgMode, cfg.Mode) },
		func() error { return put(cfgEmbedderProvider, cfg.EmbedderProvider) },
		func() error { return put(cfgOllamaHost, cfg.OllamaHost) },
		func() error { return put(cfgOllamaEmbedModel, cfg.OllamaEmbedModel) },
		func() error { return put(cfgHistoryWindowTurns, strconv.Itoa(cfg.HistoryWindowTurns)) },
		func() error { return putKey(cfgKeyGemini, cfg.GeminiAPIKey) },
		func() error { return putKey(cfgKeyDeepSeek, cfg.DeepSeekAPIKey) },
		func() error { return putKey(cfgKeyGroq, cfg.GroqAPIKey) },
		func() error { return putKey(cfgKeyOpenAICompat, cfg.OpenAICompatKey) },
		func() error { return putKey(cfgKeyJina, cfg.JinaAPIKey) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// Update is a partial-update overlay: any non-nil pointer in `patch`
// replaces the corresponding field in `base`. Strings use "" as "leave
// unchanged" except when the patch explicitly sends the empty value via
// `Clear*` booleans. API keys follow the same convention so the admin can
// rotate a single key without resending the others.
type Update struct {
	Enabled            *bool   `json:"enabled,omitempty"`
	LLMProvider        *string `json:"llm_provider,omitempty"`
	LLMModel           *string `json:"llm_model,omitempty"`
	LLMBaseURL         *string `json:"llm_base_url,omitempty"`
	LLMRPM             *int    `json:"llm_rpm,omitempty"`
	Mode               *string `json:"mode,omitempty"`
	EmbedderProvider   *string `json:"embedder_provider,omitempty"`
	OllamaHost         *string `json:"ollama_host,omitempty"`
	OllamaEmbedModel   *string `json:"ollama_embed_model,omitempty"`
	HistoryWindowTurns *int    `json:"history_window_turns,omitempty"`

	GeminiAPIKey    *string `json:"gemini_api_key,omitempty"` // nil = no change, "" = clear, "X" = set
	DeepSeekAPIKey  *string `json:"deepseek_api_key,omitempty"`
	GroqAPIKey      *string `json:"groq_api_key,omitempty"`
	OpenAICompatKey *string `json:"openai_compat_api_key,omitempty"`
	JinaAPIKey      *string `json:"jina_api_key,omitempty"`
}

// Apply merges patch on top of base, returning the new config.
func (base Config) Apply(patch Update) Config {
	out := base
	if patch.Enabled != nil {
		out.Enabled = *patch.Enabled
	}
	if patch.LLMProvider != nil {
		out.LLMProvider = strings.ToLower(strings.TrimSpace(*patch.LLMProvider))
	}
	if patch.LLMModel != nil {
		out.LLMModel = strings.TrimSpace(*patch.LLMModel)
	}
	if patch.LLMBaseURL != nil {
		out.LLMBaseURL = strings.TrimSpace(*patch.LLMBaseURL)
	}
	if patch.LLMRPM != nil {
		out.LLMRPM = *patch.LLMRPM
	}
	if patch.Mode != nil {
		out.Mode = strings.ToLower(strings.TrimSpace(*patch.Mode))
	}
	if patch.EmbedderProvider != nil {
		out.EmbedderProvider = strings.ToLower(strings.TrimSpace(*patch.EmbedderProvider))
	}
	if patch.OllamaHost != nil {
		out.OllamaHost = strings.TrimSpace(*patch.OllamaHost)
	}
	if patch.OllamaEmbedModel != nil {
		out.OllamaEmbedModel = strings.TrimSpace(*patch.OllamaEmbedModel)
	}
	if patch.HistoryWindowTurns != nil {
		n := *patch.HistoryWindowTurns
		if n < 0 {
			n = 0
		}
		if n > 10 {
			n = 10
		}
		out.HistoryWindowTurns = n
	}
	if patch.GeminiAPIKey != nil {
		out.GeminiAPIKey = strings.TrimSpace(*patch.GeminiAPIKey)
	}
	if patch.DeepSeekAPIKey != nil {
		out.DeepSeekAPIKey = strings.TrimSpace(*patch.DeepSeekAPIKey)
	}
	if patch.GroqAPIKey != nil {
		out.GroqAPIKey = strings.TrimSpace(*patch.GroqAPIKey)
	}
	if patch.OpenAICompatKey != nil {
		out.OpenAICompatKey = strings.TrimSpace(*patch.OpenAICompatKey)
	}
	if patch.JinaAPIKey != nil {
		out.JinaAPIKey = strings.TrimSpace(*patch.JinaAPIKey)
	}
	return out
}

// EncKeyFromSecret derives the at-rest cipher key from SESSION_SECRET (the
// same env var used elsewhere). Centralized here so the admin handler does
// not have to know about the crypto package directly.
func EncKeyFromSecret(secret string) [32]byte {
	return crypto.DeriveKey([]byte(secret))
}

// withEnvDefaults applies fallback defaults that depend on env (without
// overriding what was already loaded from settings).
//
// MergeEnv overlays environment variables onto the persisted config: any
// field still empty after LoadConfig falls back to its env var so a fresh
// deployment without admin-saved settings keeps working from `.env`.
func MergeEnv(cfg Config, env func(string) string) Config {
	pick := func(cur, envVar string) string {
		if cur != "" {
			return cur
		}
		return strings.TrimSpace(env(envVar))
	}
	cfg.LLMProvider = pick(cfg.LLMProvider, "LLM_PROVIDER")
	cfg.LLMModel = pick(cfg.LLMModel, "LLM_MODEL")
	cfg.LLMBaseURL = pick(cfg.LLMBaseURL, "LLM_BASE_URL")
	if cfg.LLMRPM == 0 {
		if v := strings.TrimSpace(env("LLM_RPM")); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				cfg.LLMRPM = n
			}
		}
	}
	cfg.EmbedderProvider = pick(cfg.EmbedderProvider, "EMBEDDER_PROVIDER")
	cfg.OllamaHost = pick(cfg.OllamaHost, "OLLAMA_HOST")
	cfg.OllamaEmbedModel = pick(cfg.OllamaEmbedModel, "OLLAMA_EMBED_MODEL")

	// HistoryWindowTurns: -1 means "no admin row, no env" → apply the default
	// of 3 prior turns. Explicit 0 from the admin stays as 0 (kill-switch).
	if cfg.HistoryWindowTurns < 0 {
		if v := strings.TrimSpace(env("TUTOR_HISTORY_WINDOW_TURNS")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				cfg.HistoryWindowTurns = n
			}
		}
	}
	if cfg.HistoryWindowTurns < 0 {
		cfg.HistoryWindowTurns = 3
	}
	if cfg.HistoryWindowTurns > 10 {
		cfg.HistoryWindowTurns = 10
	}

	cfg.GeminiAPIKey = pick(cfg.GeminiAPIKey, "GEMINI_API_KEY")
	cfg.DeepSeekAPIKey = pick(cfg.DeepSeekAPIKey, "DEEPSEEK_API_KEY")
	cfg.GroqAPIKey = pick(cfg.GroqAPIKey, "GROQ_API_KEY")
	cfg.OpenAICompatKey = pick(cfg.OpenAICompatKey, "LLM_API_KEY")
	cfg.JinaAPIKey = pick(cfg.JinaAPIKey, "JINA_API_KEY")

	// Filter placeholder values so they do not "look" set.
	cfg.GeminiAPIKey = clearPlaceholder(cfg.GeminiAPIKey)
	cfg.DeepSeekAPIKey = clearPlaceholder(cfg.DeepSeekAPIKey)
	cfg.GroqAPIKey = clearPlaceholder(cfg.GroqAPIKey)
	cfg.OpenAICompatKey = clearPlaceholder(cfg.OpenAICompatKey)
	cfg.JinaAPIKey = clearPlaceholder(cfg.JinaAPIKey)

	if cfg.LLMProvider == "" {
		cfg.LLMProvider = "gemini" // matches main.go fallback
	}
	if cfg.Mode == "" {
		cfg.Mode = "direct"
	}
	if cfg.EmbedderProvider == "" && cfg.OllamaHost != "" {
		cfg.EmbedderProvider = "ollama"
	}
	return cfg
}

// parseBoolDefault interprets common true/false strings; anything unrecognized
// (including the empty string from a missing settings row) returns def. This
// keeps a brand-new database — where tutor.enabled has never been written —
// reporting as enabled by default.
func parseBoolDefault(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return def
	}
}

func clearPlaceholder(s string) string {
	if strings.HasPrefix(s, "gsk_PLACEHOLDER") || strings.HasPrefix(s, "jina_PLACEHOLDER") ||
		strings.HasPrefix(s, "PLACEHOLDER") {
		return ""
	}
	return s
}

// DBHandle is the minimal DB interface SaveConfig needs from internal/store.
// The store package satisfies it via *sql.DB; tests can pass a stub.
type DBHandle = *sql.DB
