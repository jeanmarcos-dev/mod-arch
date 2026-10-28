package sessions

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

var hex128 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestOwnerToken_IsSecretAndDistinctFromID(t *testing.T) {
	m := NewManager(Config{})
	sess, err := m.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token := sess.OwnerToken()
	if !hex128.MatchString(token) {
		t.Fatalf("OwnerToken = %q, want 128-bit hex", token)
	}
	if token == sess.ID {
		t.Fatal("OwnerToken must differ from the public session ID")
	}
	if got := m.GetByOwnerToken(token); got != sess {
		t.Errorf("GetByOwnerToken(token) = %v, want the created session", got)
	}
	if got := m.GetByOwnerToken(sess.ID); got != nil {
		t.Error("the public session ID must not authenticate as owner")
	}
	if got := m.GetByOwnerToken(""); got != nil {
		t.Error("empty token must not resolve a session")
	}
}

func TestOwnerToken_ForgottenOnTerminate(t *testing.T) {
	m := NewManager(Config{})
	sess, err := m.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token := sess.OwnerToken()
	m.Kill(sess.ID)
	if m.GetByOwnerToken(token) != nil {
		t.Error("owner token must stop resolving after the session ends")
	}
}

func TestOwnerToken_SurvivesRestartAsHashOnly(t *testing.T) {
	st := openFileStore(t)
	m1 := NewManager(Config{Store: st})
	sess, err := m1.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token := sess.OwnerToken()

	row, err := st.GetSession(t.Context(), sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if row.OwnerTokenHash == "" || row.OwnerTokenHash == token {
		t.Fatalf("stored OwnerTokenHash = %q, want a non-empty hash of the token", row.OwnerTokenHash)
	}

	m2 := NewManager(Config{Store: st})
	got := m2.GetByOwnerToken(token)
	if got == nil || got.ID != sess.ID {
		t.Fatalf("GetByOwnerToken after restart = %v, want session %s", got, sess.ID)
	}
	if got.OwnerToken() != "" {
		t.Error("a rehydrated session must not hold the raw owner token")
	}
}

func TestOwnerToken_HydrationDropsRowsWithoutHash(t *testing.T) {
	st := openFileStore(t)
	now := time.Now().Unix()
	if err := st.CreateSession(t.Context(), store.SessionRow{
		ID:           "legacy",
		Mode:         "full",
		CreatedAt:    now,
		LastActiveAt: now,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m := NewManager(Config{Store: st})
	if m.Get("legacy") != nil {
		t.Error("a session without owner token hash cannot be resumed and must not be hydrated")
	}
	if _, err := st.GetSession(t.Context(), "legacy"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("legacy row should be deleted, got err=%v", err)
	}
}

func TestShareAndInviteTokensAre128Bit(t *testing.T) {
	m := NewManager(Config{})
	sess, err := m.Create(ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if v := m.ActivateShare(sess); !hex128.MatchString(v) {
		t.Errorf("view token = %q, want 128-bit hex", v)
	}
	if inv := m.CreateInvite("x"); !hex128.MatchString(inv.Token) {
		t.Errorf("invite token = %q, want 128-bit hex", inv.Token)
	}
}
