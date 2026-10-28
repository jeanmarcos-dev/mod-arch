package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/internal/programs"
	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	"github.com/jeanmarcos-dev/mod-arch/internal/sim"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

// newUITestServer brings up a Server backed by a fresh SQLite store. The
// admin layer is disabled here — the tests poke applySetting / readers
// directly, which is enough to cover the UI-settings surface without
// dragging in cookies/CSRF.
func newUITestServer(t *testing.T) (*Server, *store.Store) {
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
	return s, st
}

func TestApplyUISetting_DisabledRoutes_Valid(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	value := `["processes","memory"]`
	if status, msg := s.applySetting(r, st, SettingUIDisabledRoutes, value); status != http.StatusOK {
		t.Fatalf("status=%d msg=%q", status, msg)
	}
	got := readStringArraySetting(r.Context(), st, SettingUIDisabledRoutes)
	if len(got) != 2 || got[0] != "processes" || got[1] != "memory" {
		t.Errorf("got %v, want [processes memory]", got)
	}
}

func TestApplyUISetting_DisabledRoutes_UnknownIdentifier(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	status, msg := s.applySetting(r, st, SettingUIDisabledRoutes, `["does-not-exist"]`)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", status)
	}
	if !strings.Contains(msg, "does-not-exist") {
		t.Errorf("msg=%q must name the offending value", msg)
	}
}

func TestApplyUISetting_DisabledRoutes_InvalidJSON(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	status, _ := s.applySetting(r, st, SettingUIDisabledRoutes, `not-json`)
	if status != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestApplyUISetting_DisabledRoutes_ConflictsWithDefault(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	if status, _ := s.applySetting(r, st, SettingUIDefaultRoute, "processes"); status != http.StatusOK {
		t.Fatalf("seed default route failed")
	}
	status, msg := s.applySetting(r, st, SettingUIDisabledRoutes, `["processes"]`)
	if status != http.StatusConflict {
		t.Fatalf("status=%d msg=%q, want 409", status, msg)
	}
}

func TestApplyUISetting_DisabledPanels_Valid(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	if status, _ := s.applySetting(r, st, SettingUIDisabledPanels, `["events","ipc"]`); status != http.StatusOK {
		t.Fatalf("status not ok")
	}
	got := readStringArraySetting(r.Context(), st, SettingUIDisabledPanels)
	if len(got) != 2 {
		t.Errorf("got %v, want length 2", got)
	}
}

func TestApplyUISetting_DisabledPanels_UnknownID(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	status, _ := s.applySetting(r, st, SettingUIDisabledPanels, `["banana"]`)
	if status != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestApplyUISetting_DisabledDemos_Valid(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	first := programs.DemoNames()[0]
	value, _ := json.Marshal([]string{first})
	if status, _ := s.applySetting(r, st, SettingUIDisabledDemos, string(value)); status != http.StatusOK {
		t.Fatalf("status not ok")
	}
}

func TestApplyUISetting_DisabledDemos_UnknownName(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	status, _ := s.applySetting(r, st, SettingUIDisabledDemos, `["this_demo_does_not_exist"]`)
	if status != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestApplyUISetting_DefaultRoute_Valid(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	if status, _ := s.applySetting(r, st, SettingUIDefaultRoute, "memory"); status != http.StatusOK {
		t.Fatalf("status not ok")
	}
	if got := readDefaultRoute(r.Context(), st); got != "memory" {
		t.Errorf("default=%q, want memory", got)
	}
}

func TestApplyUISetting_DefaultRoute_Unknown(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	status, _ := s.applySetting(r, st, SettingUIDefaultRoute, "wat")
	if status != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestApplyUISetting_DefaultRoute_ConflictsWithDisabled(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	if status, _ := s.applySetting(r, st, SettingUIDisabledRoutes, `["memory"]`); status != http.StatusOK {
		t.Fatalf("seed failed")
	}
	status, _ := s.applySetting(r, st, SettingUIDefaultRoute, "memory")
	if status != http.StatusConflict {
		t.Errorf("status=%d, want 409", status)
	}
}

func TestApplyUISetting_DefaultRoute_EmptyResets(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	_, _ = s.applySetting(r, st, SettingUIDefaultRoute, "memory")
	if status, _ := s.applySetting(r, st, SettingUIDefaultRoute, ""); status != http.StatusOK {
		t.Fatalf("reset failed")
	}
	if got := readDefaultRoute(r.Context(), st); got != DefaultRouteName {
		t.Errorf("after reset = %q, want %q", got, DefaultRouteName)
	}
}

func TestApplyUISetting_DisabledRoutes_DuplicatesCollapse(t *testing.T) {
	s, st := newUITestServer(t)
	r := httptest.NewRequest(http.MethodPut, "/admin/api/settings", nil)
	if status, _ := s.applySetting(r, st, SettingUIDisabledRoutes, `["memory","memory","processes"]`); status != http.StatusOK {
		t.Fatalf("status not ok")
	}
	got := readStringArraySetting(r.Context(), st, SettingUIDisabledRoutes)
	if len(got) != 2 {
		t.Errorf("got %v, want length 2 after dedupe", got)
	}
}

func TestPublicConfig_ExposesUIFields(t *testing.T) {
	ts, st := startAuthTestServer(t)
	first := programs.DemoNames()[0]
	value, _ := json.Marshal([]string{first})
	_ = st.SetSetting(t.Context(), SettingUIDisabledDemos, string(value), "test", 0)
	_ = st.SetSetting(t.Context(), SettingUIDisabledRoutes, `["processes"]`, "test", 0)
	_ = st.SetSetting(t.Context(), SettingUIDisabledPanels, `["ipc"]`, "test", 0)
	_ = st.SetSetting(t.Context(), SettingUIDefaultRoute, "memory", "test", 0)

	resp, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET /api/config: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		DisabledRoutes []string `json:"disabled_routes"`
		DisabledPanels []string `json:"disabled_panels"`
		DisabledDemos  []string `json:"disabled_demos"`
		DefaultRoute   string   `json:"default_route"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if len(out.DisabledRoutes) != 1 || out.DisabledRoutes[0] != "processes" {
		t.Errorf("DisabledRoutes=%v", out.DisabledRoutes)
	}
	if len(out.DisabledPanels) != 1 || out.DisabledPanels[0] != "ipc" {
		t.Errorf("DisabledPanels=%v", out.DisabledPanels)
	}
	if len(out.DisabledDemos) != 1 || out.DisabledDemos[0] != first {
		t.Errorf("DisabledDemos=%v", out.DisabledDemos)
	}
	if out.DefaultRoute != "memory" {
		t.Errorf("DefaultRoute=%q, want memory", out.DefaultRoute)
	}
}

func TestPublicConfig_DefaultsWhenUnset(t *testing.T) {
	ts, _ := startAuthTestServer(t)
	resp, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET /api/config: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		DisabledRoutes []string `json:"disabled_routes"`
		DefaultRoute   string   `json:"default_route"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.DefaultRoute != DefaultRouteName {
		t.Errorf("DefaultRoute=%q, want %q", out.DefaultRoute, DefaultRouteName)
	}
	if out.DisabledRoutes == nil {
		t.Errorf("DisabledRoutes is nil, want empty array")
	}
}

func TestDemoBlockedForSession_StudentBlocked(t *testing.T) {
	s, st := newUITestServer(t)
	demoName := programs.DemoNames()[0]
	value, _ := json.Marshal([]string{demoName})
	_ = st.SetSetting(t.Context(), SettingUIDisabledDemos, string(value), "test", 0)

	sess, err := s.manager.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.demoBlockedForSession(sess, "totally_other_demo") {
		t.Errorf("non-disabled demo must not be blocked")
	}
	if !s.demoBlockedForSession(sess, demoName) {
		t.Errorf("unauthenticated session must be blocked from disabled demo")
	}
}

func TestDemoBlockedForSession_AdminBypasses(t *testing.T) {
	s, st := newUITestServer(t)
	demoName := programs.DemoNames()[0]
	value, _ := json.Marshal([]string{demoName})
	_ = st.SetSetting(t.Context(), SettingUIDisabledDemos, string(value), "test", 0)

	row := store.UserRow{ID: newAuthID(), Name: "admin", Email: "a@x", Role: store.RoleAdmin}
	if err := st.CreateUser(t.Context(), row); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	sess, err := s.manager.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sess.SetUserID(row.ID)
	if s.demoBlockedForSession(sess, demoName) {
		t.Errorf("admin must bypass the disabled-demo gate")
	}
}
