package server

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/sim"
	"github.com/jeanmarcos/modos-cpu/internal/tutor"
)

// handleTutorAsk runs the tutor pipeline for the student's question and
// pushes the answer (or a friendly error) back through the WebSocket.
//
// The call runs in its own goroutine so a slow LLM does not block the read
// loop; the WS Send channel handles concurrent writes safely.
func (s *Server) handleTutorAsk(cmd Command, sess *sessions.Session, c *sim.Client) {
	var svc *tutor.Service
	if s.tutorRegistry != nil {
		svc = s.tutorRegistry.Get()
	}
	if svc == nil {
		c.Send <- map[string]any{
			"type":       "tutor_answer",
			"request_id": cmd.RequestID,
			"error":      "tutor desactivado en este servidor",
		}
		return
	}
	if cmd.Question == "" {
		c.Send <- map[string]any{
			"type":       "tutor_answer",
			"request_id": cmd.RequestID,
			"error":      "la pregunta está vacía",
		}
		return
	}

	go func() {
		// Use a fresh context so the tutor call survives a viewer reload
		// mid-answer; the WS write loop drops messages whose client closed
		// already. Cap at 90s — way over the p99 of the Groq free tier.
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		// Stream "consultando: X" hints to the UI as the model calls tools.
		progress := func(stage string, detail map[string]any) {
			msg := map[string]any{
				"type":       "tutor_progress",
				"request_id": cmd.RequestID,
				"stage":      stage,
			}
			maps.Copy(msg, detail)
			c.Send <- msg
		}
		// Hard cap to defend against payloads with absurd history sizes; the
		// orchestrator also truncates to HistoryWindowTurns afterwards.
		const historyHardCap = 20
		hist := cmd.History
		if len(hist) > historyHardCap {
			hist = hist[len(hist)-historyHardCap:]
		}
		ans, err := svc.AskWithOptions(ctx, sess, cmd.Question, tutor.AskOptions{Mode: cmd.TutorMode, History: hist}, progress)
		if err != nil {
			var rl *tutor.RateLimitError
			if errors.As(err, &rl) {
				c.Send <- map[string]any{
					"type":            "tutor_answer",
					"request_id":      cmd.RequestID,
					"error":           rl.Error(),
					"retry_after_sec": int(rl.RetryAfter.Seconds()),
				}
				return
			}
			s.logger.Warn("tutor ask failed", "err", err, "session", sess.ID)
			c.Send <- map[string]any{
				"type":       "tutor_answer",
				"request_id": cmd.RequestID,
				"error":      "el tutor falló al procesar la pregunta: " + err.Error(),
			}
			return
		}

		c.Send <- map[string]any{
			"type":           "tutor_answer",
			"request_id":     cmd.RequestID,
			"text":           ans.Text,
			"citations":      ans.Citations,
			"latency_ms":     ans.LatencyMS,
			"provider":       ans.Provider,
			"model":          ans.Model,
			"cache_hit":      ans.CacheHit,
			"degraded":       ans.Degraded,
			"usage_tokens":   ans.UsageTokens,
			"corpus_version": ans.CorpusVersion,
		}
	}()
}
