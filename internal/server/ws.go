package server

import (
	"github.com/jeanmarcos-dev/mod-arch/internal/telemetry"

	"fmt"

	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/jeanmarcos-dev/mod-arch/internal/server/i18n"
	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	"github.com/jeanmarcos-dev/mod-arch/internal/sim"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
	"github.com/jeanmarcos-dev/mod-arch/internal/tutor"
)

// defaultAllowedOrigins are the host patterns accepted on WS upgrades when
// the ALLOWED_ORIGINS env var is unset. coder/websocket matches these against
// the host portion of the Origin header, so no scheme is included. Both the
// Vite dev server and the embedded production bundle are covered out of the
// box; production deployments behind a real domain must set ALLOWED_ORIGINS.
var defaultAllowedOrigins = []string{
	"localhost:5173",
	"localhost:5174",
	"localhost:5175",
	"localhost:9080",
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
	// Speed is the legacy single-knob field kept for older clients. New
	// clients send ClockHz + SpeedMul separately via set_clock_hz and
	// set_speed_mul.
	Speed    int     `json:"speed,omitempty"`
	ClockHz  int     `json:"clockHz,omitempty"`
	SpeedMul float64 `json:"speedMul,omitempty"`
	Quantum  int     `json:"quantum,omitempty"`
	Key      string  `json:"key,omitempty"`
	Alias    string  `json:"alias,omitempty"`
	Cores    int     `json:"cores,omitempty"`
	// IdleTask toggles the kernel swapper (PID 0). Pointer so the client
	// can distinguish "explicit false" from "field omitted"; a missing
	// IdleTask field leaves the kernel state untouched.
	IdleTask *bool `json:"idle_task,omitempty"`

	// AutoPauseIdle toggles parking the run loop once no user work remains.
	// Pointer for the same omitted-vs-false reason as IdleTask.
	AutoPauseIdle *bool `json:"auto_pause_idle,omitempty"`

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

	// Glossary cross-shell hover (cmd="glossary_hover" / "glossary_hover_off").
	// Term is the canonical term string (e.g. "EIP", "Page fault"). Every
	// <Term> instance on the viewer side that matches this string opens its
	// tooltip; multiple matches all light up, which doubles as a "this concept
	// appears here, here and here" pedagogical hint.
	Term string `json:"term,omitempty"`

	// Tutor (cmd="ask"). Question is the student's free-form question; the
	// server merges it with retrieved passages and the simulator snapshot
	// before calling the LLM. RequestID is echoed back in the response so
	// the UI can pair the answer with the pending input. TutorMode is
	// "direct" (default) or "socratic" — the orchestrator swaps the system
	// prompt to a question-driven variant in socratic mode.
	Question  string              `json:"question,omitempty"`
	RequestID string              `json:"request_id,omitempty"`
	TutorMode string              `json:"tutor_mode,omitempty"`
	History   []tutor.HistoryTurn `json:"history,omitempty"`
	// DetectionContext is a pre-rendered Spanish summary of the live
	// concurrency detectors (lost updates, deadlock cycle) the client computes
	// so the tutor can explain this run instead of generic theory.
	DetectionContext string `json:"detection_context,omitempty"`

	Event      string `json:"event,omitempty"`
	Name       string `json:"name,omitempty"`
	DurationMS *int64 `json:"duration_ms,omitempty"`
	Rating     *int   `json:"rating,omitempty"`
}

// FocusInput mirrors the wire shape the owner sends inside a presence
// command. Every field is a pointer so the client can choose to update only
// the ones that actually changed — the server merges the deltas into the
// stored Focus snapshot. The structure mirrors sessions.Focus 1:1 except
// each field is decoded server-side before propagation.

// Validate enforces strict constraints on incoming payloads to protect the
// server from malformed, missing, or maliciously sized inputs.
func (c *Command) Validate() error {
	switch c.Cmd {
	case "run":
		if c.Speed < 0 || c.Speed > 100 {
			return fmt.Errorf("speed must be between 0 and 100")
		}
	case "load":
		if len(c.Program) == 0 || len(c.Program) > 200 {
			return fmt.Errorf("invalid program name length")
		}
	case "set_speed":
		if c.Speed < 0 || c.Speed > 100 {
			return fmt.Errorf("speed must be between 0 and 100")
		}
	case "set_clock_hz":
		if c.ClockHz <= 0 || c.ClockHz > 1000 {
			return fmt.Errorf("clockHz must be between 1 and 1000")
		}
	case "set_speed_mul":
		if c.SpeedMul < 0.1 || c.SpeedMul > 20.0 {
			return fmt.Errorf("speedMul must be between 0.1 and 20.0")
		}
	case "set_num_cores":
		if c.Cores < 1 || c.Cores > 32 {
			return fmt.Errorf("cores must be between 1 and 32")
		}
	case "timer_quantum":
		if c.Quantum < 1 || c.Quantum > 10000 {
			return fmt.Errorf("quantum must be between 1 and 10000")
		}
	case "push_key":
		if len(c.Key) == 0 || len(c.Key) > 10 {
			return fmt.Errorf("invalid key string length")
		}
	case "set_alias":
		if len(c.Alias) > 100 {
			return fmt.Errorf("alias too long")
		}
	case "pointer", "click":
		if len(c.PointerAnchor) > 50 || len(c.PointerRoute) > 100 {
			return fmt.Errorf("pointer strings too long")
		}
		if c.PointerOffsetX < -1.0 || c.PointerOffsetX > 2.0 || c.PointerOffsetY < -1.0 || c.PointerOffsetY > 2.0 {
			return fmt.Errorf("pointer offsets out of bounds")
		}
	case "glossary_hover":
		if len(c.Term) > 100 {
			return fmt.Errorf("glossary term too long")
		}
	case "ask":
		if len(c.Question) == 0 || len(c.Question) > 3000 {
			return fmt.Errorf("question length must be between 1 and 3000 chars")
		}
		if len(c.RequestID) > 100 {
			return fmt.Errorf("request ID too long")
		}
		if c.TutorMode != "" && c.TutorMode != "direct" && c.TutorMode != "socratic" {
			return fmt.Errorf("invalid tutor mode")
		}
	case "track":
		if c.Event != trackEventPageView {
			return fmt.Errorf("unknown track event")
		}
		if len(c.Name) == 0 || len(c.Name) > trackMaxNameLen || c.Name[0] != '/' {
			return fmt.Errorf("invalid track name")
		}
		if c.DurationMS == nil || *c.DurationMS < 0 || *c.DurationMS > trackMaxDurationMS {
			return fmt.Errorf("track duration out of range")
		}
	case "tutor_feedback":
		if len(c.RequestID) == 0 || len(c.RequestID) > 100 {
			return fmt.Errorf("invalid request ID")
		}
		if c.Rating == nil || *c.Rating < -1 || *c.Rating > 1 {
			return fmt.Errorf("rating must be -1, 0 or 1")
		}
	case "presence":
		if c.Section == "" {
			return fmt.Errorf("section cannot be empty")
		}
		if len(c.Section) > 50 || len(c.Mode) > 50 {
			return fmt.Errorf("presence strings too long")
		}
		if len(c.VisibleCore) > 32 {
			return fmt.Errorf("too many visible cores")
		}
		for _, core := range c.VisibleCore {
			if core < 0 || core > 1024 {
				return fmt.Errorf("invalid core id: %d", core)
			}
		}
		if c.Focus != nil {
			if c.Focus.MiddleTab != nil && len(*c.Focus.MiddleTab) > 50 {
				return fmt.Errorf("middle tab identifier too long")
			}
			if c.Focus.SimBarMode != nil {
				mode := *c.Focus.SimBarMode
				if mode != "docked" && mode != "floating" {
					return fmt.Errorf("invalid sim bar mode: %s", mode)
				}
			}
		}
	}
	return nil
}

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
		i18n.HTTPError(w, r, http.StatusUnauthorized, i18n.CodeAuthUnauthenticated, nil)
		return
	}
	sess.Touch()

	defer telemetry.TrackWSConnection()()

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

	go s.wsReadLoop(ctx, conn, sess, role, c, s.wsUserIDFunc(r, sess, role), cancel)
	s.wsWriteLoop(ctx, conn, c)

	sess.Sim.RemoveClient(c)
	// When the owner walks away without disarming the whiteboard toggle, push
	// a pointer_off and glossary_hover_off so every viewer drops the overlay
	// immediately instead of waiting for the 2s/5s staleness timers. Cheap to
	// emit even when the toggles were never on — viewers ignore the no-op.
	if role == sessions.RoleOwner {
		s.handleOwnerPointerOff(sess)
		s.handleOwnerGlossaryHoverOff(sess)
	}
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
func (s *Server) wsReadLoop(ctx context.Context, conn *websocket.Conn, sess *sessions.Session, role sessions.Role, c *sim.Client, userID func() string, cancel context.CancelFunc) {
	defer cancel()
	limiter := newTrackLimiter(trackLimitPerMinute, time.Minute)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var cmd Command
		if err := json.Unmarshal(data, &cmd); err != nil {
			c.Send <- map[string]any{
				"type":    "error",
				"code":    i18n.CodeWSJSONError,
				"message": i18n.T(i18n.DefaultLocale, i18n.CodeWSJSONError, map[string]string{"err": err.Error()}),
				"params":  map[string]string{"err": err.Error()},
			}
			continue
		}
		if err := cmd.Validate(); err != nil {
			if cmd.Cmd == "track" || cmd.Cmd == "tutor_feedback" {
				continue
			}
			c.Send <- map[string]any{
				"type":    "error",
				"code":    "validation_error",
				"message": err.Error(),
			}
			continue
		}
		if cmd.Cmd == "track" && !limiter.allow() {
			continue
		}
		s.dispatch(cmd, sess, role, c, userID)
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
// "ask" is allowed for viewers too: a student watching another's session can
// still ask the tutor questions without taking control.
var viewerSafeCommands = map[string]bool{
	"snapshot":       true,
	"hand_raise":     true,
	"ask":            true,
	"track":          true,
	"tutor_feedback": true,
}

// ownerOnlyCommands lists commands that only the owner may issue. presence is
// listed here so a viewer that crafts a fake presence does not overwrite the
// host's broadcast. The whiteboard cursor (pointer / pointer_off) is also
// owner-only — viewers receive but never emit.
var ownerOnlyCommands = map[string]bool{
	"presence":           true,
	"pointer":            true,
	"pointer_off":        true,
	"click":              true,
	"glossary_hover":     true,
	"glossary_hover_off": true,
}

// dispatch routes a Command to the session's Simulator. When role is Viewer,
// mutating commands are filtered out.
//
//nolint:gocyclo // command dispatcher; cases are a flat switch by design.
func (s *Server) dispatch(cmd Command, sess *sessions.Session, role sessions.Role, c *sim.Client, userID func() string) {
	sess.Touch()
	if role == sessions.RoleViewer && !viewerSafeCommands[cmd.Cmd] {
		c.Send <- map[string]any{
			"type":    "error",
			"code":    i18n.CodeWSViewerReadonly,
			"message": i18n.T(i18n.DefaultLocale, i18n.CodeWSViewerReadonly, nil),
		}
		return
	}
	if role != sessions.RoleOwner && ownerOnlyCommands[cmd.Cmd] {
		c.Send <- map[string]any{
			"type":    "error",
			"code":    i18n.CodeWSHostOnly,
			"message": i18n.T(i18n.DefaultLocale, i18n.CodeWSHostOnly, nil),
		}
		return
	}
	switch cmd.Cmd {
	case "track":
		s.handleTrack(cmd, sess, role, userID)
		return
	case "tutor_feedback":
		s.handleTutorFeedback(cmd, sess)
		return
	}
	sess.IncCommandCounter()
	switch cmd.Cmd {
	case "load":
		if s.demoBlockedForSession(sess, cmd.Program) {
			c.Send <- map[string]any{
				"type":    "error",
				"code":    "demo_disabled",
				"message": "Esta demo está deshabilitada por el administrador.",
			}
			return
		}
		if err := sess.Sim.LoadProgram(cmd.Program); err != nil {
			c.Send <- map[string]any{"type": "error", "message": err.Error()}
			return
		}
		sess.SetProgram(cmd.Program)
		s.recordUsage(sess, role, userID, store.UsageKindDemoLoad, cmd.Program, nil)
	case "step":
		_ = sess.Sim.StepOnce()
	case "run":
		sess.Sim.StartRun(cmd.Speed)
		s.recordUsage(sess, role, userID, store.UsageKindDemoRun, sess.Program(), nil)
	case "set_speed":
		sess.Sim.SetSpeed(cmd.Speed)
	case "set_clock_hz":
		sess.Sim.SetClockHz(cmd.ClockHz)
	case "set_speed_mul":
		sess.Sim.SetSpeedMul(cmd.SpeedMul)
	case "pause":
		sess.Sim.Pause()
		s.recordUsage(sess, role, userID, store.UsageKindDemoPause, sess.Program(), nil)
	case "reset":
		sess.Sim.Reset()
		s.recordUsage(sess, role, userID, store.UsageKindDemoReset, sess.Program(), nil)
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
	case "set_idle_task":
		if cmd.IdleTask == nil {
			c.Send <- map[string]any{"type": "error", "message": "set_idle_task requires an idle_task boolean"}
			return
		}
		if err := sess.Sim.SetIdleTaskEnabled(*cmd.IdleTask); err != nil {
			c.Send <- map[string]any{"type": "error", "message": err.Error()}
		}
	case "set_auto_pause_idle":
		if cmd.AutoPauseIdle == nil {
			c.Send <- map[string]any{"type": "error", "message": "set_auto_pause_idle requires an auto_pause_idle boolean"}
			return
		}
		sess.Sim.SetAutoPauseOnIdle(*cmd.AutoPauseIdle)
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
	case "click":
		s.handleOwnerClick(cmd, sess)
	case "glossary_hover":
		s.handleOwnerGlossaryHover(cmd, sess)
	case "glossary_hover_off":
		s.handleOwnerGlossaryHoverOff(sess)
	case "ask":
		s.handleTutorAsk(cmd, sess, c)
	default:
		c.Send <- map[string]any{
			"type":    "error",
			"code":    i18n.CodeWSUnknownCommand,
			"message": i18n.T(i18n.DefaultLocale, i18n.CodeWSUnknownCommand, map[string]string{"cmd": cmd.Cmd}),
			"params":  map[string]string{"cmd": cmd.Cmd},
		}
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

// whiteboardTermPattern restricts the canonical term string forwarded to
// every viewer. The glossary lookup is case-insensitive on the client so a
// generous Latin-1 + punctuation alphabet is fine; the hard cap protects
// the wire from being abused as a long-string broadcaster.
var whiteboardTermPattern = regexp.MustCompile(`^[\p{L}\p{N}0-9 _:.\-/#&()\[\]<>]{1,64}$`)

// handleOwnerGlossaryHover relays a hover-into-tooltip event so every viewer
// can open the same glossary popover on the matching <Term> in their own
// DOM. Term is the canonical key (not an instance id) so multiple Term
// instances on the page all light up — pedagogically useful since the
// concept may appear in several places at once.
func (s *Server) handleOwnerGlossaryHover(cmd Command, sess *sessions.Session) {
	term := strings.TrimSpace(cmd.Term)
	if !whiteboardTermPattern.MatchString(term) {
		return
	}
	sess.Sim.BroadcastTo(map[string]any{
		"type": "owner_glossary_hover",
		"term": term,
		"ts":   time.Now().UnixMilli(),
	}, sim.RoleViewer)
}

// handleOwnerGlossaryHoverOff signals the matching viewer tooltips to
// close. Sent when the owner moves the mouse away or the tooltip's own
// staleness timer fires.
func (s *Server) handleOwnerGlossaryHoverOff(sess *sessions.Session) {
	sess.Sim.BroadcastTo(map[string]any{
		"type": "owner_glossary_hover",
		"off":  true,
		"ts":   time.Now().UnixMilli(),
	}, sim.RoleViewer)
}

// handleOwnerClick is the click-ripple equivalent of the pointer cursor: the
// owner clicked on a panel and the viewers paint a short-lived pulse on the
// same anchor + offset so the audience sees where the host is signaling. We
// reuse the pointer coordinate validation since the two payloads share the
// same anchor + offset semantics.
func (s *Server) handleOwnerClick(cmd Command, sess *sessions.Session) {
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
		"type":     "owner_click",
		"anchor":   anchor,
		"offset_x": cmd.PointerOffsetX,
		"offset_y": cmd.PointerOffsetY,
		"route":    route,
		"ts":       time.Now().UnixMilli(),
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
