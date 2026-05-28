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
		t.Errorf("init.ParentPID = %d, want 0 (no parent)", init.ParentPID)
	}
}

// TestKillCurrent_ReparentsChildrenToInit verifies that when a parent
// process dies leaving live children, those children are transferred to
// init (PID 1) and an EventReparent is emitted for each. Zombies that the
// dying parent had not yet reaped are likewise transferred under init,
// queued on init.PendingReaps, and init is woken to the ready queue. The
// actual reap is deferred to the next dispatch (covered by
// TestDispatch_InitCollectsOrphanAndReblocks in deferred_reap_test.go) —
// orphan zombies must remain visible until init runs.
func TestKillCurrent_ReparentsChildrenToInit(t *testing.T) {
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
	// stays focused on the reparent logic in killCurrent.
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

	// Drive the parent onto the boot core so killCurrent can act on it.
	boot := k.BootCore()
	boot.CurrentPID = parentPID
	parent.State = StateRun
	parent.Core = boot.ID

	evs := k.killCurrent(boot, "exit")

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
		t.Error("childB removed from process table by killCurrent — deferred reap must keep the zombie alive until init runs")
	}
	if len(init.PendingReaps) != 1 || init.PendingReaps[0] != childBPID {
		t.Errorf("init.PendingReaps = %v, want [%d]", init.PendingReaps, childBPID)
	}
	if init.State != StateReady {
		t.Errorf("init.State = %s, want READY (woken to run the reap cycle)", init.State)
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

	// killCurrent must NOT have emitted a reap event for the orphan —
	// that fires only when the dispatcher picks init on a later Step.
	for _, e := range evs {
		if w, ok := e.(cpu.EventWaitOp); ok && w.Op == "reap" {
			t.Errorf("EventWaitOp{Op:reap} %+v emitted by killCurrent — reap must wait for init's dispatch", w)
		}
	}
}
