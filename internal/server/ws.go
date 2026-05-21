package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/sim"
)

// defaultAllowedOrigins are the host patterns accepted on WS upgrades when
// the ALLOWED_ORIGINS env var is unset. coder/websocket matches these against
// the host portion of the Origin header, so no scheme is included. Both the
// Vite dev server and the embedded production bundle are covered out of the
// box; production deployments behind a real domain must set ALLOWED_ORIGINS.
var defaultAllowedOrigins = []string{
	"localhost:5173",
	"localhost:8080",
}

// allowedWSOrigins parses ALLOWED_ORIGINS (comma-separated) and falls back to
// the defaults. The result is the exact value that websocket.AcceptOptions
// expects in OriginPatterns.
func allowedWSOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
	if raw == "" {
		return defaultAllowedOrigins
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return defaultAllowedOrigins
	}
	return out
}

const (
	writeTimeout = 5 * time.Second
	pingEvery    = 30 * time.Second
)

// simClientRole maps a session role into the sim-layer enum used to tag
// clients for selective broadcasts (presence to viewers, hand_raise to owner).
func simClientRole(r sessions.Role) sim.ClientRole {
	switch r {
	case sessions.RoleOwner:
		return sim.RoleOwner
	case sessions.RoleViewer:
		return sim.RoleViewer
	default:
		return sim.RoleAny
	}
}

// Command is the WS message the client sends to the server.
type Command struct {
	Cmd     string `json:"cmd"`
	Program string `json:"program,omitempty"`
	Speed   int    `json:"speed,omitempty"`
	Quantum int    `json:"quantum,omitempty"`
	Key     string `json:"key,omitempty"`
	Alias   string `json:"alias,omitempty"`
	Cores   int    `json:"cores,omitempty"`

	// Owner-broadcasted presence payload: the section the host is currently
	// presenting, the visible cores filter, the presentation mode preset and
	// the narrators toggle. Viewers with autofollow on mirror these to keep
	// their UI in sync.
	Section     string      `json:"section,omitempty"`
	VisibleCore []int       `json:"visible_cores,omitempty"`
	Mode        string      `json:"mode,omitempty"`
	Narrators   *bool       `json:"narrators,omitempty"`
	Focus       *FocusInput `json:"focus,omitempty"`

	// Whiteboard pointer payload (cmd="pointer"). Coordinates are DOM-anchored:
	// PointerAnchor is the id of a [data-pizarra-anchor] element under the
	// owner's mouse (e.g. "cpu-panel"), and OffsetX/OffsetY are normalized to
	// [0, 1] relative to that anchor's bounding rect. The viewer locates the
	// same anchor in its own DOM and projects the offset there, so the cursor
	// lands on the same panel even when the two shells have different sizes,
	// sidebars, or topbar heights. The "shell-content" anchor is used as a
	// fallback when no specific panel is under the cursor (e.g. over the
	// sidebar). Route is the owner's current path; viewers only render the
	// cursor when their route matches.
	PointerAnchor  string  `json:"anchor,omitempty"`
	PointerOffsetX float64 `json:"offset_x,omitempty"`
	PointerOffsetY float64 `json:"offset_y,omitempty"`
	PointerRoute   string  `json:"route,omitempty"`
}

// FocusInput mirrors the wire shape the owner sends inside a presence
// command. Every field is a pointer so the client can choose to update only
// the ones that actually changed — the server merges the deltas into the
// stored Focus snapshot. The structure mirrors sessions.Focus 1:1 except
// each field is decoded server-side before propagation.
type FocusInput struct {
	MiddleTab        *string `json:"middle_tab,omitempty"`
	MobileTab        *string `json:"mobile_tab,omitempty"`
	MemoryBaseAddr   *uint64 `json:"memory_base_addr,omitempty"`
	MemoryAutoFollow *bool   `json:"memory_auto_follow,omitempty"`
	PinnedPID        *int    `json:"pinned_pid,omitempty"`
	SimBarMode       *string `json:"sim_bar_mode,omitempty"`
	SidebarCollapsed *bool   `json:"sidebar_collapsed,omitempty"`
	IsMobile         *bool   `json:"is_mobile,omitempty"`
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
		OriginPatterns: allowedWSOrigins(),
	})
	if err != nil {
		s.logger.Warn("ws accept failed",
			"err", err,
			"session", sess.ID,
			"origin", r.Header.Get("Origin"),
			"ip", clientIP(r),
		)
		return
	}

	c := sim.NewClient(simClientRole(role))
	sess.Sim.AddClient(c)
	s.logger.Info("ws client connected", "session", sess.ID, "role", role)

	// Initial snapshot for the freshly-connected client.
	c.Send <- sess.Sim.Snapshot()
	c.Send <- map[string]any{"type": "programs", "list": s.manager.Programs()}
	c.Send <- s.sessionInfoMessage(sess, role)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Register viewer cancels with the session so a later DeactivateShare
	// can boot them in bulk. Owners are skipped — they do not get evicted
	// by share toggles.
	var viewerID int
	if role == sessions.RoleViewer {
		viewerID = sess.AddViewer(cancel)
		defer sess.RemoveViewer(viewerID)
	}

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
//
// For viewers we also embed the latest presence snapshot so a late-joiner can
// jump straight to the section the host is presenting without waiting for the
// next presence broadcast.
func (s *Server) sessionInfoMessage(sess *sessions.Session, role sessions.Role) map[string]any {
	pres := sess.Presence()
	presPayload := map[string]any{
		"section":   pres.Section,
		"cores":     pres.Cores,
		"mode":      pres.Mode,
		"narrators": pres.Narrators,
		"focus":     focusPayload(pres.Focus),
	}
	if pres.Cores == nil {
		presPayload["cores"] = []int{}
	}
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
		"presence":   presPayload,
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
// else triggers an error and is not executed. hand_raise is viewer-only by
// design: the owner has no need to "raise the hand" to themselves.
var viewerSafeCommands = map[string]bool{
	"snapshot":   true,
	"hand_raise": true,
}

// ownerOnlyCommands lists commands that only the owner may issue. presence is
// listed here so a viewer that crafts a fake presence does not overwrite the
// host's broadcast. The whiteboard cursor (pointer / pointer_off) is also
// owner-only — viewers receive but never emit.
var ownerOnlyCommands = map[string]bool{
	"presence":    true,
	"pointer":     true,
	"pointer_off": true,
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
	if role != sessions.RoleOwner && ownerOnlyCommands[cmd.Cmd] {
		c.Send <- map[string]any{
			"type":    "error",
			"message": "comando reservado al anfitrión de la sesión",
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
	case "set_speed":
		sess.Sim.SetSpeed(cmd.Speed)
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
	case "set_num_cores":
		if err := sess.Sim.SetNumCores(cmd.Cores); err != nil {
			c.Send <- map[string]any{"type": "error", "message": err.Error()}
		}
	case "set_alias":
		alias, err := cleanLabel(cmd.Alias)
		if err != nil {
			c.Send <- map[string]any{"type": "error", "message": err.Error()}
			return
		}
		sess.SetLabel(alias)
		sess.Sim.Broadcast(s.sessionInfoMessage(sess, sessions.RoleOwner))
	case "presence":
		s.handlePresence(cmd, sess)
	case "hand_raise":
		s.handleHandRaise(sess, role)
	case "pointer":
		s.handleOwnerPointer(cmd, sess)
	case "pointer_off":
		s.handleOwnerPointerOff(sess)
	default:
		c.Send <- map[string]any{"type": "error", "message": "comando desconocido: " + cmd.Cmd}
	}
}

// handlePresence stores the owner's current section + filters + view
// settings and broadcasts it to every viewer of the session so
// autofollowing students see the same thing the host is presenting.
//
// Focus is treated as an additive merge against the previous snapshot: every
// missing pointer is left alone so the client can ship deltas. The merged
// snapshot is what gets persisted in the session and rebroadcast.
func (s *Server) handlePresence(cmd Command, sess *sessions.Session) {
	section := strings.TrimSpace(cmd.Section)
	if section == "" {
		return
	}
	cores := append([]int(nil), cmd.VisibleCore...)
	mode := normalizePresentationMode(cmd.Mode)
	narrators := false
	if cmd.Narrators != nil {
		narrators = *cmd.Narrators
	}
	prev := sess.Presence()
	focus := mergeFocus(prev.Focus, cmd.Focus)
	now := time.Now()
	sess.SetPresence(sessions.Presence{
		Section:   section,
		Cores:     cores,
		Mode:      mode,
		Narrators: narrators,
		Focus:     focus,
		UpdatedAt: now,
	})
	if cores == nil {
		cores = []int{}
	}
	sess.Sim.BroadcastTo(map[string]any{
		"type":      "presence",
		"section":   section,
		"cores":     cores,
		"mode":      mode,
		"narrators": narrators,
		"focus":     focusPayload(focus),
		"at":        now.Unix(),
	}, sim.RoleViewer)
}

// focusPayload renders a sessions.Focus as the omit-empty map the wire
// expects: only fields the owner has actually set show up, so the viewer
// sees a clean delta vs noisy zero values.
func focusPayload(f sessions.Focus) map[string]any {
	out := map[string]any{}
	if f.MiddleTab != nil {
		out["middle_tab"] = *f.MiddleTab
	}
	if f.MobileTab != nil {
		out["mobile_tab"] = *f.MobileTab
	}
	if f.MemoryBaseAddr != nil {
		out["memory_base_addr"] = *f.MemoryBaseAddr
	}
	if f.MemoryAutoFollow != nil {
		out["memory_auto_follow"] = *f.MemoryAutoFollow
	}
	if f.PinnedPID != nil {
		out["pinned_pid"] = *f.PinnedPID
	}
	if f.SimBarMode != nil {
		out["sim_bar_mode"] = *f.SimBarMode
	}
	if f.SidebarCollapsed != nil {
		out["sidebar_collapsed"] = *f.SidebarCollapsed
	}
	if f.IsMobile != nil {
		out["is_mobile"] = *f.IsMobile
	}
	return out
}

// mergeFocus applies the deltas in input on top of base. Missing fields in
// input mean "no change" so a client that only flips one knob does not have
// to resend the full focus snapshot every time.
func mergeFocus(base sessions.Focus, input *FocusInput) sessions.Focus {
	out := base
	if input == nil {
		return out
	}
	if input.MiddleTab != nil {
		v := strings.TrimSpace(*input.MiddleTab)
		if v == "" {
			out.MiddleTab = nil
		} else {
			out.MiddleTab = &v
		}
	}
	if input.MobileTab != nil {
		v := strings.TrimSpace(*input.MobileTab)
		if v == "" {
			out.MobileTab = nil
		} else {
			out.MobileTab = &v
		}
	}
	if input.MemoryBaseAddr != nil {
		v := *input.MemoryBaseAddr
		out.MemoryBaseAddr = &v
	}
	if input.MemoryAutoFollow != nil {
		v := *input.MemoryAutoFollow
		out.MemoryAutoFollow = &v
	}
	if input.PinnedPID != nil {
		v := *input.PinnedPID
		out.PinnedPID = &v
	}
	if input.SimBarMode != nil {
		v := strings.TrimSpace(*input.SimBarMode)
		if v == "" {
			out.SimBarMode = nil
		} else {
			out.SimBarMode = &v
		}
	}
	if input.SidebarCollapsed != nil {
		v := *input.SidebarCollapsed
		out.SidebarCollapsed = &v
	}
	if input.IsMobile != nil {
		v := *input.IsMobile
		out.IsMobile = &v
	}
	return out
}

// whiteboardRoutePattern accepts the same shape the SPA's router accepts: a
// leading slash followed by path-safe characters. We cap at 64 chars so a
// malformed client cannot flood viewers with arbitrary strings.
var whiteboardRoutePattern = regexp.MustCompile(`^/[A-Za-z0-9/_:\-]{0,63}$`)

// whiteboardAnchorPattern restricts anchor ids to a small ASCII subset so a
// buggy or hostile client cannot inject arbitrary attribute selectors that
// the viewer would later use in document.querySelector.
var whiteboardAnchorPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// handleOwnerPointer broadcasts a pointer position from the owner to every
// viewer. Coordinates are DOM-anchored ({anchor, offset_x, offset_y}); the
// viewer locates the same anchor in its own DOM and positions the cursor
// relative to that element so it stays aligned across screen sizes.
func (s *Server) handleOwnerPointer(cmd Command, sess *sessions.Session) {
	if cmd.PointerOffsetX < 0 || cmd.PointerOffsetX > 1 || cmd.PointerOffsetY < 0 || cmd.PointerOffsetY > 1 {
		return
	}
	anchor := strings.TrimSpace(cmd.PointerAnchor)
	if !whiteboardAnchorPattern.MatchString(anchor) {
		return
	}
	route := strings.TrimSpace(cmd.PointerRoute)
	if !whiteboardRoutePattern.MatchString(route) {
		return
	}
	sess.Sim.BroadcastTo(map[string]any{
		"type":     "owner_pointer",
		"anchor":   anchor,
		"offset_x": cmd.PointerOffsetX,
		"offset_y": cmd.PointerOffsetY,
		"route":    route,
		"ts":       time.Now().UnixMilli(),
	}, sim.RoleViewer)
}

// handleOwnerPointerOff signals the viewer overlays to clear: the owner
// turned the whiteboard toggle off or moved to a non-broadcasting route.
func (s *Server) handleOwnerPointerOff(sess *sessions.Session) {
	sess.Sim.BroadcastTo(map[string]any{
		"type": "owner_pointer",
		"off":  true,
		"ts":   time.Now().UnixMilli(),
	}, sim.RoleViewer)
}

// normalizePresentationMode rejects unknown values from the client so a
// malformed payload cannot drop viewers into an undefined preset. Empty
// string passes through to mean "keep whatever the viewer had".
func normalizePresentationMode(m string) string {
	switch strings.TrimSpace(m) {
	case "compact", "pedagogic", "projector":
		return m
	default:
		return ""
	}
}

// handleHandRaise forwards the viewer's request for attention to the owner
// only. No-op if the sender is the owner — the gesture only makes sense from
// a viewer towards their host.
func (s *Server) handleHandRaise(sess *sessions.Session, role sessions.Role) {
	if role != sessions.RoleViewer {
		return
	}
	now := time.Now()
	sess.Sim.BroadcastTo(map[string]any{
		"type":  "hand_raise",
		"label": sess.Label(),
		"at":    now.Unix(),
	}, sim.RoleOwner)
}
