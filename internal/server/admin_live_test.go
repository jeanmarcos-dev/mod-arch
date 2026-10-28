package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanmarcos-dev/mod-arch/internal/sessions"
	"github.com/jeanmarcos-dev/mod-arch/internal/store"
)

type liveFixture struct {
	s   *Server
	st  *store.Store
	mgr *sessions.Manager
	mux *http.ServeMux
}

func newLiveFixture(t *testing.T) *liveFixture {
	t.Helper()
	t.Setenv("ADMIN_USER", "boss")
	t.Setenv("ADMIN_PASS_HASH", "x")
	t.Setenv("SESSION_SECRET", "live-test-secret")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mgr := sessions.NewManager(sessions.Config{Store: st})
	s := New(Config{Manager: mgr, Logger: discardLogger()})
	t.Cleanup(s.Close)
	mux := http.NewServeMux()
	s.Routes(mux)
	return &liveFixture{s: s, st: st, mgr: mgr, mux: mux}
}

func (f *liveFixture) envAdminCookie(t *testing.T) *http.Cookie {
	t.Helper()
	value, err := f.s.signCookie("boss", time.Hour)
	if err != nil {
		t.Fatalf("signCookie: %v", err)
	}
	return &http.Cookie{Name: AdminCookie, Value: value}
}

func (f *liveFixture) get(target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func (f *liveFixture) decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("not an object: %#v", v)
	}
	return m
}

func asList(t *testing.T, v any) []any {
	t.Helper()
	l, ok := v.([]any)
	if !ok {
		t.Fatalf("not an array: %#v", v)
	}
	return l
}

func TestAdminLive_AccessIsRestrictedToEnvAdmin(t *testing.T) {
	f := newLiveFixture(t)
	ctx := context.Background()

	if err := f.st.CreateUser(ctx, store.UserRow{ID: "u-db-admin", Name: "DB Admin", Role: store.RoleAdmin, CreatedAt: 1, LastLoginAt: 1}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	dbAdminSess, err := f.mgr.Create(sessions.ModeFull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	dbAdminSess.SetUserID("u-db-admin")

	tests := []struct {
		name    string
		cookies []*http.Cookie
		want    int
	}{
		{name: "env admin", cookies: []*http.Cookie{f.envAdminCookie(t)}, want: http.StatusOK},
		{name: "database admin without env cookie", cookies: []*http.Cookie{{Name: SessionCookie, Value: dbAdminSess.OwnerToken()}}, want: http.StatusNotFound},
		{name: "anonymous", want: http.StatusNotFound},
		{name: "forged cookie", cookies: []*http.Cookie{{Name: AdminCookie, Value: "abc.def"}}, want: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.get("/admin/api/live", tc.cookies...).Code; got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAdminLive_DisabledAdminReturns404(t *testing.T) {
	t.Setenv("ADMIN_USER", "")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	s := New(Config{Manager: sessions.NewManager(sessions.Config{Store: st}), Logger: discardLogger()})
	defer s.Close()
	mux := http.NewServeMux()
	s.Routes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/live", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func seedLiveData(t *testing.T, f *liveFixture, now time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := f.st.CreateUser(ctx, store.UserRow{ID: "u-1", Name: "Maria Secreta", Email: "maria.secreta@example.com", Role: store.RoleStudent, CreatedAt: 1, LastLoginAt: 1}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	mk := func(info sessions.ClientInfo) *sessions.Session {
		sess, err := f.mgr.CreateWithClient(sessions.ModeFull, info)
		if err != nil {
			t.Fatalf("CreateWithClient: %v", err)
		}
		return sess
	}
	ve1 := mk(sessions.ClientInfo{GeoCountry: "VE", IPPlain: "198.51.100.7"})
	ve2 := mk(sessions.ClientInfo{GeoCountry: "VE"})
	ar := mk(sessions.ClientInfo{GeoCountry: "AR"})
	ve1.SetUserID("u-1")
	ve1.SetPresence(sessions.Presence{Section: "/concurrencia"})
	ve2.SetPresence(sessions.Presence{Section: "/concurrencia"})
	ar.SetPresence(sessions.Presence{Section: "/procesos"})
	ve1.SetProgram("race_counter")
	ve2.SetProgram("race_counter")
	ar.SetProgram("philos_deadlock")

	ts := now.UnixMilli()
	d1, d2 := int64(60000), int64(40000)
	events := []store.UsageEvent{
		{TS: ts, SessionID: ve1.ID, UserID: "u-1", Kind: store.UsageKindPageView, Name: "/procesos", DurationMS: &d1},
		{TS: ts, SessionID: ve1.ID, UserID: "u-1", Kind: store.UsageKindPageView, Name: "/procesos", DurationMS: &d2},
		{TS: ts, SessionID: ve2.ID, Kind: store.UsageKindPageView, Name: "/memoria", DurationMS: &d1},
		{TS: ts, SessionID: ve1.ID, UserID: "u-1", Kind: store.UsageKindDemoLoad, Name: "race_counter"},
		{TS: ts, SessionID: ve2.ID, Kind: store.UsageKindDemoLoad, Name: "race_counter"},
		{TS: ts, SessionID: ve2.ID, Kind: store.UsageKindDemoRun, Name: "race_counter"},
		{TS: ts, SessionID: ar.ID, Kind: store.UsageKindDemoLoad, Name: "philos_deadlock"},
		{TS: now.Add(-3 * time.Hour).UnixMilli(), SessionID: "old", UserID: "u-old", Kind: store.UsageKindPageView, Name: "/viejo", DurationMS: &d1},
	}
	if err := f.st.InsertUsageEvents(ctx, events); err != nil {
		t.Fatalf("InsertUsageEvents: %v", err)
	}

	long := strings.Repeat("ñ", 200)
	up, down := 1, -1
	qas := []store.TutorQA{
		{TS: ts - 3000, SessionID: ve1.ID, UserID: "u-1", RequestID: "r1", Question: "¿Qué es un deadlock?", Answer: "a", CitationsJSON: `["[Glosario: Deadlock]","[Glosario: Mutex]"]`, LatencyMS: 1000, Provider: "gemini"},
		{TS: ts - 2000, SessionID: ve2.ID, RequestID: "r2", Question: long, Answer: "b", CitationsJSON: `["[Glosario: Deadlock]"]`, LatencyMS: 3000, Provider: "gemini"},
		{TS: ts - 1000, SessionID: ar.ID, RequestID: "r3", Question: "pregunta degradada", Answer: "c", Degraded: true, LatencyMS: 9000, Error: "llm down"},
		{TS: ts, SessionID: ve2.ID, RequestID: "r4", Question: "otra", Answer: "d", LatencyMS: 2000},
		{TS: now.Add(-3 * time.Hour).UnixMilli(), SessionID: "old", RequestID: "r5", Question: "vieja", Answer: "e"},
	}
	for _, qa := range qas {
		if err := f.st.InsertTutorQA(ctx, qa); err != nil {
			t.Fatalf("InsertTutorQA: %v", err)
		}
	}
	for _, rating := range []struct {
		session, request string
		value            *int
	}{{ve1.ID, "r1", &up}, {ve2.ID, "r2", &up}, {ar.ID, "r3", &down}} {
		if _, err := f.st.RateTutorQA(ctx, rating.session, rating.request, *rating.value, ts); err != nil {
			t.Fatalf("RateTutorQA: %v", err)
		}
	}
}

func TestAdminLive_AggregatesEveryBlock(t *testing.T) {
	f := newLiveFixture(t)
	seedLiveData(t, f, time.Now())

	rec := f.get("/admin/api/live?window=120", f.envAdminCookie(t))
	body := f.decode(t, rec)

	if got := body["window_minutes"]; got != float64(120) {
		t.Errorf("window_minutes = %v, want 120", got)
	}
	if body["generated_at"].(float64) <= 0 {
		t.Error("generated_at missing")
	}

	now := asMap(t, body["now"])
	for field, want := range map[string]float64{
		"sessions_active": 3, "sessions_with_user": 1, "sessions_anonymous": 2,
		"sessions_created_total": 3, "peak_sessions": 3,
	} {
		if now[field] != want {
			t.Errorf("now.%s = %v, want %v", field, now[field], want)
		}
	}
	sections := asList(t, now["sections"])
	if first := asMap(t, sections[0]); first["name"] != "/concurrencia" || first["count"] != float64(2) {
		t.Errorf("sections[0] = %v", first)
	}
	demos := asList(t, now["demos"])
	if first := asMap(t, demos[0]); first["name"] != "race_counter" || first["count"] != float64(2) {
		t.Errorf("now.demos[0] = %v", first)
	}

	window := asMap(t, body["window"])
	if window["unique_users"] != float64(1) || window["unique_sessions"] != float64(3) {
		t.Errorf("uniques = %v / %v", window["unique_users"], window["unique_sessions"])
	}
	views := asList(t, window["page_views"])
	if first := asMap(t, views[0]); first["name"] != "/procesos" || first["views"] != float64(2) || first["avg_ms"] != float64(50000) {
		t.Errorf("page_views[0] = %v", first)
	}
	wdemos := asList(t, window["demos"])
	if first := asMap(t, wdemos[0]); first["name"] != "race_counter" || first["loads"] != float64(2) || first["runs"] != float64(1) {
		t.Errorf("window.demos[0] = %v", first)
	}
	activity := asList(t, window["activity_per_minute"])
	if len(activity) != 60 {
		t.Fatalf("activity buckets = %d, want 60", len(activity))
	}
	events, questions := 0.0, 0.0
	for _, b := range activity {
		bm := asMap(t, b)
		events += bm["events"].(float64)
		questions += bm["questions"].(float64)
	}
	if events != 7 || questions != 4 {
		t.Errorf("activity totals events=%v questions=%v, want 7 and 4", events, questions)
	}

	tutor := asMap(t, body["tutor"])
	for field, want := range map[string]float64{
		"questions": 4, "askers": 3, "degraded": 1, "thumbs_up": 2, "thumbs_down": 1,
		"latency_p50_ms": 2000, "latency_p95_ms": 3000,
	} {
		if tutor[field] != want {
			t.Errorf("tutor.%s = %v, want %v", field, tutor[field], want)
		}
	}
	topics := asList(t, tutor["top_topics"])
	if first := asMap(t, topics[0]); first["label"] != "[Glosario: Deadlock]" || first["count"] != float64(2) {
		t.Errorf("top_topics[0] = %v", first)
	}
	recent := asList(t, tutor["recent"])
	if len(recent) != 4 {
		t.Fatalf("recent = %d, want 4 (old question is out of the window)", len(recent))
	}
	if first := asMap(t, recent[0]); first["question"] != "otra" || first["rating"] != nil {
		t.Errorf("recent[0] = %v", first)
	}
	truncated := asMap(t, recent[2])["question"].(string)
	if n := len([]rune(truncated)); n != 140 {
		t.Errorf("truncated question has %d runes, want 140", n)
	}
	if asMap(t, recent[3])["rating"] != float64(1) {
		t.Errorf("recent[3].rating = %v, want 1", asMap(t, recent[3])["rating"])
	}

	countries := asList(t, body["countries"])
	if first := asMap(t, countries[0]); first["code"] != "VE" || first["count"] != float64(2) {
		t.Errorf("countries[0] = %v", first)
	}

	raw := rec.Body.String()
	for _, secret := range []string{"Maria", "Secreta", "maria.secreta@example.com", "198.51.100.7", "u-1", "email", "ip"} {
		if strings.Contains(raw, secret) {
			t.Errorf("response leaks %q", secret)
		}
	}
}

func TestAdminLive_EmptyDatabaseUsesEmptyArrays(t *testing.T) {
	f := newLiveFixture(t)
	body := f.decode(t, f.get("/admin/api/live", f.envAdminCookie(t)))
	now := asMap(t, body["now"])
	window := asMap(t, body["window"])
	tutor := asMap(t, body["tutor"])
	for name, v := range map[string]any{
		"now.sections": now["sections"], "now.demos": now["demos"],
		"window.page_views": window["page_views"], "window.demos": window["demos"],
		"tutor.top_topics": tutor["top_topics"], "tutor.recent": tutor["recent"],
		"countries": body["countries"],
	} {
		if _, ok := v.([]any); !ok {
			t.Errorf("%s = %#v, want []", name, v)
		}
	}
}

func TestAdminLive_WindowParameter(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantWindow float64
		wantBucket int
	}{
		{name: "default", query: "", wantWindow: 120, wantBucket: 60},
		{name: "thirty minutes", query: "?window=30", wantWindow: 30, wantBucket: 30},
		{name: "capped to a day", query: "?window=99999", wantWindow: 1440, wantBucket: 60},
		{name: "garbage falls back", query: "?window=abc", wantWindow: 120, wantBucket: 60},
		{name: "zero falls back", query: "?window=0", wantWindow: 120, wantBucket: 60},
		{name: "negative falls back", query: "?window=-5", wantWindow: 120, wantBucket: 60},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newLiveFixture(t)
			body := f.decode(t, f.get("/admin/api/live"+tc.query, f.envAdminCookie(t)))
			if body["window_minutes"] != tc.wantWindow {
				t.Errorf("window_minutes = %v, want %v", body["window_minutes"], tc.wantWindow)
			}
			if got := len(asList(t, asMap(t, body["window"])["activity_per_minute"])); got != tc.wantBucket {
				t.Errorf("activity buckets = %d, want %d", got, tc.wantBucket)
			}
		})
	}
}

func TestAdminLive_WindowAggregatesAreCachedForTwoSeconds(t *testing.T) {
	f := newLiveFixture(t)
	cookie := f.envAdminCookie(t)
	ctx := context.Background()
	insert := func(session string) {
		d := int64(1)
		if err := f.st.InsertUsageEvents(ctx, []store.UsageEvent{{TS: time.Now().UnixMilli(), SessionID: session, Kind: store.UsageKindPageView, Name: "/x", DurationMS: &d}}); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	uniques := func() float64 {
		return asMap(t, f.decode(t, f.get("/admin/api/live?window=60", cookie))["window"])["unique_sessions"].(float64)
	}

	insert("s-1")
	if got := uniques(); got != 1 {
		t.Fatalf("first read = %v, want 1", got)
	}
	insert("s-2")
	if got := uniques(); got != 1 {
		t.Fatalf("read inside the TTL = %v, want cached 1", got)
	}
	f.s.live.mu.Lock()
	entry := f.s.live.entries[60]
	entry.at = entry.at.Add(-3 * time.Second)
	f.s.live.entries[60] = entry
	f.s.live.mu.Unlock()
	if got := uniques(); got != 2 {
		t.Fatalf("read after the TTL = %v, want refreshed 2", got)
	}
}

func TestAdminLive_RespondsFastWithFiftyThousandEvents(t *testing.T) {
	if raceEnabled {
		t.Skip("timing assertion is meaningless under the race detector")
	}
	f := newLiveFixture(t)
	ctx := context.Background()
	now := time.Now()
	kinds := []string{store.UsageKindPageView, store.UsageKindDemoLoad, store.UsageKindDemoRun}
	names := []string{"/procesos", "/memoria", "/concurrencia", "/interrupciones"}
	const total = 50000
	events := make([]store.UsageEvent, 0, total)
	dur := int64(12000)
	for i := 0; i < total; i++ {
		e := store.UsageEvent{
			TS:        now.Add(-time.Duration(i%7000) * time.Second).UnixMilli(),
			SessionID: "s-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)),
			Kind:      kinds[i%len(kinds)],
			Name:      names[i%len(names)],
		}
		if i%5 == 0 {
			e.UserID = "u-" + string(rune('a'+i%26))
		}
		if e.Kind == store.UsageKindPageView {
			e.DurationMS = &dur
		}
		events = append(events, e)
	}
	if err := f.st.InsertUsageEvents(ctx, events); err != nil {
		t.Fatalf("InsertUsageEvents: %v", err)
	}
	for i := 0; i < 500; i++ {
		qa := store.TutorQA{
			TS: now.Add(-time.Duration(i) * time.Second).UnixMilli(), SessionID: "s-q", RequestID: "r-" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26)),
			Question: "pregunta", Answer: "respuesta", CitationsJSON: `["[Glosario: Deadlock]"]`, LatencyMS: int64(1000 + i),
		}
		if err := f.st.InsertTutorQA(ctx, qa); err != nil {
			t.Fatalf("InsertTutorQA: %v", err)
		}
	}

	cookie := f.envAdminCookie(t)
	start := time.Now()
	rec := f.get("/admin/api/live?window=1440", cookie)
	elapsed := time.Since(start)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("live endpoint took %v with %d events, want < 300ms", elapsed, total)
	}
	t.Logf("live endpoint with %d events: %v", total, elapsed)
}
