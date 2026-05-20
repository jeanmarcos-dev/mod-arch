package sessions

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// TestPersist_SessionsSurviveRestart is the load-bearing test of A1.2: when a
// Store is provided, a Manager that is rebuilt against the same database
// rehydrates every previously-active session with its label, public flag and
// view_token intact. The *sim.Simulator itself is volatile by design and is
// rebuilt fresh.
func TestPersist_SessionsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	st1, err := store.Open(path)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	m1 := NewManager(Config{Store: st1})
	a, err := m1.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create a: %v", err)
	}
	a.SetLabel("equipo-uno")
	a.SetPublic(false)
	tokenA := m1.ActivateShare(a)
	if tokenA == "" {
		t.Fatal("ActivateShare returned empty")
	}

	b, err := m1.Create(ModePrivileged)
	if err != nil {
		t.Fatalf("Create b: %v", err)
	}
	b.SetLabel("invitado-prof")

	if err := st1.Close(); err != nil {
		t.Fatalf("close #1: %v", err)
	}

	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("open #2: %v", err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	m2 := NewManager(Config{Store: st2})

	got := m2.Get(a.ID)
	if got == nil {
		t.Fatalf("session %s lost across restart", a.ID)
	}
	if got.Label() != "equipo-uno" {
		t.Errorf("Label=%q, want %q", got.Label(), "equipo-uno")
	}
	if got.Public() {
		t.Error("Public should remain false after restart")
	}
	if got.ViewToken() != tokenA {
		t.Errorf("ViewToken=%q, want %q", got.ViewToken(), tokenA)
	}
	if got.Mode != ModeFull {
		t.Errorf("Mode=%v, want ModeFull", got.Mode)
	}
	if got.Sim == nil {
		t.Error("Sim should be rehydrated (fresh instance) on restart")
	}
	if resolved := m2.GetByViewToken(tokenA); resolved == nil || resolved.ID != a.ID {
		t.Errorf("GetByViewToken did not resolve session a after restart")
	}

	gotB := m2.Get(b.ID)
	if gotB == nil || gotB.Mode != ModePrivileged || gotB.Label() != "invitado-prof" {
		t.Errorf("privileged session lost or mutated across restart: %+v", gotB)
	}
}

func TestPersist_KillRemovesFromStore(t *testing.T) {
	st := openFileStore(t)
	m1 := NewManager(Config{Store: st})

	sess, err := m1.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	m1.Kill(sess.ID)

	m2 := NewManager(Config{Store: st})
	if m2.Get(sess.ID) != nil {
		t.Error("killed session must not resurrect on restart")
	}
}

func TestPersist_DeactivateShareClearsToken(t *testing.T) {
	st := openFileStore(t)
	m1 := NewManager(Config{Store: st})

	sess, err := m1.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token := m1.ActivateShare(sess)
	m1.DeactivateShare(sess)

	m2 := NewManager(Config{Store: st})
	got := m2.Get(sess.ID)
	if got == nil {
		t.Fatal("session lost across restart")
	}
	if got.ViewToken() != "" {
		t.Errorf("ViewToken=%q, want empty after Deactivate+restart", got.ViewToken())
	}
	if m2.GetByViewToken(token) != nil {
		t.Error("deactivated token should not resolve after restart")
	}
}

func TestPersist_HydrationPrunesIdleSessions(t *testing.T) {
	st := openFileStore(t)

	// Insert a row whose last_active is older than the idle TTL the
	// Manager will use. Hydration must drop it instead of resurrecting.
	oldTs := time.Now().Add(-2 * time.Hour).Unix()
	row := store.SessionRow{
		ID:           "ghost",
		Mode:         "full",
		CreatedAt:    oldTs,
		LastActiveAt: oldTs,
	}
	if err := st.CreateSession(t.Context(), row); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m := NewManager(Config{Store: st, IdleTTL: time.Hour})
	if m.Get("ghost") != nil {
		t.Error("idle session should be pruned at hydration")
	}
	if _, err := st.GetSession(t.Context(), "ghost"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ghost should also be gone from DB, got err=%v", err)
	}
}

func TestPersist_InvitesSurviveAndConsumeAtomic(t *testing.T) {
	st := openFileStore(t)
	m1 := NewManager(Config{Store: st})

	inv := m1.CreateInvite("Prof. Carlos")

	m2 := NewManager(Config{Store: st})
	label, err := m2.ConsumeInvite(inv.Token)
	if err != nil {
		t.Fatalf("ConsumeInvite across restart: %v", err)
	}
	if label != "Prof. Carlos" {
		t.Errorf("label=%q, want %q", label, "Prof. Carlos")
	}

	if _, err := m2.ConsumeInvite(inv.Token); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("second consume must fail with ErrInviteInvalid, got %v", err)
	}
}

func TestPersist_CleanupFlushesLastActive(t *testing.T) {
	st := openFileStore(t)
	m := NewManager(Config{Store: st, IdleTTL: time.Hour})

	sess, err := m.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess.Touch()
	before, err := st.GetSession(t.Context(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond) // unix-seconds resolution
	sess.Touch()
	m.CleanupOnce()

	after, err := st.GetSession(t.Context(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastActiveAt <= before.LastActiveAt {
		t.Errorf("LastActiveAt not flushed: before=%d after=%d", before.LastActiveAt, after.LastActiveAt)
	}
}

func openFileStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
