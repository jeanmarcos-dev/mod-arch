// Package sessions multiplexes N Simulators behind the Server.
//
// Each HTTP/WS client belongs to a Session identified by cookie. The Session
// owns its own *sim.Simulator, fully isolated from the rest, so multiple
// users can interact with the demo concurrently without stepping on each
// other.
//
// On top of its owner (Role=Owner), a Session can have N viewers
// (Role=Viewer) connected through a /v/<token> link. Viewers receive the
// same events as the owner but the server filters out any mutating command
// they try to send.
//
// # Persistence
//
// When a *store.Store is provided in Config, sessions and invites are
// persisted to SQLite. Identity (id, mode), display state (label, public,
// view_token) and timestamps survive process restarts; the *sim.Simulator
// runtime itself is volatile and rebuilt fresh on rehydration. With no
// store, the Manager works fully in-memory (used by unit tests).
package sessions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/sim"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// Manager errors signaling capacity caps.
var (
	// ErrCapReached is returned when the public session cap is full.
	ErrCapReached = errors.New("public session cap reached")
	// ErrPrivCapReached is returned when the privileged session cap is full.
	ErrPrivCapReached = errors.New("privileged session cap reached")
)

// Mode marks the kind of session (which cap it consumes).
type Mode int

const (
	// ModeFull is a public owner session that counts against the public cap.
	ModeFull Mode = iota
	// ModePrivileged is an admin-issued owner session that bypasses the public cap.
	ModePrivileged
)

// String returns a stable identifier for the mode.
func (m Mode) String() string {
	switch m {
	case ModeFull:
		return "full"
	case ModePrivileged:
		return "privileged"
	default:
		return "unknown"
	}
}

func modeFromString(s string) Mode {
	if s == "privileged" {
		return ModePrivileged
	}
	return ModeFull
}

// Role describes the role of a WS client within a Session.
type Role int

const (
	// RoleNone is used when the client could not be identified.
	RoleNone Role = iota
	// RoleOwner is the session owner — can issue any command.
	RoleOwner
	// RoleViewer is subscribed via /v/<token> — read-only.
	RoleViewer
)

// String returns a stable identifier for the role.
func (r Role) String() string {
	switch r {
	case RoleOwner:
		return "owner"
	case RoleViewer:
		return "viewer"
	default:
		return "none"
	}
}

// Session is an isolated instance for an owner plus its viewers.
type Session struct {
	ID        string
	Sim       *sim.Simulator
	Mode      Mode
	CreatedAt time.Time

	mgr *Manager // back-reference for persistence

	mu           sync.RWMutex
	viewToken    string // empty means share is disabled
	label        string
	public       bool
	lastActiveAt time.Time
}

// ViewToken returns the current share token ("" when not shared).
func (s *Session) ViewToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.viewToken
}

// Label returns the visible alias of the session (thread-safe).
func (s *Session) Label() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.label
}

// SetLabel updates the visible alias and persists it.
func (s *Session) SetLabel(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
	if s.mgr != nil {
		s.mgr.persistLabel(s.ID, label)
	}
}

// Public reports whether the session is listed on /full once the cap is hit.
func (s *Session) Public() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.public
}

// SetPublic toggles public visibility on /full and persists the change.
func (s *Session) SetPublic(p bool) {
	s.mu.Lock()
	s.public = p
	s.mu.Unlock()
	if s.mgr != nil {
		s.mgr.persistPublic(s.ID, p)
	}
}

// LastActiveAt returns the last touch timestamp (thread-safe).
func (s *Session) LastActiveAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastActiveAt
}

// Touch updates lastActiveAt to now. Stays in-memory; flushed to disk by the
// cleanup pass to avoid an I/O per WS command.
func (s *Session) Touch() {
	s.mu.Lock()
	s.lastActiveAt = time.Now()
	s.mu.Unlock()
}

// Config holds the Manager construction parameters.
type Config struct {
	Demos         []sim.Demo
	Logger        *slog.Logger
	Store         *store.Store  // optional; when nil the Manager is in-memory only
	MaxPublic     int           // default 10
	MaxPrivileged int           // default 5
	IdleTTL       time.Duration // default 30 min
	InviteTTL     time.Duration // default 24 h
}

// Invite is a single-use token that allows the creation of a ModePrivileged
// Session (which bypasses the public cap). Created by the admin from the
// dashboard and shared with the guest as a URL `/?t=<token>`.
type Invite struct {
	Token     string
	Label     string // pre-label for the session that will be created
	CreatedAt time.Time
	UsedAt    *time.Time // nil while still pending
}

// Manager keeps all active sessions indexed by ID and ViewToken.
type Manager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	byViewToken map[string]*Session

	// invites is only populated when store == nil; otherwise invite state
	// lives in the database.
	invites map[string]*Invite

	store         *store.Store
	demos         []sim.Demo
	logger        *slog.Logger
	maxPublic     int
	maxPrivileged int
	idleTTL       time.Duration
	inviteTTL     time.Duration

	// Metrics — read by the admin dashboard.
	metricsMu      sync.Mutex
	totalCreated   uint64 // sessions created since startup
	peakSimultaneo int    // max concurrent sessions since startup
}

// NewManager builds an empty Manager. When Config.Store is set, persisted
// sessions are rehydrated into the in-memory map with fresh Simulators.
func NewManager(cfg Config) *Manager {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxPublic <= 0 {
		cfg.MaxPublic = 10
	}
	if cfg.MaxPrivileged <= 0 {
		cfg.MaxPrivileged = 5
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 30 * time.Minute
	}
	if cfg.InviteTTL <= 0 {
		cfg.InviteTTL = 24 * time.Hour
	}
	m := &Manager{
		sessions:      make(map[string]*Session),
		byViewToken:   make(map[string]*Session),
		invites:       make(map[string]*Invite),
		store:         cfg.Store,
		demos:         cfg.Demos,
		logger:        cfg.Logger,
		maxPublic:     cfg.MaxPublic,
		maxPrivileged: cfg.MaxPrivileged,
		idleTTL:       cfg.IdleTTL,
		inviteTTL:     cfg.InviteTTL,
	}
	if m.store != nil {
		m.hydrateFromStore()
	}
	return m
}

// hydrateFromStore loads every persisted session row into memory with a
// fresh Simulator. Called once from NewManager. Sessions whose
// last_active_at predates idleTTL are dropped instead of resurrected; this
// keeps the cap clean across restarts.
func (m *Manager) hydrateFromStore() {
	ctx := context.Background()
	rows, err := m.store.ListSessions(ctx)
	if err != nil {
		m.logger.Warn("store: hydrate sessions failed", "err", err)
		return
	}
	threshold := time.Now().Add(-m.idleTTL).Unix()
	for i := range rows {
		row := rows[i]
		if row.LastActiveAt < threshold {
			if err := m.store.DeleteSession(ctx, row.ID); err != nil {
				m.logger.Warn("store: prune idle session at boot", "id", row.ID, "err", err)
			}
			continue
		}
		sess := m.newSessionFromRow(row)
		m.sessions[sess.ID] = sess
		if sess.viewToken != "" {
			m.byViewToken[sess.viewToken] = sess
		}
	}
	m.logger.Info("sessions hydrated", "count", len(m.sessions))
}

func (m *Manager) newSessionFromRow(row store.SessionRow) *Session {
	return &Session{
		ID:           row.ID,
		Sim:          sim.New(sim.Config{Demos: m.demos, Logger: m.logger}),
		Mode:         modeFromString(row.Mode),
		CreatedAt:    time.Unix(row.CreatedAt, 0),
		mgr:          m,
		viewToken:    row.ViewToken,
		label:        row.Label,
		public:       row.Public,
		lastActiveAt: time.Unix(row.LastActiveAt, 0),
	}
}

// MaxPublic returns the configured public-session cap.
func (m *Manager) MaxPublic() int { return m.maxPublic }

// MaxPrivileged returns the configured privileged-session cap.
func (m *Manager) MaxPrivileged() int { return m.maxPrivileged }

// IdleTTL returns the idle-session expiration window.
func (m *Manager) IdleTTL() time.Duration { return m.idleTTL }

// InviteTTL returns the invite expiration window.
func (m *Manager) InviteTTL() time.Duration { return m.inviteTTL }

// Create instantiates a new Session with its own Simulator.
func (m *Manager) Create(mode Mode) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	full, priv := m.countLocked()
	if mode == ModeFull && full >= m.maxPublic {
		return nil, ErrCapReached
	}
	if mode == ModePrivileged && priv >= m.maxPrivileged {
		return nil, ErrPrivCapReached
	}
	now := time.Now()
	sess := &Session{
		ID:           newID(),
		Sim:          sim.New(sim.Config{Demos: m.demos, Logger: m.logger}),
		Mode:         mode,
		CreatedAt:    now,
		mgr:          m,
		public:       true,
		lastActiveAt: now,
	}
	if m.store != nil {
		err := m.store.CreateSession(context.Background(), store.SessionRow{
			ID:           sess.ID,
			Mode:         mode.String(),
			Label:        "",
			Public:       true,
			CreatedAt:    now.Unix(),
			LastActiveAt: now.Unix(),
		})
		if err != nil {
			m.logger.Warn("store: persist session failed", "id", sess.ID, "err", err)
			return nil, err
		}
	}
	m.sessions[sess.ID] = sess
	current := len(m.sessions)
	m.metricsMu.Lock()
	m.totalCreated++
	if current > m.peakSimultaneo {
		m.peakSimultaneo = current
	}
	m.metricsMu.Unlock()
	m.logger.Info("session created", "id", sess.ID, "mode", mode)
	return sess, nil
}

// Metrics returns a snapshot of the basic counters surfaced to the admin.
func (m *Manager) Metrics() (totalCreated uint64, peak int) {
	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()
	return m.totalCreated, m.peakSimultaneo
}

// Get retrieves a Session by ID. Returns nil if it does not exist.
func (m *Manager) Get(id string) *Session {
	if id == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// GetByViewToken looks up a Session by its share token.
func (m *Manager) GetByViewToken(token string) *Session {
	if token == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byViewToken[token]
}

// ActivateShare generates a ViewToken for the session if it does not have
// one. Idempotent: calling it twice with the same session does not rotate
// the token. Returns the token.
func (m *Manager) ActivateShare(sess *Session) string {
	sess.mu.Lock()
	if sess.viewToken != "" {
		token := sess.viewToken
		sess.mu.Unlock()
		return token
	}
	token := newID()
	sess.viewToken = token
	sess.mu.Unlock()

	m.mu.Lock()
	m.byViewToken[token] = sess
	m.mu.Unlock()

	if m.store != nil {
		if err := m.store.UpdateSessionViewToken(context.Background(), sess.ID, token); err != nil {
			m.logger.Warn("store: persist view_token failed", "id", sess.ID, "err", err)
		}
	}
	m.logger.Info("share activated", "session", sess.ID, "token", token)
	return token
}

// DeactivateShare invalidates the ViewToken; already-connected viewers stay
// connected but cannot reconnect after they close.
func (m *Manager) DeactivateShare(sess *Session) {
	sess.mu.Lock()
	token := sess.viewToken
	sess.viewToken = ""
	sess.mu.Unlock()

	if token != "" {
		m.mu.Lock()
		delete(m.byViewToken, token)
		m.mu.Unlock()
		if m.store != nil {
			if err := m.store.UpdateSessionViewToken(context.Background(), sess.ID, ""); err != nil {
				m.logger.Warn("store: clear view_token failed", "id", sess.ID, "err", err)
			}
		}
		m.logger.Info("share deactivated", "session", sess.ID, "token", token)
	}
}

// Kill removes a Session and stops its Simulator.
func (m *Manager) Kill(id string) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, id)
	if t := sess.viewToken; t != "" {
		delete(m.byViewToken, t)
	}
	m.mu.Unlock()
	if sess.Sim != nil {
		sess.Sim.StopRun() // idempotent
	}
	if m.store != nil {
		if err := m.store.DeleteSession(context.Background(), id); err != nil {
			m.logger.Warn("store: delete session failed", "id", id, "err", err)
		}
	}
	m.logger.Info("session killed", "id", id)
}

// Count returns how many active sessions exist of each mode.
func (m *Manager) Count() (full, privileged int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.countLocked()
}

func (m *Manager) countLocked() (full, privileged int) {
	for _, s := range m.sessions {
		switch s.Mode {
		case ModeFull:
			full++
		case ModePrivileged:
			privileged++
		}
	}
	return
}

// All returns a snapshot of every session (for the admin dashboard).
func (m *Manager) All() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

// PublicShared returns the sessions that show up on /full: marked Public AND
// with share activated.
func (m *Manager) PublicShared() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0)
	for _, s := range m.sessions {
		if !s.Public() || s.ViewToken() == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// Programs returns the loadable demo list. Identical for every session.
func (m *Manager) Programs() []map[string]any {
	list := make([]map[string]any, 0, len(m.demos))
	for _, d := range m.demos {
		totalSize := 0
		procNames := make([]string, len(d.Procs))
		for i, p := range d.Procs {
			totalSize += len(p.Bytes)
			procNames[i] = p.Name
		}
		list = append(list, map[string]any{
			"name":        d.Name,
			"description": d.Description,
			"size":        totalSize,
			"procs":       procNames,
		})
	}
	return list
}

// persistLabel writes a label change to the store. No-op when store is nil.
func (m *Manager) persistLabel(id, label string) {
	if m.store == nil {
		return
	}
	if err := m.store.UpdateSessionLabel(context.Background(), id, label); err != nil {
		m.logger.Warn("store: update label failed", "id", id, "err", err)
	}
}

// persistPublic writes a public-flag change to the store. No-op when store is nil.
func (m *Manager) persistPublic(id string, public bool) {
	if m.store == nil {
		return
	}
	if err := m.store.UpdateSessionPublic(context.Background(), id, public); err != nil {
		m.logger.Warn("store: update public failed", "id", id, "err", err)
	}
}

// newID returns a hex-encoded 16-char (8-byte) random identifier.
func newID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// ErrInviteInvalid is the generic error returned when an invite token cannot
// be consumed (unknown, already used, or expired). The three causes are
// collapsed into one error so brute-force callers cannot distinguish them.
var ErrInviteInvalid = errors.New("invite token invalid or expired")

// CreateInvite generates a new Invite with the optional label. The returned
// token is the one embedded in the `/?t=<token>` link the admin shares.
func (m *Manager) CreateInvite(label string) *Invite {
	now := time.Now()
	inv := &Invite{
		Token:     newID(),
		Label:     label,
		CreatedAt: now,
	}
	if m.store != nil {
		err := m.store.CreateInvite(context.Background(), store.InviteRow{
			Token:     inv.Token,
			Label:     label,
			CreatedAt: now.Unix(),
			ExpiresAt: now.Add(m.inviteTTL).Unix(),
		})
		if err != nil {
			m.logger.Warn("store: persist invite failed", "token", inv.Token, "err", err)
		}
	} else {
		m.mu.Lock()
		m.invites[inv.Token] = inv
		m.mu.Unlock()
	}
	m.logger.Info("invite created", "token", inv.Token, "label", label)
	return inv
}

// ConsumeInvite validates and marks an invite as used, returning the
// associated label. Atomic: two concurrent calls with the same token will
// see only one success.
func (m *Manager) ConsumeInvite(token string) (string, error) {
	if m.store != nil {
		label, err := m.store.ConsumeInvite(context.Background(), token, time.Now().Unix())
		if errors.Is(err, store.ErrNotFound) {
			return "", ErrInviteInvalid
		}
		if err != nil {
			m.logger.Warn("store: consume invite failed", "token", token, "err", err)
			return "", ErrInviteInvalid
		}
		m.logger.Info("invite consumed", "token", token, "label", label)
		return label, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[token]
	if !ok || inv.UsedAt != nil {
		return "", ErrInviteInvalid
	}
	if time.Since(inv.CreatedAt) > m.inviteTTL {
		return "", ErrInviteInvalid
	}
	now := time.Now()
	inv.UsedAt = &now
	m.logger.Info("invite consumed", "token", token, "label", inv.Label)
	return inv.Label, nil
}

// ListInvites returns every PENDING invite (not used, not expired). Used by
// the admin dashboard.
func (m *Manager) ListInvites() []*Invite {
	if m.store != nil {
		rows, err := m.store.ListPendingInvites(context.Background(), time.Now().Unix())
		if err != nil {
			m.logger.Warn("store: list invites failed", "err", err)
			return nil
		}
		out := make([]*Invite, 0, len(rows))
		for _, r := range rows {
			out = append(out, &Invite{
				Token:     r.Token,
				Label:     r.Label,
				CreatedAt: time.Unix(r.CreatedAt, 0),
			})
		}
		return out
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Invite, 0, len(m.invites))
	now := time.Now()
	for _, inv := range m.invites {
		if inv.UsedAt != nil {
			continue
		}
		if now.Sub(inv.CreatedAt) > m.inviteTTL {
			continue
		}
		out = append(out, inv)
	}
	return out
}

// RevokeInvite removes an unused invite. Idempotent.
func (m *Manager) RevokeInvite(token string) {
	if m.store != nil {
		if err := m.store.RevokeInvite(context.Background(), token); err != nil {
			m.logger.Warn("store: revoke invite failed", "token", token, "err", err)
		}
	} else {
		m.mu.Lock()
		delete(m.invites, token)
		m.mu.Unlock()
	}
	m.logger.Info("invite revoked", "token", token)
}

// CleanupOnce performs one cleanup pass: kills sessions whose LastActiveAt
// is older than IdleTTL, drops invites that are used or expired, and
// flushes Touch timestamps from memory to disk. Returns how many sessions
// were killed and how many invites pruned.
func (m *Manager) CleanupOnce() (killedSessions, prunedInvites int) {
	now := time.Now()
	ctx := context.Background()

	// Identify idle sessions under a short read lock, then Kill outside the
	// lock so each Simulator's StopRun does not block the map.
	m.mu.RLock()
	deadIDs := make([]string, 0)
	type liveTouch struct {
		id string
		ts int64
	}
	live := make([]liveTouch, 0, len(m.sessions))
	for id, sess := range m.sessions {
		last := sess.LastActiveAt()
		if now.Sub(last) > m.idleTTL {
			deadIDs = append(deadIDs, id)
			continue
		}
		live = append(live, liveTouch{id: id, ts: last.Unix()})
	}
	m.mu.RUnlock()
	for _, id := range deadIDs {
		m.Kill(id)
		killedSessions++
	}

	if m.store != nil {
		// Flush in-memory Touch timestamps for surviving sessions.
		for _, lt := range live {
			if err := m.store.UpdateSessionLastActive(ctx, lt.id, lt.ts); err != nil {
				m.logger.Warn("store: flush last_active failed", "id", lt.id, "err", err)
			}
		}
		n, err := m.store.DeleteExpiredInvites(ctx, now.Unix())
		if err != nil {
			m.logger.Warn("store: prune invites failed", "err", err)
		} else {
			prunedInvites += n
		}
	} else {
		m.mu.Lock()
		for token, inv := range m.invites {
			if inv.UsedAt != nil || now.Sub(inv.CreatedAt) > m.inviteTTL {
				delete(m.invites, token)
				prunedInvites++
			}
		}
		m.mu.Unlock()
	}

	if killedSessions > 0 || prunedInvites > 0 {
		m.logger.Info("cleanup pass", "killed_sessions", killedSessions, "pruned_invites", prunedInvites)
	}
	return
}

// StartCleanup launches a goroutine that calls CleanupOnce every interval
// until ctx is canceled. Returns immediately.
func (m *Manager) StartCleanup(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.CleanupOnce()
			}
		}
	}()
}
