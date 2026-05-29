package rag

import (
	"fmt"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

// LoadPDFAsRawDocs reads `path`, extracts the text of every page and returns
// one RawDoc per page. The chunker downstream re-splits each page into
// retrieval-sized passages while keeping the page number in metadata so the
// tutor can cite "Silberschatz p. 142" precisely.
//
// sourceID identifies the corpus inside rag_chunks (e.g. "silberschatz-9e-es").
// Pages with no extractable text (cover, blank pages) are skipped.
func LoadPDFAsRawDocs(path, sourceID string) ([]RawDoc, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("rag/pdf: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	pages := r.NumPage()
	if pages <= 0 {
		return nil, fmt.Errorf("rag/pdf: %s has no pages", path)
	}

	out := make([]RawDoc, 0, pages)
	for p := 1; p <= pages; p++ {
		page := r.Page(p)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			// One bad page should not abort the whole indexation. The PDF
			// library sometimes trips on embedded fonts; we log and move on.
			fmt.Fprintf(os.Stderr, "rag/pdf: skipping page %d: %v\n", p, err)
			continue
		}
		text = strings.TrimSpace(normalizePDFText(text))
		if text == "" {
			continue
		}
		out = append(out, RawDoc{
			Source:   SourceSilberschatz,
			SourceID: sourceID,
			Text:     text,
			Metadata: map[string]any{
				"page": p,
			},
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("rag/pdf: %s yielded no text", path)
	}
	return out, nil
}

// normalizePDFText smooths a few quirks the extractor leaves: stray hard
// hyphenation at line ends ("inte-\nrrupciones" → "interrupciones"), runs of
// whitespace, and trailing form feeds. Kept conservative so we do not
// over-normalize body text the embedder would benefit from seeing intact.
func normalizePDFText(s string) string {
	// Join hyphenated line breaks.
	s = strings.ReplaceAll(s, "-\n", "")
	// Collapse triple+ newlines into double (paragraph breaks survive).
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	// Drop form feeds and tabs.
	s = strings.ReplaceAll(s, "\f", "")
	s = strings.ReplaceAll(s, "\t", " ")
	// Squash runs of spaces.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}
