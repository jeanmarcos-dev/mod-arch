package store

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/telemetry"
)

const (
	UsageKindPageView  = "page_view"
	UsageKindDemoLoad  = "demo_load"
	UsageKindDemoRun   = "demo_run"
	UsageKindDemoPause = "demo_pause"
	UsageKindDemoReset = "demo_reset"

	UsageRoleOwner  = "owner"
	UsageRoleViewer = "viewer"

	usageBufferSize    = 1024
	usageBatchSize     = 100
	usageFlushInterval = time.Second
	usageFlushTimeout  = 5 * time.Second
)

type UsageEvent struct {
	TS         int64
	SessionID  string
	UserID     string
	Kind       string
	Name       string
	DurationMS *int64
	Role       string
}

func (s *Store) InsertUsageEvents(ctx context.Context, events []UsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin usage batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO usage_events(ts, session_id, user_id, kind, name, duration_ms, role)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare usage insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, e := range events {
		role := e.Role
		if role == "" {
			role = UsageRoleOwner
		}
		if _, err := stmt.ExecContext(ctx, e.TS, e.SessionID, nullableString(e.UserID), e.Kind, e.Name, e.DurationMS, role); err != nil {
			return fmt.Errorf("store: insert usage event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit usage batch: %w", err)
	}
	return nil
}

type UsageRecorder struct {
	store         *Store
	logger        *slog.Logger
	events        chan UsageEvent
	batchSize     int
	flushInterval time.Duration
	dropped       atomic.Int64
	done          chan struct{}
	closeOnce     sync.Once
	stop          chan struct{}
	flushReq      chan chan struct{}
}

func NewUsageRecorder(s *Store, logger *slog.Logger) *UsageRecorder {
	return newUsageRecorder(s, logger, usageBufferSize, usageBatchSize, usageFlushInterval)
}

func newUsageRecorder(s *Store, logger *slog.Logger, buffer, batchSize int, flushInterval time.Duration) *UsageRecorder {
	if logger == nil {
		logger = slog.Default()
	}
	r := &UsageRecorder{
		store:         s,
		logger:        logger,
		events:        make(chan UsageEvent, buffer),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		done:          make(chan struct{}),
		stop:          make(chan struct{}),
		flushReq:      make(chan chan struct{}),
	}
	go r.run()
	return r
}

func (r *UsageRecorder) Record(e UsageEvent) {
	if e.TS == 0 {
		e.TS = time.Now().UnixMilli()
	}
	select {
	case r.events <- e:
	default:
		r.dropped.Add(1)
		telemetry.TrackUsageDropped()
	}
}

func (r *UsageRecorder) Dropped() int64 { return r.dropped.Load() }

func (r *UsageRecorder) Flush() {
	ack := make(chan struct{})
	select {
	case r.flushReq <- ack:
		<-ack
	case <-r.done:
	}
}

func (r *UsageRecorder) Close() {
	r.closeOnce.Do(func() { close(r.stop) })
	<-r.done
}

func (r *UsageRecorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()
	batch := make([]UsageEvent, 0, r.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), usageFlushTimeout)
		defer cancel()
		if err := r.store.InsertUsageEvents(ctx, batch); err != nil {
			r.logger.Warn("usage events flush failed", "events", len(batch), "err", err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case e := <-r.events:
			batch = append(batch, e)
			if len(batch) >= r.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case ack := <-r.flushReq:
			for drained := false; !drained; {
				select {
				case e := <-r.events:
					batch = append(batch, e)
				default:
					drained = true
				}
			}
			flush()
			close(ack)
		case <-r.stop:
			for {
				select {
				case e := <-r.events:
					batch = append(batch, e)
				default:
					flush()
					return
				}
			}
		}
	}
}
