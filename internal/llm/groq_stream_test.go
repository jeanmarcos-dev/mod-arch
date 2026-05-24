package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGroqChatStreamHappyPath simulates an SSE response with content deltas
// and a final usage/finish marker. We confirm:
//   - Token deltas surface as individual events
//   - Tool calls accumulated across chunks land in the final event
//   - The terminal Usage propagates
func TestGroqChatStreamHappyPath(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hola "}}]}`,
		`data: {"choices":[{"delta":{"content":"mundo"}}]}`,
		`data: {"choices":[{"delta":{"content":"."},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sse)
	}))
	defer srv.Close()

	g, err := NewGroq(GroqConfig{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewGroq: %v", err)
	}
	stream, err := g.ChatStream(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hola"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer stream.Close()

	var deltas []string
	var finalUsage Usage
	var finishReason string
	for ev := range stream.Events {
		if ev.Delta != "" {
			deltas = append(deltas, ev.Delta)
		}
		if ev.FinishReason != "" {
			finishReason = ev.FinishReason
			finalUsage = ev.Usage
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream err: %v", err)
	}
	if got := strings.Join(deltas, ""); got != "Hola mundo." {
		t.Errorf("accumulated text = %q", got)
	}
	if finishReason != "stop" {
		t.Errorf("finish = %q", finishReason)
	}
	if finalUsage.TotalTokens != 10 {
		t.Errorf("usage.total = %d", finalUsage.TotalTokens)
	}
}

// TestGroqChatStreamToolCalls verifies that fragmented tool_call args land
// in a single concatenated ToolCall at the end of the stream.
func TestGroqChatStreamToolCalls(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_processes","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, sse)
	}))
	defer srv.Close()

	g, _ := NewGroq(GroqConfig{APIKey: "k", BaseURL: srv.URL})
	stream, err := g.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "?"}}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer stream.Close()

	var tools []ToolCall
	for ev := range stream.Events {
		if len(ev.ToolCalls) > 0 {
			tools = ev.ToolCalls
		}
	}
	if len(tools) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(tools))
	}
	if tools[0].Name != "get_processes" {
		t.Errorf("name = %q", tools[0].Name)
	}
	if tools[0].Arguments != "{}" {
		t.Errorf("arguments = %q, want concatenated '{}'", tools[0].Arguments)
	}
}
