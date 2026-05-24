package rag

import (
	"path/filepath"
	"testing"
)

func TestLoadGlossaryFromTS(t *testing.T) {
	path := filepath.Join("..", "..", "web", "src", "data", "glossary.ts")
	entries, err := LoadGlossaryFromTS(path)
	if err != nil {
		t.Fatalf("LoadGlossaryFromTS: %v", err)
	}
	if len(entries) < 50 {
		t.Fatalf("len(entries) = %d, expected >= 50", len(entries))
	}

	// Verify a well-known entry is present with all expected fields.
	var pl3 *GlossaryEntry
	for i := range entries {
		if entries[i].Term == "USER MODE (PL3)" {
			pl3 = &entries[i]
			break
		}
	}
	if pl3 == nil {
		t.Fatal("entry 'USER MODE (PL3)' not found")
	}
	if pl3.Category != "Modos" {
		t.Errorf("category = %q, want Modos", pl3.Category)
	}
	if pl3.Short == "" || pl3.Detail == "" {
		t.Errorf("short or detail empty: short=%q detail=%q", pl3.Short, pl3.Detail)
	}

	// Verify aliases are parsed when present.
	var rpl *GlossaryEntry
	for i := range entries {
		if entries[i].Term == "RPL — Requested Privilege Level" {
			rpl = &entries[i]
			break
		}
	}
	if rpl == nil {
		t.Fatal("entry 'RPL — Requested Privilege Level' not found")
	}
	if len(rpl.Aliases) == 0 {
		t.Error("aliases should be parsed for RPL entry")
	}
}

func TestUnquoteTSStringEscapes(t *testing.T) {
	cases := map[string]string{
		`'hola'`:           "hola",
		`'tab\there'`:      "tab\there",
		`'newline\nhere'`:  "newline\nhere",
		`'it\'s ok'`:       "it's ok",
		`"double quoted"`:  "double quoted",
		"`backtick`":       "backtick",
	}
	for in, want := range cases {
		if got := unquoteTSString(in); got != want {
			t.Errorf("unquoteTSString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseStringArray(t *testing.T) {
	got := parseStringArray(`['a', 'b', "c"]`)
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("got %v", got)
	}
}
