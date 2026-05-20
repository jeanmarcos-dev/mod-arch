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
