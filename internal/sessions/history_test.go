package sessions

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// TestTerminate_WritesHistory exercises the core E4 contract: every session
// that ends — whether by admin Kill, by idle cleanup, or by login replacement
// — should leave one row in session_history with the correct end_reason and
// aggregated counters.
func TestTerminate_WritesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	m := NewManager(Config{Store: st, IdleTTL: 100 * time.Millisecond})

	sess, err := m.CreateWithClient(ModeFull, ClientInfo{
		IPHash:     "h1",
		UAShort:    "Firefox 122 / Linux",
		GeoCountry: "VE",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Simulate user activity for counter persistence.
	sess.IncCommandCounter()
	sess.IncCommandCounter()
	sess.IncTutorCounter()

	m.Kill(sess.ID)

	rows, total, err := st.ListSessionHistory(t.Context(), store.SessionHistoryFilter{})
	if err != nil {
		t.Fatalf("ListSessionHistory: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 history row, got %d", total)
	}
	r := rows[0]
	if r.OriginalSessionID != sess.ID {
		t.Errorf("OriginalSessionID = %q, want %q", r.OriginalSessionID, sess.ID)
	}
	if r.EndReason != store.EndReasonAdminKill {
		t.Errorf("EndReason = %q, want admin_kill", r.EndReason)
	}
	if r.ClientIPHash != "h1" || r.UAShort != "Firefox 122 / Linux" || r.GeoCountry != "VE" {
		t.Errorf("tracking metadata lost: %+v", r)
	}
	if r.TotalCommands != 2 || r.TotalTutorQueries != 1 {
		t.Errorf("counters: cmds=%d tutor=%d, want 2/1", r.TotalCommands, r.TotalTutorQueries)
	}

	if got := m.Get(sess.ID); got != nil {
		t.Error("session should be gone from live map after Kill")
	}
}

// TestTerminate_IdleReason verifies the cleanup pass tags the row as 'idle'.
func TestTerminate_IdleReason(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	m := NewManager(Config{Store: st, IdleTTL: 1 * time.Millisecond})
	sess, err := m.Create(ModeFull)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Push lastActiveAt into the past so CleanupOnce sees an idle session.
	sess.mu.Lock()
	sess.lastActiveAt = time.Now().Add(-1 * time.Hour)
	sess.mu.Unlock()

	killed, _ := m.CleanupOnce()
	if killed != 1 {
		t.Fatalf("CleanupOnce killed = %d, want 1", killed)
	}

	rows, _, _ := st.ListSessionHistory(t.Context(), store.SessionHistoryFilter{})
	if len(rows) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(rows))
	}
	if rows[0].EndReason != store.EndReasonIdle {
		t.Errorf("EndReason = %q, want idle", rows[0].EndReason)
	}
}

// TestTerminate_ReplacedReason: the one-session-per-user policy should label
// dropped sessions as 'replaced', not 'admin_kill'.
func TestTerminate_ReplacedReason(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := st.CreateUser(t.Context(), store.UserRow{
		ID: "u1", Name: "alice", Role: store.RoleStudent,
		CreatedAt: 1, LastLoginAt: 1,
	}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	m := NewManager(Config{Store: st, IdleTTL: time.Hour})
	a, _ := m.Create(ModeFull)
	a.SetUserID("u1")
	b, _ := m.Create(ModeFull)
	b.SetUserID("u1")

	victims := m.KillOtherUserSessions("u1", b.ID)
	if len(victims) != 1 || victims[0] != a.ID {
		t.Fatalf("victims = %v, want [%s]", victims, a.ID)
	}

	rows, _, _ := st.ListSessionHistory(t.Context(), store.SessionHistoryFilter{})
	if len(rows) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(rows))
	}
	if rows[0].EndReason != store.EndReasonReplaced {
		t.Errorf("EndReason = %q, want replaced", rows[0].EndReason)
	}
}
