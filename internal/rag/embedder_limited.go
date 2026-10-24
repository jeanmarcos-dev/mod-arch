package rag

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

var defaultRateLimitBackoff = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2 * time.Second,
}

type LimitedEmbedder struct {
	inner   Embedder
	slots   chan struct{}
	backoff []time.Duration
	logger  *slog.Logger
}

func NewLimitedEmbedder(inner Embedder, maxConcurrent int, logger *slog.Logger) *LimitedEmbedder {
	return newLimitedEmbedder(inner, maxConcurrent, defaultRateLimitBackoff, logger)
}

func newLimitedEmbedder(inner Embedder, maxConcurrent int, backoff []time.Duration, logger *slog.Logger) *LimitedEmbedder {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LimitedEmbedder{
		inner:   inner,
		slots:   make(chan struct{}, maxConcurrent),
		backoff: backoff,
		logger:  logger,
	}
}

func (l *LimitedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	for attempt := 0; ; attempt++ {
		vecs, err := l.embedOnce(ctx, texts)
		if !errors.Is(err, ErrEmbedderRateLimited) {
			return vecs, err
		}
		if attempt >= len(l.backoff) {
			l.logger.Warn("embedder rate limit retries exhausted", "retries", len(l.backoff), "err", err)
			return nil, err
		}
		if waitErr := sleepCtx(ctx, l.backoff[attempt]); waitErr != nil {
			l.logger.Warn("embedder rate limit retry aborted", "attempt", attempt+1, "err", waitErr)
			return nil, err
		}
	}
}

func (l *LimitedEmbedder) Model() string { return l.inner.Model() }

func (l *LimitedEmbedder) Dim() int { return l.inner.Dim() }

func (l *LimitedEmbedder) embedOnce(ctx context.Context, texts []string) ([][]float32, error) {
	select {
	case l.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-l.slots }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.inner.Embed(ctx, texts)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
