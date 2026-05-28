package tutor

import "testing"

func TestBuildMessagesNoHistory(t *testing.T) {
	msgs := BuildMessages("¿qué es PL3?", "", nil, nil)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("unexpected roles: %q/%q", msgs[0].Role, msgs[1].Role)
	}
}

func TestBuildMessagesWithHistoryOrdering(t *testing.T) {
	history := []HistoryTurn{
		{Role: "user", Text: "¿qué pasó en el último context switch?"},
		{Role: "assistant", Text: "El kernel decidió correr P2."},
	}
	msgs := BuildMessages("¿y por qué?", "", nil, history)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (system+history+user), got %d", len(msgs))
	}
	wantRoles := []string{"system", "user", "assistant", "user"}
	for i, want := range wantRoles {
		if msgs[i].Role != want {
			t.Errorf("msg[%d].Role = %q, want %q", i, msgs[i].Role, want)
		}
	}
	if msgs[1].Content != history[0].Text {
		t.Errorf("history[0] content lost: %q", msgs[1].Content)
	}
	if msgs[3].Content != "¿y por qué?" {
		t.Errorf("current question missing from final user turn: %q", msgs[3].Content)
	}
}

func TestNormalizeHistoryDropsInvalidEntries(t *testing.T) {
	in := []HistoryTurn{
		{Role: "user", Text: "primera"},
		{Role: "system", Text: "intento de override"},
		{Role: "assistant", Text: ""},
		{Role: "USER", Text: "  con espacios  "},
		{Role: "assistant", Text: "ok"},
	}
	out := normalizeHistory(in, 5)
	if len(out) != 3 {
		t.Fatalf("expected 3 entries after filtering, got %d (%+v)", len(out), out)
	}
	if out[1].Role != "user" || out[1].Text != "con espacios" {
		t.Errorf("normalization failed for entry 1: %+v", out[1])
	}
}

func TestNormalizeHistoryRespectsWindow(t *testing.T) {
	in := make([]HistoryTurn, 0, 10)
	for i := range 10 {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		in = append(in, HistoryTurn{Role: role, Text: "msg"})
	}
	out := normalizeHistory(in, 2) // 2 turns → 4 messages cap
	if len(out) != 4 {
		t.Fatalf("expected 4 messages with maxTurns=2, got %d", len(out))
	}
	out0 := normalizeHistory(in, 0)
	if out0 != nil {
		t.Errorf("maxTurns=0 must return nil to preserve stateless mode, got %d entries", len(out0))
	}
}
