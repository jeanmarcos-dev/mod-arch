package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

func seedUser(t *testing.T, st *store.Store, u store.UserRow) {
	t.Helper()
	u.CreatedAt, u.LastLoginAt = 1, 1
	if err := st.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
}

func assertVerificationRequired(t *testing.T, status int, body []byte) {
	t.Helper()
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", status, body)
	}
	var got struct {
		Code string `json:"error_code"`
	}
	_ = json.Unmarshal(body, &got)
	if got.Code != "auth.verification_required" {
		t.Errorf("code = %q, want auth.verification_required", got.Code)
	}
}

func assertUserName(t *testing.T, st *store.Store, id, want string) {
	t.Helper()
	u, err := st.GetUser(context.Background(), id)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Name != want {
		t.Errorf("user %s name = %q, want %q", id, u.Name, want)
	}
}

func TestAuth_UnverifiedLoginCannotClaimPrivilegedUser(t *testing.T) {
	for _, tc := range []struct {
		name string
		body map[string]string
	}{
		{"by student id", map[string]string{"name": "Eve", "student_id": "M-1"}},
		{"by email without smtp", map[string]string{"name": "Eve", "email": "mod@example.edu"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts, st := startAuthTestServer(t)
			seedUser(t, st, store.UserRow{ID: "mod", Name: "Mod", Email: "mod@example.edu", StudentID: "M-1", Role: store.RoleModerator})

			c := newJarClient()
			status, body := postJSON(t, c, ts.URL+"/api/auth/login", tc.body)
			assertVerificationRequired(t, status, body)
			assertUserName(t, st, "mod", "Mod")

			resp, err := c.Get(ts.URL + "/api/auth/me")
			if err != nil {
				t.Fatalf("GET /me: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("/me = %d, want 401", resp.StatusCode)
			}
		})
	}
}

func TestAuth_StudentIDLoginCannotClaimEmailUserWhenSMTPIsOn(t *testing.T) {
	ts, st := startAuthTestServer(t)
	enableMockSMTP(t, st)
	seedUser(t, st, store.UserRow{ID: "s1", Name: "Ana", Email: "ana@example.edu", StudentID: "S-1", Role: store.RoleStudent})

	status, body := postJSON(t, newJarClient(), ts.URL+"/api/auth/login", map[string]string{"name": "Eve", "student_id": "S-1"})
	assertVerificationRequired(t, status, body)
	assertUserName(t, st, "s1", "Ana")
}

func TestAuth_PendingMagicLinkDoesNotRenameExistingUser(t *testing.T) {
	ts, st := startAuthTestServer(t)
	enableMockSMTP(t, st)
	sendMagicLinkEmailHook = func(context.Context, string, string) error { return nil }
	t.Cleanup(func() { sendMagicLinkEmailHook = nil })
	seedUser(t, st, store.UserRow{ID: "s1", Name: "Ana", Email: "ana@example.edu", Role: store.RoleStudent})

	status, body := postJSON(t, newJarClient(), ts.URL+"/api/auth/login", map[string]string{"name": "Eve", "email": "ana@example.edu"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body=%s", status, body)
	}
	assertUserName(t, st, "s1", "Ana")
}

func TestAuth_UnverifiedStudentLoginStillWorksWithoutSMTP(t *testing.T) {
	ts, st := startAuthTestServer(t)
	seedUser(t, st, store.UserRow{ID: "s1", Name: "Ana", StudentID: "S-1", Role: store.RoleStudent})

	status, body := postJSON(t, newJarClient(), ts.URL+"/api/auth/login", map[string]string{"name": "Ana María", "student_id": "S-1"})
	if status != http.StatusOK {
		t.Fatalf("status = %d, body=%s", status, body)
	}
	assertUserName(t, st, "s1", "Ana María")
}
