package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeContextJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "{}"},
		{"valid object", `{"route":"/perfil","build":"abc"}`, `{"route":"/perfil","build":"abc"}`},
		{"whitespace tolerated", "  { \"k\":1 }  ", `{ "k":1 }`},
		{"array rejected", `["nope"]`, "{}"},
		{"primitive rejected", `42`, "{}"},
		{"broken json rejected", `{not json}`, "{}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeContextJSON(json.RawMessage(c.in))
			if got != c.want {
				t.Errorf("sanitizeContextJSON(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeContextJSON_OversizeRejected(t *testing.T) {
	// 5 KiB blob — over the maxReportContextBytes cap (4096).
	huge := `{"x":"` + strings.Repeat("a", maxReportContextBytes+100) + `"}`
	if got := sanitizeContextJSON(json.RawMessage(huge)); got != "{}" {
		t.Errorf("oversize payload should collapse to {}, got len=%d", len(got))
	}
}

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"}, {1, "1"}, {42, "42"}, {-7, "-7"}, {1234567890, "1234567890"},
	}
	for _, c := range cases {
		if got := itoa(c.in); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
