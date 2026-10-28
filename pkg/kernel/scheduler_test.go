package kernel

import (
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// TestDispatchIdleCores_AssignsReadyToIdle covers the canonical scenario:
// three procs loaded onto a 2-core kernel. Two go to RUN, one stays in the
// ready queue, and ten events are emitted (SchedDecision + PCBRestore +
// CoreAssign + ProcessState + ModeSwitch per dispatched proc). The final
// SchedDecision must carry the post-pop ready queue so the frontend never
// sees a RUN process still queued.
func TestDispatchIdleCores_AssignsReadyToIdle(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	if _, err := k.AddProcess("p1", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.AddProcess("p2", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.AddProcess("p3", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}

	evs := k.DispatchIdleCores()

	if k.Cores[0].CurrentPID != 1 || k.Cores[1].CurrentPID != 2 {
		t.Fatalf("cores = [%d,%d], want [1,2]", k.Cores[0].CurrentPID, k.Cores[1].CurrentPID)
	}
	if k.Processes[1].State != StateRun || k.Processes[2].State != StateRun {
		t.Errorf("p1/p2 states = %s/%s, want RUN/RUN", k.Processes[1].State, k.Processes[2].State)
	}
	if k.Processes[3].State != StateReady {
		t.Errorf("p3 state = %s, want READY", k.Processes[3].State)
	}
	if snap := k.Ready.Snapshot(); len(snap) != 1 || snap[0] != 3 {
		t.Errorf("ready queue = %v, want [3]", snap)
	}
	if len(evs) != 10 {
		t.Errorf("events emitted = %d, want 10 (5 per dispatched proc)", len(evs))
	}
	var modeSwitches []cpu.EventModeSwitch
	for _, e := range evs {
		if ms, ok := e.(cpu.EventModeSwitch); ok {
			modeSwitches = append(modeSwitches, ms)
		}
	}
	if len(modeSwitches) != 2 {
		t.Fatalf("ModeSwitch events = %d, want 2", len(modeSwitches))
	}
	for _, ms := range modeSwitches {
		if ms.Kind != "SW" || ms.Cause != "dispatch" {
			t.Errorf("ModeSwitch = %+v, want Kind=SW Cause=dispatch", ms)
		}
		if ms.From != cpu.ModeKernel || ms.To != cpu.ModeUser {
			t.Errorf("ModeSwitch From/To = %d/%d, want 0/3", ms.From, ms.To)
		}
	}
	// Every dispatched proc must carry one SchedDecision; the last one must
	// reflect the post-pop ready queue (just P3 left).
	var schedDecisions []cpu.EventSchedDecision
	for _, e := range evs {
		if sd, ok := e.(cpu.EventSchedDecision); ok {
			schedDecisions = append(schedDecisions, sd)
		}
	}
	if len(schedDecisions) != 2 {
		t.Fatalf("SchedDecision events = %d, want 2", len(schedDecisions))
	}
	last := schedDecisions[len(schedDecisions)-1]
	if last.Reason != "dispatch" || last.ToPID != 2 {
		t.Errorf("last SchedDecision = %+v, want Reason=dispatch ToPID=2", last)
	}
	if len(last.ReadyQueue) != 1 || last.ReadyQueue[0] != 3 {
		t.Errorf("last SchedDecision ready queue = %v, want [3]", last.ReadyQueue)
	}
	// Cores assigned must come out of halt so HasWork sees them.
	if k.Cores[0].CPU.Halted || k.Cores[1].CPU.Halted {
		t.Errorf("cores halted after dispatch: %v/%v", k.Cores[0].CPU.Halted, k.Cores[1].CPU.Halted)
	}
}

// TestDispatchIdleCores_NoopWhenAllBusy makes the operation idempotent so
// the simulator can safely invoke it at every StepOnce.
func TestDispatchIdleCores_NoopWhenAllBusy(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 1)
	if _, err := k.AddProcess("p1", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}
	_ = k.DispatchIdleCores()    // first call: p1 takes the only core
	evs := k.DispatchIdleCores() // second call: nothing to do
	if evs != nil {
		t.Errorf("expected nil events on noop, got %d events", len(evs))
	}
}

// TestDispatchIdleCores_NoopWhenReadyEmpty guards the path where the kernel
// has free cores but no work to put on them — e.g. a fresh kernel.
func TestDispatchIdleCores_NoopWhenReadyEmpty(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	evs := k.DispatchIdleCores()
	if evs != nil {
		t.Errorf("expected nil events on empty ready queue, got %d events", len(evs))
	}
	for i, c := range k.Cores {
		if c.CurrentPID != 0 {
			t.Errorf("core %d CurrentPID = %d, want 0", i, c.CurrentPID)
		}
	}
}

// TestDispatchIdleCores_StampsCoreAndTick ensures every kernel event emitted
// by the dispatcher carries the CoreID of the core receiving the work and the
// global clock as its Tick. The frontend relies on both fields to place each
// event in its column / row of the multicore log grid.
func TestDispatchIdleCores_StampsCoreAndTick(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	if _, err := k.AddProcess("p1", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.AddProcess("p2", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}

	// Set the global clock non-zero so "stamped" is distinguishable from the
	// zero-value default. Every event of this step shares this single tick.
	k.Clock = 9

	evs := k.DispatchIdleCores()
	if len(evs) == 0 {
		t.Fatal("DispatchIdleCores returned no events")
	}

	for _, e := range evs {
		switch v := e.(type) {
		case cpu.EventSchedDecision:
			wantCore := -1
			switch v.ToPID {
			case 1:
				wantCore = 0
			case 2:
				wantCore = 1
			}
			if v.CoreID != wantCore || v.Tick != k.Clock {
				t.Errorf("SchedDecision ToPID=%d: CoreID=%d Tick=%d, want CoreID=%d Tick=%d",
					v.ToPID, v.CoreID, v.Tick, wantCore, k.Clock)
			}
		case cpu.EventCoreAssign:
			if v.Tick != k.Clock {
				t.Errorf("CoreAssign{CoreID:%d PID:%d}: Tick=%d, want %d", v.CoreID, v.PID, v.Tick, k.Clock)
			}
		case cpu.EventPCBRestore:
			if v.Tick != k.Clock {
				t.Errorf("PCBRestore{CoreID:%d PID:%d}: Tick=%d, want %d", v.CoreID, v.PID, v.Tick, k.Clock)
			}
		case cpu.EventProcessState:
			if v.Tick != k.Clock {
				t.Errorf("ProcessState{PID:%d State:%s}: Tick=%d, want %d", v.PID, v.State, v.Tick, k.Clock)
			}
			if v.CoreID == 0 && v.PID == 2 {
				t.Errorf("ProcessState for PID 2 went to CoreID 0 (expected 1)")
			}
		}
	}
}

// TestStampKernelContext_TargetCorePreserved verifies that events naming a
// target core (CoreAssign, CoreRelease, PCBSave, PCBRestore, MutexOp, SemOp)
// keep the CoreID the caller set, even when stamped by a different executing
// core. This matters when a wake-on-unlock hands a PID to an idle core: the
// stamper must not overwrite idle.ID with the unlocking core's ID.
func TestStampKernelContext_TargetCorePreserved(t *testing.T) {
	evs := []cpu.Event{
		cpu.EventCoreAssign{CoreID: 3, PID: 5},
		cpu.EventPCBRestore{CoreID: 3, PID: 5},
		cpu.EventMutexOp{CoreID: 2, PID: 7, MutexID: 1, Op: "acquired"},
		cpu.EventSchedDecision{ToPID: 5, Reason: "wake"},
	}
	out := stampKernelContext(evs, 1, 42)

	assign := out[0].(cpu.EventCoreAssign)
	if assign.CoreID != 3 || assign.Tick != 42 {
		t.Errorf("CoreAssign: CoreID=%d Tick=%d, want CoreID=3 Tick=42", assign.CoreID, assign.Tick)
	}
	restore := out[1].(cpu.EventPCBRestore)
	if restore.CoreID != 3 || restore.Tick != 42 {
		t.Errorf("PCBRestore: CoreID=%d Tick=%d, want CoreID=3 Tick=42", restore.CoreID, restore.Tick)
	}
	mtx := out[2].(cpu.EventMutexOp)
	if mtx.CoreID != 2 || mtx.Tick != 42 {
		t.Errorf("MutexOp: CoreID=%d Tick=%d, want CoreID=2 Tick=42", mtx.CoreID, mtx.Tick)
	}
	sched := out[3].(cpu.EventSchedDecision)
	if sched.CoreID != 1 || sched.Tick != 42 {
		t.Errorf("SchedDecision (execute-on-core): CoreID=%d Tick=%d, want CoreID=1 Tick=42",
			sched.CoreID, sched.Tick)
	}
}

// TestStampKernelContext_HaltTickStamped guards the Tick stamping for
// EventHalt. Two call sites (keyboard.go, kernel_work.go doHaltPhase) emit
// EventHalt without a Tick and rely on stampKernelContext to fill it; if the
// stamper skips EventHalt the terminal HALT ends up with Tick=0 and the
// EventsLog UI groups it under tick 0 next to the program's first instruction.
// Emitters that already set Tick (sync.go) must be preserved.
func TestStampKernelContext_HaltTickStamped(t *testing.T) {
	evs := []cpu.Event{
		cpu.EventHalt{CoreID: 0, PID: 4},          // unstamped — must be filled
		cpu.EventHalt{CoreID: 0, Tick: 5, PID: 7}, // pre-stamped — must be preserved
	}
	out := stampKernelContext(evs, 0, 42)

	unstamped := out[0].(cpu.EventHalt)
	if unstamped.Tick != 42 {
		t.Errorf("unstamped EventHalt: Tick=%d, want 42 (filled by stamper)", unstamped.Tick)
	}
	preserved := out[1].(cpu.EventHalt)
	if preserved.Tick != 5 {
		t.Errorf("pre-stamped EventHalt: Tick=%d, want 5 (preserved by stamper)", preserved.Tick)
	}
}
