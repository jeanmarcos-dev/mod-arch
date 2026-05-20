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
	for range 6 {
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
		if msg["type"] == "halt" {
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
