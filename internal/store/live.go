package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const (
	liveActivityMaxMinutes = 60
	liveRecentQuestions    = 20
	liveQuestionMaxRunes   = 140
	liveTopTopics          = 8
)

type LivePageView struct {
	Name  string `json:"name"`
	Views int    `json:"views"`
	AvgMS int64  `json:"avg_ms"`
}

type LiveDemo struct {
	Name  string `json:"name"`
	Loads int    `json:"loads"`
	Runs  int    `json:"runs"`
}

type LiveMinute struct {
	Minute    int64 `json:"minute"`
	Events    int   `json:"events"`
	Questions int   `json:"questions"`
}

type LiveTopic struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type LiveQuestion struct {
	TS       int64  `json:"ts"`
	Question string `json:"question"`
	Rating   *int   `json:"rating"`
	Degraded bool   `json:"degraded"`
}

type LiveWindow struct {
	UniqueUsers       int            `json:"unique_users"`
	UniqueSessions    int            `json:"unique_sessions"`
	PageViews         []LivePageView `json:"page_views"`
	Demos             []LiveDemo     `json:"demos"`
	ActivityPerMinute []LiveMinute   `json:"activity_per_minute"`
}

type LiveTutor struct {
	Questions    int            `json:"questions"`
	Askers       int            `json:"askers"`
	LatencyP50MS int64          `json:"latency_p50_ms"`
	LatencyP95MS int64          `json:"latency_p95_ms"`
	Degraded     int            `json:"degraded"`
	ThumbsUp     int            `json:"thumbs_up"`
	ThumbsDown   int            `json:"thumbs_down"`
	TopTopics    []LiveTopic    `json:"top_topics"`
	Recent       []LiveQuestion `json:"recent"`
}

func (s *Store) LiveWindowStats(ctx context.Context, now time.Time, windowMinutes int) (LiveWindow, error) {
	sinceMS := now.Add(-time.Duration(windowMinutes) * time.Minute).UnixMilli()
	out := LiveWindow{
		PageViews:         []LivePageView{},
		Demos:             []LiveDemo{},
		ActivityPerMinute: []LiveMinute{},
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT user_id), COUNT(DISTINCT session_id)
		FROM usage_events WHERE ts >= ?`, sinceMS).Scan(&out.UniqueUsers, &out.UniqueSessions); err != nil {
		return out, fmt.Errorf("store: live uniques: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT name, COUNT(*), COALESCE(CAST(AVG(duration_ms) AS INTEGER), 0)
		FROM usage_events WHERE kind = ? AND ts >= ?
		GROUP BY name ORDER BY COUNT(*) DESC, name ASC`, UsageKindPageView, sinceMS)
	if err != nil {
		return out, fmt.Errorf("store: live page views: %w", err)
	}
	for rows.Next() {
		var pv LivePageView
		if err := rows.Scan(&pv.Name, &pv.Views, &pv.AvgMS); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("store: scan live page view: %w", err)
		}
		out.PageViews = append(out.PageViews, pv)
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT name,
		       SUM(CASE WHEN kind = ? THEN 1 ELSE 0 END),
		       SUM(CASE WHEN kind = ? THEN 1 ELSE 0 END)
		FROM usage_events WHERE kind IN (?, ?) AND ts >= ? AND name <> ''
		GROUP BY name ORDER BY SUM(CASE WHEN kind = ? THEN 1 ELSE 0 END) DESC, name ASC`,
		UsageKindDemoLoad, UsageKindDemoRun, UsageKindDemoLoad, UsageKindDemoRun, sinceMS, UsageKindDemoLoad)
	if err != nil {
		return out, fmt.Errorf("store: live demos: %w", err)
	}
	for rows.Next() {
		var d LiveDemo
		if err := rows.Scan(&d.Name, &d.Loads, &d.Runs); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("store: scan live demo: %w", err)
		}
		out.Demos = append(out.Demos, d)
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}

	activity, err := s.liveActivity(ctx, now, windowMinutes)
	if err != nil {
		return out, err
	}
	out.ActivityPerMinute = activity
	return out, nil
}

func (s *Store) liveActivity(ctx context.Context, now time.Time, windowMinutes int) ([]LiveMinute, error) {
	n := min(windowMinutes, liveActivityMaxMinutes)
	if n < 1 {
		n = 1
	}
	endMinute := now.UnixMilli() / 60000 * 60000
	startMinute := endMinute - int64(n-1)*60000
	buckets := make([]LiveMinute, n)
	for i := range buckets {
		buckets[i].Minute = startMinute + int64(i)*60000
	}

	fill := func(query string, set func(b *LiveMinute, c int)) error {
		rows, err := s.db.QueryContext(ctx, query, startMinute)
		if err != nil {
			return fmt.Errorf("store: live activity: %w", err)
		}
		for rows.Next() {
			var minute int64
			var count int
			if err := rows.Scan(&minute, &count); err != nil {
				_ = rows.Close()
				return fmt.Errorf("store: scan live activity: %w", err)
			}
			if idx := (minute - startMinute) / 60000; idx >= 0 && idx < int64(n) {
				set(&buckets[idx], count)
			}
		}
		return closeRows(rows)
	}
	if err := fill(`SELECT ts / 60000 * 60000, COUNT(*) FROM usage_events WHERE ts >= ? GROUP BY 1`,
		func(b *LiveMinute, c int) { b.Events = c }); err != nil {
		return nil, err
	}
	if err := fill(`SELECT ts / 60000 * 60000, COUNT(*) FROM tutor_qa WHERE ts >= ? GROUP BY 1`,
		func(b *LiveMinute, c int) { b.Questions = c }); err != nil {
		return nil, err
	}
	return buckets, nil
}

func (s *Store) LiveTutorStats(ctx context.Context, now time.Time, windowMinutes int) (LiveTutor, error) {
	sinceMS := now.Add(-time.Duration(windowMinutes) * time.Minute).UnixMilli()
	out := LiveTutor{TopTopics: []LiveTopic{}, Recent: []LiveQuestion{}}

	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT COALESCE(user_id, session_id)),
		       COALESCE(SUM(degraded), 0),
		       COALESCE(SUM(CASE WHEN rating = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN rating = -1 THEN 1 ELSE 0 END), 0)
		FROM tutor_qa WHERE ts >= ?`, sinceMS).Scan(&out.Questions, &out.Askers, &out.Degraded, &out.ThumbsUp, &out.ThumbsDown); err != nil {
		return out, fmt.Errorf("store: live tutor totals: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT latency_ms FROM tutor_qa WHERE ts >= ? AND error = '' ORDER BY latency_ms ASC`, sinceMS)
	if err != nil {
		return out, fmt.Errorf("store: live tutor latency: %w", err)
	}
	var latencies []int64
	for rows.Next() {
		var l int64
		if err := rows.Scan(&l); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("store: scan live latency: %w", err)
		}
		latencies = append(latencies, l)
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}
	out.LatencyP50MS = percentile(latencies, 50)
	out.LatencyP95MS = percentile(latencies, 95)

	rows, err = s.db.QueryContext(ctx, `SELECT citations_json FROM tutor_qa WHERE ts >= ? AND citations_json <> '[]'`, sinceMS)
	if err != nil {
		return out, fmt.Errorf("store: live tutor topics: %w", err)
	}
	counts := map[string]int{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("store: scan live topics: %w", err)
		}
		var labels []string
		if json.Unmarshal([]byte(raw), &labels) != nil {
			continue
		}
		seen := map[string]bool{}
		for _, l := range labels {
			if l != "" && !seen[l] {
				seen[l] = true
				counts[l]++
			}
		}
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}
	for label, c := range counts {
		out.TopTopics = append(out.TopTopics, LiveTopic{Label: label, Count: c})
	}
	sort.Slice(out.TopTopics, func(i, j int) bool {
		if out.TopTopics[i].Count != out.TopTopics[j].Count {
			return out.TopTopics[i].Count > out.TopTopics[j].Count
		}
		return out.TopTopics[i].Label < out.TopTopics[j].Label
	})
	if len(out.TopTopics) > liveTopTopics {
		out.TopTopics = out.TopTopics[:liveTopTopics]
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT ts, question, rating, degraded FROM tutor_qa
		WHERE ts >= ? ORDER BY ts DESC, id DESC LIMIT ?`, sinceMS, liveRecentQuestions)
	if err != nil {
		return out, fmt.Errorf("store: live tutor recent: %w", err)
	}
	for rows.Next() {
		var q LiveQuestion
		var degraded int
		if err := rows.Scan(&q.TS, &q.Question, &q.Rating, &degraded); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("store: scan live recent: %w", err)
		}
		q.Question = truncateRunes(q.Question, liveQuestionMaxRunes)
		q.Degraded = degraded != 0
		out.Recent = append(out.Recent, q)
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}
	return out, nil
}

type rowsCloser interface {
	Err() error
	Close() error
}

func closeRows(rows rowsCloser) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("store: iterate rows: %w", err)
	}
	return rows.Close()
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted)*p + 99) / 100
	if idx < 1 {
		idx = 1
	}
	return sorted[idx-1]
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
