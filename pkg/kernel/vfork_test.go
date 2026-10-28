package kernel

import (
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// Program flow (vfork shares memory; parent BLOCKED until child sys_exit):
//
//	vfork()
//	cmp eax, 0
//	jz child
//	; parent path
//	mov eax, [0x350000]   ; read what the child wrote
//	mov ebx, eax          ; exit code = the read value
//	mov eax, 1
//	int 0x80
//
// child:
//	mov eax, 0xDEADBEEF
//	mov [0x350000], eax   ; write into parent-visible memory
//	mov ebx, 42
//	mov eax, 1
//	int 0x80
//
// SysVfork = 190 = 0xBE. Shared scratch address 0x350000 lives in
// user_stack (User RW for both parent and child).
var vforkSharedMemProg = []byte{
	0xB8, 0xBE, 0x00, 0x00, 0x00, // 0x00: mov eax, 190 (SysVfork)
	0xCD, 0x80, // 0x05: int 0x80
	0x81, 0xF8, 0x00, 0x00, 0x00, 0x00, // 0x07: cmp eax, 0
	0x74, 0x0E, // 0x0D: jz +14 -> child at 0x1D
	0xA1, 0x00, 0x00, 0x35, 0x00, // 0x0F: mov eax, [0x350000]
	0x89, 0xC3, // 0x14: mov ebx, eax
	0xB8, 0x01, 0x00, 0x00, 0x00, // 0x16: mov eax, 1 (SysExit)
	0xCD, 0x80, // 0x1B: int 0x80
	// child label at 0x1D
	0xB8, 0xEF, 0xBE, 0xAD, 0xDE, // 0x1D: mov eax, 0xDEADBEEF
	0xA3, 0x00, 0x00, 0x35, 0x00, // 0x22: mov [0x350000], eax
	0xBB, 0x2A, 0x00, 0x00, 0x00, // 0x27: mov ebx, 42
	0xB8, 0x01, 0x00, 0x00, 0x00, // 0x2C: mov eax, 1 (SysExit)
	0xCD, 0x80, // 0x31: int 0x80
}

// TestSysVfork_ParentSeesChildMemoryMutation exercises the central pedagogical
// contrast between fork and vfork in this simulator: vfork does not copy the
// parent's address space, so a write performed by the child is visible to the
// parent when it wakes. The parent's exit code carries the value the child
// wrote, proving the read landed on the same physical bytes.
func TestSysVfork_ParentSeesChildMemoryMutation(t *testing.T) {
	k := newTestKernel(t, vforkSharedMemProg)
	k.TimerQuantum = 0
	parentPID := 1

	for i := 0; i < 2000; i++ {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		// HasWork instead of AllHalted: a vfork parent may be momentarily
		// off-core (BLOCKED waiting for the child's exit, then queued for
		// dispatch after the wake) and AllHalted would stop the loop in
		// that window.
		if !k.HasWork() {
			break
		}
	}

	parent := k.Processes[parentPID]
	if parent == nil {
		t.Fatalf("parent PID %d disappeared", parentPID)
	}
	if parent.State != StateZombie {
		t.Errorf("parent.State = %s, want ZOMBIE (the parent should have exited)", parent.State)
	}
	if parent.ExitCode != 0xDEADBEEF {
		t.Errorf("parent exit code = 0x%08X, want 0xDEADBEEF (the value the vfork child stored)", parent.ExitCode)
	}
}

// TestSysVfork_EmitsForkEventWithIsVforkFlag confirms that vfork emits an
// EventFork with IsVfork=true so the frontend can distinguish the
// "memory shared" flavour in the narrator.
func TestSysVfork_EmitsForkEventWithIsVforkFlag(t *testing.T) {
	k := newTestKernel(t, vforkSharedMemProg)
	k.TimerQuantum = 0

	for i := 0; i < 200; i++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if ev, ok := findEvent[cpu.EventFork](evs); ok {
			if !ev.IsVfork {
				t.Errorf("EventFork.IsVfork = false, want true for sys_vfork")
			}
			return
		}
	}
	t.Fatal("no EventFork emitted within 200 steps")
}
