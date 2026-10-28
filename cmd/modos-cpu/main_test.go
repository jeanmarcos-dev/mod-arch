package main

import (
	"strings"
	"testing"
)

func TestWeakSessionSecret(t *testing.T) {
	for _, tc := range []struct {
		secret string
		weak   bool
	}{
		{"", true},
		{"short", true},
		{strings.Repeat("a", 31), true},
		{strings.Repeat("a", 32), false},
		{strings.Repeat("f", 64), false},
	} {
		if got := weakSessionSecret(tc.secret); got != tc.weak {
			t.Errorf("weakSessionSecret(len=%d) = %v, want %v", len(tc.secret), got, tc.weak)
		}
	}
}
