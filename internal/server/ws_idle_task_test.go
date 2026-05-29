package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestWS_SetIdleTask_FlipsToggleAndAnnouncesProcs validates the wire contract
// of the new command. The client sends {cmd:"set_idle_task", idle_task:true}
// and the simulator must (a) not reply with the "unknown command" error and
// (b) push a procs broadcast that includes PID 0 with is_idle=true so the
// frontend can render the compact swapper band.
func TestWS_SetIdleTask_FlipsToggleAndAnnouncesProcs(t *testing.T) {
	_, ts, wsURL := startTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _ := dialWithSession(t, ctx, ts, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "")

	drainN(t, ctx, conn, initialMessages)

	// Load any demo first — SetIdleTaskEnabled only spawns PID 0 on a
	// kernel that already has init wired up.
	send(t, ctx, conn, Command{Cmd: "load", Program: "add"})
	enabled := true
	send(t, ctx, conn, Command{Cmd: "set_idle_task", IdleTask: &enabled})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var msg map[string]any
		_ = json.Unmarshal(data, &msg)
		if kind, _ := msg["type"].(string); kind == "error" {
			t.Fatalf("server rejected set_idle_task: %v", msg["message"])
		}
		if kind, _ := msg["type"].(string); kind != "procs" {
			continue
		}
		procs, _ := msg["procs"].([]any)
		for _, raw := range procs {
			p, _ := raw.(map[string]any)
			pid, _ := p["pid"].(float64) // JSON numbers decode as float64
			if int(pid) != 0 {
				continue
			}
			if isIdle, _ := p["is_idle"].(bool); !isIdle {
				t.Errorf("PID 0 in procs broadcast had is_idle = %v, want true", p["is_idle"])
			}
			return
		}
	}
	t.Fatal("never received a procs broadcast containing PID 0 after set_idle_task")
}
