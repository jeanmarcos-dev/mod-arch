package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jeanmarcos/modos-cpu/internal/programs"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/sim"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

func startUsageTestServer(t *testing.T) (*Server, *store.Store, *httptest.Server, string) {
	t.Helper()
	demos := make([]sim.Demo, 0)
	for _, d := range programs.Demos() {
		procs := make([]sim.Proc, 0, len(d.Procs))
		for _, p := range d.Procs {
			procs = append(procs, sim.Proc{Name: p.Name, Bytes: p.Bytes})
		}
		demos = append(demos, sim.Demo{Name: d.Name, Description: d.Description, Procs: procs})
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mgr := sessions.NewManager(sessions.Config{Demos: demos, Store: st})
	s := New(Config{Manager: mgr, Logger: discardLogger()})
	t.Cleanup(s.Close)
	mux := http.NewServeMux()
	s.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return s, st, ts, "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
}

func usageCount(t *testing.T, s *Server, st *store.Store, where string, args ...any) int {
	t.Helper()
	s.usage.Flush()
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM usage_events WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatalf("count usage: %v", err)
	}
	return n
}

func waitUsage(t *testing.T, s *Server, st *store.Store, want int, where string, args ...any) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	got := 0
	for time.Now().Before(deadline) {
		got = usageCount(t, s, st, where, args...)
		if got >= want {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	return got
}

func i64(v int64) *int64 { return &v }
func i(v int) *int       { return &v }

func TestCommandValidate_TrackAndFeedback(t *testing.T) {
	tests := []struct {
		name    string
		cmd     Command
		wantErr bool
	}{
		{name: "valid page view", cmd: Command{Cmd: "track", Event: "page_view", Name: "/procesos", DurationMS: i64(40213)}},
		{name: "zero duration", cmd: Command{Cmd: "track", Event: "page_view", Name: "/", DurationMS: i64(0)}},
		{name: "max duration", cmd: Command{Cmd: "track", Event: "page_view", Name: "/x", DurationMS: i64(21_600_000)}, wantErr: false},
		{name: "unknown event", cmd: Command{Cmd: "track", Event: "click", Name: "/x", DurationMS: i64(1)}, wantErr: true},
		{name: "name without slash", cmd: Command{Cmd: "track", Event: "page_view", Name: "procesos", DurationMS: i64(1)}, wantErr: true},
		{name: "empty name", cmd: Command{Cmd: "track", Event: "page_view", DurationMS: i64(1)}, wantErr: true},
		{name: "name too long", cmd: Command{Cmd: "track", Event: "page_view", Name: "/" + strings.Repeat("a", 100), DurationMS: i64(1)}, wantErr: true},
		{name: "name at limit", cmd: Command{Cmd: "track", Event: "page_view", Name: "/" + strings.Repeat("a", 99), DurationMS: i64(1)}},
		{name: "negative duration", cmd: Command{Cmd: "track", Event: "page_view", Name: "/x", DurationMS: i64(-1)}, wantErr: true},
		{name: "duration above six hours", cmd: Command{Cmd: "track", Event: "page_view", Name: "/x", DurationMS: i64(21_600_001)}, wantErr: true},
		{name: "missing duration", cmd: Command{Cmd: "track", Event: "page_view", Name: "/x"}, wantErr: true},
		{name: "feedback up", cmd: Command{Cmd: "tutor_feedback", RequestID: "r-1", Rating: i(1)}},
		{name: "feedback down", cmd: Command{Cmd: "tutor_feedback", RequestID: "r-1", Rating: i(-1)}},
		{name: "feedback clear", cmd: Command{Cmd: "tutor_feedback", RequestID: "r-1", Rating: i(0)}},
		{name: "feedback out of range", cmd: Command{Cmd: "tutor_feedback", RequestID: "r-1", Rating: i(2)}, wantErr: true},
		{name: "feedback missing rating", cmd: Command{Cmd: "tutor_feedback", RequestID: "r-1"}, wantErr: true},
		{name: "feedback missing request id", cmd: Command{Cmd: "tutor_feedback", Rating: i(1)}, wantErr: true},
		{name: "feedback request id too long", cmd: Command{Cmd: "tutor_feedback", RequestID: strings.Repeat("r", 101), Rating: i(1)}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestViewerSafeCommandsIncludeTelemetry(t *testing.T) {
	for _, cmd := range []string{"track", "tutor_feedback"} {
		if !viewerSafeCommands[cmd] {
			t.Errorf("%s must be allowed for viewers", cmd)
		}
		if ownerOnlyCommands[cmd] {
			t.Errorf("%s must not be owner-only", cmd)
		}
	}
}

func TestTrackLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newTrackLimiter(60, time.Minute)
	l.now = func() time.Time { return now }

	allowed := 0
	for range 500 {
		if l.allow() {
			allowed++
		}
	}
	if allowed != 60 {
		t.Fatalf("allowed in first window = %d, want 60", allowed)
	}
	now = now.Add(59 * time.Second)
	if l.allow() {
		t.Fatal("still inside the window, must be rejected")
	}
	now = now.Add(2 * time.Second)
	if !l.allow() {
		t.Fatal("new window must allow again")
	}
}

func TestWS_DemoLifecycleEvents(t *testing.T) {
	s, st, ts, wsURL := startUsageTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, conn, initialMessages)

	send(t, ctx, conn, Command{Cmd: "load", Program: "counter"})
	send(t, ctx, conn, Command{Cmd: "run", Speed: 10})
	send(t, ctx, conn, Command{Cmd: "pause"})
	send(t, ctx, conn, Command{Cmd: "reset"})

	for _, kind := range []string{store.UsageKindDemoLoad, store.UsageKindDemoRun, store.UsageKindDemoPause, store.UsageKindDemoReset} {
		if got := waitUsage(t, s, st, 1, `kind = ? AND name = ? AND role = 'owner'`, kind, "counter"); got != 1 {
			t.Errorf("%s events named counter = %d, want 1", kind, got)
		}
	}
}

func TestWS_FailedLoadIsNotRecorded(t *testing.T) {
	s, st, ts, wsURL := startUsageTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, conn, initialMessages)

	send(t, ctx, conn, Command{Cmd: "load", Program: "no_such_demo"})
	send(t, ctx, conn, Command{Cmd: "snapshot"})
	drainN(t, ctx, conn, 2)

	if got := usageCount(t, s, st, `kind = ?`, store.UsageKindDemoLoad); got != 0 {
		t.Fatalf("demo_load events = %d, want 0", got)
	}
}

func TestWS_TrackPageViewsAreStoredAndThrottled(t *testing.T) {
	s, st, ts, wsURL := startUsageTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, conn, initialMessages)

	send(t, ctx, conn, Command{Cmd: "track", Event: "page_view", Name: "/procesos", DurationMS: i64(40213)})
	if got := waitUsage(t, s, st, 1, `kind = 'page_view' AND name = '/procesos' AND duration_ms = 40213`); got != 1 {
		t.Fatalf("stored page_view = %d, want 1", got)
	}

	s.manager.All()[0].SetUserID("u-1")
	send(t, ctx, conn, Command{Cmd: "track", Event: "page_view", Name: "/with-user", DurationMS: i64(7)})
	if got := waitUsage(t, s, st, 1, `name = '/with-user' AND user_id = 'u-1'`); got != 1 {
		t.Fatalf("page view with user = %d, want 1", got)
	}

	send(t, ctx, conn, Command{Cmd: "track", Event: "bogus", Name: "/x", DurationMS: i64(1)})
	for range 500 {
		send(t, ctx, conn, Command{Cmd: "track", Event: "page_view", Name: "/flood", DurationMS: i64(5)})
	}
	send(t, ctx, conn, Command{Cmd: "snapshot"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(string(data), `"type":"snapshot"`) {
			break
		}
	}
	flood := usageCount(t, s, st, `name = '/flood'`)
	if flood != trackLimitPerMinute-2 {
		t.Fatalf("flood events stored = %d, want %d", flood, trackLimitPerMinute-2)
	}
	if got := usageCount(t, s, st, `name = '/x'`); got != 0 {
		t.Fatalf("invalid event stored: %d", got)
	}
}

func TestWS_TutorFeedbackIsScopedToSession(t *testing.T) {
	s, st, ts, wsURL := startUsageTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, conn, initialMessages)

	all := s.manager.All()
	if len(all) != 1 {
		t.Fatalf("sessions = %d, want 1", len(all))
	}
	own := all[0].ID
	for _, row := range []store.TutorQA{
		{TS: 1, SessionID: own, RequestID: "r-own", Question: "q"},
		{TS: 1, SessionID: "someone-else", RequestID: "r-foreign", Question: "q"},
	} {
		if err := st.InsertTutorQA(ctx, row); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	send(t, ctx, conn, Command{Cmd: "tutor_feedback", RequestID: "r-foreign", Rating: i(1)})
	send(t, ctx, conn, Command{Cmd: "tutor_feedback", RequestID: "r-own", Rating: i(1)})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := st.GetTutorQA(ctx, own, "r-own")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Rating != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	own1, _ := st.GetTutorQA(ctx, own, "r-own")
	if own1.Rating == nil || *own1.Rating != 1 {
		t.Fatalf("own rating = %v, want 1", own1.Rating)
	}
	foreign, _ := st.GetTutorQA(ctx, "someone-else", "r-foreign")
	if foreign.Rating != nil {
		t.Fatalf("foreign rating = %d, want NULL", *foreign.Rating)
	}

	send(t, ctx, conn, Command{Cmd: "tutor_feedback", RequestID: "r-own", Rating: i(0)})
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := st.GetTutorQA(ctx, own, "r-own")
		if got.Rating == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("rating 0 did not clear the rating")
}

func TestWS_ViewerPageViewsAreRecordedInObservedSession(t *testing.T) {
	s, st, ts, wsURL := startUsageTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	owner, _ := dialWithSession(t, ctx, ts, wsURL)
	defer owner.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, owner, initialMessages)
	ownerSess := s.manager.All()[0]

	viewer := dialAsViewer(t, ctx, ts, wsURL, ownerSess, s.manager)
	defer viewer.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, viewer, initialMessages)

	send(t, ctx, viewer, Command{Cmd: "track", Event: "page_view", Name: "/concurrencia", DurationMS: i64(20000)})
	got := waitUsage(t, s, st, 1, `kind = 'page_view' AND name = '/concurrencia' AND role = 'viewer' AND session_id = ?`, ownerSess.ID)
	if got != 1 {
		t.Fatalf("viewer page views = %d, want 1", got)
	}
}

func TestWS_AskWithoutTutorReturnsStableCode(t *testing.T) {
	_, ts, wsURL := startTestServer(t)
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, conn, initialMessages)

	send(t, ctx, conn, Command{Cmd: "ask", Question: "¿qué es PL3?", RequestID: "r-1"})
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("json: %v", err)
		}
		if msg["type"] != "tutor_answer" {
			continue
		}
		if msg["code"] != "tutor_unavailable" {
			t.Errorf("code = %v, want tutor_unavailable", msg["code"])
		}
		if msg["error"] != "ARCHIUS no está disponible en este momento." {
			t.Errorf("error = %v", msg["error"])
		}
		if msg["request_id"] != "r-1" {
			t.Errorf("request_id = %v", msg["request_id"])
		}
		return
	}
}
