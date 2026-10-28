package kernel

import (
	"strings"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
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

	threadEv, ok := findEvent[cpu.EventThreadCreate](events)
	if !ok {
		t.Fatal("no EventThreadCreate emitted by Clone in thread mode")
	}
	if threadEv.TID != child.PID {
		t.Errorf("EventThreadCreate.TID=%d, want %d", threadEv.TID, child.PID)
	}
	if threadEv.TGID != parent.TGID {
		t.Errorf("EventThreadCreate.TGID=%d, want %d", threadEv.TGID, parent.TGID)
	}
	if threadEv.LeaderPID != parent.TGID {
		t.Errorf("EventThreadCreate.LeaderPID=%d, want %d", threadEv.LeaderPID, parent.TGID)
	}
	if threadEv.FnEIP != threadFn {
		t.Errorf("EventThreadCreate.FnEIP=0x%X, want 0x%X", threadEv.FnEIP, threadFn)
	}
	if threadEv.StackTop != threadStack {
		t.Errorf("EventThreadCreate.StackTop=0x%X, want 0x%X", threadEv.StackTop, threadStack)
	}
	// EventFork must NOT be emitted on thread create — the UI distinguishes
	// "spawned a thread" from "forked a child" via this very signal.
	if _, ok := findEvent[cpu.EventFork](events); ok {
		t.Error("EventFork emitted on CLONE_THREAD — expected only EventThreadCreate")
	}
}

// TestPCBSnapshot_ExposesThreadGroupFields anchors the JSON contract toward
// the frontend: every PCB snapshot must carry tgid / tid / is_leader so the
// UI can fold sibling threads under their leader without inferring the
// relationship from the PID layout.
func TestPCBSnapshot_ExposesThreadGroupFields(t *testing.T) {
	k := newTestKernel(t, []byte{0xF4})
	parent := k.Processes[1]
	child, _, err := k.Clone(k.Cores[0], parent, CloneVM|CloneFiles|CloneThread, cloneOptions{
		childStackTop: 0x3A0000,
		childEIP:      0x200080,
	})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	leaderSnap := parent.Snapshot()
	if leaderSnap["tgid"] != parent.TGID {
		t.Errorf("leader snapshot tgid=%v, want %d", leaderSnap["tgid"], parent.TGID)
	}
	if leaderSnap["tid"] != parent.PID {
		t.Errorf("leader snapshot tid=%v, want %d", leaderSnap["tid"], parent.PID)
	}
	if leaderSnap["is_leader"] != true {
		t.Errorf("leader snapshot is_leader=%v, want true", leaderSnap["is_leader"])
	}

	threadSnap := child.Snapshot()
	if threadSnap["tgid"] != parent.TGID {
		t.Errorf("thread snapshot tgid=%v, want %d (leader's TGID)", threadSnap["tgid"], parent.TGID)
	}
	if threadSnap["tid"] != child.PID {
		t.Errorf("thread snapshot tid=%v, want %d", threadSnap["tid"], child.PID)
	}
	if threadSnap["is_leader"] != false {
		t.Errorf("thread snapshot is_leader=%v, want false", threadSnap["is_leader"])
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

// TestClone_PIDSpaceAfterKThreads guards the boot-time PID budget against the
// regression where MaxProcesses (the user-code slot count) doubled as the PID
// ceiling. Once SpawnKThreadd + SpawnKernelThreads added kthreadd and a per-
// core kernel/N, the old `nextPID > MaxProcesses+1` check rejected the second
// CLONE_THREAD a user program issued — the threads_race demo lost its second
// sibling silently. Each row spawns a realistic kernel (init + kthreadd +
// kernel threads), then asks the leader to clone N siblings; all of them must
// land with distinct TIDs and an EventThreadCreate apiece.
func TestClone_PIDSpaceAfterKThreads(t *testing.T) {
	tests := []struct {
		name     string
		cores    int
		nThreads int
	}{
		{"two threads single core", 1, 2},
		{"two threads two cores", 2, 2},
		{"three threads (fills shared stack slots)", 1, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := NewMulti(cpu.DefaultMemSize, tc.cores)
			if _, err := k.SpawnInit(); err != nil {
				t.Fatalf("SpawnInit: %v", err)
			}
			k.SpawnKThreadd()
			k.SpawnKernelThreads()
			leaderPID, err := k.AddProcess("leader", []byte{0xF4})
			if err != nil {
				t.Fatalf("AddProcess: %v", err)
			}
			parent := k.Processes[leaderPID]

			seenTIDs := map[int]bool{}
			for i := 0; i < tc.nThreads; i++ {
				child, evs, err := k.Clone(k.Cores[0], parent, CloneVM|CloneFiles|CloneThread, cloneOptions{
					childEIP: 0x200080,
				})
				if err != nil {
					t.Fatalf("clone #%d (after %d kthreads + leader) failed: %v",
						i+1, leaderPID-1, err)
				}
				if seenTIDs[child.PID] {
					t.Errorf("clone #%d reused TID %d", i+1, child.PID)
				}
				seenTIDs[child.PID] = true

				ev, ok := findEvent[cpu.EventThreadCreate](evs)
				if !ok {
					t.Errorf("clone #%d: no EventThreadCreate emitted", i+1)
					continue
				}
				if ev.TID != child.PID {
					t.Errorf("clone #%d: EventThreadCreate.TID=%d, want %d", i+1, ev.TID, child.PID)
				}
				if ev.TGID != parent.TGID {
					t.Errorf("clone #%d: EventThreadCreate.TGID=%d, want %d", i+1, ev.TGID, parent.TGID)
				}
			}
			if len(seenTIDs) != tc.nThreads {
				t.Errorf("got %d unique TIDs, want %d", len(seenTIDs), tc.nThreads)
			}
		})
	}
}

// TestClone_PIDExhaustionReportsError forces the PID ceiling and asserts both
// the error path of Clone() and the EventSyscall.Error stamping done by
// handleSyscall. Without the latter, a failed clone would surface to the UI
// as just result=0xFFFFFFFF with no human-readable reason — the very bug that
// hid the threads_race regression for so long.
func TestClone_PIDExhaustionReportsError(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 1)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	k.SpawnKThreadd()
	k.SpawnKernelThreads()
	leaderPID, err := k.AddProcess("leader", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	parent := k.Processes[leaderPID]

	// Push nextPID one past the ceiling so the very next Clone fails.
	k.nextPID = MaxPIDs + 1

	_, _, err = k.Clone(k.Cores[0], parent, CloneVM|CloneFiles|CloneThread, cloneOptions{
		childEIP: 0x200080,
	})
	if err == nil {
		t.Fatal("expected error when nextPID > MaxPIDs, got nil")
	}
	if !strings.Contains(err.Error(), "PID space exhausted") {
		t.Errorf("error = %q, want substring 'PID space exhausted'", err.Error())
	}

	// Re-arm and exercise the full handleSyscall path so we can assert that
	// the failure surfaces on EventSyscall.Error (and not just on the return
	// tuple of Clone).
	k.nextPID = MaxPIDs + 1
	core := k.Cores[0]
	core.CurrentPID = leaderPID
	core.CPU.R.EAX = SysClone
	core.CPU.R.EBX = uint32(CloneVM | CloneFiles | CloneThread)
	core.CPU.R.ECX = 0
	core.CPU.R.EDX = 0x200080

	evs := k.handleSyscall(core)
	syscallEv, ok := findEvent[cpu.EventSyscall](evs)
	if !ok {
		t.Fatal("handleSyscall emitted no EventSyscall")
	}
	if syscallEv.Name != "clone" {
		t.Errorf("EventSyscall.Name=%q, want %q", syscallEv.Name, "clone")
	}
	if syscallEv.Result != ^uint32(0) {
		t.Errorf("EventSyscall.Result=0x%X, want 0xFFFFFFFF (failure sentinel)", syscallEv.Result)
	}
	if syscallEv.Error == "" {
		t.Error("EventSyscall.Error is empty — failed clone surfaces silently to the UI")
	}
	if !strings.Contains(syscallEv.Error, "PID space exhausted") {
		t.Errorf("EventSyscall.Error=%q, want substring 'PID space exhausted'", syscallEv.Error)
	}
}
