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

// Snapshot returns a copy of the current metrics, calculating averages safely.
func Snapshot() Metrics {
	m := Metrics{
		ActiveWSConnections: atomic.LoadInt64(&globalMetrics.ActiveWSConnections),
		TotalWSConnections:  atomic.LoadInt64(&globalMetrics.TotalWSConnections),
		LLMQueries:          atomic.LoadInt64(&globalMetrics.LLMQueries),
		LLMErrors:           atomic.LoadInt64(&globalMetrics.LLMErrors),
	}
	
	totalLat := atomic.LoadInt64(&globalMetrics.TotalLLMLatencyMs)
	if m.LLMQueries > 0 {
		m.AvgLLMLatencyMs = float64(totalLat) / float64(m.LLMQueries)
	}
	return m
}
