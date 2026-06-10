package store

import (
	"testing"
)

func TestSessionHistory_InsertAndList(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	mustCreateUser(t, s, "u1")
	mustCreateUser(t, s, "u2")

	rows := []SessionHistoryRow{
		{
			OriginalSessionID: "s1", UserID: "u1", Mode: "full",
			CreatedAt: 1000, EndedAt: 1600, DurationSeconds: 600,
			EndReason: EndReasonIdle, ClientIPHash: "h1", GeoCountry: "VE",
			TotalCommands: 12, TotalTutorQueries: 3,
		},
		{
			OriginalSessionID: "s2", UserID: "u1", Mode: "full",
			CreatedAt: 2000, EndedAt: 2300, DurationSeconds: 300,
			EndReason: EndReasonAdminKill, ClientIPHash: "h1", GeoCountry: "VE",
			TotalCommands: 4, TotalTutorQueries: 0,
		},
		{
			OriginalSessionID: "s3", UserID: "u2", Mode: "privileged",
			CreatedAt: 1500, EndedAt: 1800, DurationSeconds: 300,
			EndReason: EndReasonReplaced, ClientIPHash: "h2", GeoCountry: "US",
			TotalCommands: 50, TotalTutorQueries: 7,
		},
	}
	for _, r := range rows {
		if err := s.InsertSessionHistory(ctx, r); err != nil {
			t.Fatalf("InsertSessionHistory: %v", err)
		}
	}

	all, total, err := s.ListSessionHistory(ctx, SessionHistoryFilter{})
	if err != nil {
		t.Fatalf("ListSessionHistory: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("got %d rows total=%d, want 3", len(all), total)
	}
	if all[0].OriginalSessionID != "s2" {
		t.Errorf("expected newest-first; got %q first", all[0].OriginalSessionID)
	}

	u1, total, err := s.ListSessionHistory(ctx, SessionHistoryFilter{UserID: "u1"})
	if err != nil {
		t.Fatalf("ListSessionHistory(u1): %v", err)
	}
	if total != 2 || len(u1) != 2 {
		t.Errorf("u1 filter: got %d/%d, want 2", len(u1), total)
	}

	idle, total, err := s.ListSessionHistory(ctx, SessionHistoryFilter{EndReason: EndReasonIdle})
	if err != nil {
		t.Fatalf("ListSessionHistory(idle): %v", err)
	}
	if total != 1 || len(idle) != 1 || idle[0].OriginalSessionID != "s1" {
		t.Errorf("idle filter mismatch: got %+v", idle)
	}

	ve, _, _ := s.ListSessionHistory(ctx, SessionHistoryFilter{Country: "VE"})
	if len(ve) != 2 {
		t.Errorf("country filter: got %d, want 2", len(ve))
	}
}

func TestSessionHistory_Pagination(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	for i := range 12 {
		row := SessionHistoryRow{
			OriginalSessionID: "sess" + intToStr(i),
			Mode:              "full",
			CreatedAt:         int64(i * 100),
			EndedAt:           int64(i*100 + 50),
			DurationSeconds:   50,
			EndReason:         EndReasonIdle,
		}
		if err := s.InsertSessionHistory(ctx, row); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	page1, total, _ := s.ListSessionHistory(ctx, SessionHistoryFilter{Page: 1, PageSize: 5})
	page2, _, _ := s.ListSessionHistory(ctx, SessionHistoryFilter{Page: 2, PageSize: 5})
	page3, _, _ := s.ListSessionHistory(ctx, SessionHistoryFilter{Page: 3, PageSize: 5})
	if total != 12 {
		t.Errorf("total = %d, want 12", total)
	}
	if len(page1) != 5 || len(page2) != 5 || len(page3) != 2 {
		t.Errorf("page sizes = %d/%d/%d, want 5/5/2", len(page1), len(page2), len(page3))
	}
	// Ensure no overlap.
	seen := map[int64]bool{}
	for _, r := range append(append(page1, page2...), page3...) {
		if seen[r.ID] {
			t.Errorf("duplicate ID across pages: %d", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestUserActivity(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	mustCreateUser(t, s, "u1")
	mustCreateUser(t, s, "u2")
	rows := []SessionHistoryRow{
		{OriginalSessionID: "a", UserID: "u1", Mode: "full",
			CreatedAt: 1000, EndedAt: 1600, DurationSeconds: 600,
			EndReason: EndReasonIdle, GeoCountry: "VE",
			TotalCommands: 10, TotalTutorQueries: 2},
		{OriginalSessionID: "b", UserID: "u1", Mode: "full",
			CreatedAt: 2000, EndedAt: 2300, DurationSeconds: 300,
			EndReason: EndReasonAdminKill, GeoCountry: "VE",
			TotalCommands: 5, TotalTutorQueries: 1},
		{OriginalSessionID: "c", UserID: "u2", Mode: "full",
			CreatedAt: 3000, EndedAt: 3050, DurationSeconds: 50,
			EndReason: EndReasonIdle, GeoCountry: "US"},
	}
	for _, r := range rows {
		if err := s.InsertSessionHistory(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	act, err := s.GetUserActivity(ctx, "u1", 0, 0)
	if err != nil {
		t.Fatalf("GetUserActivity: %v", err)
	}
	if act.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", act.TotalSessions)
	}
	if act.TotalActiveSeconds != 900 {
		t.Errorf("TotalActiveSeconds = %d, want 900", act.TotalActiveSeconds)
	}
	if act.TotalCommands != 15 || act.TotalTutorQueries != 3 {
		t.Errorf("counters: cmds=%d tutor=%d, want 15/3", act.TotalCommands, act.TotalTutorQueries)
	}
	if act.LastSeenAt != 2300 || act.LastSeenCountry != "VE" {
		t.Errorf("last seen: at=%d country=%q, want 2300/VE", act.LastSeenAt, act.LastSeenCountry)
	}
	if len(act.ByCountry) != 1 || act.ByCountry[0].Country != "VE" {
		t.Errorf("by_country: %+v", act.ByCountry)
	}

	// Empty user returns zero stats not 404.
	empty, err := s.GetUserActivity(ctx, "ghost", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error for unknown user: %v", err)
	}
	if empty.TotalSessions != 0 {
		t.Errorf("expected zero for unknown user, got %+v", empty)
	}
}

func TestPurgeSessionHistory(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	for i := range 5 {
		_ = s.InsertSessionHistory(ctx, SessionHistoryRow{
			OriginalSessionID: "x", Mode: "full",
			CreatedAt: int64(i * 1000), EndedAt: int64(i*1000 + 100),
			EndReason: EndReasonIdle,
		})
	}
	n, err := s.PurgeSessionHistoryBefore(ctx, 2500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("purged = %d, want 3", n)
	}
	_, total, _ := s.ListSessionHistory(ctx, SessionHistoryFilter{})
	if total != 2 {
		t.Errorf("remaining = %d, want 2", total)
	}
}

func TestAuditPaginated(t *testing.T) {
	s := openMem(t)
	ctx := t.Context()
	for i := range 8 {
		_ = s.AppendAudit(ctx, AuditEntry{TS: int64(1000 + i), Actor: "admin:root", Action: "session.kill"})
	}
	_ = s.AppendAudit(ctx, AuditEntry{TS: 2000, Actor: "admin:other", Action: "invite.create"})

	page, total, err := s.ListAuditPaginated(ctx, AuditFilter{Page: 1, PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if total != 9 || len(page) != 3 {
		t.Errorf("first page: %d/%d, want 9/3", len(page), total)
	}

	filtered, total, _ := s.ListAuditPaginated(ctx, AuditFilter{Actor: "other"})
	if total != 1 || len(filtered) != 1 {
		t.Errorf("filtered: %d/%d, want 1/1", len(filtered), total)
	}
}

func mustCreateUser(t *testing.T, s *Store, id string) {
	t.Helper()
	if err := s.CreateUser(t.Context(), UserRow{
		ID:          id,
		Name:        "test " + id,
		Role:        RoleStudent,
		CreatedAt:   1,
		LastLoginAt: 1,
	}); err != nil {
		t.Fatalf("CreateUser(%s): %v", id, err)
	}
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
