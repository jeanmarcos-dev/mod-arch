package rag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// docFile describes an educational markdown file we want to index. Only
// teaching material lives in this list — deploy runbooks and operational
// READMEs are excluded so the tutor never recommends "ssh into the LXC".
var educationalDocs = []struct {
	relPath string
	docID   string
	title   string
	// skipPrefixes is a list of heading slugs (lower-cased, no #) to drop
	// from the chunk stream. Useful to exclude operational sections like
	// "Quickstart" from a README that is otherwise educational.
	skipHeadings []string
}{
	{
		relPath:      "README.es.md",
		docID:        "readme",
		title:        "README del proyecto",
		skipHeadings: []string{"stack", "estructura del repositorio", "inicio rápido", "corpus propio", "configuración", "deploy", "seguridad", "licencia"},
	},
	{
		relPath:      "docs/reporte-defensa.md",
		docID:        "reporte-defensa",
		title:        "Reporte de defensa técnica",
		skipHeadings: nil,
	},
	{
		relPath:      "docs/ayuda-soporte.md",
		docID:        "ayuda-soporte",
		title:        "Cómo pedir ayuda en MODARCH",
		skipHeadings: nil,
	},
}

// LoadDocsAsRawDocs walks the educational markdown files anchored at repoRoot
// and splits them by H2/H3 headings. Each section becomes one RawDoc so the
// retriever can return a focused passage instead of the whole document.
func LoadDocsAsRawDocs(repoRoot string) ([]RawDoc, error) {
	var out []RawDoc
	for _, d := range educationalDocs {
		path := filepath.Join(repoRoot, d.relPath)
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("docs: read %s: %w", path, err)
		}
		sections := splitMarkdownByHeadings(string(raw))
		skip := map[string]bool{}
		for _, h := range d.skipHeadings {
			skip[strings.ToLower(h)] = true
		}
		for _, sec := range sections {
			if skip[strings.ToLower(strings.TrimSpace(sec.heading))] {
				continue
			}
			body := strings.TrimSpace(sec.body)
			if body == "" {
				continue
			}
			id := d.docID
			if sec.heading != "" {
				id = d.docID + "#" + slugify(sec.heading)
			}
			text := body
			if sec.heading != "" {
				text = sec.heading + "\n\n" + body
			}
			out = append(out, RawDoc{
				Source:   SourceDocs,
				SourceID: id,
				Text:     text,
				Metadata: map[string]any{
					"doc":     d.docID,
					"title":   d.title,
					"section": sec.heading,
				},
			})
		}
	}
	return out, nil
}

type mdSection struct {
	heading string // h2/h3 text without leading '#' chars
	body    string
}

// splitMarkdownByHeadings cuts the document at any H1/H2/H3 boundary. The
// preamble before the first heading (if any) is attached to a synthetic
// section with the empty heading so it does not get lost.
func splitMarkdownByHeadings(src string) []mdSection {
	lines := strings.Split(src, "\n")
	var out []mdSection
	cur := mdSection{}
	var body strings.Builder
	flush := func() {
		cur.body = body.String()
		out = append(out, cur)
		cur = mdSection{}
		body.Reset()
	}
	inFence := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		// Track code fences so a "## foo" inside a fenced block is not
		// treated as a heading.
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			body.WriteString(line)
			body.WriteByte('\n')
			continue
		}
		if !inFence && (strings.HasPrefix(trim, "# ") || strings.HasPrefix(trim, "## ") || strings.HasPrefix(trim, "### ")) {
			if body.Len() > 0 || cur.heading != "" {
				flush()
			}
			cur.heading = strings.TrimLeft(trim, "# ")
			continue
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}
	if body.Len() > 0 || cur.heading != "" {
		flush()
	}
	return out
}

// slugify makes a heading safe to use inside a source_id by lower-casing it
// and replacing non-alphanumeric runs with dashes.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
