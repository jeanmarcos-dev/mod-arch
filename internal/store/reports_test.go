package store

import (
	"errors"
	"testing"
)

func seedReport(t *testing.T, s *Store, kind, status string, ts int64, userID, ipHash string) int64 {
	t.Helper()
	id, err := s.InsertReport(t.Context(), UserReport{
		TS: ts, Kind: kind, Title: kind + " " + status,
		Body: "body", ContextJSON: "{}", Status: status,
		UserID: userID, ClientIPHash: ipHash,
	})
	if err != nil {
		t.Fatalf("InsertReport: %v", err)
	}
	return id
}

func TestReports_InsertList(t *testing.T) {
	s := openMem(t)
	mustCreateUser(t, s, "u1")
	mustCreateUser(t, s, "u2")

	seedReport(t, s, ReportKindFeedback, ReportStatusOpen, 1000, "u1", "iphash-1")
	seedReport(t, s, ReportKindBug, ReportStatusOpen, 1100, "u2", "iphash-2")
	seedReport(t, s, ReportKindBug, ReportStatusResolved, 1200, "", "iphash-1")

	rows, total, err := s.ListReports(t.Context(), ReportFilter{})
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("got %d/%d, want 3/3", len(rows), total)
	}
	if rows[0].Kind != ReportKindBug || rows[0].Status != ReportStatusResolved {
		t.Errorf("expected newest (resolved bug) first, got %+v", rows[0])
	}

	bugs, total, _ := s.ListReports(t.Context(), ReportFilter{Kind: ReportKindBug})
	if total != 2 || len(bugs) != 2 {
		t.Errorf("kind filter: %d/%d, want 2/2", len(bugs), total)
	}

	openOnly, total, _ := s.ListReports(t.Context(), ReportFilter{Status: ReportStatusOpen})
	if total != 2 || len(openOnly) != 2 {
		t.Errorf("status filter: %d/%d, want 2/2", len(openOnly), total)
	}

	u1Only, _, _ := s.ListReports(t.Context(), ReportFilter{UserID: "u1"})
	if len(u1Only) != 1 {
		t.Errorf("user filter: %d, want 1", len(u1Only))
	}
}

func TestReports_StatusTransitions(t *testing.T) {
	s := openMem(t)
	mustCreateUser(t, s, "u1")
	id := seedReport(t, s, ReportKindBug, ReportStatusOpen, 1000, "u1", "h")

	// Resolving stamps actor and timestamp.
	if err := s.UpdateReportStatus(t.Context(), id, ReportStatusResolved, "admin:root", 2000); err != nil {
		t.Fatalf("UpdateReportStatus: %v", err)
	}
	got, err := s.GetReport(t.Context(), id)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if got.Status != ReportStatusResolved || got.ResolvedAt != 2000 || got.ResolvedBy != "admin:root" {
		t.Errorf("resolved row: %+v", got)
	}

	// Reopening clears the resolution metadata.
	if err := s.UpdateReportStatus(t.Context(), id, ReportStatusOpen, "", 0); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, _ = s.GetReport(t.Context(), id)
	if got.Status != ReportStatusOpen || got.ResolvedAt != 0 || got.ResolvedBy != "" {
		t.Errorf("reopened row should clear resolution: %+v", got)
	}
}

func TestReports_GetMissing(t *testing.T) {
	s := openMem(t)
	if _, err := s.GetReport(t.Context(), 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestReports_CountSince(t *testing.T) {
	s := openMem(t)
	mustCreateUser(t, s, "u1")
	seedReport(t, s, ReportKindBug, ReportStatusOpen, 1000, "u1", "ipA")
	seedReport(t, s, ReportKindBug, ReportStatusOpen, 1100, "u1", "ipA")
	seedReport(t, s, ReportKindBug, ReportStatusOpen, 1200, "u1", "ipB")

	n, err := s.CountReportsSince(t.Context(), "ipA", 1050)
	if err != nil {
		t.Fatalf("CountReportsSince: %v", err)
	}
	if n != 1 {
		t.Errorf("ipA since 1050 = %d, want 1", n)
	}

	n, _ = s.CountReportsSince(t.Context(), "ipA", 0)
	if n != 2 {
		t.Errorf("ipA all = %d, want 2", n)
	}

	// Empty hash returns 0 without querying.
	n, _ = s.CountReportsSince(t.Context(), "", 0)
	if n != 0 {
		t.Errorf("empty hash should return 0, got %d", n)
	}
}
