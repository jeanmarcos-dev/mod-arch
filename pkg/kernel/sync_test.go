package kernel

import (
	"slices"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// These unit tests exercise the mutex/semaphore primitives directly. They
// verify object state (Holder, Waiters, Count) and event ops rather than
// the deeper PCB/scheduler interaction, which has its own end-to-end
// coverage via the sync demos.

func freshKernelWithProcs(t *testing.T, numProcs int) *Kernel {
	t.Helper()
	k := New(cpu.DefaultMemSize)
	k.TimerQuantum = 0
	for i := 1; i <= numProcs; i++ {
		// Each process is just hlt; we never actually step them, we only
		// need a PCB the kernel can refer to.
		if _, err := k.AddProcess("p", []byte{0xF4}); err != nil {
			t.Fatalf("AddProcess: %v", err)
		}
	}
	// These tests assert against k.BootCore().CurrentPID, which the
	// simulator wires up via DispatchIdleCores before any tick runs.
	k.DispatchIdleCores()
	return k
}

func TestMutex_CreateReturnsIncrementingID(t *testing.T) {
	k := freshKernelWithProcs(t, 1)
	for want := 1; want <= 3; want++ {
		got := k.mutexCreate()
		if got != want {
			t.Errorf("mutexCreate #%d = %d, want %d", want, got, want)
		}
	}
}

func TestMutex_AcquireFromFree(t *testing.T) {
	k := freshKernelWithProcs(t, 1)
	id := k.mutexCreate()

	evs, blocked, ok := k.mutexLock(k.BootCore(), id)
	if !ok {
		t.Fatal("mutexLock ok=false")
	}
	if blocked {
		t.Fatal("mutexLock on free mutex should not block")
	}
	if k.Mutexes[id-1].Holder != 1 {
		t.Errorf("Holder = %d, want 1", k.Mutexes[id-1].Holder)
	}
	if got := mutexOps(evs); len(got) != 1 || got[0] != "acquired" {
		t.Errorf("ops = %v, want [acquired]", got)
	}
}

func TestMutex_ContentionEnqueuesWaiter(t *testing.T) {
	k := freshKernelWithProcs(t, 2)
	id := k.mutexCreate()
	// PID 1 owns the lock (boot core).
	_, _, _ = k.mutexLock(k.BootCore(), id)
	// Pretend PID 2 is running and tries to lock. We only flip the core's
	// CurrentPID; PCB state bookkeeping is exercised separately.
	k.BootCore().CurrentPID = 2

	_, blocked, ok := k.mutexLock(k.BootCore(), id)
	if !ok {
		t.Fatal("PID 2 lock ok=false")
	}
	if !blocked {
		t.Fatal("PID 2 lock did not block while PID 1 holds")
	}
	m := k.Mutexes[id-1]
	if m.Holder != 1 {
		t.Errorf("Holder = %d, want 1 (PID 1 still holds)", m.Holder)
	}
	if len(m.Waiters) != 1 || m.Waiters[0] != 2 {
		t.Errorf("Waiters = %v, want [2]", m.Waiters)
	}
}

func TestMutex_UnlockReleasesWhenNoWaiter(t *testing.T) {
	k := freshKernelWithProcs(t, 1)
	id := k.mutexCreate()
	_, _, _ = k.mutexLock(k.BootCore(), id)

	evs, ok := k.mutexUnlock(k.BootCore(), id)
	if !ok {
		t.Fatal("unlock ok=false")
	}
	if k.Mutexes[id-1].Holder != -1 {
		t.Errorf("Holder = %d, want -1", k.Mutexes[id-1].Holder)
	}
	if got := mutexOps(evs); len(got) != 1 || got[0] != "released" {
		t.Errorf("ops = %v, want [released]", got)
	}
}

func TestMutex_UnlockHandsOffToFirstWaiter(t *testing.T) {
	k := freshKernelWithProcs(t, 2)
	id := k.mutexCreate()
	_, _, _ = k.mutexLock(k.BootCore(), id) // PID 1 holds
	k.BootCore().CurrentPID = 2
	_, _, _ = k.mutexLock(k.BootCore(), id) // PID 2 waits
	// PID 1 unlocks.
	k.BootCore().CurrentPID = 1

	evs, ok := k.mutexUnlock(k.BootCore(), id)
	if !ok {
		t.Fatal("unlock ok=false")
	}
	m := k.Mutexes[id-1]
	if m.Holder != 2 {
		t.Errorf("Holder after handoff = %d, want 2", m.Holder)
	}
	if len(m.Waiters) != 0 {
		t.Errorf("Waiters after handoff = %v, want []", m.Waiters)
	}
	if got := mutexOps(evs); len(got) == 0 || got[0] != "handoff" {
		t.Errorf("ops = %v, want first [handoff]", got)
	}
}

func TestMutex_UnknownIDFails(t *testing.T) {
	k := freshKernelWithProcs(t, 1)
	if _, _, ok := k.mutexLock(k.BootCore(), 99); ok {
		t.Error("mutexLock(99) ok=true, want false")
	}
	if _, ok := k.mutexUnlock(k.BootCore(), 99); ok {
		t.Error("mutexUnlock(99) ok=true, want false")
	}
}

func TestMutex_UnlockByNonHolderFails(t *testing.T) {
	k := freshKernelWithProcs(t, 2)
	id := k.mutexCreate()
	_, _, _ = k.mutexLock(k.BootCore(), id) // PID 1 holds
	k.BootCore().CurrentPID = 2

	if _, ok := k.mutexUnlock(k.BootCore(), id); ok {
		t.Error("unlock by non-holder ok=true, want false")
	}
}

func TestMutex_UnlockByNonHolderEmitsRejectedEvent(t *testing.T) {
	k := freshKernelWithProcs(t, 2)
	id := k.mutexCreate()
	_, _, _ = k.mutexLock(k.BootCore(), id) // PID 1 holds
	k.BootCore().CurrentPID = 2             // PID 2 attempts unlock

	evs, ok := k.mutexUnlock(k.BootCore(), id)
	if ok {
		t.Fatal("unlock by non-holder ok=true, want false")
	}
	ops := mutexOps(evs)
	if len(ops) != 1 || ops[0] != "rejected" {
		t.Errorf("ops = %v, want [rejected]", ops)
	}
	// Mutex state must be unchanged: PID 1 still holds, no waiters.
	m := k.Mutexes[id-1]
	if m.Holder != 1 {
		t.Errorf("Holder = %d, want 1 (unchanged)", m.Holder)
	}
	if len(m.Waiters) != 0 {
		t.Errorf("Waiters = %v, want []", m.Waiters)
	}
	// The emitted event must name the offender (PID 2), not the holder.
	for _, e := range evs {
		if mo, isMutex := e.(cpu.EventMutexOp); isMutex && mo.Op == "rejected" {
			if mo.PID != 2 {
				t.Errorf("rejected event PID = %d, want 2 (the attempted unlocker)", mo.PID)
			}
			if mo.MutexID != id {
				t.Errorf("rejected event MutexID = %d, want %d", mo.MutexID, id)
			}
		}
	}
}

func TestSemaphore_WaitImmediateDecrements(t *testing.T) {
	k := freshKernelWithProcs(t, 1)
	id := k.semCreate(2)

	for i, wantCount := range []int{1, 0} {
		evs, blocked, ok := k.semWait(k.BootCore(), id)
		if !ok || blocked {
			t.Fatalf("wait #%d: ok=%v blocked=%v, want ok=true blocked=false", i, ok, blocked)
		}
		if k.Semaphores[id-1].Count != wantCount {
			t.Errorf("wait #%d: count = %d, want %d", i, k.Semaphores[id-1].Count, wantCount)
		}
		if got := semOps(evs); len(got) != 1 || got[0] != "wait_immediate" {
			t.Errorf("wait #%d: ops = %v, want [wait_immediate]", i, got)
		}
	}
}

func TestSemaphore_WaitBlocksWhenZero(t *testing.T) {
	k := freshKernelWithProcs(t, 2)
	id := k.semCreate(0)

	_, blocked, ok := k.semWait(k.BootCore(), id)
	if !ok || !blocked {
		t.Fatalf("wait on zero: ok=%v blocked=%v", ok, blocked)
	}
	s := k.Semaphores[id-1]
	if len(s.Waiters) != 1 || s.Waiters[0] != 1 {
		t.Errorf("Waiters = %v, want [1]", s.Waiters)
	}
	if s.Count != 0 {
		t.Errorf("count = %d, want 0", s.Count)
	}
}

func TestSemaphore_SignalIncrementsWhenNoWaiters(t *testing.T) {
	k := freshKernelWithProcs(t, 1)
	id := k.semCreate(0)

	evs, ok := k.semSignal(k.BootCore(), id)
	if !ok {
		t.Fatal("signal ok=false")
	}
	if k.Semaphores[id-1].Count != 1 {
		t.Errorf("count after signal = %d, want 1", k.Semaphores[id-1].Count)
	}
	if got := semOps(evs); len(got) != 1 || got[0] != "signal" {
		t.Errorf("ops = %v, want [signal]", got)
	}
}

func TestSemaphore_SignalHandsOffFirstWaiter(t *testing.T) {
	k := freshKernelWithProcs(t, 2)
	id := k.semCreate(0)

	// PID 1 waits and blocks.
	_, _, _ = k.semWait(k.BootCore(), id)
	// PID 2 signals.
	k.BootCore().CurrentPID = 2

	evs, ok := k.semSignal(k.BootCore(), id)
	if !ok {
		t.Fatal("signal ok=false")
	}
	s := k.Semaphores[id-1]
	if s.Count != 0 {
		t.Errorf("count after handoff = %d, want 0", s.Count)
	}
	if len(s.Waiters) != 0 {
		t.Errorf("Waiters after handoff = %v, want []", s.Waiters)
	}
	if got := semOps(evs); len(got) == 0 || got[0] != "handoff" {
		t.Errorf("ops = %v, want first [handoff]", got)
	}
}

func TestSemaphore_UnknownIDFails(t *testing.T) {
	k := freshKernelWithProcs(t, 1)
	if _, _, ok := k.semWait(k.BootCore(), 99); ok {
		t.Error("semWait(99) ok=true, want false")
	}
	if _, ok := k.semSignal(k.BootCore(), 99); ok {
		t.Error("semSignal(99) ok=true, want false")
	}
}

// TestMutexUnlock_NoIdleCore_EmitsReadyQueueSnapshot exercises the "no idle
// core" branch of wakeWaiter: with only one core (Boot Core, busy holding the
// holder PID), unlocking must push the next waiter onto the ready queue AND
// announce the change to the WS client via a sched_decision carrying the
// updated ReadyQueue. Without that announcement, cpu.readyQueue in the
// frontend stays empty and SimBar's haltedDead heuristic disables STEP even
// though the kernel still has work to do.
func TestMutexUnlock_NoIdleCore_EmitsReadyQueueSnapshot(t *testing.T) {
	k := freshKernelWithProcs(t, 2)
	id := k.mutexCreate()

	// PID 1 owns the mutex (Boot Core, CurrentPID=1).
	if _, _, ok := k.mutexLock(k.BootCore(), id); !ok {
		t.Fatal("PID 1 mutexLock ok=false")
	}

	// Stage PID 2 trying to lock. Remove it from the ready queue first so
	// blockOnSync does not accidentally pop PID 2 itself as the "next"
	// runnable and switch the core back to a process we are blocking.
	k.Ready.Remove(2)
	k.BootCore().CurrentPID = 2
	if _, blocked, ok := k.mutexLock(k.BootCore(), id); !ok || !blocked {
		t.Fatalf("PID 2 mutexLock: ok=%v blocked=%v, want both true", ok, blocked)
	}

	// Restore Boot Core to PID 1 so the unlock comes from the legitimate
	// holder, and keep the core busy so firstIdleCore returns nil — that is
	// the path under test.
	k.BootCore().CurrentPID = 1
	k.BootCore().CPU.Halted = false

	evs, ok := k.mutexUnlock(k.BootCore(), id)
	if !ok {
		t.Fatal("PID 1 mutexUnlock ok=false")
	}

	// PID 2 must be on the ready queue (handoff via wakeWaiter's
	// no-idle-core branch).
	if !slices.Contains(k.Ready.Snapshot(), 2) {
		t.Fatalf("PID 2 not on ready queue after unlock: %v", k.Ready.Snapshot())
	}

	// And the emitted events must carry the ready-queue snapshot so the
	// frontend can refresh cpu.readyQueue.
	var sawWakeDecision bool
	for _, e := range evs {
		sd, isSched := e.(cpu.EventSchedDecision)
		if !isSched {
			continue
		}
		if sd.ToPID != 2 {
			continue
		}
		if !slices.Contains(sd.ReadyQueue, 2) {
			t.Errorf("sched_decision ReadyQueue = %v, want it to include PID 2", sd.ReadyQueue)
		}
		sawWakeDecision = true
	}
	if !sawWakeDecision {
		t.Errorf("wakeWaiter on no-idle-core path emitted no EventSchedDecision — frontend cpu.readyQueue will drift out of sync with k.Ready")
	}
}

// TestMutex_BlockedSinceStampedAndCleared verifies the PCB carries the CPU
// tick at which it entered BLOCKED so the frontend can render "esperando hace
// N ticks", and that the field is cleared once the waiter is woken.
func TestMutex_BlockedSinceStampedAndCleared(t *testing.T) {
	k := freshKernelWithProcs(t, 2)
	id := k.mutexCreate()
	_, _, _ = k.mutexLock(k.BootCore(), id)

	// Advance the global clock so BlockedSince is provably non-zero.
	k.Clock = 42
	k.BootCore().CurrentPID = 2

	_, blocked, _ := k.mutexLock(k.BootCore(), id)
	if !blocked {
		t.Fatal("expected PID 2 to block while PID 1 holds")
	}
	p2 := k.Processes[2]
	if p2.BlockedSince != 42 {
		t.Errorf("BlockedSince = %d, want 42", p2.BlockedSince)
	}

	k.BootCore().CurrentPID = 1
	if _, ok := k.mutexUnlock(k.BootCore(), id); !ok {
		t.Fatal("unlock ok=false")
	}
	if p2.BlockedSince != 0 {
		t.Errorf("BlockedSince after handoff = %d, want 0", p2.BlockedSince)
	}
}

func mutexOps(evs []cpu.Event) []string {
	var out []string
	for _, e := range evs {
		if m, ok := e.(cpu.EventMutexOp); ok {
			out = append(out, m.Op)
		}
	}
	return out
}

func semOps(evs []cpu.Event) []string {
	var out []string
	for _, e := range evs {
		if s, ok := e.(cpu.EventSemOp); ok {
			out = append(out, s.Op)
		}
	}
	return out
}
