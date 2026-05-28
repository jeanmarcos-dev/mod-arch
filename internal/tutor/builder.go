package tutor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/llm"
	"github.com/jeanmarcos/modos-cpu/internal/rag"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// BuildDeps groups the long-lived dependencies the builder needs. Mirrored
// from main.go so /admin/api/tutor/reload can rebuild the service without
// re-reading flags or env vars beyond Config + DB.
type BuildDeps struct {
	Store         *store.Store
	Logger        *slog.Logger
	CorpusVersion int    // 0 = auto-detect from settings
	EncKey        [32]byte
	TopK          int
	MaxAnswerTok  int
	RateLimitMax  int
}

// Build wires up a fresh tutor.Service from a Config. Returns nil + an
// explanatory error when the config is incomplete (missing API key, no
// corpus indexed, etc.) so the caller can log a single line and disable
// the tutor surface without crashing the server.
func Build(ctx context.Context, deps BuildDeps, cfg Config) (*Service, error) {
	if deps.Store == nil {
		return nil, errors.New("tutor/build: store required")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if !cfg.Enabled {
		// Master switch off: report disabled without flagging it as an error.
		// The caller leaves the registry empty and the WS handler reports
		// "tutor desactivado por el administrador".
		deps.Logger.Info("tutor disabled by config (tutor.enabled=false)")
		return nil, nil
	}
	provider, err := buildLLM(cfg, deps.Logger)
	if err != nil {
		return nil, err
	}
	embedder, err := buildEmbedder(cfg, deps.Logger)
	if err != nil {
		return nil, err
	}

	corpusVersion := deps.CorpusVersion
	if corpusVersion <= 0 {
		corpusVersion = readActiveCorpus(ctx, deps.Store)
	}
	if corpusVersion <= 0 {
		return nil, errors.New("no indexed corpus found; run modos-rag-index first")
	}

	rs := rag.NewStore(deps.Store.DB())
	retriever := rag.NewRetriever(rs, embedder, corpusVersion)
	loadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := retriever.Reload(loadCtx); err != nil {
		return nil, fmt.Errorf("retriever reload: %w", err)
	}
	deps.Logger.Info("tutor enabled",
		"provider", provider.Name(),
		"corpus_version", corpusVersion,
		"chunks_loaded", retriever.Len(),
	)

	topK := deps.TopK
	if topK <= 0 {
		topK = 5
	}
	maxTok := deps.MaxAnswerTok
	if maxTok <= 0 {
		maxTok = 600
	}
	rlMax := deps.RateLimitMax
	if rlMax <= 0 {
		rlMax = 20
	}

	svc := New(retriever, rs, provider, deps.Logger, Options{
		CorpusVersion:      corpusVersion,
		TopK:               topK,
		MaxAnswerTokens:    maxTok,
		RateLimitMax:       rlMax,
		HistoryWindowTurns: cfg.HistoryWindowTurns,
	})
	svc.SetMetricsStore(NewMetricsStore(deps.Store.DB()))
	svc.SetDefaultMode(cfg.Mode)

	if cfg.LLMRPM > 0 {
		interval := time.Minute / time.Duration(cfg.LLMRPM)
		bucket := llm.NewTokenBucket(cfg.LLMRPM, interval)
		svc.SetRateBucket(bucket)
		deps.Logger.Info("llm rate limiter", "rpm", cfg.LLMRPM, "interval", interval)
	}
	return svc, nil
}

// buildLLM picks the chat backend from cfg.LLMProvider. The provider value
// is normalized to lower-case by Apply()/MergeEnv before reaching here.
func buildLLM(cfg Config, logger *slog.Logger) (llm.Provider, error) {
	provider := cfg.LLMProvider
	if provider == "" {
		provider = "gemini"
	}
	switch provider {
	case "gemini":
		if cfg.GeminiAPIKey == "" {
			return nil, errors.New("GEMINI_API_KEY no configurada")
		}
		return llm.NewGemini(llm.GeminiConfig{APIKey: cfg.GeminiAPIKey, Model: cfg.LLMModel, BaseURL: cfg.LLMBaseURL})
	case "deepseek":
		if cfg.DeepSeekAPIKey == "" {
			return nil, errors.New("DEEPSEEK_API_KEY no configurada")
		}
		return llm.NewDeepSeek(llm.DeepSeekConfig{APIKey: cfg.DeepSeekAPIKey, Model: cfg.LLMModel, BaseURL: cfg.LLMBaseURL})
	case "groq":
		if cfg.GroqAPIKey == "" {
			return nil, errors.New("GROQ_API_KEY no configurada")
		}
		return llm.NewGroq(llm.GroqConfig{APIKey: cfg.GroqAPIKey, Model: cfg.LLMModel, BaseURL: cfg.LLMBaseURL})
	case "openai_compat":
		if cfg.OpenAICompatKey == "" {
			return nil, errors.New("LLM_API_KEY no configurada para openai_compat")
		}
		if cfg.LLMBaseURL == "" {
			return nil, errors.New("LLM_BASE_URL requerido para openai_compat")
		}
		return llm.NewOpenAICompat(llm.OpenAICompatConfig{
			APIKey: cfg.OpenAICompatKey, Model: cfg.LLMModel, BaseURL: cfg.LLMBaseURL, ProviderName: "openai_compat",
		})
	default:
		return nil, fmt.Errorf("LLM provider desconocido: %q", provider)
	}
}

// buildEmbedder mirrors buildLLM for the embedding side.
func buildEmbedder(cfg Config, logger *slog.Logger) (rag.Embedder, error) {
	provider := cfg.EmbedderProvider
	if provider == "" {
		provider = "ollama"
	}
	switch provider {
	case "ollama":
		host := cfg.OllamaHost
		if host == "" {
			host = rag.OllamaDefaultURL
		}
		model := cfg.OllamaEmbedModel
		if model == "" {
			model = rag.OllamaDefaultModel
		}
		return rag.NewOllama(rag.OllamaConfig{BaseURL: host, Model: model})
	case "jina":
		if cfg.JinaAPIKey == "" {
			return nil, errors.New("JINA_API_KEY no configurada")
		}
		return rag.NewJina(rag.JinaConfig{APIKey: cfg.JinaAPIKey, Task: "retrieval.query"})
	default:
		return nil, fmt.Errorf("EMBEDDER_PROVIDER desconocido: %q", provider)
	}
}

func readActiveCorpus(ctx context.Context, st *store.Store) int {
	row := st.DB().QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'rag.corpus_version'`)
	var val string
	if err := row.Scan(&val); err != nil {
		return 0
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return n
}
