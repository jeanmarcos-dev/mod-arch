package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// hasEvent reports whether evs contains any event of type T satisfying the
// predicate. Used in the kernel_work tests so the per-Step assertions read
// as "Step 2 emits an EventWake for init" rather than imperative loops.
func hasEvent[T cpu.Event](evs []cpu.Event, pred func(T) bool) bool {
	for _, e := range evs {
		if v, ok := e.(T); ok && pred(v) {
			return true
		}
	}
	return false
}

// runUntilFirstExitPhase steps the kernel until enqueueExitPhases fires (the
// drain loop pushes phases onto PendingKernelWork) and returns the resulting
// stepwise slice. The cascade itself is NOT drained here — callers can
// inspect PendingKernelWork and drive the remaining phases by hand.
func runUntilFirstExitPhase(t *testing.T, k *Kernel, maxSteps int) [][]cpu.Event {
	t.Helper()
	var out [][]cpu.Event
	for range maxSteps {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("runUntilFirstExitPhase: %v", err)
		}
		out = append(out, evs)
		if len(k.PendingKernelWork) > 0 {
			return out
		}
		// Defensive: if a phase was queued AND drained inside this Step,
		// PendingKernelWork goes back to empty but the cascade is already
		// underway. Detect via a ZOMBIE event in the just-emitted batch.
		if hasEvent(evs, func(e cpu.EventProcessState) bool { return e.State == string(StateZombie) }) {
			return out
		}
	}
	t.Fatalf("runUntilFirstExitPhase: no exit cascade observed within %d Steps", maxSteps)
	return out
}

// TestKernelWork_CleanExitWithoutSibling exercises the canonical case the
// refactor was built for: a single user process exits while init is its
// parent. The cascade lands across distinct Steps so the alumno can click
// through mark_zombie → notify_parent (wake init) → schedule_out
// (CoreRelease) → reap_cycle (init reaps inline) → halt.
func TestKernelWork_CleanExitWithoutSibling(t *testing.T) {
	exitProg := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
		0xBB, 0x2A, 0x00, 0x00, 0x00, // mov ebx, 42
		0xCD, 0x80, // int 0x80
	}
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	pid, err := k.AddProcess("p", exitProg)
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	k.AdoptUnderInit(pid)
	k.TimerQuantum = 0

	// Step until the INT 0x80 fires and the cascade is queued. The Step
	// that issues sys_exit also drains phase 1 (mark_zombie).
	stepwise := runUntilFirstExitPhase(t, k, 30)
	// Drain the remaining four phases.
	stepwise = append(stepwise, stepUntilWorkDrained(t, k, 8)...)

	// Locate the Step that holds each canonical phase-defining event.
	zombieStep, wakeStep, releaseStep, reapStep, haltStep := -1, -1, -1, -1, -1
	for i, evs := range stepwise {
		if zombieStep == -1 && hasEvent(evs, func(e cpu.EventProcessState) bool {
			return e.PID == pid && e.State == string(StateZombie)
		}) {
			zombieStep = i
		}
		if wakeStep == -1 && hasEvent(evs, func(e cpu.EventWake) bool {
			return e.PID == k.InitPID
		}) {
			wakeStep = i
		}
		if releaseStep == -1 && hasEvent(evs, func(e cpu.EventCoreRelease) bool {
			return e.PID == pid
		}) {
			releaseStep = i
		}
		if reapStep == -1 && hasEvent(evs, func(e cpu.EventInitReap) bool {
			return len(e.Reaps) > 0 && e.Reaps[0].ChildPID == pid
		}) {
			reapStep = i
		}
		if haltStep == -1 && hasEvent(evs, func(e cpu.EventHalt) bool { return true }) {
			haltStep = i
		}
	}
	if zombieStep < 0 || wakeStep < 0 || releaseStep < 0 || reapStep < 0 || haltStep < 0 {
		t.Fatalf("phase Steps: ZOMBIE=%d WAKE=%d RELEASE=%d REAP=%d HALT=%d (one or more not found)", zombieStep, wakeStep, releaseStep, reapStep, haltStep)
	}
	if !(zombieStep < wakeStep && wakeStep < releaseStep && releaseStep < reapStep && reapStep < haltStep) {
		t.Errorf("phase Steps not strictly increasing: ZOMBIE=%d WAKE=%d RELEASE=%d REAP=%d HALT=%d", zombieStep, wakeStep, releaseStep, reapStep, haltStep)
	}
	if !k.BootCPU().Halted {
		t.Error("boot CPU not halted after the cascade")
	}
	if len(k.PendingKernelWork) != 0 {
		t.Errorf("PendingKernelWork = %+v, want empty", k.PendingKernelWork)
	}
}

// TestKernelWork_WithRunnableSibling verifies that when a sibling is ready
// to take the dying task's slot, phases 4 (reap_cycle) and 5 (halt) collapse
// transparently — the sibling has no PendingReaps and the core is no longer
// idle, so those Steps emit no events and the user does not see them.
func TestKernelWork_WithRunnableSibling(t *testing.T) {
	exitProg := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
		0xBB, 0x05, 0x00, 0x00, 0x00, // mov ebx, 5
		0xCD, 0x80, // int 0x80
	}
	loopProg := []byte{0x90, 0xEB, 0xFE} // nop ; jmp -2

	k := newTestKernelMulti(t, exitProg, loopProg)
	k.TimerQuantum = 0

	stepwise := runUntilFirstExitPhase(t, k, 30)
	stepwise = append(stepwise, stepUntilWorkDrained(t, k, 8)...)

	// Count Steps that emitted ANY event AFTER the ZOMBIE Step — those are
	// the phases the alumno actually sees. With a runnable sibling we
	// expect exactly two more Steps with content: notify_parent collapses
	// (no waiting parent), schedule_out emits SchedDecision+CoreRelease+
	// PCBRestore+CoreAssign+RUN. reap_cycle collapses (sibling has no
	// PendingReaps) and halt collapses (core has the sibling running).
	zombieStep := -1
	for i, evs := range stepwise {
		if hasEvent(evs, func(e cpu.EventProcessState) bool { return e.State == string(StateZombie) }) {
			zombieStep = i
			break
		}
	}
	if zombieStep < 0 {
		t.Fatal("never observed ZOMBIE")
	}
	visiblePhases := 0
	for i := zombieStep + 1; i < len(stepwise); i++ {
		if len(stepwise[i]) > 0 {
			visiblePhases++
		}
	}
	// schedule_out is the one visible follow-up phase. Note: drainOne
	// collapses empty phases within ONE Step, so the dispatch sibling lands
	// in the next visible Step after the ZOMBIE.
	if visiblePhases > 1 {
		t.Errorf("visible follow-up phases = %d, want 1 (notify_parent/reap_cycle/halt should collapse with a runnable sibling)", visiblePhases)
	}
	if k.BootCore().CurrentPID != 2 {
		t.Errorf("CurrentPID = %d, want 2 (sibling dispatched)", k.BootCore().CurrentPID)
	}
	if k.BootCPU().Halted {
		t.Error("core halted while a sibling was runnable")
	}
}

// TestKernelWork_ResetMidExit verifies that Reset clears any half-drained
// cascade. Without this guard a fresh LoadProgram issued mid-exit would see
// ghost phases targeting PIDs that no longer exist.
func TestKernelWork_ResetMidExit(t *testing.T) {
	exitProg := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00,
		0xBB, 0x07, 0x00, 0x00, 0x00,
		0xCD, 0x80,
	}
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	pid, err := k.AddProcess("p", exitProg)
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	k.AdoptUnderInit(pid)
	k.TimerQuantum = 0

	_ = runUntilFirstExitPhase(t, k, 30)
	// At this point the cascade has been queued and phase 1 drained. At
	// least one phase must still be pending.
	if len(k.PendingKernelWork) == 0 {
		t.Fatal("no PendingKernelWork after triggering exit — test setup is wrong")
	}

	k.Reset()
	if len(k.PendingKernelWork) != 0 {
		t.Errorf("PendingKernelWork = %+v after Reset, want empty", k.PendingKernelWork)
	}
}

// TestKernelWork_FaultDeathQueuesPhases verifies that the page-fault and #GP
// rutas de muerte stage the same five phases as sys_exit — the convergence
// on exitProcess is the whole point of the killAndNotify migration.
func TestKernelWork_FaultDeathQueuesPhases(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	pid, err := k.AddProcess("p", []byte{0xF4}) // HLT
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	k.AdoptUnderInit(pid)

	boot := k.BootCore()
	boot.CurrentPID = pid
	k.Processes[pid].State = StateRun
	k.Processes[pid].Core = boot.ID
	k.Ready.Remove(pid)

	k.setFaultExitCode(boot, SIGSEGV)
	_ = k.killAndNotify(boot, "page_fault")

	// killAndNotify enqueues five phases and drains the first inline.
	// Four must remain queued for the alumno's subsequent clicks.
	if got, want := len(k.PendingKernelWork), 4; got != want {
		t.Errorf("PendingKernelWork len = %d, want %d (one drained inline, four pending)", got, want)
	}
	expected := []kernelWorkKind{workNotifyParent, workScheduleOut, workReapCycle, workHalt}
	for i, want := range expected {
		if i >= len(k.PendingKernelWork) {
			break
		}
		if got := k.PendingKernelWork[i].kind; got != want {
			t.Errorf("phase[%d] kind = %s, want %s", i, got, want)
		}
		if got := k.PendingKernelWork[i].reason; got != "page_fault" {
			t.Errorf("phase[%d] reason = %q, want %q", i, got, "page_fault")
		}
	}
}
