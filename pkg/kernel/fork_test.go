package kernel

import (
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// TestSysFork_CreatesChildAsCopyOfParent exercises the headline behaviour of
// sys_fork: the parent invokes int 0x80 with EAX=SysFork and the kernel must
//
//   - allocate a new PID
//   - create a PCB whose ParentPID points back at the caller
//   - copy the parent's user code into the child's code slot
//   - leave the child in READY with Regs.EAX = 0 (fork returns 0 in child)
//   - return the child PID in the parent's EAX
//   - emit an EventFork with both PIDs and IsVfork=false
func TestSysFork_CreatesChildAsCopyOfParent(t *testing.T) {
	// Parent program:
	//   mov eax, 2     ; SysFork
	//   int 0x80       ; HW switches to kernel, kernel creates child, returns
	//   hlt            ; both parent and child reach this after fork
	parentProg := []byte{
		0xB8, 0x02, 0x00, 0x00, 0x00, // mov eax, 2
		0xCD, 0x80, // int 0x80
		0xF4, // hlt (sentinel — keeps EIP from drifting)
	}
	k := newTestKernel(t, parentProg)
	parentPID := 1

	var forkEv cpu.EventFork
	found := false
	for i := 0; i < 200 && !found; i++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if ev, ok := findEvent[cpu.EventFork](evs); ok {
			forkEv = ev
			found = true
		}
	}
	if !found {
		t.Fatal("no EventFork emitted within 200 steps")
	}

	if forkEv.ParentPID != parentPID {
		t.Errorf("EventFork.ParentPID=%d, want %d", forkEv.ParentPID, parentPID)
	}
	if forkEv.ChildPID == parentPID || forkEv.ChildPID == 0 {
		t.Errorf("EventFork.ChildPID=%d, want a fresh PID", forkEv.ChildPID)
	}
	if forkEv.IsVfork {
		t.Error("EventFork.IsVfork=true on plain fork, want false")
	}

	childPID := forkEv.ChildPID
	child := k.Processes[childPID]
	if child == nil {
		t.Fatalf("child PCB %d missing from process table", childPID)
	}
	if child.ParentPID != parentPID {
		t.Errorf("child.ParentPID=%d, want %d", child.ParentPID, parentPID)
	}
	if child.State != StateReady {
		t.Errorf("child.State=%s, want READY", child.State)
	}
	if child.Regs.EAX != 0 {
		t.Errorf("child.Regs.EAX=0x%X, want 0", child.Regs.EAX)
	}

	parent := k.Processes[parentPID]
	if len(parent.Children) != 1 || parent.Children[0] != childPID {
		t.Errorf("parent.Children=%v, want [%d]", parent.Children, childPID)
	}

	// Parent's view of fork's return value: at this instant the parent is
	// still RUN on the boot core, so EAX lives in the live register file,
	// not in the saved PCB.
	if got := k.BootCPU().R.EAX; got != uint32(childPID) {
		t.Errorf("parent core.EAX=0x%X, want %d", got, childPID)
	}
}

// TestSysFork_ChildCodeIsCopy ensures the child's code segment is an
// independent copy of the parent's. Writing into the parent's code base
// must NOT change bytes inside the child's code base.
func TestSysFork_ChildCodeIsCopy(t *testing.T) {
	parentProg := []byte{
		0xB8, 0x02, 0x00, 0x00, 0x00, // mov eax, 2 (SysFork)
		0xCD, 0x80, // int 0x80
		0xF4, // hlt
	}
	k := newTestKernel(t, parentProg)

	var childPID int
	for i := 0; i < 200 && childPID == 0; i++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if ev, ok := findEvent[cpu.EventFork](evs); ok {
			childPID = ev.ChildPID
		}
	}
	if childPID == 0 {
		t.Fatal("fork never fired")
	}

	parent := k.Processes[1]
	child := k.Processes[childPID]
	parentBase := parent.MemSpace().CodeBase
	childBase := child.MemSpace().CodeBase
	if parentBase == childBase {
		t.Fatalf("parent and child share CodeBase 0x%X — fork must allocate a new slot", parentBase)
	}

	// Mutate one byte in the parent's code region; the child's must stay put.
	parentByte0, _ := k.Mem.Read8(nil, parentBase, cpu.ModeKernel)
	childByte0, _ := k.Mem.Read8(nil, childBase, cpu.ModeKernel)
	if parentByte0 != childByte0 {
		t.Fatalf("byte at code_base differs before mutation: parent=0x%02X child=0x%02X", parentByte0, childByte0)
	}

	k.Mem.Bytes[parentBase] = 0x90 // NOP
	afterParent := k.Mem.Bytes[parentBase]
	afterChild := k.Mem.Bytes[childBase]
	if afterParent == afterChild {
		t.Fatal("mutating parent code byte changed the child too — child is not an independent copy")
	}
}
