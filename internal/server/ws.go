package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/sim"
)

const (
	writeTimeout = 5 * time.Second
	pingEvery    = 30 * time.Second
)

// Command is the WS message the client sends to the server.
type Command struct {
	Cmd     string `json:"cmd"`
	Program string `json:"program,omitempty"`
	Speed   int    `json:"speed,omitempty"`
	Quantum int    `json:"quantum,omitempty"`
	Key     string `json:"key,omitempty"`
	Alias   string `json:"alias,omitempty"`
}

// handleWS resolves the session from cookies and services the WebSocket
// connection until the client disconnects. The role (owner/viewer) decides
// which commands are allowed.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	sess, role := s.resolveSessionAndRole(r)
	if sess == nil {
		http.Error(w, "no hay sesión — refresque la página", http.StatusUnauthorized)
		return
	}
	sess.Touch()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.logger.Warn("ws accept failed", "err", err, "session", sess.ID)
		return
	}

	c := sim.NewClient()
	sess.Sim.AddClient(c)
	s.logger.Info("ws client connected", "session", sess.ID, "role", role)

	// Initial snapshot for the freshly-connected client.
	c.Send <- sess.Sim.Snapshot()
	c.Send <- map[string]any{"type": "programs", "list": s.manager.Programs()}
	c.Send <- s.sessionInfoMessage(sess, role)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go s.wsReadLoop(ctx, conn, sess, role, c, cancel)
	s.wsWriteLoop(ctx, conn, c)

	sess.Sim.RemoveClient(c)
	_ = conn.Close(websocket.StatusNormalClosure, "")
	s.logger.Info("ws client disconnected", "session", sess.ID, "role", role)
}

// sessionInfoMessage builds the event we send the client so it knows which
// session it belongs to and with which role. Sent on connect and whenever
// something relevant changes (share toggled, label updated, user logged
// in/out, etc.).
func (s *Server) sessionInfoMessage(sess *sessions.Session, role sessions.Role) map[string]any {
	msg := map[string]any{
		"type":       "session_info",
		"id":         sess.ID,
		"mode":       sess.Mode.String(),
		"role":       role.String(),
		"label":      sess.Label(),
		"view_token": sess.ViewToken(),
		"user_id":    "",
		"user_name":  "",
		"user_email": "",
	}
	uid := sess.UserID()
	if uid == "" {
		return msg
	}
	st := s.manager.Store()
	if st == nil {
		return msg
	}
	u, err := st.GetUser(context.Background(), uid)
	if err != nil {
		return msg
	}
	msg["user_id"] = u.ID
	msg["user_name"] = u.Name
	msg["user_email"] = u.Email
	return msg
}

// wsReadLoop receives commands from the client and dispatches them to the
// Simulator.
func (s *Server) wsReadLoop(ctx context.Context, conn *websocket.Conn, sess *sessions.Session, role sessions.Role, c *sim.Client, cancel context.CancelFunc) {
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var cmd Command
		if err := json.Unmarshal(data, &cmd); err != nil {
			c.Send <- map[string]any{"type": "error", "message": "json: " + err.Error()}
			continue
		}
		s.dispatch(cmd, sess, role, c)
	}
}

// wsWriteLoop drains the Send channel and writes to the websocket.
func (s *Server) wsWriteLoop(ctx context.Context, conn *websocket.Conn, c *sim.Client) {
	pingT := time.NewTicker(pingEvery)
	defer pingT.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-c.Send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			data, _ := json.Marshal(msg)
			if err := conn.Write(wctx, websocket.MessageText, data); err != nil {
				cancel()
				return
			}
			cancel()
		case <-pingT.C:
			pctx, cancel := context.WithTimeout(ctx, writeTimeout)
			if err := conn.Ping(pctx); err != nil {
				cancel()
				return
			}
			cancel()
		}
	}
}

// viewerSafeCommands lists the read-only commands a viewer can issue. Anything
// else triggers an error and is not executed.
var viewerSafeCommands = map[string]bool{
	"snapshot": true,
}

// dispatch routes a Command to the session's Simulator. When role is Viewer,
// mutating commands are filtered out.
//
//nolint:gocyclo // command dispatcher; cases are a flat switch by design.
func (s *Server) dispatch(cmd Command, sess *sessions.Session, role sessions.Role, c *sim.Client) {
	sess.Touch()
	if role == sessions.RoleViewer && !viewerSafeCommands[cmd.Cmd] {
		c.Send <- map[string]any{
			"type":    "error",
			"message": "modo espectador de solo lectura: el propietario es quien controla la sesión",
		}
		return
	}
	switch cmd.Cmd {
	case "load":
		if err := sess.Sim.LoadProgram(cmd.Program); err != nil {
			c.Send <- map[string]any{"type": "error", "message": err.Error()}
		}
	case "step":
		_ = sess.Sim.StepOnce()
	case "run":
		sess.Sim.StartRun(cmd.Speed)
	case "pause":
		sess.Sim.StopRun()
	case "reset":
		sess.Sim.Reset()
	case "snapshot":
		c.Send <- sess.Sim.Snapshot()
	case "timer_quantum":
		sess.Sim.SetTimerQuantum(cmd.Quantum)
	case "push_key":
		sess.Sim.PushKey(cmd.Key)
	case "share":
		s.manager.ActivateShare(sess)
		sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))
	case "unshare":
		s.manager.DeactivateShare(sess)
		sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))
	case "set_alias":
		alias := strings.TrimSpace(cmd.Alias)
		if len(alias) > 32 {
			alias = alias[:32]
		}
		sess.SetLabel(alias)
		sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))
	default:
		c.Send <- map[string]any{"type": "error", "message": "comando desconocido: " + cmd.Cmd}
	}
}
