package server

import (
	"bytes"
	"context"
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

	"github.com/jeanmarcos-dev/mod-arch/internal/crypto"
	"github.com/jeanmarcos-dev/mod-arch/internal/programs"
	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	"github.com/jeanmarcos-dev/mod-arch/internal/sim"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
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
		OK      bool   `json:"ok"`
		SMTP    string `json:"smtp"`
		Pending bool   `json:"pending"`
		User    struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	}
	_ = json.Unmarshal(body, &got)
	if !got.OK || got.User.ID == "" || got.User.Email != "ada@ucv.ve" {
		t.Errorf("login response = %+v", got)
	}
	// Test server has no SMTP configured: the verify-on-login degrades to
	// trust-on-first-use so the caller is attached immediately.
	if got.SMTP != "off" {
		t.Errorf("smtp = %q, want \"off\"", got.SMTP)
	}
	if got.Pending {
		t.Error("pending should be false when SMTP is off (direct login)")
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

// TestAuth_LoginWithSMTPRequiresVerification checks the "verify-on-login"
// flow: when SMTP is configured the POST /api/auth/login response does NOT
// attach the session — the caller has to consume the magic link first.
func TestAuth_LoginWithSMTPRequiresVerification(t *testing.T) {
	ts, st := startAuthTestServer(t)
	enableMockSMTP(t, st)

	// Stub the actual delivery so the test does not need a real SMTP server.
	var capturedTo, capturedURL string
	sendMagicLinkEmailHook = func(ctx context.Context, to, url string) error {
		capturedTo, capturedURL = to, url
		return nil
	}
	t.Cleanup(func() { sendMagicLinkEmailHook = nil })

	c := newJarClient()
	status, body := postJSON(t, c, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada", "email": "ada@ucv.ve",
	})
	if status != http.StatusOK {
		t.Fatalf("login status=%d body=%s", status, body)
	}
	var got struct {
		OK      bool   `json:"ok"`
		SMTP    string `json:"smtp"`
		Pending bool   `json:"pending"`
		User    *struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.Unmarshal(body, &got)
	if got.SMTP != "on" {
		t.Errorf("smtp = %q, want \"on\"", got.SMTP)
	}
	if !got.Pending {
		t.Error("pending should be true while waiting for the magic-link click")
	}
	if got.User != nil {
		t.Errorf("user must NOT be returned before verification, got %+v", got.User)
	}

	// The session is still anonymous — /me must reject until the link is
	// consumed.
	resp, _ := c.Get(ts.URL + "/api/auth/me")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me after pending login = %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// The mail send hook must have observed the dispatch to the right
	// recipient with a real magic-link URL.
	if capturedTo != "ada@ucv.ve" {
		t.Errorf("captured to = %q, want ada@ucv.ve", capturedTo)
	}
	if !strings.Contains(capturedURL, "/auth/magic?t=") {
		t.Errorf("captured url = %q, want a magic-link path", capturedURL)
	}
}

// TestAuth_LoginSMTPSendFailureBlocks asserts that a configured-but-broken
// SMTP backend returns 503 instead of degrading to direct login. Falling back
// silently would let an attacker DoS the mailer to bypass verification.
func TestAuth_LoginSMTPSendFailureBlocks(t *testing.T) {
	ts, st := startAuthTestServer(t)
	// "Enabled" with a bogus host so loadSMTPSender returns a non-nil sender
	// whose Dial will fail. This simulates configured-but-broken SMTP.
	enableMockSMTP(t, st)
	if err := st.SetSetting(t.Context(), "smtp_host", "127.0.0.1:1", "test", time.Now().Unix()); err != nil {
		t.Fatalf("seed bad host: %v", err)
	}
	if err := st.SetSetting(t.Context(), "smtp_port", "1", "test", time.Now().Unix()); err != nil {
		t.Fatalf("seed bad port: %v", err)
	}

	c := newJarClient()
	status, body := postJSON(t, c, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada", "email": "ada@ucv.ve",
	})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("login status=%d body=%s, want 503", status, body)
	}
	if !strings.Contains(string(body), "smtp_send_failed") {
		t.Errorf("body should carry the smtp_send_failed code: %s", body)
	}

	// Session must still be anonymous.
	resp, _ := c.Get(ts.URL + "/api/auth/me")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/me after blocked login = %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// enableMockSMTP writes the minimum set of SMTP settings to make
// loadSMTPSender return a non-nil sender. The actual delivery target is
// chosen by the caller (good or bad host) — this helper only flips the
// "enabled + minimal config" switch.
func enableMockSMTP(t *testing.T, st *store.Store) {
	t.Helper()
	now := time.Now().Unix()
	rows := map[string]string{
		"smtp_enabled":    "true",
		"smtp_host":       "mock.invalid",
		"smtp_port":       "25",
		"smtp_from_email": "noreply@modos-cpu.test",
		"smtp_from_name":  "MODOS-CPU Test",
	}
	for k, v := range rows {
		if err := st.SetSetting(t.Context(), k, v, "test", now); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}
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

// startTurnstileStub starts a httptest server impersonating Cloudflare's
// siteverify endpoint. acceptToken makes the stub return success for that
// exact token and reject everything else.
func startTurnstileStub(t *testing.T, acceptToken string) *httptest.Server {
	t.Helper()
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		ok := r.FormValue("response") == acceptToken
		w.Header().Set("Content-Type", "application/json")
		if ok {
			_, _ = w.Write([]byte(`{"success":true}`))
		} else {
			_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
		}
	}))
	t.Cleanup(stub.Close)
	prev := turnstileVerifyURL
	turnstileVerifyURL = stub.URL
	t.Cleanup(func() { turnstileVerifyURL = prev })
	return stub
}

func TestAuth_LoginTurnstileDisabledByDefault(t *testing.T) {
	ts, _ := startAuthTestServer(t)
	c := newJarClient()
	status, body := postJSON(t, c, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada", "email": "ada@ucv.ve",
	})
	if status != http.StatusOK {
		t.Fatalf("login without turnstile = %d body=%s", status, body)
	}
}

func TestAuth_LoginTurnstileEnforced(t *testing.T) {
	ts, st := startAuthTestServer(t)
	startTurnstileStub(t, "GOOD-TOKEN")
	// Persist the secret so verification is enforced.
	if err := st.SetSetting(t.Context(), settingTurnstileSecret, mustSeal(t, "secret-key"), "test", time.Now().Unix()); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	// Missing token → 403.
	c := newJarClient()
	status, _ := postJSON(t, c, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada", "email": "ada@ucv.ve",
	})
	if status != http.StatusForbidden {
		t.Fatalf("missing token: status=%d want 403", status)
	}

	// Wrong token → 403.
	c = newJarClient()
	status, _ = postJSON(t, c, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada", "email": "ada@ucv.ve", "turnstile_token": "WRONG",
	})
	if status != http.StatusForbidden {
		t.Fatalf("wrong token: status=%d want 403", status)
	}

	// Good token → 200.
	c = newJarClient()
	status, _ = postJSON(t, c, ts.URL+"/api/auth/login", map[string]string{
		"name": "Ada", "email": "ada@ucv.ve", "turnstile_token": "GOOD-TOKEN",
	})
	if status != http.StatusOK {
		t.Fatalf("good token: status=%d want 200", status)
	}
}

func TestPublicConfigExposesSitekeyOnlyWhenSecretIsSet(t *testing.T) {
	ts, st := startAuthTestServer(t)

	// Sitekey alone (no secret) must NOT be exposed — otherwise the SPA
	// renders a non-functional Turnstile widget on /login.
	if err := st.SetSetting(t.Context(), settingTurnstileSitekey, "0xPUBLIC", "test", time.Now().Unix()); err != nil {
		t.Fatalf("seed sitekey: %v", err)
	}
	c := newJarClient()
	status, body := getStatusBody(t, c, ts.URL+"/api/config")
	if status != http.StatusOK {
		t.Fatalf("/api/config status=%d", status)
	}
	if !strings.Contains(string(body), `"turnstile_sitekey":""`) {
		t.Errorf("sitekey leaked without secret: body=%s", body)
	}

	// Adding the secret completes the pair → sitekey now shows up.
	if err := st.SetSetting(t.Context(), settingTurnstileSecret, mustSeal(t, "shh"), "test", time.Now().Unix()); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	c = newJarClient()
	status, body = getStatusBody(t, c, ts.URL+"/api/config")
	if status != http.StatusOK {
		t.Fatalf("/api/config status=%d", status)
	}
	if !strings.Contains(string(body), `"turnstile_sitekey":"0xPUBLIC"`) {
		t.Errorf("body=%s missing sitekey after secret was set", body)
	}
}

// mustSeal encrypts plaintext with the same SESSION_SECRET-derived key the
// server uses (empty env → sha256("") derived key).
func mustSeal(t *testing.T, plain string) string {
	t.Helper()
	key := crypto.DeriveKey([]byte{})
	sealed, err := crypto.Seal(plain, key)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return sealed
}
