package store

import (
	"testing"
)

func TestAudit_AppendAndList(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	rows := []AuditEntry{
		{TS: 100, Actor: "admin", Action: "session.kill", Target: "abc"},
		{TS: 200, Actor: "admin", Action: "settings.set", Target: "access_mode", MetaJSON: `{"value":"closed"}`},
		{TS: 300, Actor: "admin", Action: "invite.create"},
	}
	for _, e := range rows {
		if err := s.AppendAudit(ctx, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	got, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("list len = %d", len(got))
	}
	// Newest first.
	if got[0].TS != 300 || got[1].TS != 200 || got[2].TS != 100 {
		t.Errorf("order = %+v", got)
	}
	if got[1].MetaJSON != `{"value":"closed"}` {
		t.Errorf("meta roundtrip = %q", got[1].MetaJSON)
	}
}

func TestAudit_ListLimit(t *testing.T) {
	s := mustOpen(t)
	ctx := t.Context()

	for i := range 50 {
		_ = s.AppendAudit(ctx, AuditEntry{TS: int64(i), Actor: "x", Action: "a"})
	}
	got, _ := s.ListAudit(ctx, 5)
	if len(got) != 5 {
		t.Errorf("len = %d, want 5", len(got))
	}
}
