package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEval_RetrievalGoldenQueries(t *testing.T) {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		t.Skip("Skipping retrieval golden tests: OLLAMA_URL not set")
	}

	ctx := context.Background()
	db := openTestDB(t)
	store := NewStore(db)

	embedder, err := NewOllama(OllamaConfig{
		BaseURL: ollamaURL,
		Model:   "nomic-embed-text",
	})
	if err != nil {
		t.Fatalf("Failed to init embedder: %v", err)
	}

	projRoot := filepath.Join("..", "..")
	var rawDocs []RawDoc

	// 1. Glossary
	glossaryPath := filepath.Join(projRoot, "web", "src", "data", "glossary.ts")
	if entries, err := LoadGlossaryFromTS(glossaryPath); err == nil {
		rawDocs = append(rawDocs, ChunkGlossaryEntries(entries)...)
	}

	// 2. ISA
	rawDocs = append(rawDocs, LoadISAAsRawDocs()...)

	if len(rawDocs) == 0 {
		t.Skip("Skipping because no chunks were loaded")
	}

	var chunks []Chunk
	for _, d := range rawDocs {
		vecs, err := embedder.Embed(ctx, []string{d.Text})
		if err != nil {
			t.Fatalf("Embed error: %v", err)
		}
		chunks = append(chunks, Chunk{
			Source:        d.Source,
			SourceID:      d.SourceID,
			Text:          d.Text,
			Embedding:     vecs[0],
			CorpusVersion: 1,
		})
	}

	if err := store.InsertChunks(ctx, chunks); err != nil {
		t.Fatalf("Insert chunks: %v", err)
	}

	retriever := NewRetriever(store, embedder, 1)
	if err := retriever.Reload(ctx); err != nil {
		t.Fatalf("Reload retriever: %v", err)
	}

	cases := []struct {
		query          string
		expectedSource Source
		expectedID     string
	}{
		{
			query:          "¿Qué pasa cuando intento leer memoria que no existe o no tengo permiso?",
			expectedSource: SourceGlossary,
			expectedID:     "Page fault",
		},
		{
			query:          "Cómo funciona la instrucción MOV",
			expectedSource: SourceISA,
			expectedID:     "MOV",
		},
		{
			query:          "¿En qué anillo corren las aplicaciones de usuario normales?",
			expectedSource: SourceGlossary,
			expectedID:     "PL3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			results, err := retriever.Retrieve(ctx, tc.query, 3)
			if err != nil {
				t.Fatalf("Retrieve failed: %v", err)
			}

			found := false
			for i, r := range results {
				if r.Chunk.Source == tc.expectedSource && r.Chunk.SourceID == tc.expectedID {
					found = true
					t.Logf("Match found at rank %d (score: %.3f)", i+1, r.Score)
					break
				}
			}

			if !found {
				t.Errorf("Expected %s:%s in top 3, got:", tc.expectedSource, tc.expectedID)
				for i, r := range results {
					text := r.Chunk.Text
					if len(text) > 50 {
						text = text[:50]
					}
					t.Errorf("  %d: %s:%s (score: %.3f) - %q", i+1, r.Chunk.Source, r.Chunk.SourceID, r.Score, text)
				}
			}
		})
	}
}
