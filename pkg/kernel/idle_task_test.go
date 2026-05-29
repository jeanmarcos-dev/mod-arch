package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestSpawnIdle_RegistersPID0 covers the swapper's birth: SpawnIdle must add a
// PCB at PID 0 (Linux convention), keep nextPID at 1 so user PIDs are not
// shifted, mark the PCB as kernel-mode IDLE so the scheduler never preempts it
// via the timer, and place its code in the reserved kernel zone (not in a
// user-code slot) so demos keep loading at AddrUserBase.
func TestSpawnIdle_RegistersPID0(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	nextBefore := k.nextPID

	pid, err := k.SpawnIdle()
	if err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	if pid != 0 {
		t.Fatalf("SpawnIdle returned pid %d, want 0", pid)
	}
	if k.nextPID != nextBefore {
		t.Errorf("nextPID = %d, want %d (PID 0 must be a reserved sentinel, not a counter handout)", k.nextPID, nextBefore)
	}

	pcb := k.Processes[0]
	if pcb == nil {
		t.Fatal("k.Processes[0] is nil after SpawnIdle")
	}
	if pcb.Name != "swapper" {
		t.Errorf("pcb.Name = %q, want %q", pcb.Name, "swapper")
	}
	if pcb.State != StateIdle {
		t.Errorf("pcb.State = %q, want %q", pcb.State, StateIdle)
	}
	if pcb.Mode != cpu.ModeKernel {
		t.Errorf("pcb.Mode = %d, want %d (ModeKernel)", pcb.Mode, cpu.ModeKernel)
	}
	if pcb.Regs.CS != cpu.SelKernelCode {
		t.Errorf("pcb.Regs.CS = %#x, want %#x (SelKernelCode)", pcb.Regs.CS, cpu.SelKernelCode)
	}
	if pcb.Slot != -1 {
		t.Errorf("pcb.Slot = %d, want -1 (swapper does not occupy a user slot)", pcb.Slot)
	}
	if pcb.Regs.EIP < cpu.AddrKernelText {
		t.Errorf("pcb.Regs.EIP = %#x, expected to live in the kernel zone (>= %#x)", pcb.Regs.EIP, cpu.AddrKernelText)
	}
}

// TestSpawnIdle_LoadsHLTByte verifies the swapper's body is exactly one HLT
// instruction (0xF4) loaded at its EIP — the real x86 idle loop primitive.
// A regression that loaded the wrong byte would either crash the simulator
// (decoding garbage) or leave the core busy-looping instead of halting.
func TestSpawnIdle_LoadsHLTByte(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	pcb := k.Processes[0]
	got := k.Mem.Bytes[pcb.Regs.EIP]
	if got != 0xF4 {
		t.Errorf("byte at idle EIP = %#x, want 0xF4 (HLT)", got)
	}
}

// TestSpawnIdle_Idempotent guards against double-spawn corrupting the PCB.
// SetIdleTaskEnabled may call SpawnIdle every time it flips ON; the second
// call must be a no-op that returns the existing PID rather than allocating
// memory or bumping bookkeeping.
func TestSpawnIdle_Idempotent(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnIdle(); err != nil {
		t.Fatalf("first SpawnIdle: %v", err)
	}
	first := k.Processes[0]
	pid, err := k.SpawnIdle()
	if err != nil {
		t.Fatalf("second SpawnIdle: %v", err)
	}
	if pid != 0 {
		t.Errorf("second SpawnIdle returned pid %d, want 0", pid)
	}
	if k.Processes[0] != first {
		t.Errorf("second SpawnIdle replaced the PCB (idempotency violated)")
	}
}

// TestCoreIsRunningIdle covers the disambiguation helper. core.CurrentPID == 0
// is overloaded: it means "no proc" when idle is disabled, but "swapper bound"
// when idle is enabled and Processes[0] exists. coreIsRunningIdle is the only
// place that contract lives, so other callers can stay oblivious.
func TestCoreIsRunningIdle(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	core := k.Cores[0]

	if k.coreIsRunningIdle(core) {
		t.Error("coreIsRunningIdle = true on a fresh kernel (idle not enabled, PCB not spawned)")
	}

	k.IdleTaskEnabled = true
	if k.coreIsRunningIdle(core) {
		t.Error("coreIsRunningIdle = true with IdleTaskEnabled but Processes[0] still nil")
	}

	if _, err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	// CurrentPID==0 alone is the legacy "no proc" sentinel; the swapper
	// being bound requires the dedicated RunningIdle flag set by
	// bindIdleToCore. Without it, coreIsRunningIdle must return false so
	// the fallback path can re-bind without confusion.
	core.CurrentPID = 0
	core.RunningIdle = false
	if k.coreIsRunningIdle(core) {
		t.Error("coreIsRunningIdle = true with CurrentPID==0 but RunningIdle=false (would conflate the no-proc sentinel)")
	}

	k.bindIdleToCore(core)
	if !k.coreIsRunningIdle(core) {
		t.Error("coreIsRunningIdle = false after bindIdleToCore")
	}

	core.CurrentPID = 7
	if k.coreIsRunningIdle(core) {
		t.Error("coreIsRunningIdle = true when CurrentPID != 0")
	}
}

// TestBindIdleToCore_StateAndEvents covers the side-effects of binding the
// swapper to a core: PCB stays in IDLE (no transition to RUN), the core is
// woken from its halted-on-boot state so the next Step can decode the HLT
// byte, and a paired EventCoreAssign + EventCoreIdle is emitted so the
// frontend can distinguish "PID 0 took the core" from "core has no proc".
func TestBindIdleToCore_StateAndEvents(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	core := k.Cores[0]

	evs := k.bindIdleToCore(core)

	if core.CurrentPID != 0 {
		t.Errorf("core.CurrentPID = %d after bindIdleToCore, want 0", core.CurrentPID)
	}
	if core.CPU.Halted {
		t.Error("core.CPU.Halted = true after bindIdleToCore (should be false so the next Step decodes HLT)")
	}
	pcb := k.Processes[0]
	if pcb.State != StateIdle {
		t.Errorf("pcb.State = %q after bindIdleToCore, want %q (idle never transitions to RUN)", pcb.State, StateIdle)
	}
	if pcb.Core != core.ID {
		t.Errorf("pcb.Core = %d after bindIdleToCore, want %d", pcb.Core, core.ID)
	}

	var sawAssign, sawIdle bool
	for _, e := range evs {
		switch ev := e.(type) {
		case cpu.EventCoreAssign:
			if ev.CoreID == core.ID && ev.PID == 0 {
				sawAssign = true
			}
		case cpu.EventCoreIdle:
			if ev.CoreID == core.ID {
				sawIdle = true
			}
		}
	}
	if !sawAssign {
		t.Error("bindIdleToCore did not emit EventCoreAssign{core, pid:0}")
	}
	if !sawIdle {
		t.Error("bindIdleToCore did not emit EventCoreIdle for the bound core")
	}
}
