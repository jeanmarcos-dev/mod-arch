package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaDefaultModel is the embedding model used when none is specified.
// bge-m3 is the practical sweet spot: 1024 dims (matches what the rest of
// the pipeline expects), strong multilingual coverage, runs in ~1.2 GB.
const OllamaDefaultModel = "bge-m3"

// OllamaDefaultURL is the host of a local Ollama daemon.
const OllamaDefaultURL = "http://localhost:11434"

// OllamaConfig configures a local Ollama-backed embedder. Set BaseURL when
// Ollama runs on another box; everything else has a default.
type OllamaConfig struct {
	BaseURL string
	Model   string
	Dim     int
	HTTP    *http.Client
}

// Ollama is an Embedder that talks to a self-hosted Ollama daemon. No API
// keys, no token quotas. The trade-off is the user owns the daemon and the
// model (typically ~1 GB on disk).
type Ollama struct {
	baseURL string
	model   string
	dim     int
	http    *http.Client
}

// NewOllama builds an Ollama embedder. The daemon is not contacted here;
// failures surface on the first Embed call.
func NewOllama(cfg OllamaConfig) (*Ollama, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = OllamaDefaultURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Model == "" {
		cfg.Model = OllamaDefaultModel
	}
	if cfg.Dim == 0 {
		cfg.Dim = 1024
	}
	if cfg.HTTP == nil {
		// Local calls are fast but a stuck daemon should not hang the indexer.
		cfg.HTTP = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Ollama{baseURL: cfg.BaseURL, model: cfg.Model, dim: cfg.Dim, http: cfg.HTTP}, nil
}

func (o *Ollama) Model() string { return o.model }
func (o *Ollama) Dim() int      { return o.dim }

// ollamaEmbedRequest mirrors the /api/embed schema (the newer batched API,
// supported since Ollama 0.3.x). Falls back to /api/embeddings (single input)
// is not implemented here because all Ollama versions that ship a working
// daemon today support /api/embed.
type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
	// Older daemons returned "embedding" (singular). We tolerate both.
	Embedding []float32 `json:"embedding"`
}

// Embed runs the batched request. Ollama processes inputs sequentially under
// the hood, so the timing scales linearly with len(texts); call sites use
// modest batch sizes (~16) to keep memory pressure bounded.
func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(ollamaEmbedRequest{Model: o.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("rag/embedder/ollama: marshal: %w", err)
	}
	url := o.baseURL + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("rag/embedder/ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rag/embedder/ollama: do (is Ollama running at %s?): %w", o.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("rag/embedder/ollama: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(respBody))
		switch {
		case resp.StatusCode >= 500:
			return nil, fmt.Errorf("%w (status %d): %s", ErrEmbedderServer, resp.StatusCode, msg)
		case resp.StatusCode == http.StatusNotFound:
			return nil, fmt.Errorf("%w (status %d): modelo %q no disponible — corra `ollama pull %s`", ErrEmbedderBadRequest, resp.StatusCode, o.model, o.model)
		default:
			return nil, fmt.Errorf("%w (status %d): %s", ErrEmbedderBadRequest, resp.StatusCode, msg)
		}
	}

	var raw ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("rag/embedder/ollama: decode: %w", err)
	}
	if len(raw.Embeddings) == 0 && len(raw.Embedding) > 0 {
		// Older daemon shape: one input → one "embedding".
		if len(texts) != 1 {
			return nil, fmt.Errorf("rag/embedder/ollama: daemon returned single embedding but %d inputs were requested; upgrade Ollama", len(texts))
		}
		return [][]float32{raw.Embedding}, nil
	}
	if len(raw.Embeddings) != len(texts) {
		return nil, fmt.Errorf("rag/embedder/ollama: expected %d embeddings, got %d", len(texts), len(raw.Embeddings))
	}
	return raw.Embeddings, nil
}
