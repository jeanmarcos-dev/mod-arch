package rag

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJinaEmbedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
            "data": [
                {"index": 0, "embedding": [0.1, 0.2, 0.3]},
                {"index": 1, "embedding": [0.4, 0.5, 0.6]}
            ]
        }`)
	}))
	defer srv.Close()

	emb, err := NewJina(JinaConfig{APIKey: "k", BaseURL: srv.URL, Dim: 3})
	if err != nil {
		t.Fatalf("NewJina: %v", err)
	}
	vecs, err := emb.Embed(context.Background(), []string{"hola", "mundo"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 {
		t.Fatalf("unexpected shape: %v", vecs)
	}
	if vecs[0][0] != 0.1 || vecs[1][2] != 0.6 {
		t.Errorf("values mismatch: %v", vecs)
	}
}

func TestJinaEmbedOutOfOrderResponse(t *testing.T) {
	// Jina may not return data in input order; the embedder must respect
	// the `index` field. We send back items in reverse order on purpose.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
            "data": [
                {"index": 1, "embedding": [9, 9, 9]},
                {"index": 0, "embedding": [1, 1, 1]}
            ]
        }`)
	}))
	defer srv.Close()

	emb, _ := NewJina(JinaConfig{APIKey: "k", BaseURL: srv.URL})
	vecs, err := emb.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if vecs[0][0] != 1 || vecs[1][0] != 9 {
		t.Errorf("order not respected: %v", vecs)
	}
}

func TestJinaEmbedErrorClassification(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrEmbedderAuthFailed},
		{http.StatusTooManyRequests, ErrEmbedderRateLimited},
		{http.StatusInternalServerError, ErrEmbedderServer},
		{http.StatusBadRequest, ErrEmbedderBadRequest},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, `{"detail":"boom"}`)
		}))
		emb, _ := NewJina(JinaConfig{APIKey: "k", BaseURL: srv.URL})
		_, err := emb.Embed(context.Background(), []string{"x"})
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: got %v, want %v", tc.status, err, tc.want)
		}
		srv.Close()
	}
}

func TestHashQueryNormalization(t *testing.T) {
	if HashQuery("¿qué es PL3?") != HashQuery("  ¿QUÉ ES PL3?  ") {
		t.Error("HashQuery should be case- and whitespace-insensitive")
	}
}
