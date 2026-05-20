package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAccess_ClosedBlocksNewSessions(t *testing.T) {
	ts, st := startAuthTestServer(t)
	if err := st.SetSetting(t.Context(), SettingAccessMode, AccessClosed, "test", 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c := newJarClient()
	resp, err := c.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ACCESO RESTRINGIDO") {
		t.Errorf("body missing closed page marker")
	}
}

func TestAccess_InviteOnlyBlocksWithoutToken(t *testing.T) {
	ts, st := startAuthTestServer(t)
	_ = st.SetSetting(t.Context(), SettingAccessMode, AccessInviteOnly, "test", 0)
	c := newJarClient()
	resp, _ := c.Get(ts.URL + "/")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("invite_only status = %d, want 503", resp.StatusCode)
	}
}

func TestAccess_ExistingSessionSurvivesPolicyChange(t *testing.T) {
	ts, st := startAuthTestServer(t)
	c := newJarClient()

	// First request creates the session under access_mode=open (default).
	resp, _ := c.Get(ts.URL + "/")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initial GET = %d", resp.StatusCode)
	}

	// Tighten policy. The existing cookie must keep working.
	_ = st.SetSetting(t.Context(), SettingAccessMode, AccessClosed, "test", 0)
	resp, _ = c.Get(ts.URL + "/")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("returning client = %d, want 200 (cookie sticks)", resp.StatusCode)
	}

	// A fresh client (no cookie) is blocked.
	c2 := newJarClient()
	resp, _ = c2.Get(ts.URL + "/")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("fresh client = %d, want 503", resp.StatusCode)
	}
}
