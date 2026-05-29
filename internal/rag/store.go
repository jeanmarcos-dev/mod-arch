// Package rag contains the retrieval-augmented-generation pipeline used by
// the tutor: chunking, embeddings, vector storage and similarity search.
// Vectors are stored as little-endian []float32 BLOBs inside the same SQLite
// file that the rest of the app uses, so there is no separate vector store
// to operate.
package rag

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// Source identifies where a chunk came from. New sources can be added without
// touching the schema; the value is just stored as TEXT.
type Source string

const (
	SourceGlossary  Source = "glossary"
	SourceSilberschatz Source = "silberschatz"
	// Project-native sources cover what neither the textbook nor the
	// glossary can: the simulator's own demos, its simplified ISA and the
	// repo's educational docs. They let the tutor speak about this exact
	// codebase instead of generic x86.
	SourceDemos Source = "demos"
	SourceISA   Source = "isa"
	SourceDocs  Source = "docs"
)

// Chunk is one indexed unit of knowledge. ID is set by the database on insert.
type Chunk struct {
	ID            int64
	Source        Source
	SourceID      string
	Text          string
	Metadata      map[string]any
	Embedding     []float32
	CorpusVersion int
	CreatedAt     time.Time
}

// ScoredChunk pairs a Chunk with its similarity score (0..1, higher is better).
type ScoredChunk struct {
	Chunk Chunk
	Score float32
}

// Store is the SQLite-backed persistence layer for RAG state. It is safe for
// concurrent use; *sql.DB handles connection pooling.
type Store struct {
	db *sql.DB
}

// NewStore wraps a *sql.DB. The DB must already be migrated to schema v5+.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// InsertChunks writes a batch of chunks in a single transaction. Embedding
// length must be consistent within the batch (the retriever assumes a fixed
// dimensionality per corpus_version).
func (s *Store) InsertChunks(ctx context.Context, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rag/store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO rag_chunks(source, source_id, text, metadata, embedding, corpus_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("rag/store: prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().Unix()
	for i := range chunks {
		c := &chunks[i]
		meta := "{}"
		if c.Metadata != nil {
			b, err := json.Marshal(c.Metadata)
			if err != nil {
				return fmt.Errorf("rag/store: marshal metadata: %w", err)
			}
			meta = string(b)
		}
		if _, err := stmt.ExecContext(ctx,
			string(c.Source), c.SourceID, c.Text, meta,
			encodeEmbedding(c.Embedding), c.CorpusVersion, now,
		); err != nil {
			return fmt.Errorf("rag/store: insert chunk %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// DeleteCorpusVersion removes every chunk at a given corpus_version. Used to
// rebuild an index without piling up stale rows.
func (s *Store) DeleteCorpusVersion(ctx context.Context, version int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM rag_chunks WHERE corpus_version = ?`, version)
	if err != nil {
		return 0, fmt.Errorf("rag/store: delete version: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListChunks returns every chunk at a given corpus_version. Filters by source
// when source != "". The retriever calls this once at startup and keeps the
// slice in memory for similarity search.
func (s *Store) ListChunks(ctx context.Context, version int, source Source) ([]Chunk, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if source == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, source, source_id, text, metadata, embedding, corpus_version, created_at
			FROM rag_chunks WHERE corpus_version = ?
		`, version)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, source, source_id, text, metadata, embedding, corpus_version, created_at
			FROM rag_chunks WHERE corpus_version = ? AND source = ?
		`, version, string(source))
	}
	if err != nil {
		return nil, fmt.Errorf("rag/store: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Chunk
	for rows.Next() {
		var (
			c        Chunk
			srcStr   string
			metaStr  string
			embBytes []byte
			createdU int64
		)
		if err := rows.Scan(&c.ID, &srcStr, &c.SourceID, &c.Text, &metaStr, &embBytes, &c.CorpusVersion, &createdU); err != nil {
			return nil, fmt.Errorf("rag/store: scan: %w", err)
		}
		c.Source = Source(srcStr)
		c.CreatedAt = time.Unix(createdU, 0)
		c.Embedding = decodeEmbedding(embBytes)
		if metaStr != "" {
			_ = json.Unmarshal([]byte(metaStr), &c.Metadata)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountByCorpus returns how many chunks exist at the given version, broken
// down by source. Useful for the indexer's sanity-check output.
func (s *Store) CountByCorpus(ctx context.Context, version int) (map[Source]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT source, COUNT(*) FROM rag_chunks WHERE corpus_version = ? GROUP BY source
	`, version)
	if err != nil {
		return nil, fmt.Errorf("rag/store: count: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[Source]int)
	for rows.Next() {
		var (
			src string
			n   int
		)
		if err := rows.Scan(&src, &n); err != nil {
			return nil, err
		}
		out[Source(src)] = n
	}
	return out, rows.Err()
}

// GetCachedQueryEmbedding returns the embedding for queryHash if present and
// bumps its hits/last_used counters. The second return is false when missing.
func (s *Store) GetCachedQueryEmbedding(ctx context.Context, queryHash string) ([]float32, bool, error) {
	var embBytes []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT embedding FROM rag_query_cache WHERE query_hash = ?`, queryHash,
	).Scan(&embBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("rag/store: query cache get: %w", err)
	}
	now := time.Now().Unix()
	_, _ = s.db.ExecContext(ctx,
		`UPDATE rag_query_cache SET hits_count = hits_count + 1, last_used = ? WHERE query_hash = ?`,
		now, queryHash,
	)
	return decodeEmbedding(embBytes), true, nil
}

// PutQueryEmbedding stores an embedding for queryHash. Upsert semantics: if
// the row already exists, refresh last_used without touching hits_count.
func (s *Store) PutQueryEmbedding(ctx context.Context, queryHash string, emb []float32) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rag_query_cache(query_hash, embedding, hits_count, last_used)
		VALUES (?, ?, 1, ?)
		ON CONFLICT(query_hash) DO UPDATE SET last_used = excluded.last_used
	`, queryHash, encodeEmbedding(emb), now)
	if err != nil {
		return fmt.Errorf("rag/store: query cache put: %w", err)
	}
	return nil
}

// CachedAnswer is the payload of the answer cache.
type CachedAnswer struct {
	Answer    string
	Citations []byte // raw JSON; the tutor decodes into its own type
}

// GetCachedAnswer returns a cached answer if present and bumps last_used.
func (s *Store) GetCachedAnswer(ctx context.Context, cacheKey string) (*CachedAnswer, error) {
	var ca CachedAnswer
	err := s.db.QueryRowContext(ctx,
		`SELECT answer, citations FROM rag_answer_cache WHERE cache_key = ?`, cacheKey,
	).Scan(&ca.Answer, &ca.Citations)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("rag/store: answer cache get: %w", err)
	}
	now := time.Now().Unix()
	_, _ = s.db.ExecContext(ctx,
		`UPDATE rag_answer_cache SET last_used = ? WHERE cache_key = ?`, now, cacheKey,
	)
	return &ca, nil
}

// PutAnswer caches a generated answer. citationsJSON is opaque to this
// package; the tutor is responsible for its shape.
func (s *Store) PutAnswer(ctx context.Context, cacheKey, answer string, citationsJSON []byte) error {
	now := time.Now().Unix()
	if len(citationsJSON) == 0 {
		citationsJSON = []byte("[]")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rag_answer_cache(cache_key, answer, citations, created_at, last_used)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			answer = excluded.answer,
			citations = excluded.citations,
			last_used = excluded.last_used
	`, cacheKey, answer, string(citationsJSON), now, now)
	if err != nil {
		return fmt.Errorf("rag/store: answer cache put: %w", err)
	}
	return nil
}

// SweepAnswerCache deletes answer-cache rows whose last_used is older than
// olderThan. Returns the number of rows removed.
func (s *Store) SweepAnswerCache(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM rag_answer_cache WHERE last_used < ?`, olderThan.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("rag/store: sweep: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// encodeEmbedding packs a []float32 as little-endian bytes. We use the IEEE
// 754 layout so a future migration to sqlite-vec (which expects the same
// representation) is a drop-in switch.
func encodeEmbedding(v []float32) []byte {
	buf := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(x))
	}
	return buf
}

func decodeEmbedding(buf []byte) []float32 {
	if len(buf)%4 != 0 {
		return nil
	}
	out := make([]float32, len(buf)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return out
}
