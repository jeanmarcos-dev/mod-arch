// Package llm is a provider-agnostic client for OpenAI-compatible chat
// completion APIs (Groq, OpenRouter, Cerebras, etc.). It deliberately exposes
// the minimum surface the tutor needs: synchronous chat with a system prompt
// and a single user turn. Streaming and tool-calls are out of scope here.
package llm

import (
	"context"
	"errors"
	"sync"
)

// Role identifies who produced a Message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	// RoleTool is the role used to feed back the result of a tool call into
	// the conversation, per the OpenAI function-calling protocol.
	RoleTool Role = "tool"
)

// Message is one turn in a chat conversation.
//
// For RoleTool messages, ToolCallID must match the id of the tool_call this
// message answers. For RoleAssistant messages, ToolCalls (if non-empty) lists
// the functions the model wants invoked; Content is usually empty in that case.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"` // optional for tool messages
}

// ToolDef declares a tool the model may invoke. Parameters is a JSON Schema
// describing the function arguments; the provider forwards it verbatim.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall is a single function the model has asked us to invoke.
// Arguments is a raw JSON string (per the OpenAI schema); the caller decodes
// it according to the tool's declared Parameters schema.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatRequest is the input to Provider.Chat. Fields map to the OpenAI chat
// completions schema so providers can forward them with minimal translation.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
	Stop        []string  `json:"stop,omitempty"`

	// Tools declares functions the model may invoke. ToolChoice picks the
	// invocation strategy: "" or "auto" lets the model decide, "none" forbids
	// tool use even when tools are declared, "required" forces at least one.
	Tools      []ToolDef `json:"-"`
	ToolChoice string    `json:"-"`
}

// ChatResponse is the result of Provider.Chat. When the model emitted tool
// calls, FinishReason will be "tool_calls" and ToolCalls will list them; the
// orchestrator is expected to execute each and re-invoke Chat with the
// results appended as RoleTool messages.
type ChatResponse struct {
	Content      string
	Model        string
	FinishReason string
	ToolCalls    []ToolCall
	Usage        Usage
}

// Usage records token counts reported by the provider. Useful for budgeting
// against free-tier quotas.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Provider is implemented by every backend (Groq, OpenRouter, ...). One method
// keeps the interface small enough that mocks in tests are trivial.
type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Name() string
}

// Streamer is an optional capability some providers expose. Callers that
// want token-by-token output type-assert Provider into Streamer; providers
// that do not implement it fall back to the synchronous Chat.
type Streamer interface {
	ChatStream(ctx context.Context, req ChatRequest) (*StreamReader, error)
}

// StreamEvent carries one piece of a streamed response. Either Delta is set
// (text token) or ToolCalls is populated (the model emitted tool calls in
// this turn). Both can be empty when only FinishReason advances.
type StreamEvent struct {
	Delta        string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage // populated on the last event
}

// StreamReader is the consumer side of a streamed chat. Events is closed by
// the producer when the stream finishes; Err exposes the terminal error if
// any (read after Events drains). Call Close to abort early.
type StreamReader struct {
	Events <-chan StreamEvent
	close  func()
	errMu  sync.Mutex
	err    error
}

// NewStreamReader builds a StreamReader. Used by provider implementations.
func NewStreamReader(events <-chan StreamEvent, closer func()) *StreamReader {
	return &StreamReader{Events: events, close: closer}
}

// Close releases the underlying HTTP body. Safe to call multiple times.
func (r *StreamReader) Close() {
	if r == nil || r.close == nil {
		return
	}
	r.close()
	r.close = nil
}

// SetErr is called by the producer to record a terminal error before closing
// the channel. Subsequent Err() calls return it.
func (r *StreamReader) SetErr(err error) {
	r.errMu.Lock()
	r.err = err
	r.errMu.Unlock()
}

// Err returns the terminal error if any. nil means the stream ended cleanly.
func (r *StreamReader) Err() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.err
}

// Sentinel errors. Callers can use errors.Is to react differently to each
// failure mode (retry on RateLimited, surface AuthFailed to admins, ...).
var (
	ErrAuthFailed   = errors.New("llm: authentication failed")
	ErrRateLimited  = errors.New("llm: rate limited")
	ErrServerError  = errors.New("llm: provider server error")
	ErrBadRequest   = errors.New("llm: bad request")
	ErrEmptyContent = errors.New("llm: provider returned empty content")
)
