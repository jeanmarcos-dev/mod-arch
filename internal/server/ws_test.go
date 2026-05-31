package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jeanmarcos/modos-cpu/internal/programs"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/sim"
)

// startTestServer brings up a Server preloaded with demo programs inside an
// httptest.Server. It also returns the ws:// URL to dial.
func startTestServer(t *testing.T) (*Server, *httptest.Server, string) {
	t.Helper()
	demos := make([]sim.Demo, 0)
	for _, d := range programs.Demos() {
		procs := make([]sim.Proc, 0, len(d.Procs))
		for _, p := range d.Procs {
			procs = append(procs, sim.Proc{Name: p.Name, Bytes: p.Bytes})
		}
		demos = append(demos, sim.Demo{Name: d.Name, Description: d.Description, Procs: procs})
	}
	mgr := sessions.NewManager(sessions.Config{Demos: demos})
	s := New(Config{Manager: mgr})
	mux := http.NewServeMux()
	s.Routes(mux)
	ts := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	return s, ts, wsURL
}

// dialWithSession issues GET / so the server installs the session cookie,
// then dials the WS using that cookie. Returns the underlying http.Client so
// callers can issue extra HTTP requests sharing the session (e.g. /api/memory).
func dialWithSession(t *testing.T, ctx context.Context, ts *httptest.Server, wsURL string) (*websocket.Conn, *http.Client) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Jar: jar}

	resp, err := httpClient.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("warmup get: %v", err)
	}
	_ = resp.Body.Close()

	conn, dialResp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	return conn, httpClient
}

// initialMessages is the number of messages the server sends to a freshly
// connected WS client before going idle: snapshot + programs + session_info.
const initialMessages = 3

func TestWS_ConnectsAndReceivesSnapshot(t *testing.T) {
	_, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")

	gotSnapshot, gotPrograms, gotSessionInfo := false, false, false
	for range initialMessages {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("json: %v", err)
		}
		switch msg["type"] {
		case "snapshot":
			gotSnapshot = true
		case "programs":
			gotPrograms = true
		case "session_info":
			gotSessionInfo = true
		}
	}
	if !gotSnapshot {
		t.Error("initial snapshot not received")
	}
	if !gotPrograms {
		t.Error("programs list not received")
	}
	if !gotSessionInfo {
		t.Error("session_info not received")
	}
}

func TestWS_LoadAndStep(t *testing.T) {
	_, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")

	drainN(t, ctx, conn, initialMessages)

	send(t, ctx, conn, Command{Cmd: "load", Program: "add"})
	// "add" runs a handful of instructions and exits. With the do_exit
	// cascade split across five phases (mark_zombie → notify_parent →
	// schedule_out → reap_cycle → halt), the program needs more clicks to
	// reach HALT than before — keep the margin generous so this test is
	// not coupled to the exact Step count of the demo.
	for range 16 {
		send(t, ctx, conn, Command{Cmd: "step"})
	}

	deadline := time.Now().Add(2 * time.Second)
	gotHalt := false
	for time.Now().Before(deadline) && !gotHalt {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var msg map[string]any
		_ = json.Unmarshal(data, &msg)
		if containsEvent(msg, "halt") {
			gotHalt = true
		}
		if msg["type"] == "state" && msg["halted"] == true {
			gotHalt = true
		}
	}
	if !gotHalt {
		t.Fatal("never received halt event")
	}
}

func TestWS_UnknownCommandReturnsError(t *testing.T) {
	_, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, conn, initialMessages)

	send(t, ctx, conn, Command{Cmd: "buenas-tardes"})

	deadline := time.Now().Add(2 * time.Second)
	gotErr := false
	for time.Now().Before(deadline) && !gotErr {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg map[string]any
		_ = json.Unmarshal(data, &msg)
		if msg["type"] == "error" {
			gotErr = true
		}
	}
	if !gotErr {
		t.Fatal("unknown command should emit an error event")
	}
}

// TestWS_TwoSessionsAreIsolated checks the key isolation property: two
// clients with different cookies own different Simulators and cannot observe
// each other.
func TestWS_TwoSessionsAreIsolated(t *testing.T) {
	_, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connA, _ := dialWithSession(t, ctx, ts, wsURL)
	defer connA.Close(websocket.StatusNormalClosure, "")
	connB, _ := dialWithSession(t, ctx, ts, wsURL)
	defer connB.Close(websocket.StatusNormalClosure, "")

	drainN(t, ctx, connA, initialMessages)
	drainN(t, ctx, connB, initialMessages)

	// A loads "add" — B must not receive any "loaded" event.
	send(t, ctx, connA, Command{Cmd: "load", Program: "add"})

	// Drain whatever arrives at A (it should see its own loaded event).
	deadlineA := time.Now().Add(500 * time.Millisecond)
	gotLoadedA := false
	for time.Now().Before(deadlineA) {
		readCtx, cancelRead := context.WithTimeout(ctx, 200*time.Millisecond)
		_, data, err := connA.Read(readCtx)
		cancelRead()
		if err != nil {
			break
		}
		var msg map[string]any
		_ = json.Unmarshal(data, &msg)
		if msg["type"] == "loaded" {
			gotLoadedA = true
		}
	}
	if !gotLoadedA {
		t.Fatal("session A did not receive its own loaded event")
	}

	// B should not see "loaded" because its simulator is independent.
	readCtx, cancelRead := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancelRead()
	for {
		_, data, err := connB.Read(readCtx)
		if err != nil {
			// Expected timeout: B never received the foreign event.
			return
		}
		var msg map[string]any
		_ = json.Unmarshal(data, &msg)
		if msg["type"] == "loaded" {
			t.Fatal("session B received a foreign loaded event — sessions are not isolated")
		}
	}
}

func TestWS_RejectsForeignOrigin(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "localhost:5173")
	_, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Jar: jar}
	resp, err := httpClient.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("warmup get: %v", err)
	}
	_ = resp.Body.Close()

	_, dialResp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient:   httpClient,
		HTTPHeader:   http.Header{"Origin": []string{"http://evil.example.com"}},
		Subprotocols: nil,
	})
	if err == nil {
		t.Fatal("expected dial to fail when Origin is foreign")
	}
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
}

func TestAllowedWSOrigins(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("ALLOWED_ORIGINS", "")
		got := allowedWSOrigins()
		if len(got) != len(defaultAllowedOrigins) {
			t.Fatalf("got %v, want defaults", got)
		}
	})
	t.Run("parse csv", func(t *testing.T) {
		t.Setenv("ALLOWED_ORIGINS", "a.example.com, b.example.com ,")
		got := allowedWSOrigins()
		want := []string{"a.example.com", "b.example.com"}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
	t.Run("all empty falls back to defaults", func(t *testing.T) {
		t.Setenv("ALLOWED_ORIGINS", " , , ")
		got := allowedWSOrigins()
		if len(got) != len(defaultAllowedOrigins) {
			t.Fatalf("got %v, want defaults", got)
		}
	})
}

func send(t *testing.T, ctx context.Context, conn *websocket.Conn, cmd Command) {
	t.Helper()
	data, _ := json.Marshal(cmd)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write %v: %v", cmd, err)
	}
}

func drainN(t *testing.T, ctx context.Context, conn *websocket.Conn, n int) {
	t.Helper()
	for range n {
		if _, _, err := conn.Read(ctx); err != nil {
			t.Fatalf("drain: %v", err)
		}
	}
}

// dialAsViewer activates share on the owner session, hits /v/<token> to set
// the viewer cookie on the returned client, then dials the WS as that viewer.
// Returns the viewer WS conn.
func dialAsViewer(t *testing.T, ctx context.Context, ts *httptest.Server, wsURL string, ownerSess *sessions.Session, mgr *sessions.Manager) *websocket.Conn {
	t.Helper()
	token := mgr.ActivateShare(ownerSess)
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Jar: jar}
	resp, err := httpClient.Get(ts.URL + "/v/" + token)
	if err != nil {
		t.Fatalf("GET /v/%s: %v", token, err)
	}
	_ = resp.Body.Close()
	conn, dialResp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("viewer dial: %v", err)
	}
	if dialResp != nil && dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	return conn
}

// containsEvent reports whether msg matches typeWanted directly or wraps an
// event of that type inside a "tick_events" batch (the simulator coalesces
// per-tick events into one frame, so individual event types no longer appear
// at the top level).
func containsEvent(msg map[string]any, typeWanted string) bool {
	if msg["type"] == typeWanted {
		return true
	}
	if msg["type"] != "tick_events" {
		return false
	}
	events, ok := msg["events"].([]any)
	if !ok {
		return false
	}
	for _, ev := range events {
		evMap, ok := ev.(map[string]any)
		if !ok {
			continue
		}
		if evMap["type"] == typeWanted {
			return true
		}
	}
	return false
}

// readUntil reads messages until one matches typeWanted or the deadline
// elapses. Returns the matching message or nil on timeout.
func readUntil(t *testing.T, ctx context.Context, conn *websocket.Conn, typeWanted string, within time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, time.Until(deadline))
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			return nil
		}
		var msg map[string]any
		_ = json.Unmarshal(data, &msg)
		if msg["type"] == typeWanted {
			return msg
		}
	}
	return nil
}

func TestWS_PresenceBroadcastsToViewersAndPersists(t *testing.T) {
	s, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	owner, _ := dialWithSession(t, ctx, ts, wsURL)
	defer owner.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, owner, initialMessages)

	all := s.manager.All()
	if len(all) != 1 {
		t.Fatalf("want 1 session, got %d", len(all))
	}
	ownerSess := all[0]

	viewer := dialAsViewer(t, ctx, ts, wsURL, ownerSess, s.manager)
	defer viewer.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, viewer, initialMessages)

	narratorsOn := true
	send(t, ctx, owner, Command{
		Cmd:         "presence",
		Section:     "/memoria",
		VisibleCore: []int{0, 2},
		Mode:        "pedagogic",
		Narrators:   &narratorsOn,
	})

	msg := readUntil(t, ctx, viewer, "presence", 2*time.Second)
	if msg == nil {
		t.Fatal("viewer never received presence broadcast")
	}
	if msg["section"] != "/memoria" {
		t.Errorf("section = %v, want /memoria", msg["section"])
	}
	cores, _ := msg["cores"].([]any)
	if len(cores) != 2 {
		t.Errorf("cores = %v, want [0 2]", msg["cores"])
	}
	if msg["mode"] != "pedagogic" {
		t.Errorf("mode = %v, want pedagogic", msg["mode"])
	}
	if msg["narrators"] != true {
		t.Errorf("narrators = %v, want true", msg["narrators"])
	}

	if got := ownerSess.Presence(); got.Section != "/memoria" || got.Mode != "pedagogic" || !got.Narrators {
		t.Errorf("server-side presence not persisted: %+v", got)
	}
}

func TestWS_PresenceNormalizesUnknownMode(t *testing.T) {
	s, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	owner, _ := dialWithSession(t, ctx, ts, wsURL)
	defer owner.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, owner, initialMessages)

	ownerSess := s.manager.All()[0]
	viewer := dialAsViewer(t, ctx, ts, wsURL, ownerSess, s.manager)
	defer viewer.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, viewer, initialMessages)

	send(t, ctx, owner, Command{
		Cmd:     "presence",
		Section: "/procesos",
		Mode:    "evil-preset",
	})
	msg := readUntil(t, ctx, viewer, "presence", 2*time.Second)
	if msg == nil {
		t.Fatal("viewer never received presence broadcast")
	}
	if msg["mode"] != "" {
		t.Errorf("unknown mode should be normalized to empty string, got %v", msg["mode"])
	}
}

func TestWS_ViewerCannotSendPresence(t *testing.T) {
	s, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	owner, _ := dialWithSession(t, ctx, ts, wsURL)
	defer owner.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, owner, initialMessages)

	ownerSess := s.manager.All()[0]
	viewer := dialAsViewer(t, ctx, ts, wsURL, ownerSess, s.manager)
	defer viewer.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, viewer, initialMessages)

	send(t, ctx, viewer, Command{Cmd: "presence", Section: "/procesos"})
	msg := readUntil(t, ctx, viewer, "error", 2*time.Second)
	if msg == nil {
		t.Fatal("viewer presence should be rejected with an error event")
	}
	if got := ownerSess.Presence(); got.Section != "" {
		t.Errorf("server should not persist viewer presence, got %+v", got)
	}
}

func TestWS_HandRaiseReachesOwnerOnly(t *testing.T) {
	s, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	owner, _ := dialWithSession(t, ctx, ts, wsURL)
	defer owner.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, owner, initialMessages)

	ownerSess := s.manager.All()[0]
	ownerSess.SetLabel("Estudiante demo")

	viewer := dialAsViewer(t, ctx, ts, wsURL, ownerSess, s.manager)
	defer viewer.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, viewer, initialMessages)

	send(t, ctx, viewer, Command{Cmd: "hand_raise"})

	msg := readUntil(t, ctx, owner, "hand_raise", 2*time.Second)
	if msg == nil {
		t.Fatal("owner never received hand_raise")
	}
	if msg["label"] != "Estudiante demo" {
		t.Errorf("label = %v, want 'Estudiante demo'", msg["label"])
	}

	// Viewer must not receive their own hand_raise echo.
	if echo := readUntil(t, ctx, viewer, "hand_raise", 500*time.Millisecond); echo != nil {
		t.Fatal("viewer should not receive a hand_raise echo")
	}
}

func TestWS_OwnerHandRaiseIsNoOp(t *testing.T) {
	_, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	owner, _ := dialWithSession(t, ctx, ts, wsURL)
	defer owner.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, owner, initialMessages)

	send(t, ctx, owner, Command{Cmd: "hand_raise"})

	// Owner should not loop the gesture back to themselves.
	if echo := readUntil(t, ctx, owner, "hand_raise", 500*time.Millisecond); echo != nil {
		t.Fatal("owner-sent hand_raise should be a no-op")
	}
}

func TestWS_LateViewerGetsPresenceInSessionInfo(t *testing.T) {
	s, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	owner, _ := dialWithSession(t, ctx, ts, wsURL)
	defer owner.Close(websocket.StatusNormalClosure, "")
	drainN(t, ctx, owner, initialMessages)

	ownerSess := s.manager.All()[0]
	send(t, ctx, owner, Command{Cmd: "presence", Section: "/interrupciones", VisibleCore: []int{1}})

	// Give the server a tick to persist presence before the viewer joins.
	if got := readUntil(t, ctx, owner, "error", 200*time.Millisecond); got != nil {
		t.Fatalf("owner presence rejected: %v", got)
	}
	_ = ownerSess // keep referenced for clarity

	viewer := dialAsViewer(t, ctx, ts, wsURL, ownerSess, s.manager)
	defer viewer.Close(websocket.StatusNormalClosure, "")

	info := readUntil(t, ctx, viewer, "session_info", 2*time.Second)
	if info == nil {
		t.Fatal("viewer never received session_info on connect")
	}
	pres, ok := info["presence"].(map[string]any)
	if !ok {
		t.Fatalf("session_info.presence missing or not an object: %v", info["presence"])
	}
	if pres["section"] != "/interrupciones" {
		t.Errorf("session_info.presence.section = %v, want /interrupciones", pres["section"])
	}
}
