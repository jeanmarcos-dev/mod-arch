package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestNewMulti_ClampsCores verifies the NumCores argument is clamped to
// [1, MaxCores] so callers cannot accidentally spin up more cores than the
// kernel knows how to lay out kernel stacks for.
func TestNewMulti_ClampsCores(t *testing.T) {
	if k := NewMulti(cpu.DefaultMemSize, 0); k.NumCores() != 1 {
		t.Errorf("NewMulti(_, 0) -> %d cores, want 1", k.NumCores())
	}
	if k := NewMulti(cpu.DefaultMemSize, -5); k.NumCores() != 1 {
		t.Errorf("NewMulti(_, -5) -> %d cores, want 1", k.NumCores())
	}
	if k := NewMulti(cpu.DefaultMemSize, MaxCores+5); k.NumCores() != MaxCores {
		t.Errorf("NewMulti(_, MaxCores+5) -> %d cores, want %d", k.NumCores(), MaxCores)
	}
}

// TestNewMulti_SharedMemory makes sure every core sees the same physical RAM.
// Writes done by one core are visible from another — the property race_counter
// and friends rely on.
func TestNewMulti_SharedMemory(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	k.Cores[0].CPU.Mem.Bytes[cpu.AddrUserBase+0x10] = 0xAB
	got := k.Cores[1].CPU.Mem.Bytes[cpu.AddrUserBase+0x10]
	if got != 0xAB {
		t.Fatalf("core 1 read 0x%02X, want 0xAB (shared memory)", got)
	}
}

// TestNewMulti_DistinctKernelStacks ensures each core lands on its own kernel
// stack region on trap entry, so concurrent IRQs would not stomp each other's
// frames if we ever moved beyond sequential ticking.
func TestNewMulti_DistinctKernelStacks(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 3)
	seen := make(map[uint32]bool)
	for _, c := range k.Cores {
		if seen[c.CPU.KernelStackTop] {
			t.Errorf("KernelStackTop=0x%X reused across cores", c.CPU.KernelStackTop)
		}
		seen[c.CPU.KernelStackTop] = true
	}
}

// TestMulticore_TwoProcessesAdvanceOnTwoCores loads two trivial processes on a
// 2-core kernel and verifies both cores run their assigned process to halt.
// With 2 cores and 2 procs, neither needs to wait in the ready queue.
func TestMulticore_TwoProcessesAdvanceOnTwoCores(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	// p1: inc eax; inc eax; hlt
	if _, err := k.AddProcess("p1", []byte{0x40, 0x40, 0xF4}); err != nil {
		t.Fatalf("AddProcess p1: %v", err)
	}
	// p2: inc ebx; inc ebx; hlt
	if _, err := k.AddProcess("p2", []byte{0x43, 0x43, 0xF4}); err != nil {
		t.Fatalf("AddProcess p2: %v", err)
	}

	if k.Cores[0].CurrentPID != 1 {
		t.Fatalf("core 0 CurrentPID = %d, want 1", k.Cores[0].CurrentPID)
	}
	if k.Cores[1].CurrentPID != 2 {
		t.Fatalf("core 1 CurrentPID = %d, want 2 (second proc to second idle core)", k.Cores[1].CurrentPID)
	}

	// Step until both cores halt or we run out of budget.
	for step := 0; step < 200 && !k.AllHalted(); step++ {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
	}
	if !k.AllHalted() {
		t.Fatal("not every core halted after 200 steps")
	}

	if got := k.Cores[0].CPU.R.EAX; got != 2 {
		t.Errorf("core 0 EAX = %d, want 2 (two inc eax)", got)
	}
	if got := k.Cores[1].CPU.R.EBX; got != 2 {
		t.Errorf("core 1 EBX = %d, want 2 (two inc ebx)", got)
	}
}

// TestMulticore_EmitsCoreAssign verifies that loading a process emits a
// core_assign event for the core it landed on. The frontend uses these to
// draw which PID lives on which core.
func TestMulticore_EmitsCoreAssign(t *testing.T) {
	// We assert on AddProcess indirectly: the very first instruction of the
	// process executes on the core it was assigned to. To capture core_assign
	// events explicitly, run until the process halts; the boot path itself is
	// silent, but a subsequent context switch (or kill) emits assign/release.
	k := NewMulti(cpu.DefaultMemSize, 1)
	// Two processes on a single core to force ready-queue handoff via exit.
	// p1: int 0x80 (eax=1, sys_exit). p2: hlt.
	pExit := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (SysExit)
		0xCD, 0x80, // int 0x80
	}
	pHalt := []byte{0xF4}
	if _, err := k.AddProcess("p1", pExit); err != nil {
		t.Fatal(err)
	}
	if _, err := k.AddProcess("p2", pHalt); err != nil {
		t.Fatal(err)
	}

	var sawAssign, sawRelease bool
	for step := 0; step < 60 && !k.AllHalted(); step++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for _, e := range evs {
			switch e.(type) {
			case cpu.EventCoreAssign:
				sawAssign = true
			case cpu.EventCoreRelease:
				sawRelease = true
			}
		}
	}
	if !sawAssign {
		t.Error("no EventCoreAssign emitted across the handoff")
	}
	if !sawRelease {
		t.Error("no EventCoreRelease emitted across the handoff")
	}
}

// TestHasWork_PendingIRQWakesParkedKernel guards the regression where every
// core was halted (e.g. the only process is blocked in sys_read) and PushKey
// queued IRQ 0x21 on the boot core but AllHalted still reported true. The
// simulator stopped its run loop and the IRQ never dispatched, so the user
// keystroke vanished. HasWork must keep the loop alive while any core has a
// pending interrupt because the next Step will wake it via dispatchIRQ.
func TestHasWork_PendingIRQWakesParkedKernel(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	for _, c := range k.Cores {
		c.CPU.Halted = true
	}
	if k.HasWork() {
		t.Fatal("HasWork = true on fully halted kernel without IRQs")
	}
	k.PushKey('x')
	if !k.HasWork() {
		t.Fatal("HasWork = false after PushKey enqueued IRQ on boot core")
	}
}

// TestMulticore_ResetKeepsNonBootCoresHalted checks that Reset() leaves
// non-boot cores Halted=true so AllHalted() is true on an empty kernel. The
// per-core CPU.Reset clears Halted, so kernel.Reset has to put it back —
// otherwise a Reset between demos would leave the run loop spinning forever
// on idle cores.
func TestMulticore_ResetKeepsNonBootCoresHalted(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 3)
	if _, err := k.AddProcess("p1", []byte{0x40, 0xF4}); err != nil {
		t.Fatal(err)
	}
	k.Reset()
	if !k.AllHalted() {
		halted := make([]bool, 0, len(k.Cores))
		for _, c := range k.Cores {
			halted = append(halted, c.CPU.Halted)
		}
		t.Fatalf("AllHalted() = false after Reset; per-core Halted=%v", halted)
	}
}

// TestMulticore_IdleCoreWhenSingleProcess covers the (intentional) asymmetry:
// with N=2 cores but only 1 process, the second core sits idle (CurrentPID=0)
// and AllHalted returns true once the lone process halts.
//
// TimerQuantum is disabled so the PIT does not wake the halted process and
// resume it past the program's last byte (which would then decode as 0x00).
func TestMulticore_IdleCoreWhenSingleProcess(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	k.TimerQuantum = 0
	if _, err := k.AddProcess("only", []byte{0x40, 0xF4}); err != nil {
		t.Fatal(err)
	}

	if k.Cores[1].CurrentPID != 0 {
		t.Errorf("core 1 CurrentPID = %d, want 0 (idle)", k.Cores[1].CurrentPID)
	}

	for step := 0; step < 50 && !k.AllHalted(); step++ {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
	}
	if !k.AllHalted() {
		t.Fatal("not halted after 50 steps")
	}
}
