package tutor

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MetricRow is one persisted Ask() observation. The dashboard aggregates
// across rows to expose query volume, latency percentiles and tool usage.
type MetricRow struct {
	ID             int64
	Timestamp      time.Time
	SessionID      string
	UserID         string
	QuestionDigest string
	QuestionChars  int
	LatencyMS      int64
	CacheHit       bool
	Degraded       bool
	ToolsUsed      []string
	TokensTotal    int
	CitationsCount int
	Error          string
}

// MetricsStore writes/reads tutor_metrics. Wraps a *sql.DB so it can share
// the same connection pool as the rest of the app.
type MetricsStore struct {
	db *sql.DB
}

// NewMetricsStore wires a MetricsStore around an already-migrated DB.
func NewMetricsStore(db *sql.DB) *MetricsStore { return &MetricsStore{db: db} }

// Record inserts one row. Failure is logged by the caller; metrics never
// block answer delivery.
func (m *MetricsStore) Record(ctx context.Context, row MetricRow) error {
	if m == nil || m.db == nil {
		return nil
	}
	toolsJSON, _ := json.Marshal(row.ToolsUsed)
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO tutor_metrics(
			ts, session_id, user_id, question_digest, question_chars,
			latency_ms, cache_hit, degraded, tools_used, tokens_total,
			citations_count, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		row.Timestamp.Unix(), row.SessionID, row.UserID, row.QuestionDigest, row.QuestionChars,
		row.LatencyMS, boolToInt(row.CacheHit), boolToInt(row.Degraded), string(toolsJSON), row.TokensTotal,
		row.CitationsCount, row.Error,
	)
	if err != nil {
		return fmt.Errorf("tutor/metrics: insert: %w", err)
	}
	return nil
}

// Summary captures the aggregate numbers the admin dashboard renders.
type Summary struct {
	Window        string         `json:"window"`
	TotalQueries  int            `json:"total_queries"`
	UniqueAskers  int            `json:"unique_askers"`
	CacheHits     int            `json:"cache_hits"`
	CacheHitRate  float64        `json:"cache_hit_rate"`
	DegradedCount int            `json:"degraded_count"`
	LatencyP50    int64          `json:"latency_p50_ms"`
	LatencyP95    int64          `json:"latency_p95_ms"`
	LatencyP99    int64          `json:"latency_p99_ms"`
	AvgTokens     float64        `json:"avg_tokens"`
	ToolUsage     map[string]int `json:"tool_usage"`
	ByHour        []HourBucket   `json:"by_hour"`
	RecentErrors  []ErrorEntry   `json:"recent_errors"`
}

// HourBucket counts queries in a one-hour slice. Used for the bar chart.
type HourBucket struct {
	HourStart time.Time `json:"hour_start"`
	Count     int       `json:"count"`
}

// ErrorEntry is the most-recent N errors, for the troubleshooting card.
type ErrorEntry struct {
	Timestamp time.Time `json:"ts"`
	SessionID string    `json:"session_id"`
	Error     string    `json:"error"`
}

// SummaryWindow controls the time range Summarize covers.
type SummaryWindow struct {
	Since time.Time
	Until time.Time
}

// Summarize computes the aggregates over [Since, Until]. Built on top of
// LoadAll so percentile math runs in Go — at the volumes the tutor sees
// (hundreds per day at most), this is faster than juggling SQL window
// functions and keeps the code straightforward.
func (m *MetricsStore) Summarize(ctx context.Context, w SummaryWindow) (*Summary, error) {
	if m == nil || m.db == nil {
		return &Summary{Window: "n/a"}, nil
	}
	if w.Until.IsZero() {
		w.Until = time.Now()
	}
	if w.Since.IsZero() {
		w.Since = w.Until.Add(-24 * time.Hour)
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT ts, session_id, latency_ms, cache_hit, degraded, tools_used,
		       tokens_total, citations_count, error
		FROM tutor_metrics
		WHERE ts BETWEEN ? AND ?
		ORDER BY ts DESC
	`, w.Since.Unix(), w.Until.Unix())
	if err != nil {
		return nil, fmt.Errorf("tutor/metrics: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := Summary{
		Window:       formatWindow(w),
		ToolUsage:    map[string]int{},
		ByHour:       []HourBucket{},
		RecentErrors: []ErrorEntry{},
	}
	latencies := make([]int64, 0, 256)
	totalTokens := 0
	hourly := map[int64]int{}
	askers := map[string]struct{}{}

	for rows.Next() {
		var (
			ts       int64
			sessID   sql.NullString
			latency  int64
			cacheHit int
			degraded int
			tools    string
			tokens   int
			cites    int
			errMsg   sql.NullString
		)
		if err := rows.Scan(&ts, &sessID, &latency, &cacheHit, &degraded, &tools, &tokens, &cites, &errMsg); err != nil {
			return nil, fmt.Errorf("tutor/metrics: scan: %w", err)
		}
		out.TotalQueries++
		if cacheHit == 1 {
			out.CacheHits++
		}
		if degraded == 1 {
			out.DegradedCount++
		}
		latencies = append(latencies, latency)
		totalTokens += tokens
		hourStart := time.Unix(ts, 0).Truncate(time.Hour).Unix()
		hourly[hourStart]++
		if sessID.Valid {
			askers[sessID.String] = struct{}{}
		}
		var parsed []string
		_ = json.Unmarshal([]byte(tools), &parsed)
		for _, t := range parsed {
			out.ToolUsage[t]++
		}
		if errMsg.Valid && errMsg.String != "" && len(out.RecentErrors) < 10 {
			out.RecentErrors = append(out.RecentErrors, ErrorEntry{
				Timestamp: time.Unix(ts, 0),
				SessionID: sessID.String,
				Error:     errMsg.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out.UniqueAskers = len(askers)
	if out.TotalQueries > 0 {
		out.CacheHitRate = float64(out.CacheHits) / float64(out.TotalQueries)
		out.AvgTokens = float64(totalTokens) / float64(out.TotalQueries)
	}
	out.LatencyP50, out.LatencyP95, out.LatencyP99 = percentilesMS(latencies)
	out.ByHour = sortedHourBuckets(hourly)
	return &out, nil
}

func formatWindow(w SummaryWindow) string {
	d := w.Until.Sub(w.Since).Round(time.Hour)
	if d <= 0 {
		return "now"
	}
	return fmt.Sprintf("últimas %s", d)
}

func percentilesMS(values []int64) (p50, p95, p99 int64) {
	n := len(values)
	if n == 0 {
		return 0, 0, 0
	}
	// Sort in place (caller owns the slice and we are done with it).
	// Insertion sort is fine for n < a few thousand.
	for i := 1; i < n; i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
	pick := func(p float64) int64 {
		idx := int(float64(n-1) * p)
		return values[idx]
	}
	return pick(0.50), pick(0.95), pick(0.99)
}

func sortedHourBuckets(by map[int64]int) []HourBucket {
	out := make([]HourBucket, 0, len(by))
	for ts, n := range by {
		out = append(out, HourBucket{HourStart: time.Unix(ts, 0), Count: n})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].HourStart.Before(out[j-1].HourStart); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// DigestQuestion is the SHA-256 hex of a normalized question. Stored to let
// the dashboard count repeated questions without persisting their content.
func DigestQuestion(q string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(q))))
	return hex.EncodeToString(h[:])
}
