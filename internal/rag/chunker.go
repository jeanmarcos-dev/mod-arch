package rag

import (
	"maps"
	"strings"
	"unicode/utf8"
)

// RawDoc is the input to Chunk: a piece of source material to be split into
// retrieval units. Metadata flows through to every produced chunk so we can
// cite "Silberschatz cap 3.2" or "Glosario: PL3" later.
type RawDoc struct {
	Source   Source
	SourceID string
	Text     string
	Metadata map[string]any
}

// ChunkOptions controls splitter behavior. Tokens are estimated from words
// (1 token ≈ 0.75 words for Spanish/English mixed text).
type ChunkOptions struct {
	TargetTokens int     // soft target for chunk size; default 450
	OverlapRatio float64 // fraction of TargetTokens to overlap; default 0.10
}

func (o ChunkOptions) withDefaults() ChunkOptions {
	if o.TargetTokens <= 0 {
		o.TargetTokens = 450
	}
	if o.OverlapRatio < 0 || o.OverlapRatio > 0.5 {
		o.OverlapRatio = 0.10
	}
	return o
}

// ChunkGlossaryEntries produces one RawDoc per glossary entry. Each chunk's
// text concatenates short + detail + aliases so a retrieval hit surfaces
// everything the UI shows for that term.
func ChunkGlossaryEntries(entries []GlossaryEntry) []RawDoc {
	out := make([]RawDoc, 0, len(entries))
	for _, e := range entries {
		var b strings.Builder
		b.WriteString(e.Term)
		b.WriteString("\n")
		if e.Short != "" {
			b.WriteString(e.Short)
			b.WriteString("\n")
		}
		if e.Detail != "" {
			b.WriteString(e.Detail)
			b.WriteString("\n")
		}
		if len(e.Aliases) > 0 {
			b.WriteString("Aliases: ")
			b.WriteString(strings.Join(e.Aliases, ", "))
		}
		out = append(out, RawDoc{
			Source:   SourceGlossary,
			SourceID: e.Term,
			Text:     strings.TrimSpace(b.String()),
			Metadata: map[string]any{
				"category": e.Category,
				"aliases":  e.Aliases,
			},
		})
	}
	return out
}

// SplitParagraphs splits free-form text into chunks of approximately
// TargetTokens, breaking on paragraph boundaries when possible and on word
// boundaries otherwise. Successive chunks share OverlapRatio*TargetTokens
// tokens so a concept that straddles a boundary stays retrievable.
//
// Caller supplies Source/SourceID/Metadata; this function only deals with
// raw text. Returned chunks inherit those fields with a "chunk_index" added.
func SplitParagraphs(doc RawDoc, opts ChunkOptions) []RawDoc {
	opts = opts.withDefaults()
	paragraphs := splitOnBlankLines(doc.Text)

	target := opts.TargetTokens
	overlap := int(float64(target) * opts.OverlapRatio)

	var chunks []RawDoc
	var cur strings.Builder
	curTokens := 0

	flush := func() {
		text := strings.TrimSpace(cur.String())
		if text == "" {
			return
		}
		meta := cloneMeta(doc.Metadata)
		meta["chunk_index"] = len(chunks)
		chunks = append(chunks, RawDoc{
			Source:   doc.Source,
			SourceID: doc.SourceID,
			Text:     text,
			Metadata: meta,
		})
		if overlap > 0 {
			tail := tailTokens(text, overlap)
			cur.Reset()
			cur.WriteString(tail)
			cur.WriteString("\n\n")
			curTokens = estimateTokens(tail)
		} else {
			cur.Reset()
			curTokens = 0
		}
	}

	for _, p := range paragraphs {
		pTokens := estimateTokens(p)
		// A single paragraph larger than the target: split it on word
		// boundaries instead of dropping it whole into one chunk.
		if pTokens > target {
			if curTokens > 0 {
				flush()
			}
			for _, piece := range splitOnWords(p, target) {
				if curTokens+estimateTokens(piece) > target && curTokens > 0 {
					flush()
				}
				cur.WriteString(piece)
				cur.WriteString("\n\n")
				curTokens += estimateTokens(piece)
				if curTokens >= target {
					flush()
				}
			}
			continue
		}
		if curTokens+pTokens > target && curTokens > 0 {
			flush()
		}
		cur.WriteString(p)
		cur.WriteString("\n\n")
		curTokens += pTokens
	}
	if strings.TrimSpace(cur.String()) != "" {
		flush()
	}
	return chunks
}

func splitOnBlankLines(s string) []string {
	parts := strings.Split(s, "\n\n")
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// splitOnWords cuts an oversized paragraph into pieces of ~target tokens by
// scanning whitespace boundaries. Returned pieces still concatenate cleanly
// (no leading/trailing whitespace).
func splitOnWords(s string, target int) []string {
	words := strings.Fields(s)
	wordsPerChunk := max(target*4/3, 1) // tokens→words at 0.75 token/word
	var out []string
	for i := 0; i < len(words); i += wordsPerChunk {
		end := min(i+wordsPerChunk, len(words))
		out = append(out, strings.Join(words[i:end], " "))
	}
	return out
}

// tailTokens returns the last `n` tokens of text, used as overlap between
// successive chunks.
func tailTokens(text string, n int) string {
	words := strings.Fields(text)
	wordsPerToken := 4.0 / 3.0
	wantWords := int(float64(n) * wordsPerToken)
	if wantWords <= 0 || wantWords >= len(words) {
		return text
	}
	return strings.Join(words[len(words)-wantWords:], " ")
}

// estimateTokens converts a word count into a rough token count. Good enough
// for chunking decisions where the exact tokenizer is irrelevant.
func estimateTokens(s string) int {
	words := 0
	inWord := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' {
			inWord = false
			continue
		}
		if !inWord {
			words++
			inWord = true
		}
	}
	// Treat very short paragraphs as at least one token to avoid infinite
	// loops when a single character paragraph slips through.
	if words == 0 && utf8.RuneCountInString(s) > 0 {
		return 1
	}
	return words * 4 / 3
}

func cloneMeta(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+1)
	maps.Copy(out, m)
	return out
}
