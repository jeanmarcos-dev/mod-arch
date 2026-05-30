package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestHasWork_IdleEnabled_ReturnsTrue covers the run-loop contract: when the
// swapper is wired up and idle is enabled, the simulator must never auto-stop.
// HasWork is the only switch Simulator.StartRun consults, so this short-circuit
// is what keeps PID 0's HLT loop alive indefinitely instead of bouncing the
// run loop to a halted state every time a user proc blocks.
func TestHasWork_IdleEnabled_ReturnsTrue(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	k.IdleTaskEnabled = true
	// Park every core so the legacy HasWork path would say "no work":
	// halted, no pending IRQs, no ready queue, no kernel work.
	for _, c := range k.Cores {
		c.CPU.Halted = true
		c.CPU.PendingIRQs = nil
	}
	if !k.HasWork() {
		t.Error("HasWork() = false with idle enabled and per-core swappers spawned; want true (the swappers themselves are work)")
	}
}

// TestHasWork_IdleDisabled_MatchesLegacy guards the default-OFF path: with
// the flag clear, HasWork must behave exactly like it did before idle was
// introduced. Otherwise every existing demo would lose its auto-stop and
// terminal-halted broadcast.
func TestHasWork_IdleDisabled_MatchesLegacy(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	// Flag stays false. Spawning the PCB alone must not lie about HasWork:
	// the legacy answer is "no" when every core is halted and queues are
	// empty.
	for _, c := range k.Cores {
		c.CPU.Halted = true
		c.CPU.PendingIRQs = nil
	}
	if k.HasWork() {
		t.Error("HasWork() = true with idle disabled (regression: legacy auto-stop semantics broken)")
	}
}

// TestReset_PreservesIdleTaskEnabled documents that the toggle is a kernel
// preference that survives Reset/LoadProgram, while the per-instance state
// (IdlePID, Processes[0]) is cleared so a fresh LoadProgram can re-spawn
// PID 0 cleanly. Without this contract the user would have to re-tick the
// chip every time they switch demos.
func TestReset_PreservesIdleTaskEnabled(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	k.IdleTaskEnabled = true
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}

	k.Reset()

	if !k.IdleTaskEnabled {
		t.Error("Reset() cleared IdleTaskEnabled; the user preference must survive demo reloads")
	}
	if k.IdlePID != 0 {
		t.Errorf("Reset() left IdlePID=%d; want 0 (the swapper PCB was cleared)", k.IdlePID)
	}
	if len(k.IdleByCore) != 0 {
		t.Error("Reset() left IdleByCore populated; want it cleared so LoadProgram can re-spawn fresh per-core swappers")
	}
}
