package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// helloBytes is a hand-encoded "hello" program. We reuse literal bytes so a
// change in the programs.go definition does not affect these unit tests.
var helloBytes = []byte{
	0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4
	0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
	0xB9, 0x22, 0x00, 0x20, 0x00, // mov ecx, 0x200022
	0xBA, 0x08, 0x00, 0x00, 0x00, // mov edx, 8
	0xCD, 0x80, // int 0x80
	0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
	0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
	0xCD, 0x80, // int 0x80
	0x68, 0x6F, 0x6C, 0x61, 0x20, 0x53, 0x4F, 0x0A, // "hola SO\n"
}

func TestSyscall_WriteEmitsConsole(t *testing.T) {
	k := newTestKernel(t, helloBytes)
	k.TimerQuantum = 0 // disable timer; isolate the syscall

	var consoleEv *cpu.EventConsoleOut
	for range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if c, ok := e.(cpu.EventConsoleOut); ok {
				ce := c
				consoleEv = &ce
				goto done
			}
		}
	}
done:
	if consoleEv == nil {
		t.Fatal("no EventConsoleOut emitted")
	}
	if consoleEv.Text != "hola SO\n" {
		t.Errorf("Text = %q, want %q", consoleEv.Text, "hola SO\n")
	}
	if len(consoleEv.Bytes) != 8 {
		t.Errorf("Bytes len = %d, want 8", len(consoleEv.Bytes))
	}
}

func TestSyscall_WriteEmitsEvent(t *testing.T) {
	k := newTestKernel(t, helloBytes)
	k.TimerQuantum = 0

	var writeEv *cpu.EventSyscall
	for range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if s, ok := e.(cpu.EventSyscall); ok && s.Name == "write" {
				se := s
				writeEv = &se
				goto done
			}
		}
	}
done:
	if writeEv == nil {
		t.Fatal("no write EventSyscall emitted")
	}
	if writeEv.Num != SysWrite || writeEv.Args[0] != 1 || writeEv.Args[2] != 8 {
		t.Errorf("EventSyscall = %+v", writeEv)
	}
	if writeEv.Result != 8 {
		t.Errorf("Result = %d, want 8 (bytes written)", writeEv.Result)
	}
}

func TestSyscall_ExitHalts(t *testing.T) {
	k := newTestKernel(t, helloBytes)
	k.TimerQuantum = 0

	maxSteps := 40
	for i := range maxSteps {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if k.BootCPU().Halted {
			break
		}
	}
	if !k.BootCPU().Halted {
		t.Fatalf("CPU did not halt after sys_exit within %d steps", maxSteps)
	}
	if k.Processes[1].State != StateZombie {
		t.Errorf("P1 state = %s, want ZOMBIE", k.Processes[1].State)
	}
}

// TestSyscall_ExitEmitsPCBSaveAndExitCode covers the bug where the frontend
// saw a ZOMBIE process with stale registers and exit_code=0 after sys_exit:
// killCurrent did not snapshot the CPU into the PCB before flipping the
// state, and EventProcessState did not carry the exit code. The fix has to
// emit both an EventPCBSave with the final registers and an EventProcessState
// whose ExitCode points at the value sys_exit stored in pcb.ExitCode.
func TestSyscall_ExitEmitsPCBSaveAndExitCode(t *testing.T) {
	// Program: mov ebx, 42 ; mov eax, 1 (sys_exit) ; int 0x80
	exitProg := []byte{
		0xBB, 0x2A, 0x00, 0x00, 0x00, // mov ebx, 42
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
		0xCD, 0x80, // int 0x80
	}
	k := newTestKernel(t, exitProg)
	k.TimerQuantum = 0

	var pcbSave *cpu.EventPCBSave
	var procState *cpu.EventProcessState
	for range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("Step: %v", err)
		}
		for _, e := range evs {
			switch v := e.(type) {
			case cpu.EventPCBSave:
				if v.PID == 1 {
					ev := v
					pcbSave = &ev
				}
			case cpu.EventProcessState:
				if v.PID == 1 && v.State == string(StateZombie) {
					ev := v
					procState = &ev
				}
			}
		}
		if k.BootCPU().Halted {
			break
		}
	}

	if pcbSave == nil {
		t.Fatal("killCurrent did not emit EventPCBSave for the exiting process")
	}
	if got := pcbSave.Regs["EBX"]; got != 42 {
		t.Errorf("PCBSave EBX = %d, want 42 (exit code copied to PCB)", got)
	}
	if got := pcbSave.Regs["EAX"]; got != 42 {
		t.Errorf("PCBSave EAX = %d, want 42 (sysExit sets pcb.Regs.EAX = code)", got)
	}

	if procState == nil {
		t.Fatal("killCurrent did not emit EventProcessState{State:ZOMBIE} for PID 1")
	}
	if procState.ExitCode == nil {
		t.Fatal("EventProcessState{ZOMBIE} did not carry an ExitCode pointer — frontend cannot see exit_code")
	}
	if *procState.ExitCode != 42 {
		t.Errorf("EventProcessState ExitCode = %d, want 42", *procState.ExitCode)
	}
}

func TestSyscall_ExitSchedulesNext(t *testing.T) {
	// Two processes: the first calls exit, the second (READY) must take RUN.
	exitProg := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
		0xBB, 0x05, 0x00, 0x00, 0x00, // mov ebx, 5 (status)
		0xCD, 0x80, // int 0x80
	}
	loopProg := []byte{0x90, 0xEB, 0xFE} // nop + jmp -2 (infinite loop)

	k := newTestKernelMulti(t, exitProg, loopProg)
	k.TimerQuantum = 0 // timer disabled; we verify the exit path only

	for i := range 30 {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if k.BootCore().CurrentPID == 2 {
			break
		}
	}
	if k.BootCore().CurrentPID != 2 {
		t.Fatalf("CurrentPID = %d, want 2 (P1 exited and should yield to P2)", k.BootCore().CurrentPID)
	}
	if k.Processes[1].State != StateZombie {
		t.Errorf("P1 state = %s, want ZOMBIE", k.Processes[1].State)
	}
	if k.Processes[2].State != StateRun {
		t.Errorf("P2 state = %s, want RUN", k.Processes[2].State)
	}
	if k.Processes[1].Regs.EAX != 5 {
		t.Errorf("P1 EAX = %d, want 5 (exit code)", k.Processes[1].Regs.EAX)
	}
}

func TestSyscall_WriteRejectsKernelPointer(t *testing.T) {
	// User asks sys_write to read from 0x100000 (inside kernel). The kernel
	// must refuse with -EFAULT and a page fault event, NOT exfiltrate kernel
	// bytes to stdout.
	prog := []byte{
		0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4   (sys_write)
		0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1   (stdout)
		0xB9, 0x00, 0x00, 0x10, 0x00, // mov ecx, 0x100000  (KERNEL!)
		0xBA, 0x04, 0x00, 0x00, 0x00, // mov edx, 4
		0xCD, 0x80, // int 0x80
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1   (sys_exit)
		0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
		0xCD, 0x80, // int 0x80
	}
	k := newTestKernel(t, prog)
	k.TimerQuantum = 0

	var sawFault bool
	var sawConsole bool
	var syscallRet uint32
	for range 50 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			switch ev := e.(type) {
			case cpu.EventPageFault:
				sawFault = true
				if ev.Addr != 0x100000 {
					t.Errorf("page fault addr = 0x%X, want 0x100000", ev.Addr)
				}
				if ev.PermNeeded != "R" {
					t.Errorf("page fault perm = %q, want R", ev.PermNeeded)
				}
			case cpu.EventConsoleOut:
				sawConsole = true
			case cpu.EventSyscall:
				if ev.Num == 4 {
					syscallRet = ev.Result
				}
			}
		}
		if k.Processes[1].State == StateZombie {
			break
		}
	}
	if !sawFault {
		t.Error("no EventPageFault emitted for kernel-pointer sys_write")
	}
	if sawConsole {
		t.Error("EventConsoleOut emitted — kernel exfiltrated bytes (security bug)")
	}
	if syscallRet != ^uint32(0) {
		t.Errorf("sys_write result = 0x%X, want 0xFFFFFFFF (-EFAULT)", syscallRet)
	}
}

func TestSyscall_ReadRejectsKernelPointer(t *testing.T) {
	// User asks sys_read to write the keyboard byte into 0x100008 (inside
	// kernel). The kernel must refuse with -EFAULT and a page fault event,
	// NOT let user clobber the IDT or kernel handlers.
	prog := []byte{
		0xB8, 0x03, 0x00, 0x00, 0x00, // mov eax, 3   (sys_read)
		0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0   (stdin)
		0xB9, 0x08, 0x00, 0x10, 0x00, // mov ecx, 0x100008 (KERNEL!)
		0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
		0xCD, 0x80, // int 0x80
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
		0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
		0xCD, 0x80, // int 0x80
	}
	k := newTestKernel(t, prog)
	k.TimerQuantum = 0
	// Pre-load a byte in the keyboard buffer so the fast path runs; the fault
	// must fire BEFORE that byte is copied.
	k.KeyboardBuf = append(k.KeyboardBuf, 'X')

	var sawFault bool
	var syscallRet uint32
	for range 50 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			switch ev := e.(type) {
			case cpu.EventPageFault:
				sawFault = true
				if ev.PermNeeded != "W" {
					t.Errorf("page fault perm = %q, want W", ev.PermNeeded)
				}
			case cpu.EventSyscall:
				if ev.Num == 3 {
					syscallRet = ev.Result
				}
			}
		}
		if k.Processes[1].State == StateZombie {
			break
		}
	}
	if !sawFault {
		t.Error("no EventPageFault emitted for kernel-pointer sys_read")
	}
	if syscallRet != ^uint32(0) {
		t.Errorf("sys_read result = 0x%X, want 0xFFFFFFFF (-EFAULT)", syscallRet)
	}
	// Kernel memory at 0x100008 should still be whatever it was (here zero):
	// the rejected write must not have happened.
	if k.Mem.Bytes[0x100008] == 'X' {
		t.Error("kernel byte was overwritten — read clobbered memory before validating")
	}
}
