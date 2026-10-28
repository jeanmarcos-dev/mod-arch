package tutor

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/llm"
	"github.com/jeanmarcos-dev/mod-arch/internal/rag"
	storepkg "github.com/jeanmarcos-dev/mod-arch/internal/store"
)

type stubLLM struct {
	calls   int
	resp    string
	err     error
	lastReq llm.ChatRequest // captured for assertions on prompt shape
}

func (s *stubLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	s.calls++
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	return &llm.ChatResponse{
		Content:      s.resp,
		Model:        "stub-model",
		FinishReason: "stop",
		Usage:        llm.Usage{TotalTokens: 42},
	}, nil
}
func (s *stubLLM) Name() string { return "stub" }

type stubEmbedderSvc struct{}

func (stubEmbedderSvc) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.5, 0.5, 0.5}
	}
	return out, nil
}
func (stubEmbedderSvc) Model() string { return "stub-emb" }
func (stubEmbedderSvc) Dim() int      { return 3 }

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	st, err := storepkg.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st.DB()
}

func setupService(t *testing.T, l llm.Provider) *Service {
	t.Helper()
	db := openDB(t)
	rs := rag.NewStore(db)
	_ = rs.InsertChunks(context.Background(), []rag.Chunk{
		{Source: rag.SourceGlossary, SourceID: "PL3", Text: "user mode (PL3): aplicaciones corren aquí", Embedding: []float32{0.5, 0.5, 0.5}, CorpusVersion: 1},
		{Source: rag.SourceGlossary, SourceID: "PL0", Text: "kernel mode (PL0): privilegio total", Embedding: []float32{0.5, 0.5, 0.5}, CorpusVersion: 1},
	})
	ret := rag.NewRetriever(rs, stubEmbedderSvc{}, 1)
	if err := ret.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(testLogWriter{t: t}, nil))
	return New(ret, rs, l, logger, Options{TopK: 2, CorpusVersion: 1})
}

type testLogWriter struct{ t *testing.T }

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func TestAskHappyPath(t *testing.T) {
	llmStub := &stubLLM{resp: "PL3 es el modo usuario. [Glosario: PL3]"}
	svc := setupService(t, llmStub)
	defer svc.Close()

	ans, err := svc.Ask(context.Background(), nil, "¿qué es PL3?", nil)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(ans.Text, "PL3") {
		t.Errorf("text = %q", ans.Text)
	}
	if len(ans.Citations) == 0 {
		t.Error("expected citations")
	}
	if ans.CacheHit {
		t.Error("first call should not be a cache hit")
	}
}

func TestAskUsesAnswerCache(t *testing.T) {
	llmStub := &stubLLM{resp: "respuesta original"}
	svc := setupService(t, llmStub)
	defer svc.Close()

	_, _ = svc.Ask(context.Background(), nil, "¿qué es PL3?", nil)
	ans, err := svc.Ask(context.Background(), nil, "¿qué es PL3?", nil)
	if err != nil {
		t.Fatalf("second Ask: %v", err)
	}
	if !ans.CacheHit {
		t.Error("second identical question should hit answer cache")
	}
	if llmStub.calls != 1 {
		t.Errorf("llm calls = %d, want 1 (second served from cache)", llmStub.calls)
	}
}

func TestAskFallbackOnLLMError(t *testing.T) {
	llmStub := &stubLLM{err: errors.New("upstream down")}
	svc := setupService(t, llmStub)
	defer svc.Close()

	ans, err := svc.Ask(context.Background(), nil, "¿qué es PL3?", nil)
	if err != nil {
		t.Fatalf("Ask should not fail when LLM is down: %v", err)
	}
	if !ans.Degraded {
		t.Error("expected Degraded=true on LLM failure")
	}
	if !strings.Contains(ans.Text, "modelo de lenguaje") {
		t.Errorf("fallback text missing disclaimer: %q", ans.Text)
	}
	if len(ans.Citations) == 0 {
		t.Error("fallback should still include citations")
	}
}

func TestAskHistoryWindowTruncation(t *testing.T) {
	llmStub := &stubLLM{resp: "respuesta"}
	svc := setupService(t, llmStub)
	defer svc.Close()
	svc.opts.HistoryWindowTurns = 2 // keep only 2 turns = 4 messages

	history := []HistoryTurn{
		{Role: "user", Text: "q1"},
		{Role: "assistant", Text: "a1"},
		{Role: "user", Text: "q2"},
		{Role: "assistant", Text: "a2"},
		{Role: "user", Text: "q3"},
		{Role: "assistant", Text: "a3"},
	}
	_, err := svc.AskWithOptions(context.Background(), nil, "follow-up", AskOptions{History: history}, nil)
	if err != nil {
		t.Fatalf("AskWithOptions: %v", err)
	}
	// Expect: system + last 4 history messages + final user → 6 messages.
	if got := len(llmStub.lastReq.Messages); got != 6 {
		t.Fatalf("len(messages) = %d, want 6 (system+4history+user)", got)
	}
	// The retained history should be the last two turns (q2/a2/q3/a3).
	wantContents := []string{"q2", "a2", "q3", "a3"}
	for i, want := range wantContents {
		got := llmStub.lastReq.Messages[i+1].Content
		if got != want {
			t.Errorf("messages[%d] = %q, want %q", i+1, got, want)
		}
	}
}

func TestAskHistoryInvalidatesCache(t *testing.T) {
	llmStub := &stubLLM{resp: "respuesta A"}
	svc := setupService(t, llmStub)
	defer svc.Close()
	svc.opts.HistoryWindowTurns = 3

	histA := []HistoryTurn{{Role: "user", Text: "primera pregunta"}, {Role: "assistant", Text: "primera respuesta"}}
	histB := []HistoryTurn{{Role: "user", Text: "otra pregunta"}, {Role: "assistant", Text: "otra respuesta"}}

	_, _ = svc.AskWithOptions(context.Background(), nil, "follow-up", AskOptions{History: histA}, nil)
	ans, err := svc.AskWithOptions(context.Background(), nil, "follow-up", AskOptions{History: histB}, nil)
	if err != nil {
		t.Fatalf("AskWithOptions: %v", err)
	}
	if ans.CacheHit {
		t.Error("different history should not reuse the cached answer")
	}
	if llmStub.calls != 2 {
		t.Errorf("llm calls = %d, want 2 (history change forces a new call)", llmStub.calls)
	}
}

func TestAskRateLimit(t *testing.T) {
	llmStub := &stubLLM{resp: "ok"}
	// Build a service with a tiny limit so we trip it quickly.
	db := openDB(t)
	rs := rag.NewStore(db)
	_ = rs.InsertChunks(context.Background(), []rag.Chunk{
		{Source: rag.SourceGlossary, SourceID: "x", Text: "x", Embedding: []float32{1, 0, 0}, CorpusVersion: 1},
	})
	ret := rag.NewRetriever(rs, stubEmbedderSvc{}, 1)
	_ = ret.Reload(context.Background())
	svc := New(ret, rs, llmStub, slog.Default(), Options{TopK: 1, CorpusVersion: 1, RateLimitMax: 2, RateLimitWindow: time.Hour})
	defer svc.Close()

	// Build a fake session ID. We can't easily construct a *sessions.Session
	// here without circular deps, so we drive the limiter through its
	// internal API by passing a stub with an ID via the public path. The
	// orchestrator skips the limiter when sess is nil, so we exercise the
	// limiter directly to make sure the wiring at the orchestrator level is
	// what catches the cap.
	for i := range 2 {
		if ok, _ := svc.limiter.Allow("sess1"); !ok {
			t.Fatalf("iter %d should be allowed", i)
		}
	}
	if ok, _ := svc.limiter.Allow("sess1"); ok {
		t.Error("third call should be rate-limited")
	}
}

func TestStripCitationTokens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no tokens leaves text untouched",
			in:   "El planificador elige el próximo proceso.",
			want: "El planificador elige el próximo proceso.",
		},
		{
			name: "single trailing demo token",
			in:   "Round robin reparte CPU por quantum. [Demo: scheduler_round_robin]",
			want: "Round robin reparte CPU por quantum.",
		},
		{
			name: "multiple trailing tokens on their own lines",
			in:   "Texto del tutor.\n\n[Demo: scheduler_round_robin]\n[Docs: Procesos]\n[Docs: Planificación]",
			want: "Texto del tutor.",
		},
		{
			name: "comma-separated enumeration in prose",
			in:   "Ver [Demo: a], [Docs: b] para detalles.",
			want: "Ver, para detalles.",
		},
		{
			name: "silberschatz with page number",
			in:   "Mode switch detallado. [Silberschatz p. 132]",
			want: "Mode switch detallado.",
		},
		{
			name: "glosario and isa tokens",
			in:   "El EIP avanza. [Glosario: EIP] [ISA: jmp]",
			want: "El EIP avanza.",
		},
		{
			name: "markdown link must survive",
			in:   "Consulte la [guía oficial](https://example.org/x) para más.",
			want: "Consulte la [guía oficial](https://example.org/x) para más.",
		},
		{
			name: "trailing comma and semicolon trimmed",
			in:   "Tres ejemplos: [Demo: a], [Demo: b]; [Docs: c]",
			want: "Tres ejemplos:",
		},
		{
			name: "empty input stays empty",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripCitationTokens(tc.in)
			if got != tc.want {
				t.Errorf("stripCitationTokens(%q)\n got:  %q\n want: %q", tc.in, got, tc.want)
			}
		})
	}
}
