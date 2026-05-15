package kernel

import (
	"fmt"
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// newTestKernel builds a kernel with a single user process and returns it.
func newTestKernel(t *testing.T, userProg []byte) *Kernel {
	t.Helper()
	k := New(cpu.DefaultMemSize)
	if _, err := k.AddProcess("p1", userProg); err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	return k
}

// newTestKernelMulti loads several processes in order and returns the kernel.
func newTestKernelMulti(t *testing.T, progs ...[]byte) *Kernel {
	t.Helper()
	k := New(cpu.DefaultMemSize)
	for i, p := range progs {
		if _, err := k.AddProcess(fmt.Sprintf("p%d", i+1), p); err != nil {
			t.Fatalf("AddProcess %d: %v", i+1, err)
		}
	}
	return k
}

// findEvent looks up the first event of the given type in the slice.
func findEvent[T cpu.Event](evs []cpu.Event) (T, bool) {
	var zero T
	for _, e := range evs {
		if v, ok := e.(T); ok {
			return v, true
		}
	}
	return zero, false
}

func TestIDT_Installed(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	for _, vec := range []uint8{IRQDivideError, IRQPageFault, IRQTimer, IRQKeyboard, IRQSyscall} {
		addr := k.HandlerAddr(vec)
		if addr == 0 {
			t.Errorf("IDT[0x%02X] not installed", vec)
		}
		if addr < cpu.AddrKernelText {
			t.Errorf("IDT[0x%02X] = 0x%X, should point into kernel text", vec, addr)
		}
	}
}

func TestSoftInt_EntersKernel(t *testing.T) {
	// User program: int 0x80 ; hlt
	k := newTestKernel(t, []byte{
		0xCD, 0x80, // int 0x80
		0xF4, // hlt (should not run until after IRET)
	})

	evs, err := k.Step()
	if err != nil {
		t.Fatalf("step int 0x80: %v", err)
	}

	if k.CPU.Mode != cpu.ModeKernel {
		t.Fatalf("after int 0x80, mode = %d, want kernel(0)", k.CPU.Mode)
	}

	irq, ok := findEvent[cpu.EventIRQFire](evs)
	if !ok {
		t.Fatal("no EventIRQFire emitted")
	}
	if irq.Vector != 0x80 || irq.Source != "SW" {
		t.Errorf("IRQFire = %+v, want {vector:0x80, source:SW}", irq)
	}

	ms, ok := findEvent[cpu.EventModeSwitch](evs)
	if !ok {
		t.Fatal("no EventModeSwitch emitted")
	}
	if ms.From != cpu.ModeUser || ms.To != cpu.ModeKernel || ms.Cause != "INT" {
		t.Errorf("ModeSwitch = %+v", ms)
	}

	if k.CPU.R.EIP != cpu.AddrKernelText+0x40 {
		t.Errorf("EIP = 0x%X, want 0x%X (syscall handler)", k.CPU.R.EIP, cpu.AddrKernelText+0x40)
	}

	if k.CPU.FlagOn(cpu.FlagIF) {
		t.Error("IF should be off after dispatch")
	}

	if k.CPU.R.ESP > cpu.KernelStackTop || k.CPU.R.ESP < cpu.AddrKernelBase {
		t.Errorf("ESP = 0x%X, does not look like the kernel stack", k.CPU.R.ESP)
	}
}

func TestIRET_ReturnsToUser(t *testing.T) {
	// int 0x80 ; nop ; hlt -- the syscall handler (nop nop iret) should
	// return into the nop.
	k := newTestKernel(t, []byte{
		0xCD, 0x80, // int 0x80
		0x90, // nop (IRET target)
		0xF4, // hlt
	})

	userEIPBeforeINT := k.CPU.R.EIP
	userESPBeforeINT := k.CPU.R.ESP

	if _, err := k.Step(); err != nil {
		t.Fatal(err)
	}
	if k.CPU.Mode != cpu.ModeKernel {
		t.Fatalf("should be in kernel after int")
	}

	// The syscall handler is nop nop iret (3 instructions).
	for i := range 3 {
		_, err := k.Step()
		if err != nil {
			t.Fatalf("step kernel %d: %v", i, err)
		}
	}

	if k.CPU.Mode != cpu.ModeUser {
		t.Errorf("after IRET mode = %d, want user(3)", k.CPU.Mode)
	}
	// int 0x80 is 2 bytes; return EIP = userEIPBeforeINT + 2
	if k.CPU.R.EIP != userEIPBeforeINT+2 {
		t.Errorf("EIP = 0x%X, want 0x%X (instruction after int)", k.CPU.R.EIP, userEIPBeforeINT+2)
	}
	if k.CPU.R.ESP != userESPBeforeINT {
		t.Errorf("user ESP = 0x%X, want 0x%X (restored)", k.CPU.R.ESP, userESPBeforeINT)
	}
}

func TestTimer_FiresAtQuantum(t *testing.T) {
	k := newTestKernel(t, []byte{
		0x90, 0x90, 0x90, 0x90,
		0x90, 0x90, 0x90, 0x90,
		0x90, 0x90,
		0xF4,
	})
	k.TimerQuantum = 8

	var timerFired bool
	for step := range 30 {
		if timerFired {
			break
		}
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for _, e := range evs {
			if irq, ok := e.(cpu.EventIRQFire); ok {
				if irq.Vector == IRQTimer && irq.Source == "HW" {
					timerFired = true
				}
			}
		}
	}
	if !timerFired {
		t.Fatal("timer never fired within 30 steps")
	}
}

func TestTimer_OnlyFiresWithIFEnabled(t *testing.T) {
	// IF=0 must suppress timer dispatch (mirrors a handler that does not want
	// to be re-interrupted).
	k := newTestKernel(t, []byte{0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0xF4})
	k.TimerQuantum = 4
	k.CPU.FlagSet(cpu.FlagIF, false)

	// The timer will still queue (PendingIRQs grows) but the CPU must not
	// dispatch while IF=0.
	for step := range 10 {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for _, e := range evs {
			if _, ok := e.(cpu.EventIRQFire); ok {
				t.Fatalf("step %d: IRQ fired with IF=0", step)
			}
			if _, ok := e.(cpu.EventModeSwitch); ok {
				t.Fatalf("step %d: mode switch happened with IF=0", step)
			}
		}
	}
}

func TestAddProcess_FirstIsRun(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	pid, err := k.AddProcess("p1", []byte{0x90, 0xF4})
	if err != nil {
		t.Fatal(err)
	}
	if pid != 1 {
		t.Errorf("first PID = %d, want 1", pid)
	}
	if k.CurrentPID != 1 {
		t.Errorf("CurrentPID = %d, want 1", k.CurrentPID)
	}
	if len(k.ReadyQueue) != 0 {
		t.Errorf("ready queue should be empty, got: %v", k.ReadyQueue)
	}
	if k.Processes[1].State != StateRun {
		t.Errorf("state = %s, want RUN", k.Processes[1].State)
	}
}

func TestAddProcess_SecondIsReady(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	_, _ = k.AddProcess("p1", []byte{0x90, 0xF4})
	_, _ = k.AddProcess("p2", []byte{0x90, 0xF4})

	if k.CurrentPID != 1 {
		t.Errorf("CurrentPID = %d, want 1 (p2 should be in ready queue)", k.CurrentPID)
	}
	if len(k.ReadyQueue) != 1 || k.ReadyQueue[0] != 2 {
		t.Errorf("ready queue = %v, want [2]", k.ReadyQueue)
	}
	if k.Processes[2].State != StateReady {
		t.Errorf("p2 state = %s, want READY", k.Processes[2].State)
	}
}

func TestContextSwitch_TwoCounters(t *testing.T) {
	// Two processes each incrementing a different register in an infinite
	// loop. With quantum=4 the timer should interleave them and we expect
	// both registers to grow.
	progP1 := []byte{
		0xB8, 0x00, 0x00, 0x00, 0x00, // mov eax, 0
		0x40,       // inc eax
		0xEB, 0xFD, // jmp -3
	}
	progP2 := []byte{
		0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
		0x43,       // inc ebx
		0xEB, 0xFD, // jmp -3
	}
	k := newTestKernelMulti(t, progP1, progP2)
	k.TimerQuantum = 4

	for range 60 {
		if _, err := k.Step(); err != nil {
			t.Fatal(err)
		}
	}

	p1 := k.Processes[1]
	p2 := k.Processes[2]

	// The running process keeps its regs live on the CPU; the other lives
	// in its PCB.
	var eaxP1, ebxP2 uint32
	if k.CurrentPID == 1 {
		eaxP1 = k.CPU.R.EAX
		ebxP2 = p2.Regs.EBX
	} else {
		eaxP1 = p1.Regs.EAX
		ebxP2 = k.CPU.R.EBX
	}

	if eaxP1 == 0 {
		t.Errorf("p1 EAX = 0, did not progress")
	}
	if ebxP2 == 0 {
		t.Errorf("p2 EBX = 0, did not progress")
	}
}

func TestContextSwitch_EmitsExpectedEvents(t *testing.T) {
	// On a context switch all four key events must be emitted.
	k := newTestKernelMulti(t,
		[]byte{0x40, 0xEB, 0xFE}, // p1: inc eax; jmp -2
		[]byte{0x43, 0xEB, 0xFE}, // p2: inc ebx; jmp -2
	)
	k.TimerQuantum = 4

	var (
		gotSave, gotRestore, gotSched, gotProcState bool
		schedDecision                               cpu.EventSchedDecision
	)
	for range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			switch ev := e.(type) {
			case cpu.EventPCBSave:
				gotSave = true
			case cpu.EventPCBRestore:
				gotRestore = true
			case cpu.EventSchedDecision:
				gotSched = true
				schedDecision = ev
			case cpu.EventProcessState:
				gotProcState = true
			}
		}
		if gotSave && gotRestore && gotSched && gotProcState {
			break
		}
	}
	if !gotSave {
		t.Error("no EventPCBSave emitted")
	}
	if !gotRestore {
		t.Error("no EventPCBRestore emitted")
	}
	if !gotSched {
		t.Error("no EventSchedDecision emitted")
	}
	if !gotProcState {
		t.Error("no EventProcessState emitted")
	}
	if gotSched && (schedDecision.FromPID == 0 || schedDecision.ToPID == 0) {
		t.Errorf("sched_decision incomplete: %+v", schedDecision)
	}
}

func TestContextSwitch_RegsArePreserved(t *testing.T) {
	// The EAX that p1 held when preempted must be the exact value stored in
	// p1.PCB.Regs.EAX and must be restored when p1 returns to RUN.
	progP1 := []byte{
		0xB8, 0x00, 0x00, 0x00, 0x00, // mov eax, 0
		0x40,       // inc eax
		0xEB, 0xFD, // jmp -3
	}
	progP2 := []byte{0x90, 0xEB, 0xFE} // p2 only nops; does not touch regs
	k := newTestKernelMulti(t, progP1, progP2)
	k.TimerQuantum = 4

	var eaxP1WhenPreempted uint32
	var switchedToP2 bool
	for range 40 {
		_, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		if !switchedToP2 && k.CurrentPID == 2 {
			eaxP1WhenPreempted = k.Processes[1].Regs.EAX
			switchedToP2 = true
		}
	}
	if !switchedToP2 {
		t.Fatal("never switched to p2")
	}
	if eaxP1WhenPreempted == 0 {
		t.Errorf("p1 saved EAX = 0, should have grown before preemption")
	}

	// Run until p1 resumes and verify EAX did not break.
	for range 40 {
		_, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		if k.CurrentPID == 1 {
			// p1 resumed; EAX should be >= the saved value (it kept growing).
			if k.CPU.R.EAX < eaxP1WhenPreempted {
				t.Errorf("p1 EAX on resume = %d, want >= %d (not restored)",
					k.CPU.R.EAX, eaxP1WhenPreempted)
			}
			return
		}
	}
	t.Fatal("p1 never returned to RUN")
}
