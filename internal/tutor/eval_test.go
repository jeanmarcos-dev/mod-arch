package tutor

import (
	"context"
	"strings"
	"testing"
)

// TestEval_GoldenQueries acts as an offline evaluation suite for the RAG/Tutor pipeline.
// It ensures that pedagogical constraints (like Socratic vs Direct modes) and citation
// logic do not regress over time without needing an external LLM call.
func TestEval_GoldenQueries(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		mode       string
		mockResp   string
		wantSubstr string
		wantCites  bool
	}{
		{
			name:       "Direct Mode provides straightforward answer",
			query:      "¿Qué es PL3?",
			mode:       "direct",
			mockResp:   "PL3 es el modo usuario. [Glosario: PL3]",
			wantSubstr: "PL3 es el modo usuario",
			wantCites:  true,
		},
		{
			name:       "Socratic Mode responds with a question",
			query:      "No entiendo la diferencia entre PL0 y PL3",
			mode:       "socratic",
			mockResp:   "Esa es una excelente pregunta. ¿Qué crees que pasaría si un proceso de usuario tuviera acceso total al hardware? [Glosario: PL0]",
			wantSubstr: "¿Qué crees que pasaría",
			wantCites:  true, // It should still cite even in socratic mode
		},
		{
			name:       "Out of scope queries should be handled safely",
			query:      "¿Cuál es la receta de la pizza?",
			mode:       "direct",
			mockResp:   "Lo siento, solo puedo ayudarte con temas relacionados a Sistemas Operativos y la simulación.",
			wantSubstr: "Sistemas Operativos",
			wantCites:  false, // No citations for out of scope answers
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			llmStub := &stubLLM{resp: tc.mockResp}
			svc := setupService(t, llmStub)
			defer svc.Close()

			opts := AskOptions{Mode: tc.mode}
			ans, err := svc.AskWithOptions(context.Background(), nil, tc.query, opts, nil)
			if err != nil {
				t.Fatalf("Ask() failed: %v", err)
			}

			// 1. Validate response contains expected pedagogical text
			if !strings.Contains(ans.Text, tc.wantSubstr) {
				t.Errorf("Expected response to contain %q, got: %q", tc.wantSubstr, ans.Text)
			}

			// 2. Validate citation parsing and presence
			hasCites := len(ans.Citations) > 0
			if hasCites != tc.wantCites {
				t.Errorf("Expected citations presence to be %v, got %v (citations: %v)", tc.wantCites, hasCites, ans.Citations)
			}
		})
	}
}
