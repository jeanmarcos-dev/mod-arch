package rag

import (
	"fmt"
	"os"
	"strings"
)

// GlossaryEntry mirrors the shape of GlossaryEntry in
// web/src/data/glossary.ts. We keep them isomorphic so the indexer's output
// matches what the UI displays.
type GlossaryEntry struct {
	Term     string
	Category string
	Short    string
	Detail   string
	Aliases  []string
}

// LoadGlossaryFromTS parses the TypeScript source at path and extracts every
// entry inside the `GLOSSARY_ENTRIES` array. The parser handles the single-
// quoted string literals used by the file (with `\'` escapes) and the
// optional `aliases: [...]` field. Comments and whitespace between entries
// are ignored.
//
// This is a deliberately narrow parser: it expects the exact key order the
// file uses today (term → category → short → detail → aliases?) which holds
// across every entry as of this writing. If that ever changes, prefer
// exporting a JSON alongside the .ts and reading that instead.
func LoadGlossaryFromTS(path string) ([]GlossaryEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("rag/glossary: read %s: %w", path, err)
	}
	src := string(raw)

	start := strings.Index(src, "GLOSSARY_ENTRIES")
	if start < 0 {
		return nil, fmt.Errorf("rag/glossary: GLOSSARY_ENTRIES not found in %s", path)
	}
	// Skip past the `= [` assignment so we land on the array literal rather
	// than the `[]` in the `GlossaryEntry[]` type annotation.
	eq := strings.Index(src[start:], "=")
	if eq < 0 {
		return nil, fmt.Errorf("rag/glossary: '=' after GLOSSARY_ENTRIES not found")
	}
	open := strings.Index(src[start+eq:], "[")
	if open < 0 {
		return nil, fmt.Errorf("rag/glossary: opening [ of array literal not found")
	}
	body, err := sliceMatchingBracket(src[start+eq+open:], '[', ']')
	if err != nil {
		return nil, fmt.Errorf("rag/glossary: %w", err)
	}

	var out []GlossaryEntry
	p := &tsParser{src: body, pos: 0}
	for p.skipUntil('{') {
		entryText, err := p.readBlock('{', '}')
		if err != nil {
			return nil, err
		}
		entry, err := parseEntry(entryText)
		if err != nil {
			return nil, err
		}
		if entry.Term == "" {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

// sliceMatchingBracket returns the substring between s[0] (which must be
// `open`) and the matching `close`, handling nested brackets and string
// literals. The returned slice does not include the outer brackets.
func sliceMatchingBracket(s string, open, close byte) (string, error) {
	if len(s) == 0 || s[0] != open {
		return "", fmt.Errorf("expected %c at start", open)
	}
	depth := 0
	inString := false
	var stringQuote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if c == '\\' {
				i++
				continue
			}
			if c == stringQuote {
				inString = false
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			inString = true
			stringQuote = c
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[1:i], nil
			}
		}
	}
	return "", fmt.Errorf("unmatched %c", open)
}

// tsParser is a tiny cursor over a TypeScript substring. It only knows how
// to skip whitespace/comments and pull balanced blocks.
type tsParser struct {
	src string
	pos int
}

func (p *tsParser) skipUntil(c byte) bool {
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == c {
			return true
		}
		// Skip // line comments and /* block comments to avoid stray braces.
		if ch == '/' && p.pos+1 < len(p.src) {
			if p.src[p.pos+1] == '/' {
				nl := strings.IndexByte(p.src[p.pos:], '\n')
				if nl < 0 {
					p.pos = len(p.src)
					return false
				}
				p.pos += nl + 1
				continue
			}
			if p.src[p.pos+1] == '*' {
				end := strings.Index(p.src[p.pos:], "*/")
				if end < 0 {
					p.pos = len(p.src)
					return false
				}
				p.pos += end + 2
				continue
			}
		}
		p.pos++
	}
	return false
}

func (p *tsParser) readBlock(open, close byte) (string, error) {
	if p.pos >= len(p.src) || p.src[p.pos] != open {
		return "", fmt.Errorf("expected %c at pos %d", open, p.pos)
	}
	body, err := sliceMatchingBracket(p.src[p.pos:], open, close)
	if err != nil {
		return "", err
	}
	// Advance the cursor past the closing bracket.
	p.pos += len(body) + 2
	return body, nil
}

// parseEntry extracts term/category/short/detail/aliases from one entry's
// body (the substring between `{` and `}`). Keys may appear in any order.
func parseEntry(body string) (GlossaryEntry, error) {
	var e GlossaryEntry
	fields := splitTopLevelFields(body)
	for _, f := range fields {
		key, val, ok := splitKeyValue(f)
		if !ok {
			continue
		}
		switch key {
		case "term":
			e.Term = unquoteTSString(val)
		case "category":
			e.Category = unquoteTSString(val)
		case "short":
			e.Short = unquoteTSString(val)
		case "detail":
			e.Detail = unquoteTSString(val)
		case "aliases":
			e.Aliases = parseStringArray(val)
		}
	}
	return e, nil
}

// splitTopLevelFields slices an object body at commas that live outside any
// string or nested bracket. Trailing comma after the last field is ignored.
func splitTopLevelFields(body string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	inString := false
	var quote byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		if inString {
			cur.WriteByte(c)
			if c == '\\' && i+1 < len(body) {
				cur.WriteByte(body[i+1])
				i++
				continue
			}
			if c == quote {
				inString = false
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			inString = true
			quote = c
			cur.WriteByte(c)
		case '[', '{', '(':
			depth++
			cur.WriteByte(c)
		case ']', '}', ')':
			depth--
			cur.WriteByte(c)
		case ',':
			if depth == 0 {
				out = append(out, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteByte(c)
		default:
			cur.WriteByte(c)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, cur.String())
	}
	return out
}

// splitKeyValue splits "key: value" respecting that the value side may
// contain colons (e.g. inside strings). The first top-level colon is the
// separator.
func splitKeyValue(f string) (string, string, bool) {
	f = strings.TrimSpace(f)
	inString := false
	var quote byte
	for i := 0; i < len(f); i++ {
		c := f[i]
		if inString {
			if c == '\\' && i+1 < len(f) {
				i++
				continue
			}
			if c == quote {
				inString = false
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			inString = true
			quote = c
		case ':':
			key := strings.TrimSpace(f[:i])
			val := strings.TrimSpace(f[i+1:])
			return key, val, true
		}
	}
	return "", "", false
}

// unquoteTSString strips surrounding single/double/back quotes and processes
// the limited subset of escapes the glossary uses today: \' \" \\ \n \t.
func unquoteTSString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}
	if s[0] != '\'' && s[0] != '"' && s[0] != '`' {
		return s
	}
	if s[len(s)-1] != s[0] {
		return s
	}
	body := s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			switch body[i+1] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '\\':
				b.WriteByte('\\')
			case '\'':
				b.WriteByte('\'')
			case '"':
				b.WriteByte('"')
			case '`':
				b.WriteByte('`')
			default:
				b.WriteByte(body[i+1])
			}
			i++
			continue
		}
		b.WriteByte(body[i])
	}
	return b.String()
}

// parseStringArray reads ['a', 'b', "c"] into []string{"a", "b", "c"}.
func parseStringArray(s string) []string {
	s = strings.TrimSpace(s)
	body, err := sliceMatchingBracket(s, '[', ']')
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range splitTopLevelFields(body) {
		v := unquoteTSString(strings.TrimSpace(f))
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
