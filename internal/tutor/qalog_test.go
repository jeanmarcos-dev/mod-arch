package tutor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	storepkg "github.com/jeanmarcos-dev/mod-arch/internal/store"
)

func TestAskRecordsQuestionAndAnswer(t *testing.T) {
	tests := []struct {
		name          string
		llm           *stubLLM
		rateLimit     bool
		wantErr       bool
		check         func(t *testing.T, row storepkg.TutorQA)
		secondRequest bool
	}{
		{
			name: "answered with citations",
			llm:  &stubLLM{resp: "PL3 es el modo usuario. [Glosario: PL3]"},
			check: func(t *testing.T, row storepkg.TutorQA) {
				if row.Question != "¿qué es PL3?" || !strings.Contains(row.Answer, "modo usuario") {
					t.Errorf("question/answer = %q / %q", row.Question, row.Answer)
				}
				if row.Provider != "stub" || row.Model != "stub-model" {
					t.Errorf("provider/model = %q / %q", row.Provider, row.Model)
				}
				if row.Degraded || row.Error != "" {
					t.Errorf("degraded = %v error = %q", row.Degraded, row.Error)
				}
				if row.Retrieved == 0 {
					t.Error("retrieved = 0, want > 0")
				}
				if !strings.Contains(row.CitationsJSON, "PL3") {
					t.Errorf("citations_json = %s", row.CitationsJSON)
				}
				if row.UserID != "u-1" {
					t.Errorf("user_id = %q, want u-1", row.UserID)
				}
			},
		},
		{
			name: "degraded when the llm fails",
			llm:  &stubLLM{err: errors.New("upstream down")},
			check: func(t *testing.T, row storepkg.TutorQA) {
				if !row.Degraded {
					t.Error("degraded = false, want true")
				}
				if !strings.Contains(row.Error, "upstream down") {
					t.Errorf("error = %q", row.Error)
				}
				if row.Retrieved == 0 || !strings.Contains(row.Answer, "modelo de lenguaje") {
					t.Errorf("retrieved = %d answer = %q", row.Retrieved, row.Answer)
				}
			},
		},
		{
			name:      "tutor rate limit is logged with empty answer",
			llm:       &stubLLM{resp: "x"},
			rateLimit: true,
			wantErr:   true,
			check: func(t *testing.T, row storepkg.TutorQA) {
				if row.Answer != "" || row.Error == "" {
					t.Errorf("answer = %q error = %q", row.Answer, row.Error)
				}
			},
		},
		{
			name:          "cache hit is flagged",
			llm:           &stubLLM{resp: "respuesta"},
			secondRequest: true,
			check: func(t *testing.T, row storepkg.TutorQA) {
				if !row.CacheHit || row.Answer != "respuesta" {
					t.Errorf("cache_hit = %v answer = %q", row.CacheHit, row.Answer)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := setupService(t, tc.llm)
			defer svc.Close()
			logStore, err := storepkg.Open(":memory:")
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer logStore.Close()
			svc.SetQALog(logStore)
			if tc.rateLimit {
				svc.limiter = NewRateLimiter(1, time.Minute)
				defer svc.limiter.Close()
			}

			mgr := sessions.NewManager(sessions.Config{})
			sess, err := mgr.Create(sessions.ModeFull)
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			sess.SetUserID("u-1")

			ctx := context.Background()
			question := "¿qué es PL3?"
			requestID := "r-1"
			if tc.rateLimit {
				if _, err := svc.AskWithOptions(ctx, sess, "primera", AskOptions{RequestID: "r-0"}, nil); err != nil {
					t.Fatalf("first ask: %v", err)
				}
			}
			if tc.secondRequest {
				if _, err := svc.AskWithOptions(ctx, sess, question, AskOptions{RequestID: "r-0"}, nil); err != nil {
					t.Fatalf("first ask: %v", err)
				}
			}
			_, err = svc.AskWithOptions(ctx, sess, question, AskOptions{RequestID: requestID}, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			row, err := logStore.GetTutorQA(ctx, sess.ID, requestID)
			if err != nil {
				t.Fatalf("GetTutorQA: %v", err)
			}
			tc.check(t, row)
		})
	}
}

func TestAskWithoutRequestIDStillRecords(t *testing.T) {
	svc := setupService(t, &stubLLM{resp: "ok"})
	defer svc.Close()
	logStore, err := storepkg.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer logStore.Close()
	svc.SetQALog(logStore)
	sess, err := sessions.NewManager(sessions.Config{}).Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Ask(context.Background(), sess, "¿qué es PL3?", nil); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	var n int
	if err := logStore.DB().QueryRow(`SELECT COUNT(*) FROM tutor_qa WHERE session_id = ?`, sess.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows = %d err = %v, want 1", n, err)
	}
}
