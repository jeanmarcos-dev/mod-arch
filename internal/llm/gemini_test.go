package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewGeminiModelSelection(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantModel string
	}{
		{name: "default model", wantModel: "gemini-3.5-flash-lite"},
		{name: "explicit model", model: "gemini-3.5-flash", wantModel: "gemini-3.5-flash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotModel string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &body)
				gotModel = body.Model
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices": [{"message": {"content": "ok"}, "finish_reason": "stop"}]}`)
			}))
			defer srv.Close()

			g, err := NewGemini(GeminiConfig{APIKey: "key", Model: tt.model, BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("NewGemini: %v", err)
			}
			if _, err := g.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hola"}}}); err != nil {
				t.Fatalf("Chat: %v", err)
			}
			if gotModel != tt.wantModel {
				t.Fatalf("model = %q, want %q", gotModel, tt.wantModel)
			}
		})
	}
}
