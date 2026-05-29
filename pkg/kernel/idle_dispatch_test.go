package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestDispatchIdleCores_NoFallbackWhenDisabled guards the legacy default-OFF
// path. Without IdleTaskEnabled, DispatchIdleCores must leave idle cores
// untouched even if Processes[0] happens to exist — the flag, not the PCB,
// gates the swapper from owning hardware. A regression here would silently
// activate idle for existing demos.
func TestDispatchIdleCores_NoFallbackWhenDisabled(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	// Flag remains false.
	core := k.Cores[0]

	evs := k.DispatchIdleCores()

	if core.RunningIdle {
		t.Error("Core.RunningIdle = true after DispatchIdleCores with idle disabled")
	}
	for _, e := range evs {
		if _, ok := e.(cpu.EventCoreIdle); ok {
			t.Errorf("DispatchIdleCores emitted EventCoreIdle with idle disabled: %T", e)
		}
	}
}

// TestDispatchIdleCores_BindsIdleWhenNoReady is the headline behavior: with
// idle enabled and the ready queue empty, every core without a real process
// receives PID 0. This is what keeps the run loop alive when user procs are
// all blocked or done.
func TestDispatchIdleCores_BindsIdleWhenNoReady(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	if _, err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	k.IdleTaskEnabled = true

	evs := k.DispatchIdleCores()

	for _, c := range k.Cores {
		if !c.RunningIdle {
			t.Errorf("Core %d not marked RunningIdle after DispatchIdleCores", c.ID)
		}
		if c.CurrentPID != 0 {
			t.Errorf("Core %d.CurrentPID = %d after DispatchIdleCores, want 0", c.ID, c.CurrentPID)
		}
	}

	idleEvents := 0
	for _, e := range evs {
		if _, ok := e.(cpu.EventCoreIdle); ok {
			idleEvents++
		}
	}
	if idleEvents != len(k.Cores) {
		t.Errorf("got %d EventCoreIdle, want %d (one per core)", idleEvents, len(k.Cores))
	}
}

// TestDispatchIdleCores_Idempotent guards against the WS-saturation bug:
// once idle is bound to a core, repeated DispatchIdleCores calls (one per
// Step in the run loop) must NOT re-emit EventCoreIdle or call
// loadCPUFromPCB again. Otherwise every tick would reset Halted=false and
// re-run HLT, flooding the event stream and breaking the wait-for-IRQ
// semantics of the swapper.
func TestDispatchIdleCores_Idempotent(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	k.IdleTaskEnabled = true

	// First pass: legitimately binds.
	_ = k.DispatchIdleCores()

	// Second pass: must be a no-op event-wise.
	evs := k.DispatchIdleCores()
	for _, e := range evs {
		if _, ok := e.(cpu.EventCoreIdle); ok {
			t.Errorf("second DispatchIdleCores re-emitted EventCoreIdle: idle binding is not idempotent")
		}
	}
}

// TestDispatchIdleCores_EvictsIdleForRealProc covers preemption: a core
// happily running PID 0 must yield when a user proc joins the ready queue.
// The kernel emits EventCoreRelease{PID:0, "idle preempted"} so the frontend
// can animate the swap, and the loop's normal assign-from-ready logic then
// binds the new proc on the same tick.
func TestDispatchIdleCores_EvictsIdleForRealProc(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	k.IdleTaskEnabled = true

	// First dispatch: core gets idle.
	_ = k.DispatchIdleCores()
	if !k.Cores[0].RunningIdle {
		t.Fatal("setup: core not running idle after first dispatch")
	}

	// Add a real user proc to the ready queue.
	pid, err := k.AddProcess("demo", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}

	// Second dispatch: must preempt idle, bind the user proc, and emit
	// EventCoreRelease for PID 0.
	evs := k.DispatchIdleCores()

	if k.Cores[0].RunningIdle {
		t.Error("core still marked RunningIdle after a real proc joined READY")
	}
	if k.Cores[0].CurrentPID != pid {
		t.Errorf("core.CurrentPID = %d after dispatch, want %d (the new proc)", k.Cores[0].CurrentPID, pid)
	}

	var sawRelease bool
	for _, e := range evs {
		if rel, ok := e.(cpu.EventCoreRelease); ok && rel.PID == 0 {
			sawRelease = true
		}
	}
	if !sawRelease {
		t.Error("DispatchIdleCores did not emit EventCoreRelease{PID:0} when preempting idle")
	}
}
