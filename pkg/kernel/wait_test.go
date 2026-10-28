package kernel

import (
	"testing"
)

// Program flow:
//
//	fork()
//	cmp eax, 0     ; ZF=1 in the child (fork returns 0)
//	jz child
//	; parent path: waitpid(-1, 0, 0) ; exit(reaped_pid)
//	mov eax, 7
//	mov ebx, -1
//	mov ecx, 0
//	mov edx, 0
//	int 0x80          ; EAX = reaped child PID
//	mov ebx, eax      ; carry it in the exit code so the test can read it
//	mov eax, 1
//	int 0x80
//
// child:
//
//	mov ebx, 42
//	mov eax, 1
//	int 0x80
var forkWaitExitProg = []byte{
	0xB8, 0x02, 0x00, 0x00, 0x00, // 0x00: mov eax, 2 (SysFork)
	0xCD, 0x80, // 0x05: int 0x80
	0x81, 0xF8, 0x00, 0x00, 0x00, 0x00, // 0x07: cmp eax, 0
	0x74, 0x1F, // 0x0D: jz +31 -> child at 0x2E
	0xB8, 0x07, 0x00, 0x00, 0x00, // 0x0F: mov eax, 7 (SysWaitpid)
	0xBB, 0xFF, 0xFF, 0xFF, 0xFF, // 0x14: mov ebx, -1
	0xB9, 0x00, 0x00, 0x00, 0x00, // 0x19: mov ecx, 0
	0xBA, 0x00, 0x00, 0x00, 0x00, // 0x1E: mov edx, 0
	0xCD, 0x80, // 0x23: int 0x80
	0x89, 0xC3, // 0x25: mov ebx, eax (exit code = reaped pid)
	0xB8, 0x01, 0x00, 0x00, 0x00, // 0x27: mov eax, 1 (SysExit)
	0xCD, 0x80, // 0x2C: int 0x80
	// child label
	0xBB, 0x2A, 0x00, 0x00, 0x00, // 0x2E: mov ebx, 42
	0xB8, 0x01, 0x00, 0x00, 0x00, // 0x33: mov eax, 1 (SysExit)
	0xCD, 0x80, // 0x38: int 0x80
}

// TestSysWaitpid_ParentReapsChildZombie covers the canonical fork/wait/exit
// flow: parent forks, parent calls waitpid(-1), child exits with code 42,
// and the kernel must reap the child PCB and hand back the child PID to the
// parent through EAX.
func TestSysWaitpid_ParentReapsChildZombie(t *testing.T) {
	k := newTestKernel(t, forkWaitExitProg)
	// Disable the timer so the parent's waitpid setup runs uninterrupted
	// and the child can run to sys_exit without preemption.
	k.TimerQuantum = 0
	parentPID := 1

	for i := 0; i < 2000; i++ {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		// HasWork is the right signal here: with deferred reap the boot
		// core may be momentarily halted between a child's exit and the
		// parent's dispatch from the ready queue. AllHalted would stop
		// the loop too early in that window.
		if !k.HasWork() {
			break
		}
	}

	parent := k.Processes[parentPID]
	if parent == nil {
		t.Fatalf("parent PID %d disappeared", parentPID)
	}
	// The parent's exit code carries the reaped child PID — the program
	// stashes EAX (which sys_waitpid filled with the reaped PID) into EBX
	// before calling sys_exit. Fork allocates PID 2 for the first child.
	if parent.ExitCode != 2 {
		t.Errorf("parent.ExitCode = %d; want 2 (reaped child PID)", parent.ExitCode)
	}
	// Child PCB must be gone from the process table.
	if _, ok := k.Processes[2]; ok {
		t.Errorf("child PID 2 still in process table — was not reaped")
	}
	if len(parent.Children) != 0 {
		t.Errorf("parent.Children = %v after reap, want []", parent.Children)
	}
}
