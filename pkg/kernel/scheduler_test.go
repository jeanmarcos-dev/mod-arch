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
