package tutor

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/internal/rag"
)

func TestBuildEmbedderWrapsOnlyJina(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		name        string
		cfg         Config
		wantLimited bool
		wantErr     bool
	}{
		{name: "jina is wrapped", cfg: Config{EmbedderProvider: "jina", JinaAPIKey: "k"}, wantLimited: true},
		{name: "ollama is not wrapped", cfg: Config{EmbedderProvider: "ollama"}},
		{name: "default provider is not wrapped", cfg: Config{}},
		{name: "jina without key fails", cfg: Config{EmbedderProvider: "jina"}, wantErr: true},
		{name: "unknown provider fails", cfg: Config{EmbedderProvider: "nope"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			emb, err := buildEmbedder(tc.cfg, logger, 0)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildEmbedder: %v", err)
			}
			_, limited := emb.(*rag.LimitedEmbedder)
			if limited != tc.wantLimited {
				t.Fatalf("limited = %v, want %v", limited, tc.wantLimited)
			}
		})
	}
}
