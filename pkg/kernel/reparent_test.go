package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestSpawnInit_PID1 verifies that when SpawnInit is the first PCB created
// on a fresh kernel, init takes PID 1 and the kernel records it in InitPID.
func TestSpawnInit_PID1(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	pid, err := k.SpawnInit()
	if err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	if pid != 1 {
		t.Errorf("init PID = %d, want 1", pid)
	}
	if k.InitPID != pid {
		t.Errorf("k.InitPID = %d, want %d", k.InitPID, pid)
	}
	init := k.Processes[pid]
	if init == nil {
		t.Fatal("init PCB missing from process table")
	}
	if init.Name != "init" {
		t.Errorf("init.Name = %q, want \"init\"", init.Name)
	}
	if init.ParentPID != 0 {
		t.Errorf("init.ParentPID = %d, want 0 (swapper/PID 0, Linux-style)", init.ParentPID)
	}
}

// TestMarkZombieAndReparent_ReparentsChildrenToInit verifies that when a
// parent process dies leaving live children, those children are transferred
// to init (PID 1) and an EventReparent is emitted for each. Zombies that
// the dying parent had not yet reaped are likewise transferred under init,
// queued on init.PendingReaps, and init is woken to the ready queue. The
// children carry WasReparentedToInit=true so the frontend can render the
// "init reaped orphan" wording for them, as opposed to children adopted at
// load time via AdoptUnderInit. This test calls markZombieAndReparent in
// isolation (skipping the notifyParentOfExit/finalizeExit chain) to keep
// the assertions focused on the reparent logic; the end-to-end flow is
// covered by TestSyscall_ExitWithInitParent_NoSpuriousHaltButTerminalHalt.
func TestMarkZombieAndReparent_ReparentsChildrenToInit(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	initPID, err := k.SpawnInit()
	if err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	// Use a trivial 1-byte hlt program for the synthetic processes; they
	// will not actually run, we only need their PCBs.
	parentPID, err := k.AddProcess("parent", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess parent: %v", err)
	}
	childAPID, err := k.AddProcess("childA", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess childA: %v", err)
	}
	childBPID, err := k.AddProcess("childB", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess childB: %v", err)
	}

	// Wire the parent-child relationships by hand: AddProcess does not
	// know about parentage, but sys_fork sets these fields. Tests that
	// drive the kernel through fork are covered separately; this one
	// stays focused on the reparent logic.
	parent := k.Processes[parentPID]
	childA := k.Processes[childAPID]
	childB := k.Processes[childBPID]
	parent.Children = []int{childAPID, childBPID}
	childA.ParentPID = parentPID
	childB.ParentPID = parentPID
	// Mark one of them as zombie so we cover the "orphan zombie carried
	// to init" path.
	childB.State = StateZombie
	childB.ExitCode = 7

	// Drive the parent onto the boot core so markZombieAndReparent can act on it.
	boot := k.BootCore()
	boot.CurrentPID = parentPID
	parent.State = StateRun
	parent.Core = boot.ID

	evs := k.markZombieAndReparent(boot, "exit")

	if got := childA.ParentPID; got != initPID {
		t.Errorf("childA.ParentPID = %d, want %d (init)", got, initPID)
	}
	// childB.ParentPID was stamped to init before the auto-reap; the
	// pointer remains valid even though the PCB is gone from the table.
	if got := childB.ParentPID; got != initPID {
		t.Errorf("childB.ParentPID = %d, want %d (init)", got, initPID)
	}
	init := k.Processes[initPID]

	// Both children — live and zombie — must land on init.Children. The
	// zombie stays in the table; only init.PendingReaps marks it for
	// collection on the next dispatch.
	foundA, foundB := false, false
	for _, c := range init.Children {
		if c == childAPID {
			foundA = true
		}
		if c == childBPID {
			foundB = true
		}
	}
	if !foundA {
		t.Errorf("init.Children=%v missing live child %d", init.Children, childAPID)
	}
	if !foundB {
		t.Errorf("init.Children=%v missing orphan zombie %d — reaper must be able to see it", init.Children, childBPID)
	}
	if _, stillThere := k.Processes[childBPID]; !stillThere {
		t.Error("childB removed from process table by markZombieAndReparent — the orphan zombie must remain in the table until consumePendingReaps runs")
	}
	// Both reparented children must carry the flag so the frontend's reap
	// label can distinguish them from birth-adopted children of init.
	if !childA.WasReparentedToInit {
		t.Error("childA.WasReparentedToInit = false, want true (parent died while child was alive)")
	}
	if !childB.WasReparentedToInit {
		t.Error("childB.WasReparentedToInit = false, want true (parent died while child was a zombie)")
	}
	if len(init.PendingReaps) != 1 || init.PendingReaps[0] != childBPID {
		t.Errorf("init.PendingReaps = %v, want [%d]", init.PendingReaps, childBPID)
	}
	// markZombieAndReparent now leaves init BLOCKED — the wake of init for
	// the queued orphan reaps was moved out of phase 1 into phase 2
	// (doNotifyParentPhase) so the alumno sees "dying task finalizes" and
	// "init is woken to reap" as two distinct Steps. End-to-end coverage of
	// the wake lives in TestSyscall_ExitWithInitParent_NoSpuriousHaltButTerminalHalt.
	if init.State != StateBlocked {
		t.Errorf("init.State = %s, want BLOCKED (wake moved to phase 2)", init.State)
	}
	if len(parent.Children) != 0 {
		t.Errorf("parent.Children = %v after death, want []", parent.Children)
	}

	// Two EventReparent events expected, one per child.
	count := 0
	for _, e := range evs {
		if rep, ok := e.(cpu.EventReparent); ok {
			if rep.OldParent != parentPID || rep.NewParent != initPID {
				t.Errorf("EventReparent %+v, want OldParent=%d NewParent=%d", rep, parentPID, initPID)
			}
			count++
		}
	}
	if count != 2 {
		t.Errorf("EventReparent count = %d, want 2", count)
	}

	// markZombieAndReparent must NOT emit a reap event for the orphan —
	// that fires only when consumePendingReaps runs, either inside
	// runInitReapCycle (inline in finalizeExit or via DispatchIdleCores)
	// or when a non-init parent is dispatched.
	for _, e := range evs {
		if w, ok := e.(cpu.EventWaitOp); ok && w.Op == "reap" {
			t.Errorf("EventWaitOp{Op:reap} %+v emitted by markZombieAndReparent — reap must wait for consumePendingReaps", w)
		}
	}
}
