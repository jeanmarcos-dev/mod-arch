package rag

import (
	"strings"
	"testing"
)

func TestChunkGlossaryEntriesOnePerEntry(t *testing.T) {
	entries := []GlossaryEntry{
		{Term: "PL3", Category: "Modos", Short: "user mode", Detail: "no kernel access", Aliases: []string{"user mode"}},
		{Term: "PL0", Category: "Modos", Short: "kernel mode", Detail: "full access"},
	}
	docs := ChunkGlossaryEntries(entries)
	if len(docs) != 2 {
		t.Fatalf("len = %d, want 2", len(docs))
	}
	if !strings.Contains(docs[0].Text, "PL3") || !strings.Contains(docs[0].Text, "user mode") {
		t.Errorf("doc0 text missing keys: %q", docs[0].Text)
	}
	if !strings.Contains(docs[0].Text, "Aliases:") {
		t.Error("aliases line missing from doc0")
	}
	if docs[0].Metadata["category"] != "Modos" {
		t.Errorf("category metadata = %v", docs[0].Metadata["category"])
	}
}

func TestSplitParagraphsRespectsTarget(t *testing.T) {
	// Build a doc with 10 paragraphs of ~50 words each. Target 150 tokens
	// (~200 words) should yield ~4-5 chunks.
	paras := make([]string, 10)
	for i := range paras {
		paras[i] = strings.Repeat("foo bar baz ", 50)
	}
	doc := RawDoc{Source: SourceSilberschatz, SourceID: "ch1", Text: strings.Join(paras, "\n\n")}
	chunks := SplitParagraphs(doc, ChunkOptions{TargetTokens: 150, OverlapRatio: 0.1})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		toks := estimateTokens(c.Text)
		if toks > 250 {
			t.Errorf("chunk %d tokens=%d, too large", i, toks)
		}
		if c.Metadata["chunk_index"] != i {
			t.Errorf("chunk %d index metadata = %v", i, c.Metadata["chunk_index"])
		}
	}
}

func TestSplitParagraphsHandlesOversizedParagraph(t *testing.T) {
	// One paragraph way larger than the target should still split.
	big := strings.Repeat("alfa beta gamma delta ", 400)
	doc := RawDoc{Text: big}
	chunks := SplitParagraphs(doc, ChunkOptions{TargetTokens: 100})
	if len(chunks) < 3 {
		t.Errorf("oversized paragraph yielded %d chunks, want >=3", len(chunks))
	}
}

func TestEstimateTokensCountsWords(t *testing.T) {
	if got := estimateTokens("uno dos tres"); got != 4 {
		t.Errorf("uno dos tres → %d tokens, want 4 (3 words × 4/3)", got)
	}
	if got := estimateTokens(""); got != 0 {
		t.Errorf("empty → %d, want 0", got)
	}
}
