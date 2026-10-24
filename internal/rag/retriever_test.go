package rag

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubEmbedder maps each text to a fixed deterministic vector so tests can
// reason about cosine similarity without hitting the network.
type stubEmbedder struct {
	vectors map[string][]float32
}

func (s *stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = s.vectors[t]
	}
	return out, nil
}
func (s *stubEmbedder) Model() string { return "stub" }
func (s *stubEmbedder) Dim() int      { return 3 }

func TestRetrieveTopK(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// Three chunks with embeddings clearly different on the third axis so a
	// query aligned to (0,0,1) should rank the third one first.
	_ = store.InsertChunks(ctx, []Chunk{
		{Source: SourceGlossary, SourceID: "PL3", Text: "user mode", Embedding: []float32{1, 0, 0}, CorpusVersion: 1},
		{Source: SourceGlossary, SourceID: "PL0", Text: "kernel mode", Embedding: []float32{0, 1, 0}, CorpusVersion: 1},
		{Source: SourceGlossary, SourceID: "INT", Text: "interrupt", Embedding: []float32{0, 0, 1}, CorpusVersion: 1},
	})

	emb := &stubEmbedder{vectors: map[string][]float32{
		"qué es una interrupción": {0, 0, 1},
	}}
	r := NewRetriever(store, emb, 1)
	if err := r.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if r.Len() != 3 {
		t.Fatalf("Len = %d, want 3", r.Len())
	}

	got, err := r.Retrieve(ctx, "qué es una interrupción", 2)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Chunk.SourceID != "INT" {
		t.Errorf("top1 = %s, want INT", got[0].Chunk.SourceID)
	}
	if got[0].Score < 0.99 {
		t.Errorf("score = %f, want ~1.0", got[0].Score)
	}
}

func TestRetrieveUsesQueryCache(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	_ = store.InsertChunks(ctx, []Chunk{
		{Source: SourceGlossary, SourceID: "PL3", Text: "user mode", Embedding: []float32{1, 0, 0}, CorpusVersion: 1},
	})

	calls := 0
	emb := &countingEmbedder{calls: &calls, vectors: map[string][]float32{
		"pl3": {1, 0, 0},
	}}
	r := NewRetriever(store, emb, 1)
	_ = r.Reload(ctx)

	if _, err := r.Retrieve(ctx, "PL3", 1); err != nil {
		t.Fatalf("Retrieve 1: %v", err)
	}
	if _, err := r.Retrieve(ctx, "PL3", 1); err != nil {
		t.Fatalf("Retrieve 2: %v", err)
	}
	if calls != 1 {
		t.Errorf("embedder calls = %d, want 1 (second should hit cache)", calls)
	}
}

type countingEmbedder struct {
	calls   *int
	vectors map[string][]float32
}

func (c *countingEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	*c.calls++
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = c.vectors[normalize(t)]
	}
	return out, nil
}
func (c *countingEmbedder) Model() string { return "count" }
func (c *countingEmbedder) Dim() int      { return 3 }

func normalize(s string) string {
	// Match HashQuery's normalization so the stub vectors line up with what
	// the retriever asks for after cache misses.
	out := []rune{}
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}

type blockingEmbedder struct{}

func (blockingEmbedder) Embed(ctx context.Context, _ []string) ([][]float32, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingEmbedder) Model() string { return "blocking" }
func (blockingEmbedder) Dim() int      { return 3 }

func TestRetrieveBoundsEmbedWithOwnDeadline(t *testing.T) {
	r := NewRetriever(NewStore(openTestDB(t)), blockingEmbedder{}, 1)
	if r.queryTimeout != queryEmbedTimeout {
		t.Fatalf("queryTimeout = %v, want %v", r.queryTimeout, queryEmbedTimeout)
	}
	r.queryTimeout = 20 * time.Millisecond

	start := time.Now()
	_, err := r.Retrieve(context.Background(), "pregunta", 3)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("embed was not bounded by the query timeout")
	}
}
