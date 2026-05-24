package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Embedder converts text into a dense vector. Implementations may batch input
// to reduce HTTP overhead; callers should not assume per-call latency is
// linear in len(texts).
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Model() string
	Dim() int
}

// Sentinel errors. Mirrors internal/llm so the tutor can branch on the same
// failure modes regardless of which sub-system blew up.
var (
	ErrEmbedderAuthFailed  = errors.New("rag/embedder: authentication failed")
	ErrEmbedderRateLimited = errors.New("rag/embedder: rate limited")
	ErrEmbedderServer      = errors.New("rag/embedder: server error")
	ErrEmbedderBadRequest  = errors.New("rag/embedder: bad request")
)

// JinaDefaultModel is the multilingual embedding model used by default.
// 1024 dimensions, 8K context, strong on Spanish technical text.
const JinaDefaultModel = "jina-embeddings-v3"

// JinaEndpoint is the v1 embeddings URL.
const JinaEndpoint = "https://api.jina.ai/v1/embeddings"

// JinaConfig configures a Jina embedder.
type JinaConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Dim     int // override only if you change Model to a non-1024 variant
	HTTP    *http.Client
	// Task tells Jina what the embedding is for. "retrieval.passage" when
	// indexing the corpus, "retrieval.query" when embedding a user question.
	Task string
}

// Jina is the default Embedder backed by the Jina v1 embeddings API.
type Jina struct {
	apiKey  string
	model   string
	baseURL string
	dim     int
	http    *http.Client
	task    string
}

// NewJina builds a Jina embedder. APIKey is required.
func NewJina(cfg JinaConfig) (*Jina, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("rag/embedder: API key required")
	}
	if cfg.Model == "" {
		cfg.Model = JinaDefaultModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = JinaEndpoint
	}
	if cfg.Dim == 0 {
		cfg.Dim = 1024
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Task == "" {
		cfg.Task = "retrieval.passage"
	}
	return &Jina{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: cfg.BaseURL,
		dim:     cfg.Dim,
		http:    cfg.HTTP,
		task:    cfg.Task,
	}, nil
}

func (j *Jina) Model() string { return j.model }
func (j *Jina) Dim() int      { return j.dim }

// jinaRequest mirrors the JSON the Jina API expects. Task is optional but
// useful: it tells the model whether the input is a passage being indexed
// or a query being looked up, which improves recall noticeably.
type jinaRequest struct {
	Model string   `json:"model"`
	Task  string   `json:"task,omitempty"`
	Input []string `json:"input"`
}

type jinaResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed batches all inputs into a single HTTP call. Order of the returned
// slice matches the order of texts.
func (j *Jina) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	payload := jinaRequest{Model: j.model, Task: j.task, Input: texts}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("rag/embedder: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("rag/embedder: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.apiKey)

	resp, err := j.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rag/embedder: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("rag/embedder: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, classifyEmbedderError(resp.StatusCode, respBody)
	}

	var raw jinaResponse
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("rag/embedder: decode: %w", err)
	}
	if len(raw.Data) != len(texts) {
		return nil, fmt.Errorf("rag/embedder: expected %d embeddings, got %d", len(texts), len(raw.Data))
	}

	out := make([][]float32, len(texts))
	for _, item := range raw.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("rag/embedder: response index %d out of range", item.Index)
		}
		out[item.Index] = item.Embedding
	}
	return out, nil
}

func classifyEmbedderError(status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrEmbedderAuthFailed, msg)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrEmbedderRateLimited, msg)
	case status >= 500:
		return fmt.Errorf("%w (%d): %s", ErrEmbedderServer, status, msg)
	default:
		return fmt.Errorf("%w (%d): %s", ErrEmbedderBadRequest, status, msg)
	}
}

// HashQuery returns the sha256 hex of a normalized query. The retriever uses
// this both as a cache key and to avoid double-embedding the same question.
func HashQuery(q string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(q))))
	return hex.EncodeToString(h[:])
}
