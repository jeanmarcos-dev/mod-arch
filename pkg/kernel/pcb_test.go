package kernel

import "testing"

// TestStateIdleConstant pins the IDLE state string. The frontend keys CSS
// classes off this exact value (e.g. .state-IDLE), so renaming it silently
// would desync the renderer.
func TestStateIdleConstant(t *testing.T) {
	if got := string(StateIdle); got != "IDLE" {
		t.Errorf("StateIdle = %q, want %q", got, "IDLE")
	}
}

// TestPCB_IsIdle covers the swapper-identity helper: only PID 0 is the kernel
// idle task. Any other PID — including init (1) and user procs — must report
// false so the scheduler's idle-eviction logic and the frontend's compact
// PID-0 band can rely on the helper without further checks.
func TestPCB_IsIdle(t *testing.T) {
	cases := []struct {
		name string
		pid  int
		want bool
	}{
		{"pid zero is idle", 0, true},
		{"init is not idle", 1, false},
		{"user proc is not idle", 7, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &PCB{PID: tc.pid}
			if got := p.IsIdle(); got != tc.want {
				t.Errorf("PCB{PID:%d}.IsIdle() = %v, want %v", tc.pid, got, tc.want)
			}
		})
	}
}
