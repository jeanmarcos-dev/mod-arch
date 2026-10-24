package tutor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

const qaLogTimeout = 3 * time.Second

type QALog interface {
	InsertTutorQA(ctx context.Context, qa store.TutorQA) error
}

func (s *Service) SetQALog(l QALog) { s.qaLog = l }

func (s *Service) recordQA(ctx context.Context, sess *sessions.Session, question string, opts AskOptions, ans *Answer, callErr error) {
	if s.qaLog == nil || sess == nil {
		return
	}
	now := time.Now()
	requestID := opts.RequestID
	if requestID == "" {
		requestID = fmt.Sprintf("srv-%d", now.UnixNano())
	}
	row := store.TutorQA{
		TS:            now.UnixMilli(),
		SessionID:     sess.ID,
		UserID:        sess.UserID(),
		RequestID:     requestID,
		Question:      question,
		CitationsJSON: "[]",
	}
	if ans != nil {
		row.Answer = ans.Text
		row.CitationsJSON = citationLabelsJSON(ans.Citations)
		row.Degraded = ans.Degraded
		row.CacheHit = ans.CacheHit
		row.Retrieved = ans.RetrievedCount
		row.LatencyMS = ans.LatencyMS
		row.Provider = ans.Provider
		row.Model = ans.Model
		row.Error = ans.LastError
	}
	if callErr != nil {
		row.Error = callErr.Error()
	}
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), qaLogTimeout)
	defer cancel()
	if err := s.qaLog.InsertTutorQA(logCtx, row); err != nil {
		s.logger.Warn("tutor: qa log failed", "err", err)
	}
}

func citationLabelsJSON(citations []Citation) string {
	labels := make([]string, 0, len(citations))
	for _, c := range citations {
		labels = append(labels, c.Label)
	}
	data, err := json.Marshal(labels)
	if err != nil {
		return "[]"
	}
	return string(data)
}
