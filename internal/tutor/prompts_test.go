package tutor

import (
	"strings"
	"testing"
)

func TestBuildMessagesDetectionContext(t *testing.T) {
	const summary = "Interbloqueo activo desde el tick 42: ciclo de espera P1 → P2 → P1."
	cases := []struct {
		name             string
		detectionContext string
		wantSection      bool
	}{
		{name: "non-empty appends section", detectionContext: summary, wantSection: true},
		{name: "empty appends nothing", detectionContext: "", wantSection: false},
		{name: "whitespace appends nothing", detectionContext: "   \n  ", wantSection: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs := BuildMessages("¿por qué se cuelga?", "", nil, nil, nil, tc.detectionContext)
			user := msgs[len(msgs)-1]
			if user.Role != "user" {
				t.Fatalf("last message role = %q, want user", user.Role)
			}
			hasHeader := strings.Contains(user.Content, "DETECTORES DE CONCURRENCIA (estado en vivo):")
			if hasHeader != tc.wantSection {
				t.Errorf("section present = %v, want %v; content:\n%s", hasHeader, tc.wantSection, user.Content)
			}
			if tc.wantSection && !strings.Contains(user.Content, summary) {
				t.Errorf("detection context text missing from user message:\n%s", user.Content)
			}
			if !tc.wantSection && strings.Contains(user.Content, "DETECTORES") {
				t.Errorf("unexpected detector text leaked when context empty:\n%s", user.Content)
			}
		})
	}
}

func TestBuildMessagesNoHistory(t *testing.T) {
	msgs := BuildMessages("¿qué es PL3?", "", nil, nil, nil, "")
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
	msgs := BuildMessages("¿y por qué?", "", nil, nil, history, "")
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
