package rag

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

const queryEmbedTimeout = 20 * time.Second

// Retriever performs cosine similarity search over an in-memory copy of the
// corpus. Loading is O(N) once at startup; subsequent queries are O(N×D)
// arithmetic, which is fast enough for the tutor scale (<10K chunks).
type Retriever struct {
	store        *Store
	embedder     Embedder
	corpus       []Chunk
	corpusNorms  []float32 // L2 norm precomputed per chunk
	corpusVer    int
	queryTimeout time.Duration
	mu           sync.RWMutex
}

// NewRetriever wires a store, an embedder configured for QUERY embeddings,
// and the corpus_version to search against. Call Reload to populate the
// in-memory copy before the first query.
func NewRetriever(store *Store, embedder Embedder, corpusVersion int) *Retriever {
	return &Retriever{
		store:        store,
		embedder:     embedder,
		corpusVer:    corpusVersion,
		queryTimeout: queryEmbedTimeout,
	}
}

// Reload re-reads the corpus from SQLite and precomputes per-chunk L2 norms
// for cosine similarity. Safe to call concurrently with queries; readers see
// a consistent snapshot via the read lock.
func (r *Retriever) Reload(ctx context.Context) error {
	chunks, err := r.store.ListChunks(ctx, r.corpusVer, "")
	if err != nil {
		return fmt.Errorf("rag/retriever: list: %w", err)
	}
	norms := make([]float32, len(chunks))
	for i := range chunks {
		norms[i] = l2norm(chunks[i].Embedding)
	}

	r.mu.Lock()
	r.corpus = chunks
	r.corpusNorms = norms
	r.mu.Unlock()
	return nil
}

// Len reports how many chunks the retriever currently holds.
func (r *Retriever) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.corpus)
}

// Retrieve returns the top-K chunks most similar to query. It uses the query
// cache to avoid double-embedding the same string.
func (r *Retriever) Retrieve(ctx context.Context, query string, topK int) ([]ScoredChunk, error) {
	if topK <= 0 {
		topK = 5
	}
	queryVec, err := r.embedQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	return r.RetrieveByVector(queryVec, topK), nil
}

// RetrieveByVector is the same as Retrieve but skips embedding (caller has
// the vector already). Useful in tests and when the same vector is reused.
func (r *Retriever) RetrieveByVector(queryVec []float32, topK int) []ScoredChunk {
	r.mu.RLock()
	defer r.mu.RUnlock()

	qn := l2norm(queryVec)
	if qn == 0 {
		return nil
	}
	scored := make([]ScoredChunk, 0, len(r.corpus))
	for i := range r.corpus {
		cn := r.corpusNorms[i]
		if cn == 0 {
			continue
		}
		sim := dot(queryVec, r.corpus[i].Embedding) / (qn * cn)
		scored = append(scored, ScoredChunk{Chunk: r.corpus[i], Score: sim})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if topK < len(scored) {
		scored = scored[:topK]
	}
	return scored
}

// embedQuery checks the cache, falls back to the embedder, and writes the
// result back to the cache.
func (r *Retriever) embedQuery(ctx context.Context, query string) ([]float32, error) {
	hash := HashQuery(query)
	if cached, ok, err := r.store.GetCachedQueryEmbedding(ctx, hash); err == nil && ok {
		return cached, nil
	}
	embedCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	vecs, err := r.embedder.Embed(embedCtx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("rag/retriever: embed query: %w", err)
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("rag/retriever: embedder returned %d vectors, want 1", len(vecs))
	}
	_ = r.store.PutQueryEmbedding(ctx, hash, vecs[0])
	return vecs[0], nil
}

func dot(a, b []float32) float32 {
	n := min(len(a), len(b))
	var s float32
	for i := range n {
		s += a[i] * b[i]
	}
	return s
}

func l2norm(v []float32) float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return float32(math.Sqrt(s))
}
