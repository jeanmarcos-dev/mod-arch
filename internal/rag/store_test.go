package rag

import (
	"context"
	"database/sql"
	"testing"

	storepkg "github.com/jeanmarcos/modos-cpu/internal/store"
)

// openTestDB spins up an in-memory SQLite migrated to the latest schema and
// returns the underlying *sql.DB the RAG store needs. Single connection
// because :memory: is per-connection.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	st, err := storepkg.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st.DB()
}

func TestInsertAndListChunks(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()

	chunks := []Chunk{
		{Source: SourceGlossary, SourceID: "PL3", Text: "user mode", Metadata: map[string]any{"category": "Modos"}, Embedding: []float32{0.1, 0.2, 0.3}, CorpusVersion: 1},
		{Source: SourceGlossary, SourceID: "PL0", Text: "kernel mode", Metadata: map[string]any{"category": "Modos"}, Embedding: []float32{0.4, 0.5, 0.6}, CorpusVersion: 1},
		{Source: SourceSilberschatz, SourceID: "ch3.2", Text: "scheduling", Embedding: []float32{0.7, 0.8, 0.9}, CorpusVersion: 1},
	}
	if err := s.InsertChunks(ctx, chunks); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	all, err := s.ListChunks(ctx, 1, "")
	if err != nil {
		t.Fatalf("ListChunks: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}

	gloss, err := s.ListChunks(ctx, 1, SourceGlossary)
	if err != nil {
		t.Fatalf("ListChunks glossary: %v", err)
	}
	if len(gloss) != 2 {
		t.Fatalf("len(gloss) = %d, want 2", len(gloss))
	}

	// Embedding round-trip preserves values.
	got := gloss[0].Embedding
	want := []float32{0.1, 0.2, 0.3}
	if gloss[0].SourceID != "PL3" {
		got = gloss[1].Embedding
		want = []float32{0.4, 0.5, 0.6}
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("embedding[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if gloss[0].Metadata["category"] != "Modos" {
		t.Errorf("metadata lost: got %v", gloss[0].Metadata)
	}
}

func TestCountByCorpus(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	_ = s.InsertChunks(ctx, []Chunk{
		{Source: SourceGlossary, SourceID: "a", Text: "x", Embedding: []float32{1}, CorpusVersion: 1},
		{Source: SourceGlossary, SourceID: "b", Text: "y", Embedding: []float32{1}, CorpusVersion: 1},
		{Source: SourceSilberschatz, SourceID: "c", Text: "z", Embedding: []float32{1}, CorpusVersion: 1},
		{Source: SourceGlossary, SourceID: "d", Text: "w", Embedding: []float32{1}, CorpusVersion: 2},
	})
	got, err := s.CountByCorpus(ctx, 1)
	if err != nil {
		t.Fatalf("CountByCorpus: %v", err)
	}
	if got[SourceGlossary] != 2 || got[SourceSilberschatz] != 1 {
		t.Errorf("counts = %v, want {glossary:2, silberschatz:1}", got)
	}
}

func TestDeleteCorpusVersion(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	_ = s.InsertChunks(ctx, []Chunk{
		{Source: SourceGlossary, SourceID: "a", Text: "x", Embedding: []float32{1}, CorpusVersion: 1},
		{Source: SourceGlossary, SourceID: "b", Text: "y", Embedding: []float32{1}, CorpusVersion: 2},
	})
	n, err := s.DeleteCorpusVersion(ctx, 1)
	if err != nil {
		t.Fatalf("DeleteCorpusVersion: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
	left, _ := s.ListChunks(ctx, 2, "")
	if len(left) != 1 {
		t.Errorf("v2 still has %d rows, want 1", len(left))
	}
}

func TestQueryCacheRoundTrip(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	emb := []float32{0.5, -0.25, 1.0}
	if err := s.PutQueryEmbedding(ctx, "h1", emb); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok, err := s.GetCachedQueryEmbedding(ctx, "h1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	for i := range emb {
		if got[i] != emb[i] {
			t.Errorf("emb[%d] = %v, want %v", i, got[i], emb[i])
		}
	}
	if _, ok, _ := s.GetCachedQueryEmbedding(ctx, "missing"); ok {
		t.Error("missing key returned ok=true")
	}
}

func TestAnswerCacheRoundTrip(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	if err := s.PutAnswer(ctx, "k1", "PL3 es user mode", []byte(`[{"src":"glossary","id":"PL3"}]`)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ca, err := s.GetCachedAnswer(ctx, "k1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ca == nil {
		t.Fatal("expected hit, got nil")
	}
	if ca.Answer != "PL3 es user mode" {
		t.Errorf("Answer = %q", ca.Answer)
	}
	if string(ca.Citations) == "" {
		t.Error("Citations empty")
	}
}
