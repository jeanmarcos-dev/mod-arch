package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider endpoints and default models. The chat completions schema is
// uniform across all these providers; only base URL and model differ, so the
// same OpenAICompat client speaks to every one of them.
const (
	GroqEndpoint     = "https://api.groq.com/openai/v1/chat/completions"
	DeepSeekEndpoint = "https://api.deepseek.com/v1/chat/completions"
	GeminiEndpoint   = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
	OpenAIEndpoint   = "https://api.openai.com/v1/chat/completions"

	GroqDefaultModel     = "llama-3.3-70b-versatile"
	DeepSeekDefaultModel = "deepseek-chat"
	// GeminiDefaultModel is Flash-Lite, not Flash, because Flash-Lite's free
	// tier (15 RPM / 1000 RPD) is 4× more generous than Flash (10 RPM /
	// 250 RPD). Quality is good enough for tutorial-style Q&A with tools;
	// override with LLM_MODEL=gemini-3.5-flash when reasoning matters more.
	GeminiDefaultModel = "gemini-3.5-flash-lite"
)

// OpenAICompatConfig is the construction parameter set shared by every
// OpenAI-compatible provider (Groq, DeepSeek, Gemini's compat endpoint,
// OpenRouter, a self-hosted vLLM, …).
type OpenAICompatConfig struct {
	APIKey       string
	Model        string
	BaseURL      string
	ProviderName string // free-form label returned by Name()
	HTTP         *http.Client
}

// OpenAICompat talks to any OpenAI-compatible /chat/completions endpoint.
// All providers we target follow the same wire schema, so one struct covers
// them all with no per-provider branching.
type OpenAICompat struct {
	apiKey       string
	model        string
	baseURL      string
	providerName string
	http         *http.Client
}

// NewOpenAICompat builds a generic provider. ProviderName, BaseURL and APIKey
// are required; Model defaults to whatever the server picks if left empty
// (most providers ignore "" and use their account default).
func NewOpenAICompat(cfg OpenAICompatConfig) (*OpenAICompat, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("llm: API key required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("llm: base URL required")
	}
	if cfg.ProviderName == "" {
		cfg.ProviderName = "openai_compat"
	}
	httpClient := cfg.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &OpenAICompat{
		apiKey:       cfg.APIKey,
		model:        cfg.Model,
		baseURL:      cfg.BaseURL,
		providerName: cfg.ProviderName,
		http:         httpClient,
	}, nil
}

// Name implements Provider.
func (c *OpenAICompat) Name() string { return c.providerName }

// Chat implements Provider. Sends the request synchronously and returns
// the first choice's content. For streaming, use ChatStream.
func (c *OpenAICompat) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	body, err := json.Marshal(buildOAIRequest(req))
	if err != nil {
		return nil, fmt.Errorf("llm/%s: marshal request: %w", c.providerName, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm/%s: build request: %w", c.providerName, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm/%s: do request: %w", c.providerName, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("llm/%s: read body: %w", c.providerName, err)
	}

	if resp.StatusCode != http.StatusOK {
		// Dump the offending request body on 4xx so misconfigured tools
		// or messages surface in the logs instead of an opaque 400.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			snippet := string(body)
			if len(snippet) > 2000 {
				snippet = snippet[:2000] + "...[truncated]"
			}
			return nil, fmt.Errorf("%w | request: %s", classifyHTTPError(resp.StatusCode, respBody), snippet)
		}
		return nil, classifyHTTPError(resp.StatusCode, respBody)
	}

	var raw oaiChatResponse
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("llm/%s: decode response: %w", c.providerName, err)
	}
	if len(raw.Choices) == 0 {
		return nil, ErrEmptyContent
	}

	choice := raw.Choices[0]
	out := &ChatResponse{
		Content:      choice.Message.Content,
		Model:        raw.Model,
		FinishReason: choice.FinishReason,
		Usage: Usage{
			PromptTokens:     raw.Usage.PromptTokens,
			CompletionTokens: raw.Usage.CompletionTokens,
			TotalTokens:      raw.Usage.TotalTokens,
		},
	}
	for _, tc := range choice.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	if len(out.ToolCalls) == 0 && strings.TrimSpace(out.Content) == "" {
		return nil, ErrEmptyContent
	}
	return out, nil
}

// ChatStream implements the Streamer capability over an SSE endpoint. Every
// supported provider follows the same OpenAI streaming convention, so one
// decoder handles them all.
func (c *OpenAICompat) ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	wire := buildOAIRequest(req)
	body, err := json.Marshal(streamingWrapper{oaiChatRequest: wire})
	if err != nil {
		return nil, fmt.Errorf("llm/%s: marshal stream request: %w", c.providerName, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm/%s: build stream request: %w", c.providerName, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm/%s: do stream: %w", c.providerName, err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		return nil, classifyHTTPError(resp.StatusCode, errBody)
	}

	events := make(chan StreamEvent, 16)
	reader := NewStreamReader(events, func() { _ = resp.Body.Close() })

	go decodeOAISSE(resp.Body, events, reader)
	return reader, nil
}

// ── Helper constructors per provider ─────────────────────────────────────

// GroqConfig kept as the old shape so existing callers compile unchanged.
type GroqConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// NewGroq returns an OpenAICompat preconfigured for Groq's free tier.
func NewGroq(cfg GroqConfig) (*OpenAICompat, error) {
	model := cfg.Model
	if model == "" {
		model = GroqDefaultModel
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = GroqEndpoint
	}
	return NewOpenAICompat(OpenAICompatConfig{
		APIKey: cfg.APIKey, Model: model, BaseURL: baseURL,
		ProviderName: "groq", HTTP: cfg.HTTP,
	})
}

// DeepSeekConfig configures the DeepSeek provider.
type DeepSeekConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// NewDeepSeek returns an OpenAICompat preconfigured for DeepSeek's API.
func NewDeepSeek(cfg DeepSeekConfig) (*OpenAICompat, error) {
	model := cfg.Model
	if model == "" {
		model = DeepSeekDefaultModel
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DeepSeekEndpoint
	}
	return NewOpenAICompat(OpenAICompatConfig{
		APIKey: cfg.APIKey, Model: model, BaseURL: baseURL,
		ProviderName: "deepseek", HTTP: cfg.HTTP,
	})
}

// GeminiConfig configures the Google Gemini provider via its OpenAI-compat
// endpoint. Gemini accepts the same Authorization: Bearer header and the
// same chat completions schema, so it slots in with no special handling.
type GeminiConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// NewGemini returns an OpenAICompat preconfigured for Gemini Flash-Lite.
func NewGemini(cfg GeminiConfig) (*OpenAICompat, error) {
	model := cfg.Model
	if model == "" {
		model = GeminiDefaultModel
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = GeminiEndpoint
	}
	return NewOpenAICompat(OpenAICompatConfig{
		APIKey: cfg.APIKey, Model: model, BaseURL: baseURL,
		ProviderName: "gemini", HTTP: cfg.HTTP,
	})
}

// ── Internal request/response wire types ────────────────────────────────

// classifyHTTPError maps provider status codes to the sentinel errors in
// client.go. The provider's JSON error body (when present) is preserved in
// the wrapped error string so logs still carry the upstream message.
//
// Gemini's free tier emits 403 PERMISSION_DENIED for quota exhaustion
// instead of the usual 429; we sniff the body to surface it as a rate
// limit so the retry path kicks in and the UI shows the right message.
func classifyHTTPError(status int, body []byte) error {
	msg := extractErrorMessage(body)
	if msg == "" {
		msg = strings.TrimSpace(string(body))
	}
	bodyStr := string(body)
	looksLikeQuota := strings.Contains(bodyStr, "RESOURCE_EXHAUSTED") ||
		strings.Contains(bodyStr, "quota") ||
		strings.Contains(bodyStr, "rate limit")
	switch {
	case status == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrAuthFailed, msg)
	case status == http.StatusForbidden:
		if looksLikeQuota {
			return fmt.Errorf("%w: %s", ErrRateLimited, msg)
		}
		return fmt.Errorf("%w: %s", ErrAuthFailed, msg)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrRateLimited, msg)
	case status >= 500:
		return fmt.Errorf("%w (status %d): %s", ErrServerError, status, msg)
	default:
		return fmt.Errorf("%w (status %d): %s", ErrBadRequest, status, msg)
	}
}

// extractErrorMessage pulls the OpenAI-style "error.message" field out of a
// provider error body. Returns "" if the payload does not match that shape.
func extractErrorMessage(body []byte) string {
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	return strings.TrimSpace(env.Error.Message)
}

// oaiChatResponse mirrors the OpenAI chat completions response shape. Only
// the fields the tutor consumes are decoded.
type oaiChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string        `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// oaiToolCall is the OpenAI-compatible shape providers return when the model
// emits function invocations.
type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// oaiChatRequest is the wire shape we send to any /chat/completions endpoint.
// We build it manually so the tools array uses the {type, function} envelope
// OpenAI mandates and we can omit tool_choice when no tools are declared.
type oaiChatRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	TopP        float64      `json:"top_p,omitempty"`
	Stop        []string     `json:"stop,omitempty"`
	Tools       []oaiTool    `json:"tools,omitempty"`
	ToolChoice  string       `json:"tool_choice,omitempty"`
}

// oaiMessage carries the role-specific fields that may or may not be set
// depending on whether the turn is a tool result, a user prompt, or an
// assistant tool-call request.
type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiToolFunc `json:"function"`
}

type oaiToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// buildOAIRequest translates the provider-neutral ChatRequest into the
// wire-level schema.
func buildOAIRequest(req ChatRequest) oaiChatRequest {
	out := oaiChatRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}
	out.Messages = make([]oaiMessage, len(req.Messages))
	for i, m := range req.Messages {
		gm := oaiMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			gm.ToolCalls = append(gm.ToolCalls, oaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		out.Messages[i] = gm
	}
	if len(req.Tools) > 0 {
		out.Tools = make([]oaiTool, len(req.Tools))
		for i, t := range req.Tools {
			out.Tools[i] = oaiTool{
				Type:     "function",
				Function: oaiToolFunc(t),
			}
		}
		out.ToolChoice = req.ToolChoice
		if out.ToolChoice == "" {
			out.ToolChoice = "auto"
		}
	}
	return out
}

// streamingWrapper adds the `stream: true` flag without polluting
// oaiChatRequest itself (Chat must not send it).
type streamingWrapper struct {
	oaiChatRequest
	Stream bool `json:"stream"`
}

// MarshalJSON merges the embedded struct with Stream=true. Without this
// override Go's JSON encoder skips the embedded fields' tags.
func (w streamingWrapper) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(w.oaiChatRequest)
	if err != nil {
		return nil, err
	}
	if len(raw) < 2 || raw[0] != '{' {
		return nil, fmt.Errorf("oai stream wrapper: unexpected base shape")
	}
	trimmed := bytes.TrimRight(raw[:len(raw)-1], " \t\n")
	if trimmed[len(trimmed)-1] == '{' {
		return []byte(`{"stream":true}`), nil
	}
	return append(trimmed, []byte(`,"stream":true}`)...), nil
}

// decodeOAISSE parses the OpenAI SSE response into StreamEvents. Chunks look
// like `data: {choices:[{delta:{content:"..."}}]}` plus a terminal
// `data: [DONE]`. Tool calls arrive piece-by-piece across multiple deltas;
// we accumulate them by index and emit once the stream ends with the
// terminal finish_reason.
func decodeOAISSE(body io.ReadCloser, events chan<- StreamEvent, reader *StreamReader) {
	defer func() {
		_ = body.Close()
		close(events)
	}()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<20)

	type chunkDelta struct {
		Content   string `json:"content"`
		ToolCalls []struct {
			Index    int    `json:"index"`
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	type chunk struct {
		Choices []struct {
			Delta        chunkDelta `json:"delta"`
			FinishReason string     `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	pendingTools := map[int]*ToolCall{}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var c chunk
		if err := json.Unmarshal([]byte(payload), &c); err != nil {
			reader.SetErr(fmt.Errorf("llm: bad SSE chunk: %w", err))
			return
		}
		if len(c.Choices) == 0 {
			if c.Usage != nil {
				events <- StreamEvent{Usage: Usage{
					PromptTokens:     c.Usage.PromptTokens,
					CompletionTokens: c.Usage.CompletionTokens,
					TotalTokens:      c.Usage.TotalTokens,
				}}
			}
			continue
		}
		choice := c.Choices[0]
		if choice.Delta.Content != "" {
			events <- StreamEvent{Delta: choice.Delta.Content}
		}
		for _, tc := range choice.Delta.ToolCalls {
			cur, ok := pendingTools[tc.Index]
			if !ok {
				cur = &ToolCall{}
				pendingTools[tc.Index] = cur
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			if tc.Function.Name != "" {
				cur.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				cur.Arguments += tc.Function.Arguments
			}
		}
		if choice.FinishReason != "" {
			final := StreamEvent{FinishReason: choice.FinishReason}
			if c.Usage != nil {
				final.Usage = Usage{
					PromptTokens:     c.Usage.PromptTokens,
					CompletionTokens: c.Usage.CompletionTokens,
					TotalTokens:      c.Usage.TotalTokens,
				}
			}
			if len(pendingTools) > 0 {
				indexes := make([]int, 0, len(pendingTools))
				for i := range pendingTools {
					indexes = append(indexes, i)
				}
				for i := 0; i < len(indexes); i++ {
					for j := i + 1; j < len(indexes); j++ {
						if indexes[j] < indexes[i] {
							indexes[i], indexes[j] = indexes[j], indexes[i]
						}
					}
				}
				for _, idx := range indexes {
					final.ToolCalls = append(final.ToolCalls, *pendingTools[idx])
				}
			}
			events <- final
		}
	}
	if err := scanner.Err(); err != nil {
		reader.SetErr(fmt.Errorf("llm: SSE read: %w", err))
	}
}
