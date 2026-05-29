package kernel

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// newTestKernel builds a kernel with a single user process and returns it
// ready to step (dispatch already done).
func newTestKernel(t *testing.T, userProg []byte) *Kernel {
	t.Helper()
	k := New(cpu.DefaultMemSize)
	if _, err := k.AddProcess("p1", userProg); err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	k.DispatchIdleCores()
	return k
}

// newTestKernelMulti loads several processes in order and returns the kernel
// ready to step (dispatch already done).
func newTestKernelMulti(t *testing.T, progs ...[]byte) *Kernel {
	t.Helper()
	k := New(cpu.DefaultMemSize)
	for i, p := range progs {
		if _, err := k.AddProcess(fmt.Sprintf("p%d", i+1), p); err != nil {
			t.Fatalf("AddProcess %d: %v", i+1, err)
		}
	}
	k.DispatchIdleCores()
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

	if k.BootCPU().Mode != cpu.ModeKernel {
		t.Fatalf("after int 0x80, mode = %d, want kernel(0)", k.BootCPU().Mode)
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

	if k.BootCPU().R.EIP != cpu.AddrKernelText+0x40 {
		t.Errorf("EIP = 0x%X, want 0x%X (syscall handler)", k.BootCPU().R.EIP, cpu.AddrKernelText+0x40)
	}

	if k.BootCPU().FlagOn(cpu.FlagIF) {
		t.Error("IF should be off after dispatch")
	}

	if k.BootCPU().R.ESP > cpu.KernelStackTop || k.BootCPU().R.ESP < cpu.AddrKernelBase {
		t.Errorf("ESP = 0x%X, does not look like the kernel stack", k.BootCPU().R.ESP)
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

	userEIPBeforeINT := k.BootCPU().R.EIP
	userESPBeforeINT := k.BootCPU().R.ESP

	if _, err := k.Step(); err != nil {
		t.Fatal(err)
	}
	if k.BootCPU().Mode != cpu.ModeKernel {
		t.Fatalf("should be in kernel after int")
	}

	// The syscall handler is nop nop iret (3 instructions).
	for i := range 3 {
		_, err := k.Step()
		if err != nil {
			t.Fatalf("step kernel %d: %v", i, err)
		}
	}

	if k.BootCPU().Mode != cpu.ModeUser {
		t.Errorf("after IRET mode = %d, want user(3)", k.BootCPU().Mode)
	}
	// int 0x80 is 2 bytes; return EIP = userEIPBeforeINT + 2
	if k.BootCPU().R.EIP != userEIPBeforeINT+2 {
		t.Errorf("EIP = 0x%X, want 0x%X (instruction after int)", k.BootCPU().R.EIP, userEIPBeforeINT+2)
	}
	if k.BootCPU().R.ESP != userESPBeforeINT {
		t.Errorf("user ESP = 0x%X, want 0x%X (restored)", k.BootCPU().R.ESP, userESPBeforeINT)
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

// TestTimer_QuantumCountsUserInstructionsExactly pins the determinism the
// userInstructionsSinceFire refactor delivers. With a fixed-cost handler
// (NOP + IRET = two kernel ticks) every quantum — the first AND every
// subsequent one — must consume EXACTLY TimerQuantum user instructions.
// Pre-refactor the first quantum had Q user instrs and later ones had
// Q - handlerCost: the kernel "ate" the handler ticks out of the next user
// slice. That divergence is what made counting clicks in the trace
// pedagogically confusing.
func TestTimer_QuantumCountsUserInstructionsExactly(t *testing.T) {
	// 32 NOPs + HLT. The NOPs give us a long, easily-counted user slice;
	// HLT at the end stops the program cleanly when the test finishes.
	prog := append(bytes.Repeat([]byte{0x90}, 32), 0xF4)
	k := newTestKernel(t, prog)
	k.TimerQuantum = 8

	var fires []int
	userInstrSinceLastFire := 0
	for range 200 {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step error: %v", err)
		}
		for _, e := range evs {
			switch v := e.(type) {
			case cpu.EventInstruction:
				// stepCore stamps every executed instruction with runningPID,
				// including handler bytes (NOP+IRET at 0x101020). Filter by
				// address: user code lives at AddrUserBase (0x200000) and up.
				if v.EIP >= cpu.AddrUserBase {
					userInstrSinceLastFire++
				}
			case cpu.EventIRQFire:
				if v.Vector == IRQTimer && v.Source == "HW" {
					fires = append(fires, userInstrSinceLastFire)
					userInstrSinceLastFire = 0
				}
			}
		}
		if len(fires) >= 3 {
			break
		}
	}
	if len(fires) < 3 {
		t.Fatalf("only %d timer fires observed, want at least 3", len(fires))
	}
	for i, n := range fires {
		if n != 8 {
			t.Errorf("fire #%d preceded by %d user instrs, want 8 (handler cost must not steal from the next quantum)", i+1, n)
		}
	}
}

func TestTimer_OnlyFiresWithIFEnabled(t *testing.T) {
	// IF=0 must suppress timer dispatch (mirrors a handler that does not want
	// to be re-interrupted).
	k := newTestKernel(t, []byte{0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0xF4})
	k.TimerQuantum = 4
	k.BootCPU().FlagSet(cpu.FlagIF, false)

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

func TestAddProcess_GoesToReadyQueue(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	pid, err := k.AddProcess("p1", []byte{0x90, 0xF4})
	if err != nil {
		t.Fatal(err)
	}
	if pid != 1 {
		t.Errorf("first PID = %d, want 1", pid)
	}
	if k.BootCore().CurrentPID != 0 {
		t.Errorf("CurrentPID = %d, want 0 (no dispatch on load)", k.BootCore().CurrentPID)
	}
	if snap := k.Ready.Snapshot(); len(snap) != 1 || snap[0] != 1 {
		t.Errorf("ready queue = %v, want [1]", snap)
	}
	if k.Processes[1].State != StateReady {
		t.Errorf("state = %s, want READY", k.Processes[1].State)
	}
}

func TestAddProcess_AllPendingInReadyQueue(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	_, _ = k.AddProcess("p1", []byte{0x90, 0xF4})
	_, _ = k.AddProcess("p2", []byte{0x90, 0xF4})

	if k.BootCore().CurrentPID != 0 {
		t.Errorf("CurrentPID = %d, want 0 (no dispatch on load)", k.BootCore().CurrentPID)
	}
	if snap := k.Ready.Snapshot(); len(snap) != 2 || snap[0] != 1 || snap[1] != 2 {
		t.Errorf("ready queue = %v, want [1 2]", snap)
	}
	if k.Processes[1].State != StateReady || k.Processes[2].State != StateReady {
		t.Errorf("states = %s/%s, want both READY", k.Processes[1].State, k.Processes[2].State)
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
	if k.BootCore().CurrentPID == 1 {
		eaxP1 = k.BootCPU().R.EAX
		ebxP2 = p2.Regs.EBX
	} else {
		eaxP1 = p1.Regs.EAX
		ebxP2 = k.BootCPU().R.EBX
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
		if !switchedToP2 && k.BootCore().CurrentPID == 2 {
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
		if k.BootCore().CurrentPID == 1 {
			// p1 resumed; EAX should be >= the saved value (it kept growing).
			if k.BootCPU().R.EAX < eaxP1WhenPreempted {
				t.Errorf("p1 EAX on resume = %d, want >= %d (not restored)",
					k.BootCPU().R.EAX, eaxP1WhenPreempted)
			}
			return
		}
	}
	t.Fatal("p1 never returned to RUN")
}

func TestHLT_InUserModeRaisesGPAndKillsProcess(t *testing.T) {
	// HLT is privileged. A user process that runs it must hit #GP (vector
	// 0x0D), enter the kernel and end up ZOMBIE with reason
	// "general_protection". The other process keeps running normally.
	hltProg := []byte{0xF4}              // hlt (illegal in PL3)
	loopProg := []byte{0x90, 0xEB, 0xFE} // nop ; jmp -2
	k := newTestKernelMulti(t, hltProg, loopProg)
	k.TimerQuantum = 0

	var sawGPFire bool
	for range 20 {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step: %v", err)
		}
		for _, e := range evs {
			if fire, ok := e.(cpu.EventIRQFire); ok && fire.Vector == 0x0D {
				sawGPFire = true
				if fire.Source != "fault" {
					t.Errorf("#GP source = %q, want fault", fire.Source)
				}
			}
		}
		if k.Processes[1].State == StateZombie {
			break
		}
	}
	if !sawGPFire {
		t.Error("no #GP (vector 0x0D) IRQFire emitted")
	}
	if k.Processes[1].State != StateZombie {
		t.Errorf("P1 state = %s, want ZOMBIE", k.Processes[1].State)
	}
	if k.Processes[1].BlockedOn != "general_protection" {
		t.Errorf("P1 reason = %q, want general_protection", k.Processes[1].BlockedOn)
	}
	if k.Processes[2].State == StateZombie {
		t.Error("P2 should not be affected by P1's #GP")
	}
}

func TestHLT_GPFrameSavesOffendingEIP(t *testing.T) {
	// For faults x86 saves the EIP of the offending instruction, not the
	// next one. Run HLT in user mode and verify the IRQFire frame points at
	// the HLT byte itself — otherwise the UI would highlight the byte AFTER
	// the fault site.
	k := newTestKernel(t, []byte{0x90, 0xF4}) // nop ; hlt
	k.TimerQuantum = 0

	var fire *cpu.EventIRQFire
	for range 10 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if f, ok := e.(cpu.EventIRQFire); ok && f.Vector == 0x0D {
				cp := f
				fire = &cp
			}
		}
		if fire != nil {
			break
		}
	}
	if fire == nil {
		t.Fatal("no #GP IRQFire emitted")
	}
	// Program loaded at AddrUserBase. NOP is at offset 0, HLT at offset 1.
	wantEIP := cpu.AddrUserBase + 1
	if fire.SavedEIP != wantEIP {
		t.Errorf("saved_eip = 0x%X, want 0x%X (HLT offset, not NOP+1)", fire.SavedEIP, wantEIP)
	}
}

func TestHLT_InKernelModeHaltsCleanly(t *testing.T) {
	// Sanity check that the privilege gate only fires in PL3. Run HLT directly
	// on a CPU set to kernel mode and verify it just halts.
	c := cpu.New(cpu.DefaultMemSize)
	c.Mode = cpu.ModeKernel
	c.R.EIP = cpu.AddrKernelText
	c.R.CS = cpu.SelKernelCode
	if err := c.Mem.LoadBytes(cpu.AddrKernelText, []byte{0xF4}); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := c.Step(); err != nil {
		t.Fatalf("step: %v", err)
	}
	if !c.Halted {
		t.Error("expected CPU to be halted after HLT in kernel mode")
	}
}

func TestEventInstruction_PIDStampedFromRunningProcess(t *testing.T) {
	// With two processes alternating under the timer, every EventInstruction
	// must carry the PID of the process that actually executed it (snapshot
	// taken before the step, since a context switch *inside* the step would
	// otherwise leak the next PID back into the past).
	k := newTestKernelMulti(t,
		[]byte{0x40, 0xEB, 0xFE}, // p1: inc eax; jmp -2
		[]byte{0x43, 0xEB, 0xFE}, // p2: inc ebx; jmp -2
	)
	k.TimerQuantum = 4

	sawPID := map[int]bool{}
	for range 40 {
		pidBefore := k.BootCore().CurrentPID
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			inst, ok := e.(cpu.EventInstruction)
			if !ok {
				continue
			}
			if inst.PID != pidBefore {
				t.Errorf("EventInstruction PID=%d, want %d (pre-step running PID)",
					inst.PID, pidBefore)
			}
			sawPID[inst.PID] = true
		}
	}
	if !sawPID[1] || !sawPID[2] {
		t.Errorf("expected EventInstruction from both PIDs, saw: %v", sawPID)
	}
}
