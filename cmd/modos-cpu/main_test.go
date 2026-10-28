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

func TestRunDemoExecutesUntilHalt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		console string
	}{
		{name: "counter"},
		{name: "hello", console: "hola SO\n"},
		{name: "safe_counter"},
		{name: "producer_consumer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			demo, ok := findDemo(tc.name)
			if !ok {
				t.Fatalf("demo %q not registered", tc.name)
			}
			res, err := runDemo(demo, 8, 20000, nil, nil)
			if err != nil {
				t.Fatalf("runDemo: %v", err)
			}
			if res.Steps == 0 {
				t.Fatal("no instruction was executed")
			}
			if !res.Halted {
				t.Fatalf("demo did not halt within the step budget (%d steps)", res.Steps)
			}
			if tc.console != "" && string(res.Console) != tc.console {
				t.Errorf("console = %q, want %q", res.Console, tc.console)
			}
		})
	}
}
