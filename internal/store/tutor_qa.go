package store

import (
	"context"
	"fmt"
)

type TutorQA struct {
	ID            int64
	TS            int64
	SessionID     string
	UserID        string
	RequestID     string
	Question      string
	Answer        string
	CitationsJSON string
	Degraded      bool
	CacheHit      bool
	Retrieved     int
	LatencyMS     int64
	Provider      string
	Model         string
	Error         string
	Rating        *int
	RatedAt       *int64
}

func (s *Store) InsertTutorQA(ctx context.Context, qa TutorQA) error {
	citations := qa.CitationsJSON
	if citations == "" {
		citations = "[]"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tutor_qa(ts, session_id, user_id, request_id, question, answer,
		                     citations_json, degraded, cache_hit, retrieved, latency_ms,
		                     provider, model, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, request_id) DO UPDATE SET
			ts = excluded.ts,
			question = excluded.question,
			answer = excluded.answer,
			citations_json = excluded.citations_json,
			degraded = excluded.degraded,
			cache_hit = excluded.cache_hit,
			retrieved = excluded.retrieved,
			latency_ms = excluded.latency_ms,
			provider = excluded.provider,
			model = excluded.model,
			error = excluded.error`,
		qa.TS, qa.SessionID, nullableString(qa.UserID), qa.RequestID, qa.Question, qa.Answer,
		citations, boolToInt(qa.Degraded), boolToInt(qa.CacheHit), qa.Retrieved, qa.LatencyMS,
		qa.Provider, qa.Model, qa.Error)
	if err != nil {
		return fmt.Errorf("store: insert tutor qa: %w", err)
	}
	return nil
}

func (s *Store) RateTutorQA(ctx context.Context, sessionID, requestID string, rating int, now int64) (bool, error) {
	var res interface{ RowsAffected() (int64, error) }
	var err error
	if rating == 0 {
		res, err = s.db.ExecContext(ctx,
			`UPDATE tutor_qa SET rating = NULL, rated_at = NULL WHERE session_id = ? AND request_id = ?`,
			sessionID, requestID)
	} else {
		res, err = s.db.ExecContext(ctx,
			`UPDATE tutor_qa SET rating = ?, rated_at = ? WHERE session_id = ? AND request_id = ?`,
			rating, now, sessionID, requestID)
	}
	if err != nil {
		return false, fmt.Errorf("store: rate tutor qa: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: rate tutor qa rows: %w", err)
	}
	return n > 0, nil
}

func (s *Store) GetTutorQA(ctx context.Context, sessionID, requestID string) (TutorQA, error) {
	var qa TutorQA
	var userID *string
	var degraded, cacheHit int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, ts, session_id, user_id, request_id, question, answer, citations_json,
		       degraded, cache_hit, retrieved, latency_ms, provider, model, error, rating, rated_at
		FROM tutor_qa WHERE session_id = ? AND request_id = ?`, sessionID, requestID).Scan(
		&qa.ID, &qa.TS, &qa.SessionID, &userID, &qa.RequestID, &qa.Question, &qa.Answer, &qa.CitationsJSON,
		&degraded, &cacheHit, &qa.Retrieved, &qa.LatencyMS, &qa.Provider, &qa.Model, &qa.Error, &qa.Rating, &qa.RatedAt)
	if err != nil {
		return TutorQA{}, fmt.Errorf("store: get tutor qa: %w", err)
	}
	if userID != nil {
		qa.UserID = *userID
	}
	qa.Degraded = degraded != 0
	qa.CacheHit = cacheHit != 0
	return qa, nil
}
