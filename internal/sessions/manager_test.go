package sessions

import (
	"errors"
	"testing"
	"time"
)

func TestManager_CreatePublicHonorsCap(t *testing.T) {
	m := NewManager(Config{MaxPublic: 2})

	a, err := m.Create(ModeFull)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}
	if a.ID == "" {
		t.Error("session without ID")
	}

	b, err := m.Create(ModeFull)
	if err != nil {
		t.Fatalf("second Create failed: %v", err)
	}
	if b.ID == a.ID {
		t.Error("two sessions sharing the same ID")
	}

	if _, err := m.Create(ModeFull); !errors.Is(err, ErrCapReached) {
		t.Fatalf("expected ErrCapReached beyond cap, got %v", err)
	}
}

func TestManager_PrivilegedDoesNotCountTowardPublicCap(t *testing.T) {
	m := NewManager(Config{MaxPublic: 1, MaxPrivileged: 2})

	if _, err := m.Create(ModeFull); err != nil {
		t.Fatalf("public Create: %v", err)
	}

	if _, err := m.Create(ModeFull); !errors.Is(err, ErrCapReached) {
		t.Fatalf("second public should fail, got %v", err)
	}

	priv, err := m.Create(ModePrivileged)
	if err != nil {
		t.Fatalf("privileged should succeed despite full public cap: %v", err)
	}
	if priv.Mode != ModePrivileged {
		t.Errorf("Mode = %v, want privileged", priv.Mode)
	}
}

func TestManager_KillFreesSlot(t *testing.T) {
	m := NewManager(Config{MaxPublic: 1})

	a, _ := m.Create(ModeFull)
	if _, err := m.Create(ModeFull); !errors.Is(err, ErrCapReached) {
		t.Fatal("should be full before kill")
	}

	m.Kill(a.ID)

	if _, err := m.Create(ModeFull); err != nil {
		t.Fatalf("after kill a slot should be free: %v", err)
	}
}

func TestManager_ShareGenerateAndLookup(t *testing.T) {
	m := NewManager(Config{MaxPublic: 5})

	sess, _ := m.Create(ModeFull)
	if sess.ViewToken() != "" {
		t.Error("fresh session should not have a viewToken")
	}

	token := m.ActivateShare(sess)
	if token == "" {
		t.Fatal("ActivateShare returned empty token")
	}
	if sess.ViewToken() != token {
		t.Errorf("Session.ViewToken=%q, want %q", sess.ViewToken(), token)
	}

	got := m.GetByViewToken(token)
	if got == nil || got.ID != sess.ID {
		t.Fatal("GetByViewToken did not resolve to the original session")
	}

	// Idempotent: calling again should not rotate the token.
	token2 := m.ActivateShare(sess)
	if token2 != token {
		t.Errorf("ActivateShare not idempotent: %q vs %q", token, token2)
	}
}

func TestManager_DeactivateShareInvalidates(t *testing.T) {
	m := NewManager(Config{MaxPublic: 5})

	sess, _ := m.Create(ModeFull)
	token := m.ActivateShare(sess)

	m.DeactivateShare(sess)

	if sess.ViewToken() != "" {
		t.Error("viewToken should be empty after DeactivateShare")
	}
	if got := m.GetByViewToken(token); got != nil {
		t.Error("GetByViewToken should not resolve after DeactivateShare")
	}
}

func TestManager_DeactivateShareDisconnectsViewers(t *testing.T) {
	m := NewManager(Config{MaxPublic: 5})
	sess, _ := m.Create(ModeFull)
	m.ActivateShare(sess)

	var fired int
	idA := sess.AddViewer(func() { fired++ })
	idB := sess.AddViewer(func() { fired++ })

	m.DeactivateShare(sess)
	if fired != 2 {
		t.Errorf("cancel fired %d times, want 2", fired)
	}

	// Late RemoveViewer must not panic even when the registry was already
	// cleared by disconnectAllViewers.
	sess.RemoveViewer(idA)
	sess.RemoveViewer(idB)
}

func TestInvite_CreateConsume(t *testing.T) {
	m := NewManager(Config{MaxPublic: 1, MaxPrivileged: 5})

	inv := m.CreateInvite("Prof. Carlos")
	if inv.Token == "" {
		t.Fatal("invite without token")
	}
	if inv.Label != "Prof. Carlos" {
		t.Errorf("label = %q, want %q", inv.Label, "Prof. Carlos")
	}

	label, err := m.ConsumeInvite(inv.Token)
	if err != nil {
		t.Fatalf("ConsumeInvite failed: %v", err)
	}
	if label != "Prof. Carlos" {
		t.Errorf("consume returned label %q, want %q", label, "Prof. Carlos")
	}
}

func TestInvite_DoubleUseRejected(t *testing.T) {
	m := NewManager(Config{})
	inv := m.CreateInvite("foo")

	if _, err := m.ConsumeInvite(inv.Token); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := m.ConsumeInvite(inv.Token); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("second consume should fail, got %v", err)
	}
}

func TestInvite_UnknownTokenRejected(t *testing.T) {
	m := NewManager(Config{})
	if _, err := m.ConsumeInvite("token-does-not-exist"); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("unknown token should fail, got %v", err)
	}
}

func TestInvite_ExpiredRejected(t *testing.T) {
	m := NewManager(Config{InviteTTL: 10 * time.Millisecond})
	inv := m.CreateInvite("foo")

	time.Sleep(20 * time.Millisecond)
	if _, err := m.ConsumeInvite(inv.Token); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("expired invite should fail, got %v", err)
	}
}

func TestInvite_ListPendingFiltersUsedAndExpired(t *testing.T) {
	m := NewManager(Config{InviteTTL: 10 * time.Millisecond})

	a := m.CreateInvite("a")
	b := m.CreateInvite("b")
	c := m.CreateInvite("c")

	// b is already used.
	_, _ = m.ConsumeInvite(b.Token)
	// a will be expired by the sleep below.
	_ = a
	time.Sleep(20 * time.Millisecond)

	d := m.CreateInvite("d") // fresh, still valid

	list := m.ListInvites()
	if len(list) != 1 {
		t.Fatalf("expected 1 pending invite, got %d (%v)", len(list), labels(list))
	}
	if list[0].Token != d.Token {
		t.Errorf("pending invite = %q, want %q", list[0].Token, d.Token)
	}
	_ = c
}

func TestInvite_RevokeDropsPending(t *testing.T) {
	m := NewManager(Config{})
	inv := m.CreateInvite("foo")

	m.RevokeInvite(inv.Token)

	if _, err := m.ConsumeInvite(inv.Token); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("revoked invite should fail, got %v", err)
	}
}

func TestInvite_PrivilegedDoesNotConsumePublicCap(t *testing.T) {
	// Real use-case: the public cap is full but a privileged invite still
	// allows a session to be created.
	m := NewManager(Config{MaxPublic: 1, MaxPrivileged: 5})

	_, _ = m.Create(ModeFull)
	if _, err := m.Create(ModeFull); !errors.Is(err, ErrCapReached) {
		t.Fatal("setup: public cap should already be full")
	}

	inv := m.CreateInvite("Prof.")
	if _, err := m.ConsumeInvite(inv.Token); err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	sess, err := m.Create(ModePrivileged)
	if err != nil {
		t.Fatalf("Create privileged failed despite full public cap: %v", err)
	}
	if sess.Mode != ModePrivileged {
		t.Errorf("Mode = %v, want privileged", sess.Mode)
	}
}

// labels returns the labels of an invite list, for debug output.
func labels(list []*Invite) []string {
	out := make([]string, len(list))
	for i, inv := range list {
		out[i] = inv.Label
	}
	return out
}

func TestCleanupOnce_KillsIdleSessions(t *testing.T) {
	m := NewManager(Config{IdleTTL: 30 * time.Millisecond})

	a, _ := m.Create(ModeFull)
	b, _ := m.Create(ModeFull)
	_ = b

	// Wait until both turn idle.
	time.Sleep(50 * time.Millisecond)

	// Keep b alive by touching it; a stays dead.
	b.Touch()

	killed, _ := m.CleanupOnce()
	if killed != 1 {
		t.Errorf("killed=%d, want 1", killed)
	}
	if m.Get(a.ID) != nil {
		t.Error("a should have been killed")
	}
	if m.Get(b.ID) == nil {
		t.Error("b should NOT have been killed (it was Touched before cleanup)")
	}
}

func TestCleanupOnce_PrunesUsedAndExpiredInvites(t *testing.T) {
	m := NewManager(Config{InviteTTL: 20 * time.Millisecond})

	used := m.CreateInvite("used")
	pending := m.CreateInvite("pending")
	_, _ = m.ConsumeInvite(used.Token)

	time.Sleep(40 * time.Millisecond)

	// The consumed one must be pruned, the pending one too (expired).
	_, pruned := m.CleanupOnce()
	if pruned < 2 {
		t.Errorf("pruned=%d, want >= 2", pruned)
	}
	_ = pending
}

func TestMetrics_TracksCreatedAndPeak(t *testing.T) {
	m := NewManager(Config{MaxPublic: 10})

	a, _ := m.Create(ModeFull)
	b, _ := m.Create(ModeFull)
	c, _ := m.Create(ModeFull)
	_ = a
	_ = c

	total, peak := m.Metrics()
	if total != 3 {
		t.Errorf("total=%d, want 3", total)
	}
	if peak != 3 {
		t.Errorf("peak=%d, want 3", peak)
	}

	// Killing one must NOT lower the peak (it is historical) or the total.
	m.Kill(b.ID)
	total, peak = m.Metrics()
	if total != 3 || peak != 3 {
		t.Errorf("after kill: total=%d peak=%d, want 3/3", total, peak)
	}
}

func TestManager_PublicSharedFilters(t *testing.T) {
	m := NewManager(Config{MaxPublic: 5})

	// No share — should not appear.
	_, _ = m.Create(ModeFull)
	if got := m.PublicShared(); len(got) != 0 {
		t.Errorf("without share should not appear, got %d", len(got))
	}

	// Shared but marked Private — should not appear.
	b, _ := m.Create(ModeFull)
	m.ActivateShare(b)
	b.SetPublic(false)
	if got := m.PublicShared(); len(got) != 0 {
		t.Errorf("private should not appear, got %d", len(got))
	}

	// Shared AND Public — should appear.
	c, _ := m.Create(ModeFull)
	m.ActivateShare(c)
	got := m.PublicShared()
	if len(got) != 1 || got[0].ID != c.ID {
		t.Errorf("PublicShared = %v, want [c]", got)
	}
}

func TestSession_PresenceRoundTrip(t *testing.T) {
	m := NewManager(Config{MaxPublic: 1})
	sess, err := m.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got := sess.Presence(); got.Section != "" || len(got.Cores) != 0 {
		t.Errorf("initial Presence should be zero, got %+v", got)
	}

	want := Presence{Section: "/memoria", Cores: []int{0, 2}, Mode: "pedagogic", Narrators: true}
	sess.SetPresence(want)
	got := sess.Presence()
	if got.Section != "/memoria" {
		t.Errorf("Section = %q, want /memoria", got.Section)
	}
	if len(got.Cores) != 2 || got.Cores[0] != 0 || got.Cores[1] != 2 {
		t.Errorf("Cores = %v, want [0 2]", got.Cores)
	}
	if got.Mode != "pedagogic" {
		t.Errorf("Mode = %q, want pedagogic", got.Mode)
	}
	if !got.Narrators {
		t.Error("Narrators should be true")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("SetPresence should stamp UpdatedAt when caller omits it")
	}

	// Mutating the returned slice must not affect future reads.
	got.Cores[0] = 99
	again := sess.Presence()
	if again.Cores[0] != 0 {
		t.Errorf("Presence leaked internal slice; Cores[0] = %d, want 0", again.Cores[0])
	}
}

func TestSession_PresenceKeepsExplicitTimestamp(t *testing.T) {
	m := NewManager(Config{MaxPublic: 1})
	sess, _ := m.Create(ModeFull)
	stamp := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	sess.SetPresence(Presence{Section: "/procesos", UpdatedAt: stamp})
	if got := sess.Presence().UpdatedAt; !got.Equal(stamp) {
		t.Errorf("UpdatedAt = %v, want %v", got, stamp)
	}
}

func TestManager_KillOtherUserSessions(t *testing.T) {
	m := NewManager(Config{MaxPublic: 10})

	// Two sessions belong to user A; one to user B; one anonymous. After
	// enforcement of A → keep=a2, only a2, b1 and anon survive. The policy
	// must never touch sessions belonging to another user or anonymous ones.
	a1, _ := m.Create(ModeFull)
	a2, _ := m.Create(ModeFull)
	b1, _ := m.Create(ModeFull)
	anon, _ := m.Create(ModeFull)
	a1.SetUserID("user-A")
	a2.SetUserID("user-A")
	b1.SetUserID("user-B")

	killed := m.KillOtherUserSessions("user-A", a2.ID)
	if len(killed) != 1 || killed[0] != a1.ID {
		t.Fatalf("killed = %v, want [%s]", killed, a1.ID)
	}
	if m.Get(a1.ID) != nil {
		t.Error("a1 should have been killed")
	}
	if m.Get(a2.ID) == nil {
		t.Error("a2 (keepID) must survive")
	}
	if m.Get(b1.ID) == nil {
		t.Error("b1 belongs to another user — must survive")
	}
	if m.Get(anon.ID) == nil {
		t.Error("anonymous session must survive — policy applies to logged users only")
	}
}

func TestManager_KillOtherUserSessions_NoOpOnAnonymous(t *testing.T) {
	m := NewManager(Config{MaxPublic: 4})
	a, _ := m.Create(ModeFull)
	b, _ := m.Create(ModeFull)
	if killed := m.KillOtherUserSessions("", a.ID); len(killed) != 0 {
		t.Errorf("anonymous enforcement should be a no-op, killed=%v", killed)
	}
	if m.Get(a.ID) == nil || m.Get(b.ID) == nil {
		t.Error("no session should have been touched")
	}
}
