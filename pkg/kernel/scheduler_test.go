package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestDispatchIdleCores_AssignsReadyToIdle covers the canonical scenario:
// three procs loaded onto a 2-core kernel. Two go to RUN, one stays in the
// ready queue, and eight events are emitted (PCBRestore + SchedDecision +
// CoreAssign + ProcessState per dispatched proc). The final SchedDecision
// must carry the post-pop ready queue so the frontend never sees a RUN
// process still queued.
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
	if len(evs) != 8 {
		t.Errorf("events emitted = %d, want 8 (4 per dispatched proc)", len(evs))
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
// by the dispatcher carries the CoreID of the core receiving the work and a
// Tick number sourced from that core's CPU. The frontend relies on both
// fields to place each event in its column / row of the multicore log grid;
// regressing here would silently misalign the timeline view.
func TestDispatchIdleCores_StampsCoreAndTick(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	if _, err := k.AddProcess("p1", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.AddProcess("p2", []byte{0x90, 0xF4}); err != nil {
		t.Fatal(err)
	}

	// Drive a couple of ticks so Tick != 0 distinguishes "stamped" from
	// "zero-value default".
	k.Cores[0].CPU.Tick = 7
	k.Cores[1].CPU.Tick = 11

	evs := k.DispatchIdleCores()
	if len(evs) == 0 {
		t.Fatal("DispatchIdleCores returned no events")
	}

	// Build the expected (CoreID, Tick) per dispatched PID. P1 went to core 0,
	// P2 to core 1, in that order.
	for _, e := range evs {
		switch v := e.(type) {
		case cpu.EventSchedDecision:
			want := struct{ core int; tick uint64 }{}
			switch v.ToPID {
			case 1:
				want = struct{ core int; tick uint64 }{0, 7}
			case 2:
				want = struct{ core int; tick uint64 }{1, 11}
			}
			if v.CoreID != want.core || v.Tick != want.tick {
				t.Errorf("SchedDecision ToPID=%d: CoreID=%d Tick=%d, want CoreID=%d Tick=%d",
					v.ToPID, v.CoreID, v.Tick, want.core, want.tick)
			}
		case cpu.EventCoreAssign:
			if v.Tick == 0 {
				t.Errorf("CoreAssign{CoreID:%d PID:%d}: Tick=0 (not stamped)", v.CoreID, v.PID)
			}
		case cpu.EventPCBRestore:
			if v.Tick == 0 {
				t.Errorf("PCBRestore{CoreID:%d PID:%d}: Tick=0 (not stamped)", v.CoreID, v.PID)
			}
		case cpu.EventProcessState:
			if v.Tick == 0 {
				t.Errorf("ProcessState{PID:%d State:%s}: Tick=0 (not stamped)", v.PID, v.State)
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
