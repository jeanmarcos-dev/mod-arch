package kernel

import (
	"slices"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// TestNotifyParentOfExit_DefersTheReap verifies that when a child exits and
// the parent is blocked in wait(), the child PCB is NOT removed from the
// process table synchronously. The zombie must remain visible (state ZOMBIE,
// still in k.Processes) for at least one scheduler dispatch cycle so the
// alumno can observe the zombie before init / parent reaps it. The reap is
// queued on parent.PendingReaps and consumed when the parent is dispatched.
func TestNotifyParentOfExit_DefersTheReap(t *testing.T) {
	k := New(cpu.DefaultMemSize)

	parentPID, err := k.AddProcess("parent", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess parent: %v", err)
	}
	childPID, err := k.AddProcess("child", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess child: %v", err)
	}

	parent := k.Processes[parentPID]
	child := k.Processes[childPID]
	parent.Children = []int{childPID}
	child.ParentPID = parentPID

	// Park the parent in blocked-on-wait, the state it would be in after
	// having called sys_waitpid(-1) with no zombie ready.
	parent.State = StateBlocked
	parent.BlockedOn = waitpidBlockedReason(-1)
	parent.Waiter = -1
	k.Ready.Remove(parentPID)

	// The child reaches ZOMBIE the moment sys_exit fires; notifyParentOfExit
	// runs afterwards.
	child.State = StateZombie
	child.ExitCode = 42

	evs := k.notifyParentOfExit(child)

	if _, stillThere := k.Processes[childPID]; !stillThere {
		t.Fatalf("child PCB removed too eagerly — deferred reap must keep ZOMBIE in the table until parent is dispatched")
	}
	if child.State != StateZombie {
		t.Errorf("child.State = %s, want ZOMBIE while reap is deferred", child.State)
	}
	if len(parent.PendingReaps) != 1 || parent.PendingReaps[0] != childPID {
		t.Errorf("parent.PendingReaps = %v, want [%d]", parent.PendingReaps, childPID)
	}
	if parent.State != StateReady {
		t.Errorf("parent.State = %s, want READY (woken to queue)", parent.State)
	}

	// No reap event should have been emitted yet; only ZOMBIE-related
	// notifications and the parent's wake to READY.
	for _, e := range evs {
		if w, ok := e.(cpu.EventWaitOp); ok && w.Op == "reap" {
			t.Errorf("EventWaitOp{Op:reap} emitted synchronously — reap must wait for dispatch (got %+v)", w)
		}
	}
}

// TestDispatch_ConsumesPendingReap covers the second half of the deferred
// reap protocol: once the parent is picked from the ready queue by
// DispatchIdleCores, the kernel drains PendingReaps before transitioning
// the parent to RUN. The reap event fires here and the zombie disappears.
func TestDispatch_ConsumesPendingReap(t *testing.T) {
	k := New(cpu.DefaultMemSize)

	parentPID, err := k.AddProcess("parent", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess parent: %v", err)
	}
	childPID, err := k.AddProcess("child", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess child: %v", err)
	}
	parent := k.Processes[parentPID]
	child := k.Processes[childPID]
	parent.Children = []int{childPID}
	child.ParentPID = parentPID
	child.State = StateZombie
	child.ExitCode = 99

	// Empty the ready queue (AddProcess pushes both PIDs); rebuild it with
	// only the parent so DispatchIdleCores picks the parent first.
	k.Ready.Reset()
	parent.PendingReaps = []int{childPID}
	parent.State = StateReady
	parent.BlockedOn = ""
	k.Ready.Push(parentPID)

	// Release the boot core so DispatchIdleCores has somewhere to put the
	// parent. The kernel sets CurrentPID at AddProcess time via the boot
	// dispatch path, but here we built state by hand and need the slot.
	boot := k.BootCore()
	boot.CurrentPID = 0
	boot.CPU.Halted = true

	evs := k.DispatchIdleCores()

	if _, stillThere := k.Processes[childPID]; stillThere {
		t.Errorf("child PCB still in table after dispatch — pending reap was not consumed")
	}
	if len(parent.PendingReaps) != 0 {
		t.Errorf("parent.PendingReaps = %v after dispatch, want [] (drained)", parent.PendingReaps)
	}
	if parent.Regs.EAX != uint32(childPID) {
		t.Errorf("parent.Regs.EAX = %d after reap, want %d (waitpid returns the reaped PID)", parent.Regs.EAX, childPID)
	}
	reapSeen := false
	for _, e := range evs {
		if w, ok := e.(cpu.EventWaitOp); ok && w.Op == "reap" && w.ChildPID == childPID {
			reapSeen = true
			if w.ExitCode != 99 {
				t.Errorf("reap event ExitCode = %d, want 99", w.ExitCode)
			}
		}
	}
	if !reapSeen {
		t.Error("DispatchIdleCores did not emit EventWaitOp{Op:reap} when draining pending reaps")
	}
}

// TestDispatch_InitCollectsOrphanAndReblocks covers the most distinctive
// piece of the new model: when init is dispatched with pending reaps it
// briefly transitions to RUN, drains the queue (one reap event per orphan),
// and goes back to BLOCKED("wait()") instead of executing its hlt program.
// This is what makes the "init woke up, cleaned up, went back to sleep"
// sequence visible in the event log.
func TestDispatch_InitCollectsOrphanAndReblocks(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	initPID, err := k.SpawnInit()
	if err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}

	orphanPID, err := k.AddProcess("orphan", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess orphan: %v", err)
	}
	orphan := k.Processes[orphanPID]
	orphan.ParentPID = initPID
	// The orphan in this test simulates a child whose original parent died
	// and was reparented to init by markZombieAndReparent. The real flow
	// would set this flag inside that function; we set it by hand here so
	// the EventInitReap surfaces Reparented=true.
	orphan.WasReparentedToInit = true
	orphan.State = StateZombie
	orphan.ExitCode = 7
	init := k.Processes[initPID]
	init.Children = []int{orphanPID}
	init.PendingReaps = []int{orphanPID}

	// Stage init as the next ready process and free the boot core.
	k.Ready.Reset()
	k.Ready.Push(initPID)
	init.State = StateReady
	init.BlockedOn = ""
	boot := k.BootCore()
	boot.CurrentPID = 0
	boot.CPU.Halted = true

	evs := k.DispatchIdleCores()

	// 1) The orphan must be reaped.
	if _, stillThere := k.Processes[orphanPID]; stillThere {
		t.Errorf("orphan PCB still in table — init did not reap it")
	}
	// 2) Init must end the dispatch back in BLOCKED("wait()"), not RUN.
	if init.State != StateBlocked {
		t.Errorf("init.State = %s after reap, want BLOCKED (init re-enters wait())", init.State)
	}
	if init.BlockedOn != initBlockedReason {
		t.Errorf("init.BlockedOn = %q, want %q", init.BlockedOn, initBlockedReason)
	}
	// 3) Boot core must not have been left assigned to init — init never
	// executes user code, so the slot stays free for the next dispatch.
	if boot.CurrentPID == initPID {
		t.Errorf("boot core left with CurrentPID=init (%d) — init must release the core after reaping", initPID)
	}
	// 4) Event log must contain the visible transition: RUN, reap, BLOCKED.
	//    Init's reap is consolidated into a single EventInitReap (one event
	//    even with N orphans); regular waitpid reaps still travel as
	//    EventWaitOp. The orphan in this test was reparented to init by
	//    markZombieAndReparent, so Reparented must be true.
	var sawRun, sawBlocked bool
	var initReap *cpu.EventInitReap
	for _, e := range evs {
		switch v := e.(type) {
		case cpu.EventProcessState:
			if v.PID == initPID && v.State == string(StateRun) {
				sawRun = true
			}
			if v.PID == initPID && v.State == string(StateBlocked) && sawRun {
				sawBlocked = true
			}
		case cpu.EventInitReap:
			ev := v
			initReap = &ev
		case cpu.EventWaitOp:
			if v.Op == "reap" && v.PID == initPID {
				t.Errorf("init must NOT emit EventWaitOp{Op:reap} %+v — its reap cycle is consolidated into EventInitReap", v)
			}
		}
	}
	if !sawRun {
		t.Error("missing EventProcessState{init, RUN} — alumno must see init wake up")
	}
	if initReap == nil {
		t.Fatal("missing EventInitReap for init's reap cycle")
	}
	if initReap.InitPID != initPID {
		t.Errorf("EventInitReap.InitPID = %d, want %d", initReap.InitPID, initPID)
	}
	if len(initReap.Reaps) != 1 {
		t.Fatalf("EventInitReap.Reaps len = %d, want 1", len(initReap.Reaps))
	}
	if initReap.Reaps[0].ChildPID != orphanPID {
		t.Errorf("EventInitReap.Reaps[0].ChildPID = %d, want %d", initReap.Reaps[0].ChildPID, orphanPID)
	}
	if !initReap.Reaps[0].Reparented {
		t.Errorf("EventInitReap.Reaps[0].Reparented = false, want true (the orphan was transferred to init by markZombieAndReparent)")
	}
	if !sawBlocked {
		t.Error("missing EventProcessState{init, BLOCKED} after the reap — init must visibly go back to wait()")
	}
}

// TestNotifyParentOfExit_EmitsReadyQueueSnapshot guards against a frontend
// desync: when a child exits and wakeToQueue pushes the parent onto the ready
// queue, the kernel must emit an EventWake carrying the updated ReadyQueue
// snapshot. Without it, the WS client never learns the parent is runnable and
// its haltedDead heuristic (cpu.halted && readyQueue.empty) keeps the STEP
// button disabled, freezing step-by-step execution after the child's
// sys_exit. EventWake replaces the EventSchedDecision{Reason:"wake"} that
// previously carried this contract — wakes are state transitions, not
// scheduling decisions, so they travel on their own event type now.
func TestNotifyParentOfExit_EmitsReadyQueueSnapshot(t *testing.T) {
	k := New(cpu.DefaultMemSize)

	parentPID, err := k.AddProcess("parent", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess parent: %v", err)
	}
	childPID, err := k.AddProcess("child", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess child: %v", err)
	}

	parent := k.Processes[parentPID]
	child := k.Processes[childPID]
	parent.Children = []int{childPID}
	child.ParentPID = parentPID

	// Park the parent in wait(P{childPID}) — the state sys_waitpid leaves it
	// in when no zombie is ready. Drop it from the ready queue to match.
	parent.State = StateBlocked
	parent.BlockedOn = waitpidBlockedReason(childPID)
	parent.Waiter = childPID
	k.Ready.Remove(parentPID)

	child.State = StateZombie
	child.ExitCode = 123

	evs := k.notifyParentOfExit(child)

	// The parent must end up on the ready queue.
	if !slices.Contains(k.Ready.Snapshot(), parentPID) {
		t.Fatalf("parent PID %d not in ready queue after wake: %v", parentPID, k.Ready.Snapshot())
	}

	// And the emitted events must announce that ready-queue change with an
	// EventWake carrying the updated snapshot.
	var sawWake bool
	for _, e := range evs {
		wake, ok := e.(cpu.EventWake)
		if !ok {
			continue
		}
		if wake.PID != parentPID {
			continue
		}
		if !slices.Contains(wake.ReadyQueue, parentPID) {
			t.Errorf("EventWake.ReadyQueue = %v, want it to include parent PID %d", wake.ReadyQueue, parentPID)
		}
		sawWake = true
	}
	if !sawWake {
		t.Errorf("notifyParentOfExit emitted no EventWake for the BLOCKED→READY transition — frontend cpu.readyQueue will drift out of sync with k.Ready")
	}
}
