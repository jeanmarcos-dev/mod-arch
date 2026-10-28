package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	"github.com/jeanmarcos-dev/mod-arch/internal/sim"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

func sessionCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie {
			return c
		}
	}
	t.Fatalf("response did not set %s", SessionCookie)
	return nil
}

func TestSessionCookieCarriesOwnerTokenNotPublicID(t *testing.T) {
	f := newLiveFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)

	body := f.decode(t, rec)
	id, _ := body["id"].(string)
	cookie := sessionCookieFrom(t, rec)
	if cookie.Value == "" || cookie.Value == id {
		t.Fatalf("cookie value %q must be a secret distinct from the public id %q", cookie.Value, id)
	}
	if !cookie.Secure {
		t.Error("session cookie must be Secure on HTTPS requests")
	}
	if got := f.mgr.GetByOwnerToken(cookie.Value); got == nil || got.ID != id {
		t.Errorf("cookie does not resolve to the session it was issued for")
	}
}

func TestPublicSessionIDDoesNotAuthenticate(t *testing.T) {
	f := newLiveFixture(t)
	victim, err := f.mgr.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	body := f.decode(t, f.get("/api/session", &http.Cookie{Name: SessionCookie, Value: victim.ID}))
	if body["id"] == victim.ID {
		t.Fatal("presenting the public session id as cookie must not grant ownership")
	}
	owner := f.decode(t, f.get("/api/session", &http.Cookie{Name: SessionCookie, Value: victim.OwnerToken()}))
	if owner["id"] != victim.ID || owner["role"] != "owner" {
		t.Errorf("owner token must resolve the session as owner, got %v", owner)
	}
}

func TestSessionInfoForViewerHidesOwnerIdentity(t *testing.T) {
	f := newLiveFixture(t)
	if err := f.st.CreateUser(context.Background(), store.UserRow{ID: "u1", Name: "Ada", Email: "ada@example.edu", CreatedAt: 1, LastLoginAt: 1}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sess, err := f.mgr.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess.SetUserID("u1")

	viewer := f.s.sessionInfoMessage(sess, sessions.RoleViewer)
	if viewer["user_email"] != "" || viewer["user_id"] != "" {
		t.Errorf("viewer payload leaks owner identity: email=%v id=%v", viewer["user_email"], viewer["user_id"])
	}
	if viewer["user_name"] != "Ada" {
		t.Errorf("viewer user_name = %v, want presenter name for the whiteboard label", viewer["user_name"])
	}
	owner := f.s.sessionInfoMessage(sess, sessions.RoleOwner)
	if owner["user_email"] != "ada@example.edu" || owner["user_id"] != "u1" {
		t.Errorf("owner payload must keep its own identity, got %v", owner)
	}
}

func TestBroadcastSessionInfoTargetsEachRole(t *testing.T) {
	f := newLiveFixture(t)
	sess, err := f.mgr.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ownerClient := sim.NewClient(sim.RoleOwner)
	viewerClient := sim.NewClient(sim.RoleViewer)
	sess.Sim.AddClient(ownerClient)
	sess.Sim.AddClient(viewerClient)

	f.s.broadcastSessionInfo(sess)

	for _, tc := range []struct {
		client *sim.Client
		role   string
	}{{ownerClient, "owner"}, {viewerClient, "viewer"}} {
		var got map[string]any
		for got == nil {
			select {
			case msg := <-tc.client.Send:
				if m, ok := msg.(map[string]any); ok && m["type"] == "session_info" {
					got = m
				}
			default:
				t.Fatalf("%s client received no session_info", tc.role)
			}
		}
		if got["role"] != tc.role {
			t.Errorf("%s client got role %v", tc.role, got["role"])
		}
	}
}
