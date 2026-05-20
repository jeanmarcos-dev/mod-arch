package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/programs"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/sim"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

// startAuthTestServer brings up a Server with persistence enabled, which
// /api/auth requires (no store ⇒ 503).
func startAuthTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	demos := make([]sim.Demo, 0)
	for _, d := range programs.Demos() {
		procs := make([]sim.Proc, 0, len(d.Procs))
		for _, p := range d.Procs {
			procs = append(procs, sim.Proc{Name: p.Name, Bytes: p.Bytes})
		}
		demos = append(demos, sim.Demo{Name: d.Name, Description: d.Description, Procs: procs})
	}
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	mgr := sessions.NewManager(sessions.Config{Demos: demos, Store: st})
	s := New(Config{Manager: mgr})
	mux := http.NewServeMux()
	s.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, st
}

func newJarClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// postJSON sends a JSON body and returns the (already-drained) status code
// and body bytes. It primes the CSRF cookie via GET /api/auth/me when the
// jar does not already have one, then echoes the cookie back in the header
// — mirroring what the SPA does in production.
func postJSON(t *testing.T, c *http.Client, target string, body any) (int, []byte) {
	t.Helper()
	csrf := ensureCSRFToken(t, c, target)
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, target, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(CSRFHeader, csrf)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func ensureCSRFToken(t *testing.T, c *http.Client, target string) string {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil || c.Jar == nil {
		return ""
	}
	base := &url.URL{Scheme: u.Scheme, Host: u.Host}
	if v := lookupCookie(c.Jar.Cookies(base), CSRFCookie); v != "" {
		return v
	}
	resp, err := c.Get(base.String() + "/api/auth/me")
	if err == nil {
		_ = resp.Body.Close()
	}
	return lookupCookie(c.Jar.Cookies(base), CSRFCookie)
}

func lookupCookie(cookies []*http.Cookie, name string) string {
	for _, k := range cookies {
		if k.Name == name {
			return k.Value
		}
	}
	return ""
}

func TestAuth_LoginCreatesUserAndSession(t *testing.T) {
	ts, st := startAuthTestServer(t)
	client := newJarClient()

	status, body := postJSON(t, client, ts.URL+"/api/auth/login", map[string]string{
		"name":  "Ada",
		"email": "ada@ucv.ve",
	})
	if status != http.StatusOK {
		t.Fatalf("login status=%d body=%s", status, body)
	}
	var got struct {
		OK        bool `json:"ok"`
		MagicSent bool `json:"magic_sent"`
		User      struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	}
	_ = json.Unmarshal(body, &got)
	if !got.OK || got.User.ID == "" || got.User.Email != "ada@ucv.ve" {
		t.Errorf("login response = %+v", got)
	}
	if !got.MagicSent {
		t.Error("magic_sent should be true when email is provided")
	}

	users, _ := st.ListUsers(t.Context())
	if len(users) != 1 || users[0].Email != "ada@ucv.ve" {
		t.Errorf("users in store = %+v", users)
	}

	resp, err := client.Get(ts.URL + "/api/auth/me")
	if err != nil {
		t.Fatalf("/me: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("me status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAuth_LoginDeduplicatesByEmail(t *testing.T) {
	ts, st := startAuthTestServer(t)

	c1 := newJarClient()
	_, _ = postJSON(t, c1, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada", "email": "ada@ucv.ve",
	})

	c2 := newJarClient()
	_, body := postJSON(t, c2, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada L.", "email": "ada@ucv.ve",
	})
	var got struct {
		User struct {
			ID, Name string
		} `json:"user"`
	}
	_ = json.Unmarshal(body, &got)
	if got.User.Name != "Ada L." {
		t.Errorf("name not updated: %+v", got)
	}

	users, _ := st.ListUsers(t.Context())
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
}

func TestAuth_LoginRejectsBadInput(t *testing.T) {
	ts, _ := startAuthTestServer(t)
	c := newJarClient()

	cases := []map[string]string{
		{},
		{"name": ""},
		{"name": "Ada", "email": "no-arroba"},
		{"name": "Ada", "student_id": "espacios no"},
		{"name": strings.Repeat("x", 40)},
	}
	for _, body := range cases {
		status, _ := postJSON(t, c, ts.URL+"/api/auth/login", body)
		if status != http.StatusBadRequest {
			t.Errorf("body=%v status=%d, want 400", body, status)
		}
	}
}

func TestAuth_LogoutDetachesUser(t *testing.T) {
	ts, _ := startAuthTestServer(t)
	c := newJarClient()

	_, _ = postJSON(t, c, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada", "email": "ada@ucv.ve",
	})
	status, _ := postJSON(t, c, ts.URL+"/api/auth/logout", map[string]any{})
	if status != http.StatusOK {
		t.Errorf("logout status=%d", status)
	}

	resp, _ := c.Get(ts.URL + "/api/auth/me")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("after logout /me = %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAuth_MagicLinkAttachesUser(t *testing.T) {
	ts, st := startAuthTestServer(t)

	now := time.Now().Unix()
	uid := newAuthID()
	if err := st.CreateUser(t.Context(), store.UserRow{
		ID: uid, Name: "Ada", Email: "ada@ucv.ve",
		CreatedAt: now, LastLoginAt: now,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	token := "magic-tok"
	if err := st.CreateMagicLink(t.Context(), store.MagicLinkRow{
		Token: token, Email: "ada@ucv.ve", CreatedAt: now, ExpiresAt: now + 600,
	}); err != nil {
		t.Fatalf("seed link: %v", err)
	}

	c := newJarClient()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Get(ts.URL + "/auth/magic?t=" + token)
	if err != nil {
		t.Fatalf("magic GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("magic status = %d, want 302", resp.StatusCode)
	}

	status, body := getStatusBody(t, c, ts.URL+"/api/auth/me")
	if status != http.StatusOK {
		t.Fatalf("/me after magic = %d body=%s", status, body)
	}
	if !strings.Contains(string(body), "ada@ucv.ve") {
		t.Errorf("body = %s", body)
	}

	status, _ = getStatusBody(t, c, ts.URL+"/auth/magic?t="+token)
	if status != http.StatusGone {
		t.Errorf("replay status = %d, want 410", status)
	}
}

func getStatusBody(t *testing.T, c *http.Client, url string) (int, []byte) {
	t.Helper()
	r, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = r.Body.Close() }()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, b
}
