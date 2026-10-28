package kernel

import (
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// TestSpawnIdle_OnePerCore covers the per-core swapper contract. Linux's
// swapper is a task group (PID 0) with one task per CPU — init/0, init/1,
// etc. Our model mirrors that: a separate PCB lives in IdleByCore for each
// core, all sharing the conceptual PID 0 but with TIDs that disambiguate
// them in the snapshot. Sharing a single PCB across cores would lie about
// the per-CPU identity the scheduler maintains in real systems.
func TestSpawnIdle_OnePerCore(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 3)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}

	if got := len(k.IdleByCore); got != 3 {
		t.Fatalf("len(IdleByCore) = %d, want 3 (one swapper per core)", got)
	}
	seen := map[*PCB]bool{}
	for i, pcb := range k.IdleByCore {
		if pcb == nil {
			t.Fatalf("IdleByCore[%d] is nil", i)
		}
		if seen[pcb] {
			t.Errorf("IdleByCore[%d] is the same PCB as a previous core; swappers must not be shared", i)
		}
		seen[pcb] = true
		if pcb.PID != 0 {
			t.Errorf("IdleByCore[%d].PID = %d, want 0 (Linux convention)", i, pcb.PID)
		}
		// TIDs are negative so they never collide with user tasks (which
		// start at PID 1 and use TID==PID by default). Frontend treats
		// negative TIDs as "kernel-internal per-core thread".
		if pcb.TID != -(i + 1) {
			t.Errorf("IdleByCore[%d].TID = %d, want %d (negative-sentinel per-core)", i, pcb.TID, -(i + 1))
		}
		if pcb.State != StateIdle {
			t.Errorf("IdleByCore[%d].State = %q, want %q", i, pcb.State, StateIdle)
		}
		if pcb.Mode != cpu.ModeKernel {
			t.Errorf("IdleByCore[%d].Mode = %d, want %d (ModeKernel)", i, pcb.Mode, cpu.ModeKernel)
		}
		if pcb.Name == "" {
			t.Errorf("IdleByCore[%d].Name is empty; want something like \"swapper/N\"", i)
		}
	}
}

// TestSpawnIdle_LoadsHLTLoop verifies the swapper's body is the HLT + JMP
// back loop. Without the JMP, the IRET after an IRQ wake would advance EIP
// past the HLT byte and land in uninitialised memory.
func TestSpawnIdle_LoadsHLTLoop(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	pcb := k.IdleByCore[0]
	if k.Mem.Bytes[pcb.Regs.EIP] != 0xF4 {
		t.Errorf("byte at idle EIP = %#x, want 0xF4 (HLT)", k.Mem.Bytes[pcb.Regs.EIP])
	}
	if k.Mem.Bytes[pcb.Regs.EIP+1] != 0xEB || k.Mem.Bytes[pcb.Regs.EIP+2] != 0xFD {
		t.Errorf("bytes after HLT = %#x %#x, want 0xEB 0xFD (JMP -3)",
			k.Mem.Bytes[pcb.Regs.EIP+1], k.Mem.Bytes[pcb.Regs.EIP+2])
	}
}

// TestSpawnIdle_Idempotent guards against double-spawn corrupting the PCBs.
// SetIdleTaskEnabled may call SpawnIdle every time it flips ON; the second
// call must be a no-op rather than allocating new memory.
func TestSpawnIdle_Idempotent(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("first SpawnIdle: %v", err)
	}
	firsts := append([]*PCB(nil), k.IdleByCore...)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("second SpawnIdle: %v", err)
	}
	for i, pcb := range k.IdleByCore {
		if pcb != firsts[i] {
			t.Errorf("second SpawnIdle replaced IdleByCore[%d] (idempotency violated)", i)
		}
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
		t.Error("coreIsRunningIdle = true with IdleTaskEnabled but IdleByCore still empty")
	}

	if err := k.SpawnIdle(); err != nil {
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

// TestIdleLoop_ResumesHLTAfterWake covers the real-OS behavior: after an IRQ
// wakes the swapper, the IRET must return to the HLT instruction, not to the
// byte after it. A one-byte idleProgram would advance EIP past the HLT and
// IRET into uninitialised memory (opcode 0x00 → "not implemented" error).
// The fix is a HLT + JMP back loop, mirroring how Linux's idle thread is
// implemented in assembly.
func TestIdleLoop_ResumesHLTAfterWake(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	core := k.Cores[0]
	k.bindIdleToCore(core)

	// Step once: the HLT byte runs, the CPU parks halted.
	if _, err := core.CPU.Step(); err != nil {
		t.Fatalf("step 1 (HLT): %v", err)
	}
	if !core.CPU.Halted {
		t.Fatal("CPU not halted after stepping over the HLT byte")
	}
	startEIP := core.CPU.R.EIP

	// Simulate an IRQ wake by clearing Halted and resuming execution.
	// The CPU should land on the JMP that closes the idle loop and bounce
	// back to the HLT byte, parking the CPU again on the next step.
	core.CPU.Halted = false
	if _, err := core.CPU.Step(); err != nil {
		t.Fatalf("step 2 (JMP back): %v", err)
	}
	if core.CPU.R.EIP != idleCodeBase {
		t.Fatalf("after JMP back, EIP = %#x, want %#x (HLT byte)", core.CPU.R.EIP, idleCodeBase)
	}
	if _, err := core.CPU.Step(); err != nil {
		t.Fatalf("step 3 (HLT again): %v", err)
	}
	if !core.CPU.Halted {
		t.Errorf("CPU did not re-park after returning to the HLT byte (EIP started at %#x)", startEIP)
	}
}

// TestBindIdleToCore_StateAndEvents covers the side-effects of binding the
// swapper to a core: PCB stays in IDLE (no transition to RUN), the core is
// woken from its halted-on-boot state so the next Step can decode the HLT
// byte, and a paired EventCoreAssign + EventCoreIdle is emitted so the
// frontend can distinguish "PID 0 took the core" from "core has no proc".
func TestBindIdleToCore_StateAndEvents(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if err := k.SpawnIdle(); err != nil {
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
	pcb := k.IdleByCore[core.ID]
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

// TestIdleLoop_DispatchIRQDoesNotFault covers the kernel-stack contract for
// the swapper. While a core is parked on HLT, an arriving IRQ enters from
// ModeKernel — dispatchIRQ does NOT consult KernelStackTop in that path (real
// x86 only switches stacks on ring transitions), so the swapper PCB itself
// must already carry a valid stack pointer. If SavedUserESP is zero on the
// idle PCB, loadCPUFromPCB clobbers c.R.ESP to 0 and the first pushKernel
// underflows to 0xFFFFFFFC, which the physical memory rejects.
func TestIdleLoop_DispatchIRQDoesNotFault(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if err := k.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	core := k.Cores[0]
	pcb := k.IdleByCore[0]
	k.bindIdleToCore(core)

	if _, err := core.CPU.Step(); err != nil {
		t.Fatalf("step 1 (HLT): %v", err)
	}
	if !core.CPU.Halted {
		t.Fatal("core did not halt after stepping the HLT byte")
	}
	preIRQEFLAGS := core.CPU.R.EFLAGS

	core.CPU.PendingIRQs = append(core.CPU.PendingIRQs, IRQKeyboard)
	if _, err := core.CPU.Step(); err != nil {
		t.Fatalf("dispatchIRQ on idle core failed: %v", err)
	}

	wantESP := pcb.StackTop - 12
	if core.CPU.R.ESP != wantESP {
		t.Fatalf("ESP after IRQ entry = %#x, want %#x (stackTop - 12)", core.CPU.R.ESP, wantESP)
	}
	gotEFLAGS, err := core.CPU.Mem.Read32(nil, core.CPU.R.ESP+8, cpu.ModeKernel)
	if err != nil {
		t.Fatalf("read pushed EFLAGS: %v", err)
	}
	if gotEFLAGS != preIRQEFLAGS {
		t.Errorf("pushed EFLAGS = %#x, want %#x", gotEFLAGS, preIRQEFLAGS)
	}
}
