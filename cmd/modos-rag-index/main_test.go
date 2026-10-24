package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jeanmarcos/modos-cpu/internal/rag"
)

type scriptedEmbedder struct {
	failures []error
	calls    int
}

func (e *scriptedEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.calls++
	if len(e.failures) > 0 {
		err := e.failures[0]
		e.failures = e.failures[1:]
		return nil, err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1}
	}
	return out, nil
}

func (e *scriptedEmbedder) Model() string { return "scripted" }
func (e *scriptedEmbedder) Dim() int      { return 1 }

func TestEmbedDocs(t *testing.T) {
	docs := []rag.RawDoc{{Text: "a"}, {Text: "b"}, {Text: "c"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	opts := retryOptions{maxRetries: 2, wait: 0}

	tests := []struct {
		name      string
		failures  []error
		wantErr   error
		wantCalls int
	}{
		{name: "no failures", wantCalls: 2},
		{name: "rate limit then success", failures: []error{rag.ErrEmbedderRateLimited, rag.ErrEmbedderRateLimited}, wantCalls: 4},
		{name: "rate limit exhausts retries", failures: []error{rag.ErrEmbedderRateLimited, rag.ErrEmbedderRateLimited, rag.ErrEmbedderRateLimited}, wantErr: rag.ErrEmbedderRateLimited, wantCalls: 3},
		{name: "other errors are not retried", failures: []error{rag.ErrEmbedderAuthFailed}, wantErr: rag.ErrEmbedderAuthFailed, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emb := &scriptedEmbedder{failures: tt.failures}
			chunks, err := embedDocs(context.Background(), emb, docs, 2, 7, opts, logger)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if emb.calls != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", emb.calls, tt.wantCalls)
			}
			if tt.wantErr != nil {
				return
			}
			if len(chunks) != len(docs) {
				t.Fatalf("chunks = %d, want %d", len(chunks), len(docs))
			}
			for _, c := range chunks {
				if c.CorpusVersion != 7 {
					t.Fatalf("corpus version = %d, want 7", c.CorpusVersion)
				}
			}
		})
	}
}

func TestEmbedDocsStopsWaitingWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	emb := &scriptedEmbedder{failures: []error{rag.ErrEmbedderRateLimited}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := embedDocs(ctx, emb, []rag.RawDoc{{Text: "a"}}, 1, 1, retryOptions{maxRetries: 3, wait: 1 << 40}, logger)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
