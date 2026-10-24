package rag

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeEmbedder struct {
	mu      sync.Mutex
	calls   int
	active  int
	peak    int
	hold    time.Duration
	failFor int
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	f.calls++
	f.active++
	if f.active > f.peak {
		f.peak = f.active
	}
	shouldFail := f.calls <= f.failFor
	f.mu.Unlock()
	time.Sleep(f.hold)
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	if shouldFail {
		return nil, ErrEmbedderRateLimited
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1}
	}
	return out, nil
}

func (f *fakeEmbedder) Model() string { return "fake" }
func (f *fakeEmbedder) Dim() int      { return 1 }

func fastBackoff() []time.Duration {
	return []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
}

func TestLimitedEmbedderCapsConcurrency(t *testing.T) {
	tests := []struct {
		name string
		max  int
	}{
		{name: "two slots", max: 2},
		{name: "one slot", max: 1},
		{name: "zero is clamped to one", max: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := &fakeEmbedder{hold: 5 * time.Millisecond}
			limited := newLimitedEmbedder(inner, tc.max, fastBackoff(), nil)
			var wg sync.WaitGroup
			var failures atomic.Int32
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := limited.Embed(context.Background(), []string{"q"}); err != nil {
						failures.Add(1)
					}
				}()
			}
			wg.Wait()
			want := max(tc.max, 1)
			if inner.peak > want {
				t.Fatalf("peak concurrency = %d, want <= %d", inner.peak, want)
			}
			if inner.calls != 12 || failures.Load() != 0 {
				t.Fatalf("calls = %d, failures = %d, want 12 and 0", inner.calls, failures.Load())
			}
		})
	}
}

func TestLimitedEmbedderRateLimit(t *testing.T) {
	tests := []struct {
		name      string
		failFor   int
		wantErr   bool
		wantCalls int
		wantWarn  bool
	}{
		{name: "transient recovers", failFor: 1, wantCalls: 2},
		{name: "fails three times then recovers", failFor: 3, wantCalls: 4},
		{name: "persistent errors after four retries", failFor: 100, wantErr: true, wantCalls: 5, wantWarn: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			inner := &fakeEmbedder{failFor: tc.failFor}
			limited := newLimitedEmbedder(inner, 2, fastBackoff(), logger)
			vecs, err := limited.Embed(context.Background(), []string{"q"})
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "rate limited") {
					t.Fatalf("err = %v, want rate limited", err)
				}
			} else if err != nil || len(vecs) != 1 {
				t.Fatalf("vecs = %v, err = %v", vecs, err)
			}
			if inner.calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", inner.calls, tc.wantCalls)
			}
			if got := strings.Contains(logs.String(), "level=WARN"); got != tc.wantWarn {
				t.Fatalf("warn logged = %v, want %v; logs: %s", got, tc.wantWarn, logs.String())
			}
		})
	}
}

func TestLimitedEmbedderContext(t *testing.T) {
	t.Run("expired context never reaches the embedder", func(t *testing.T) {
		inner := &fakeEmbedder{}
		limited := newLimitedEmbedder(inner, 1, fastBackoff(), nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := limited.Embed(ctx, []string{"q"}); err == nil {
			t.Fatal("expected error")
		}
		if inner.calls != 0 {
			t.Fatalf("calls = %d, want 0", inner.calls)
		}
	})

	t.Run("deadline cuts the wait in the queue", func(t *testing.T) {
		inner := &fakeEmbedder{hold: 200 * time.Millisecond}
		limited := newLimitedEmbedder(inner, 1, fastBackoff(), nil)
		go func() { _, _ = limited.Embed(context.Background(), []string{"slow"}) }()
		time.Sleep(20 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := limited.Embed(ctx, []string{"queued"})
		if err == nil {
			t.Fatal("expected error")
		}
		if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
			t.Fatalf("waited %v, want under 150ms", elapsed)
		}
		inner.mu.Lock()
		calls := inner.calls
		inner.mu.Unlock()
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
	})

	t.Run("deadline cuts the retry wait", func(t *testing.T) {
		inner := &fakeEmbedder{failFor: 100}
		slow := []time.Duration{time.Hour}
		limited := newLimitedEmbedder(inner, 1, slow, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, err := limited.Embed(ctx, []string{"q"}); err == nil {
			t.Fatal("expected error")
		}
		if time.Since(start) > time.Second {
			t.Fatal("retry wait ignored the context")
		}
	})
}

func TestLimitedEmbedderPassesThroughMetadata(t *testing.T) {
	limited := NewLimitedEmbedder(&fakeEmbedder{}, 2, nil)
	if limited.Model() != "fake" || limited.Dim() != 1 {
		t.Fatalf("Model = %q, Dim = %d", limited.Model(), limited.Dim())
	}
}
