package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestClone_ThreadModeSharesMemoryAndFDs exercises the pthread_create code
// path: clone(CLONE_VM|CLONE_FILES|CLONE_THREAD) must hand the child a task
// that shares its parent's AddressSpace, FDSpace and TGID instead of
// allocating fresh ones. The user-supplied stack and entry point land on
// the child's Regs without the kernel-frame read fork uses.
func TestClone_ThreadModeSharesMemoryAndFDs(t *testing.T) {
	k := newTestKernel(t, []byte{0xF4})
	parent := k.Processes[1]
	parentMem := parent.MemSpace()
	parentFDs := parent.FDSpace()

	// Pretend user space already mmap'd a 64 KB stack region for the
	// thread and points to the entry function inside the shared text.
	const (
		threadStack = uint32(0x3A0000)
		threadFn    = uint32(0x200080)
	)

	child, events, err := k.Clone(k.Cores[0], parent, CloneVM|CloneFiles|CloneThread, cloneOptions{
		childStackTop: threadStack,
		childEIP:      threadFn,
	})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if child.MemSpace() != parentMem {
		t.Errorf("child MemSpace=%p, want parent's %p (CLONE_VM)", child.MemSpace(), parentMem)
	}
	if child.FDSpace() != parentFDs {
		t.Errorf("child FDSpace=%p, want parent's %p (CLONE_FILES)", child.FDSpace(), parentFDs)
	}
	if child.TGID != parent.TGID {
		t.Errorf("child TGID=%d, want %d (CLONE_THREAD)", child.TGID, parent.TGID)
	}
	if child.PID == parent.PID {
		t.Error("child PID equals parent's — Clone must allocate a fresh PID")
	}
	if child.StackTop != threadStack {
		t.Errorf("child StackTop=0x%X, want 0x%X (user-supplied)", child.StackTop, threadStack)
	}
	if child.Regs.EIP != threadFn {
		t.Errorf("child EIP=0x%X, want 0x%X (user-supplied entry)", child.Regs.EIP, threadFn)
	}
	if child.Regs.EAX != 0 {
		t.Errorf("child Regs.EAX=%d, want 0 (clone returns 0 in child)", child.Regs.EAX)
	}
	if child.VforkParent != 0 {
		t.Errorf("child VforkParent=%d, want 0 (CLONE_THREAD is not vfork)", child.VforkParent)
	}

	if forkEv, ok := findEvent[cpu.EventFork](events); !ok {
		t.Error("no EventFork emitted by Clone in thread mode")
	} else if forkEv.IsVfork {
		t.Error("EventFork.IsVfork=true on CLONE_THREAD — must be false")
	}
}

// TestClone_VMWithoutExplicitStack covers the kernel-allocated-stack
// branch: when CLONE_VM is set but the caller did not provide a stack,
// Clone reaches into the shared AddressSpace's slot bitmap and carves
// the next free 64 KB sub-region. The result must land below the
// parent's StackTop (the leader already holds the top slot).
func TestClone_VMWithoutExplicitStack(t *testing.T) {
	k := newTestKernel(t, []byte{0xF4})
	parent := k.Processes[1]
	parentMem := parent.MemSpace()

	child, _, err := k.Clone(k.Cores[0], parent, CloneVM|CloneThread, cloneOptions{
		childEIP: 0x200100,
	})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if child.MemSpace() != parentMem {
		t.Error("Clone with CLONE_VM allocated a new MemSpace")
	}
	if child.StackTop >= parent.StackTop {
		t.Errorf("child StackTop=0x%X overlaps parent's 0x%X — slots should descend", child.StackTop, parent.StackTop)
	}
	want := parent.StackTop - ThreadStackSlotSize
	if child.StackTop != want {
		t.Errorf("child StackTop=0x%X, want 0x%X (slot just below the leader)", child.StackTop, want)
	}
}

// TestClone_StackSlotExhaustion verifies the bitmap honors the
// per-AddressSpace cap: the leader holds slot ThreadStackSlots-1, so
// ThreadStackSlots-1 additional threads can be spawned before the next
// allocation fails. Real kernels return EAGAIN here; the simulator
// signals the failure with an error.
func TestClone_StackSlotExhaustion(t *testing.T) {
	k := newTestKernel(t, []byte{0xF4})
	parent := k.Processes[1]

	for i := 1; i < ThreadStackSlots; i++ {
		if _, _, err := k.Clone(k.Cores[0], parent, CloneVM|CloneThread, cloneOptions{
			childEIP: 0x200100,
		}); err != nil {
			t.Fatalf("Clone #%d (still room for threads): %v", i, err)
		}
	}

	if _, _, err := k.Clone(k.Cores[0], parent, CloneVM|CloneThread, cloneOptions{
		childEIP: 0x200100,
	}); err == nil {
		t.Error("Clone past slot capacity did not error — bitmap accounting is off")
	}
}

// TestClone_ForkPathProducesIndependentSpaces covers the zero-flag path:
// the child must get its own MemSpace and its own FDSpace, with a TGID
// rooted at its own PID. This is what glibc's fork() looks like once
// translated to a clone() call: clone(SIGCHLD).
//
// The integration through sys_fork is exercised by fork_test.go; here we
// call Clone directly with childEIP set so the test does not depend on
// a hand-laid interrupt frame.
func TestClone_ForkPathProducesIndependentSpaces(t *testing.T) {
	k := newTestKernel(t, []byte{0xB8, 0x02, 0x00, 0x00, 0x00, 0xCD, 0x80, 0xF4})
	parent := k.Processes[1]
	parentMem := parent.MemSpace()
	parentFDs := parent.FDSpace()

	child, _, err := k.Clone(k.Cores[0], parent, 0, cloneOptions{
		childEIP: 0x200007,
	})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if child.MemSpace() == parentMem {
		t.Error("fork-style Clone shares MemSpace with parent — must allocate fresh")
	}
	if child.MemSpace().CodeBase == parentMem.CodeBase {
		t.Errorf("child CodeBase=0x%X equals parent's — fork must carve a new slot", child.MemSpace().CodeBase)
	}
	if child.FDSpace() == parentFDs {
		t.Error("fork-style Clone shares FDSpace with parent — must clone")
	}
	if child.TGID != child.PID {
		t.Errorf("child TGID=%d, want %d (fork starts a new thread group)", child.TGID, child.PID)
	}
}
