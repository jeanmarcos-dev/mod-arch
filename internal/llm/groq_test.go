package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGroqRequiresAPIKey(t *testing.T) {
	if _, err := NewGroq(GroqConfig{}); err == nil {
		t.Fatal("expected error when APIKey is empty")
	}
}

func TestGroqChatSuccess(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
            "model": "llama-3.3-70b-versatile",
            "choices": [{"message": {"content": "Hola, usted preguntó por PL3."}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18}
        }`)
	}))
	defer srv.Close()

	g, err := NewGroq(GroqConfig{APIKey: "sk-test", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewGroq: %v", err)
	}

	resp, err := g.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: "eres tutor"},
			{Role: RoleUser, Content: "¿qué es PL3?"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if !strings.Contains(resp.Content, "PL3") {
		t.Errorf("Content = %q, want it to mention PL3", resp.Content)
	}
	if resp.Usage.TotalTokens != 18 {
		t.Errorf("Usage.TotalTokens = %d, want 18", resp.Usage.TotalTokens)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", gotAuth)
	}

	// Verify the request body used the default model and forwarded messages.
	var sent ChatRequest
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Model != GroqDefaultModel {
		t.Errorf("sent.Model = %q, want %q", sent.Model, GroqDefaultModel)
	}
	if len(sent.Messages) != 2 {
		t.Fatalf("sent.Messages len = %d, want 2", len(sent.Messages))
	}
}

func TestGroqChatErrorClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"bad key"}}`, ErrAuthFailed},
		{"forbidden", http.StatusForbidden, `{"error":{"message":"nope"}}`, ErrAuthFailed},
		{"rate limit", http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`, ErrRateLimited},
		{"server", http.StatusBadGateway, `upstream broken`, ErrServerError},
		{"bad request", http.StatusBadRequest, `{"error":{"message":"missing model"}}`, ErrBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			g, err := NewGroq(GroqConfig{APIKey: "sk-test", BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("NewGroq: %v", err)
			}
			_, err = g.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestGroqEmptyChoicesReturnsEmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"model":"x","choices":[],"usage":{}}`)
	}))
	defer srv.Close()

	g, _ := NewGroq(GroqConfig{APIKey: "sk-test", BaseURL: srv.URL})
	_, err := g.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrEmptyContent) {
		t.Errorf("err = %v, want ErrEmptyContent", err)
	}
}
