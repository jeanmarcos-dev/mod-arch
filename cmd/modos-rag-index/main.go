// modos-rag-index builds (or rebuilds) the RAG corpus inside a SQLite file.
// Reads source materials (glossary, PDFs), produces chunks, embeds them via
// Jina, and writes rows into rag_chunks at a fresh corpus_version. The Tutor
// orchestrator points to the active version through the settings table.
//
//	export JINA_API_KEY=...
//	go run ./cmd/modos-rag-index -source glossary -db ./data/modos-cpu.db
//
// Flags:
//
//	-source     one of: glossary
//	-db         path to the SQLite file (default ./data/modos-cpu.db)
//	-glossary   path to web/src/data/glossary.ts (default auto-detected)
//	-version    new corpus version; defaults to (current max)+1
//	-purge      delete existing rows for the chosen version before inserting
//	-batch      embedder batch size (default 16)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/envfile"
	"github.com/jeanmarcos/modos-cpu/internal/programs"
	"github.com/jeanmarcos/modos-cpu/internal/rag"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

func main() {
	_ = envfile.Load(".env")

	source := flag.String("source", "glossary", "corpus a indexar: glossary | pdf | demos | isa | docs | all")
	dbPath := flag.String("db", "", "ruta al SQLite (default: misma que usa el server, vía store.DefaultPath)")
	glossaryPath := flag.String("glossary", defaultGlossaryPath(), "ruta a glossary.ts")
	pdfPath := flag.String("pdf", "", "ruta al PDF (cuando -source=pdf o all)")
	pdfSourceID := flag.String("pdf-source-id", "silberschatz-9e-es", "identificador corto del libro, usado en citas")
	chunkTokens := flag.Int("chunk-tokens", 450, "tamaño objetivo de chunk para PDF (en tokens)")
	chunkOverlap := flag.Float64("chunk-overlap", 0.10, "overlap entre chunks de PDF (0..0.5)")
	repoRoot := flag.String("repo-root", ".", "ruta al repo (para -source=docs o all)")
	version := flag.Int("version", 0, "corpus_version a escribir (0 = auto)")
	purge := flag.Bool("purge", false, "borrar rag_chunks de esa version antes de insertar")
	batchSize := flag.Int("batch", 16, "batch del embedder")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	provider := strings.ToLower(strings.TrimSpace(os.Getenv("EMBEDDER_PROVIDER")))
	if provider == "" {
		// Sensible auto-pick: Ollama if reachable locally, Jina if a real
		// key is in the environment. Default to Jina when both are missing
		// so the existing user flow keeps working.
		if strings.TrimSpace(os.Getenv("OLLAMA_HOST")) != "" {
			provider = "ollama"
		} else {
			provider = "jina"
		}
	}

	resolvedDBPath := *dbPath
	if resolvedDBPath == "" {
		p, err := store.DefaultPath()
		if err != nil {
			fail("resolviendo DB path por default: " + err.Error())
		}
		resolvedDBPath = p
	}
	st, err := store.Open(resolvedDBPath)
	if err != nil {
		fail("abriendo store: " + err.Error())
	}
	defer func() { _ = st.Close() }()

	rs := rag.NewStore(st.DB())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	resolvedVersion, err := resolveVersion(ctx, st, *version)
	if err != nil {
		fail("resolviendo version: " + err.Error())
	}
	logger.Info("indexando corpus", "source", *source, "corpus_version", resolvedVersion, "db", resolvedDBPath)

	if *purge {
		n, err := rs.DeleteCorpusVersion(ctx, resolvedVersion)
		if err != nil {
			fail("purge: " + err.Error())
		}
		logger.Info("purga previa", "rows_deleted", n)
	}

	embedder, err := buildEmbedder(provider, "passage", logger)
	if err != nil {
		fail("inicializando embedder: " + err.Error())
	}

	chunkOpts := rag.ChunkOptions{TargetTokens: *chunkTokens, OverlapRatio: *chunkOverlap}

	var rawDocs []rag.RawDoc
	switch *source {
	case "glossary":
		rawDocs = mustLoadGlossary(logger, *glossaryPath)
	case "pdf":
		if *pdfPath == "" {
			fail("debe pasar -pdf <ruta> con -source=pdf")
		}
		rawDocs = mustLoadPDF(logger, *pdfPath, *pdfSourceID, chunkOpts)
	case "demos":
		rawDocs = mustLoadDemos(logger)
	case "isa":
		rawDocs = mustLoadISA(logger)
	case "docs":
		rawDocs = mustLoadDocs(logger, *repoRoot)
	case "all":
		rawDocs = append(rawDocs, mustLoadGlossary(logger, *glossaryPath)...)
		if *pdfPath != "" {
			rawDocs = append(rawDocs, mustLoadPDF(logger, *pdfPath, *pdfSourceID, chunkOpts)...)
		} else {
			logger.Info("PDF omitido en -source=all (no se pasó -pdf)")
		}
		rawDocs = append(rawDocs, mustLoadDemos(logger)...)
		rawDocs = append(rawDocs, mustLoadISA(logger)...)
		rawDocs = append(rawDocs, mustLoadDocs(logger, *repoRoot)...)
	default:
		fail("source desconocida: " + *source)
	}

	logger.Info("chunks a embeber", "n", len(rawDocs))
	if len(rawDocs) == 0 {
		fail("no hay nada para indexar")
	}

	start := time.Now()
	chunks, err := embedDocs(ctx, embedder, rawDocs, *batchSize, resolvedVersion, logger)
	if err != nil {
		fail("embedding: " + err.Error())
	}
	logger.Info("embeddings listos", "n", len(chunks), "elapsed", time.Since(start).Round(time.Millisecond))

	if err := rs.InsertChunks(ctx, chunks); err != nil {
		fail("insertando chunks: " + err.Error())
	}

	counts, _ := rs.CountByCorpus(ctx, resolvedVersion)
	if err := setActiveCorpusVersion(ctx, st, resolvedVersion); err != nil {
		fail("registrando version activa: " + err.Error())
	}
	logger.Info("indexación completa", "counts", counts, "active_version", resolvedVersion)
}

// resolveVersion picks the corpus_version to write into. If the caller passed
// --version=N, honor it. Otherwise read settings(rag.corpus_version) and add 1.
func resolveVersion(ctx context.Context, st *store.Store, requested int) (int, error) {
	if requested > 0 {
		return requested, nil
	}
	current, _ := readActiveCorpusVersion(ctx, st)
	return current + 1, nil
}

const activeCorpusVersionKey = "rag.corpus_version"

func readActiveCorpusVersion(ctx context.Context, st *store.Store) (int, error) {
	row := st.DB().QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, activeCorpusVersionKey)
	var val string
	if err := row.Scan(&val); err != nil {
		return 0, err
	}
	var n int
	_, err := fmt.Sscanf(val, "%d", &n)
	return n, err
}

func setActiveCorpusVersion(ctx context.Context, st *store.Store, version int) error {
	now := time.Now().Unix()
	_, err := st.DB().ExecContext(ctx, `
		INSERT INTO settings(key, value, updated_at, updated_by)
		VALUES (?, ?, ?, 'modos-rag-index')
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, activeCorpusVersionKey, fmt.Sprintf("%d", version), now)
	return err
}

// embedDocs runs the embedder in batches so a 1000-chunk corpus does not get
// shipped as one giant HTTP body. Each batch is a single round-trip.
func embedDocs(ctx context.Context, emb rag.Embedder, docs []rag.RawDoc, batch, version int, logger *slog.Logger) ([]rag.Chunk, error) {
	if batch <= 0 {
		batch = 16
	}
	out := make([]rag.Chunk, 0, len(docs))
	for i := 0; i < len(docs); i += batch {
		end := min(i+batch, len(docs))
		texts := make([]string, end-i)
		for j, d := range docs[i:end] {
			texts[j] = d.Text
		}
		vecs, err := emb.Embed(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}
		if len(vecs) != len(texts) {
			return nil, errors.New("embedder returned wrong vector count")
		}
		for j, d := range docs[i:end] {
			out = append(out, rag.Chunk{
				Source:        d.Source,
				SourceID:      d.SourceID,
				Text:          d.Text,
				Metadata:      d.Metadata,
				Embedding:     vecs[j],
				CorpusVersion: version,
			})
		}
		logger.Info("batch embebido", "rango", fmt.Sprintf("%d-%d", i, end))
	}
	return out, nil
}

// defaultGlossaryPath assumes the binary is run from the repo root.
func defaultGlossaryPath() string {
	return filepath.Join("web", "src", "data", "glossary.ts")
}

// buildEmbedder picks the right embedder backend from env vars. side is
// "passage" during indexing and "query" at retrieval time; only Jina uses
// it (Ollama treats both identically).
func buildEmbedder(provider, side string, logger *slog.Logger) (rag.Embedder, error) {
	switch provider {
	case "ollama":
		host := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
		if host == "" {
			host = rag.OllamaDefaultURL
		}
		model := strings.TrimSpace(os.Getenv("OLLAMA_EMBED_MODEL"))
		if model == "" {
			model = rag.OllamaDefaultModel
		}
		logger.Info("embedder: ollama", "model", model, "host", host)
		return rag.NewOllama(rag.OllamaConfig{BaseURL: host, Model: model})
	case "jina":
		key := strings.TrimSpace(os.Getenv("JINA_API_KEY"))
		if key == "" || strings.HasPrefix(key, "jina_PLACEHOLDER") {
			return nil, fmt.Errorf("JINA_API_KEY no está definida (o es placeholder); use EMBEDDER_PROVIDER=ollama para self-host")
		}
		task := "retrieval.passage"
		if side == "query" {
			task = "retrieval.query"
		}
		logger.Info("embedder: jina", "model", rag.JinaDefaultModel, "task", task)
		return rag.NewJina(rag.JinaConfig{APIKey: key, Task: task})
	default:
		return nil, fmt.Errorf("EMBEDDER_PROVIDER desconocido: %q (use ollama o jina)", provider)
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "modos-rag-index: "+msg)
	os.Exit(1)
}

// Per-source loaders share the same pattern: log a summary and panic via
// fail() on error. Returning []RawDoc keeps the switch in main short.

func mustLoadGlossary(logger *slog.Logger, path string) []rag.RawDoc {
	entries, err := rag.LoadGlossaryFromTS(path)
	if err != nil {
		fail("cargando glosario: " + err.Error())
	}
	logger.Info("glosario cargado", "entries", len(entries), "path", path)
	return rag.ChunkGlossaryEntries(entries)
}

func mustLoadPDF(logger *slog.Logger, pdfPath, sourceID string, opts rag.ChunkOptions) []rag.RawDoc {
	pages, err := rag.LoadPDFAsRawDocs(pdfPath, sourceID)
	if err != nil {
		fail("cargando PDF: " + err.Error())
	}
	logger.Info("PDF cargado", "pages", len(pages), "path", pdfPath, "source_id", sourceID)
	out := make([]rag.RawDoc, 0, len(pages)*2)
	for _, page := range pages {
		out = append(out, rag.SplitParagraphs(page, opts)...)
	}
	return out
}

func mustLoadDemos(logger *slog.Logger) []rag.RawDoc {
	docs := rag.LoadDemosAsRawDocs(programs.Demos())
	logger.Info("demos cargadas", "n", len(docs))
	return docs
}

func mustLoadISA(logger *slog.Logger) []rag.RawDoc {
	docs := rag.LoadISAAsRawDocs()
	logger.Info("ISA cargado", "n", len(docs))
	return docs
}

func mustLoadDocs(logger *slog.Logger, repoRoot string) []rag.RawDoc {
	docs, err := rag.LoadDocsAsRawDocs(repoRoot)
	if err != nil {
		fail("cargando docs: " + err.Error())
	}
	logger.Info("docs cargados", "n", len(docs), "root", repoRoot)
	return docs
}
