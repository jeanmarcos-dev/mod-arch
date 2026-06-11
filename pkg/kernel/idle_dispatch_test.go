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
	if err := k.SpawnIdle(); err != nil {
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
	if err := k.SpawnIdle(); err != nil {
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
	if err := k.SpawnIdle(); err != nil {
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

// TestDispatchIdleCores_NoEvictionWhenStaysIdle guards against the noisy
// preempt+reassign pattern when an idle core has no user proc to swap to.
// With more cores than ready procs only the cores that will actually be
// dispatched should emit EventCoreRelease; the ones that bounce back to idle
// stay silent.
func TestDispatchIdleCores_NoEvictionWhenStaysIdle(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 4)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	k.IdleTaskEnabled = true

	_ = k.DispatchIdleCores()
	for _, c := range k.Cores {
		if !c.RunningIdle {
			t.Fatalf("setup: core %d not idle after bootstrap", c.ID)
		}
	}

	if _, err := k.AddProcess("p1", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.AddProcess("p2", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}

	evs := k.DispatchIdleCores()

	releases := 0
	for _, e := range evs {
		if rel, ok := e.(cpu.EventCoreRelease); ok && rel.PID == 0 {
			releases++
		}
	}
	if releases != 2 {
		t.Errorf("idle EventCoreRelease count = %d, want 2 (one per dispatched proc)", releases)
	}
}

// TestStepCore_IdleTimerFires covers the local-APIC timer on idle cores: a
// swapper-bound core must receive its timer IRQ every TimerQuantum HLT ticks,
// so a core that sits in idle is not silently isolated from the scheduler.
// The idle gate queues the IRQ and falls through to CPU.Step in the same
// tick — symmetric with user cores' shouldFireTimer path — so the test
// asserts on the dispatched EventIRQFire rather than on PendingIRQs.
func TestStepCore_IdleTimerFires(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 1)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	k.IdleTaskEnabled = true
	k.TimerQuantum = 4
	_ = k.DispatchIdleCores()

	core := k.Cores[0]
	if !core.RunningIdle {
		t.Fatalf("setup: core 0 not RunningIdle")
	}

	var fired *cpu.EventIRQFire
	for i := 0; i < k.TimerQuantum+1; i++ {
		evs, err := k.stepCore(core)
		if err != nil {
			t.Fatalf("stepCore #%d: %v", i, err)
		}
		for j := range evs {
			if irq, ok := evs[j].(cpu.EventIRQFire); ok && irq.Vector == IRQTimer {
				fired = &irq
			}
		}
	}
	if fired == nil {
		t.Fatalf("timer did not fire within %d idle ticks", k.TimerQuantum+1)
	}
	if fired.Source != "HW" {
		t.Errorf("EventIRQFire.Source = %q, want HW", fired.Source)
	}
}

// TestStepCore_IdleHaltTickPreIncrement guards the EventHalt tick stamp on
// the idle gate. The very first stepCore decodes the HLT byte (Tick=0); the
// next call lands in the gate and must stamp EventHalt with Tick=1, matching
// pre-increment semantics used by EventInstruction.
func TestStepCore_IdleHaltStampedWithGlobalClock(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 1)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	k.IdleTaskEnabled = true
	_ = k.DispatchIdleCores()
	core := k.Cores[0]

	if _, err := k.stepCore(core); err != nil {
		t.Fatalf("stepCore #0 (HLT): %v", err)
	}
	k.Clock = 5
	evs, err := k.stepCore(core)
	if err != nil {
		t.Fatalf("stepCore #1 (idle gate): %v", err)
	}
	var halt *cpu.EventHalt
	for i := range evs {
		if h, ok := evs[i].(cpu.EventHalt); ok {
			halt = &h
			break
		}
	}
	if halt == nil {
		t.Fatalf("no EventHalt emitted from the idle gate")
	}
	if halt.Tick != k.Clock {
		t.Errorf("idle gate EventHalt.Tick = %d, want %d (global clock)", halt.Tick, k.Clock)
	}
}

// TestDispatchIdleCores_EvictsIdleForRealProc covers preemption: a core
// happily running PID 0 must yield when a user proc joins the ready queue.
// The kernel emits EventCoreRelease{PID:0, "idle preempted"} so the frontend
// can animate the swap, and the loop's normal assign-from-ready logic then
// binds the new proc on the same tick.
func TestDispatchIdleCores_EvictsIdleForRealProc(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if err := k.SpawnIdle(); err != nil {
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
