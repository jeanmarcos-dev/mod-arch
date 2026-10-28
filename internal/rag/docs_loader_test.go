package rag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDocsIndexesSpanishReadmeWithoutOperationalSections(t *testing.T) {
	root := t.TempDir()
	readme := strings.Join([]string{
		"# MOD-ARCH",
		"## Stack", "Go y Vue.",
		"## ISA", "Subconjunto x86 de 32 bits.",
		"## Inicio rápido", "make dev",
		"## Tutor (ARCHIUS)", "Tutor con RAG.",
		"### Corpus propio", "go run ./cmd/modos-rag-index",
		"## Configuración", "SESSION_SECRET=",
		"## Licencia", "MIT",
	}, "\n\n")
	if err := os.WriteFile(filepath.Join(root, "README.es.md"), []byte(readme), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("## ISA\n\nEnglish copy that must not be indexed."), 0o600); err != nil {
		t.Fatal(err)
	}

	docs, err := LoadDocsAsRawDocs(root)
	if err != nil {
		t.Fatalf("LoadDocsAsRawDocs: %v", err)
	}
	var all strings.Builder
	for _, d := range docs {
		all.WriteString(d.Text)
		all.WriteString("\n")
	}
	text := all.String()
	for _, want := range []string{"Subconjunto x86", "Tutor con RAG"} {
		if !strings.Contains(text, want) {
			t.Errorf("indexed docs miss %q", want)
		}
	}
	for _, unwanted := range []string{"Go y Vue", "make dev", "modos-rag-index", "SESSION_SECRET", "MIT", "English copy"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("indexed docs include operational or English text %q", unwanted)
		}
	}
}
