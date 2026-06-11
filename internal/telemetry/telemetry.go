package telemetry

import (
	"sync/atomic"
	"time"
)

type Metrics struct {
	ActiveWSConnections int64   `json:"active_ws_connections"`
	TotalWSConnections  int64   `json:"total_ws_connections"`
	LLMQueries          int64   `json:"llm_queries"`
	LLMErrors           int64   `json:"llm_errors"`
	AvgLLMLatencyMs     float64 `json:"avg_llm_latency_ms"`
	TotalLLMLatencyMs   int64   `json:"-"`

	SnapshotCount    int64 `json:"snapshot_count"`
	SnapshotUnder1ms int64 `json:"snapshot_under_1ms"`
	Snapshot1to5ms   int64 `json:"snapshot_1_to_5ms"`
	Snapshot5to20ms  int64 `json:"snapshot_5_to_20ms"`
	SnapshotOver20ms int64 `json:"snapshot_over_20ms"`
}

var globalMetrics Metrics

// TrackWSConnection increments the active and total WS connection counters.
// Returns a func to defer on disconnect.
func TrackWSConnection() func() {
	atomic.AddInt64(&globalMetrics.ActiveWSConnections, 1)
	atomic.AddInt64(&globalMetrics.TotalWSConnections, 1)
	return func() {
		atomic.AddInt64(&globalMetrics.ActiveWSConnections, -1)
	}
}

// TrackLLMQuery records a single LLM query execution time and status.
func TrackLLMQuery(start time.Time, err error) {
	durationMs := time.Since(start).Milliseconds()
	atomic.AddInt64(&globalMetrics.LLMQueries, 1)
	atomic.AddInt64(&globalMetrics.TotalLLMLatencyMs, durationMs)
	if err != nil {
		atomic.AddInt64(&globalMetrics.LLMErrors, 1)
	}
}

// TrackSnapshotLatency records the execution time of the simulator's Snapshot function.
func TrackSnapshotLatency(start time.Time) {
	durationMs := time.Since(start).Milliseconds()
	atomic.AddInt64(&globalMetrics.SnapshotCount, 1)

	if durationMs < 1 {
		atomic.AddInt64(&globalMetrics.SnapshotUnder1ms, 1)
	} else if durationMs <= 5 {
		atomic.AddInt64(&globalMetrics.Snapshot1to5ms, 1)
	} else if durationMs <= 20 {
		atomic.AddInt64(&globalMetrics.Snapshot5to20ms, 1)
	} else {
		atomic.AddInt64(&globalMetrics.SnapshotOver20ms, 1)
	}
}

// Snapshot returns a copy of the current metrics, calculating averages safely.
func Snapshot() Metrics {
	m := Metrics{
		ActiveWSConnections: atomic.LoadInt64(&globalMetrics.ActiveWSConnections),
		TotalWSConnections:  atomic.LoadInt64(&globalMetrics.TotalWSConnections),
		LLMQueries:          atomic.LoadInt64(&globalMetrics.LLMQueries),
		LLMErrors:           atomic.LoadInt64(&globalMetrics.LLMErrors),
		SnapshotCount:       atomic.LoadInt64(&globalMetrics.SnapshotCount),
		SnapshotUnder1ms:    atomic.LoadInt64(&globalMetrics.SnapshotUnder1ms),
		Snapshot1to5ms:      atomic.LoadInt64(&globalMetrics.Snapshot1to5ms),
		Snapshot5to20ms:     atomic.LoadInt64(&globalMetrics.Snapshot5to20ms),
		SnapshotOver20ms:    atomic.LoadInt64(&globalMetrics.SnapshotOver20ms),
	}

	totalLat := atomic.LoadInt64(&globalMetrics.TotalLLMLatencyMs)
	if m.LLMQueries > 0 {
		m.AvgLLMLatencyMs = float64(totalLat) / float64(m.LLMQueries)
	}
	return m
}
