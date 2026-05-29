package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// idleProgram is the body of the swapper (PID 0). A single HLT instruction
// is the canonical x86 idle primitive: the CPU halts and waits for the next
// IRQ to resume. The kernel never advances EIP past it; on every wake-up the
// PCB is rebound at idleCodeBase so execution falls back into HLT after the
// handler returns. This mirrors how a real OS implements its per-CPU idle
// loop (Linux: `cpu_idle_poll` / `default_idle_call`).
var idleProgram = []byte{0xF4}

// idleCodeBase places the swapper just past init's reserved slice inside the
// kernel-text region. It must stay above initCodeBase+len(initProgram) so
// the two bodies do not overlap, and below KernelStackTop so the per-core
// kernel stacks (which grow downward) never trample it.
const idleCodeBase uint32 = initCodeBase + 0x100

// idleStackTop is the initial ESP for the swapper's tiny kernel stack. PID 0
// never enters user mode, but loadCPUFromPCB still expects a valid ESP.
const idleStackTop uint32 = idleCodeBase + 0x80

// SpawnIdle allocates the kernel idle task (PID 0, "swapper") and parks it
// in StateIdle. Idempotent: calling twice returns the existing PID without
// re-allocating memory.
//
// Unlike SpawnInit, the swapper carries Mode=Kernel and runs from
// kernel-text. That gives two pedagogical benefits: it matches the real
// Linux convention (swapper is ring-0), and shouldFireTimer already excludes
// kernel-mode tasks so the timer never preempts PID 0 — exactly the
// semantics we want for an idle loop.
func (k *Kernel) SpawnIdle() (int, error) {
	if pcb := k.Processes[0]; pcb != nil {
		return 0, nil
	}

	for i := idleCodeBase; i < idleCodeBase+uint32(len(idleProgram)); i++ {
		k.Mem.Bytes[i] = 0
	}
	if err := k.Mem.LoadBytes(idleCodeBase, idleProgram); err != nil {
		return 0, err
	}

	pcb := &PCB{
		PID:       0,
		Name:      "swapper",
		State:     StateIdle,
		Mode:      cpu.ModeKernel,
		StackTop:  idleStackTop,
		Slot:      -1,
		Core:      -1,
		TGID:      0,
		ParentPID: 0,
		memSpace:  k.newMemSpace(idleCodeBase, uint32(len(idleProgram)), idleCodeBase+0x40),
		fdSpace:   k.newFDSpace(),
	}
	pcb.Regs.EIP = idleCodeBase
	pcb.Regs.ESP = idleStackTop
	pcb.Regs.EFLAGS = cpu.FlagIF
	pcb.Regs.CS = cpu.SelKernelCode
	pcb.Regs.DS = cpu.SelKernelData
	pcb.Regs.SS = cpu.SelKernelData

	k.Processes[0] = pcb
	k.IdlePID = -1
	return 0, nil
}

// bindIdleToCore makes the swapper own the given core: the PCB is wired up
// with Core/Slot bookkeeping, the CPU regs are loaded from the PCB so the
// next Step decodes the HLT byte, and a paired EventCoreAssign + EventCoreIdle
// is emitted. The PCB stays in StateIdle (no transition to RUN); it is a
// resting state, not a runnable one.
func (k *Kernel) bindIdleToCore(core *Core) []cpu.Event {
	pcb := k.Processes[0]
	if pcb == nil {
		return nil
	}
	pcb.State = StateIdle
	pcb.Core = core.ID
	core.CurrentPID = 0
	core.RunningIdle = true
	k.loadCPUFromPCB(core, pcb)
	return []cpu.Event{
		cpu.EventCoreAssign{CoreID: core.ID, PID: 0},
		cpu.EventCoreIdle{CoreID: core.ID, Tick: core.CPU.Tick},
	}
}

// coreIsRunningIdle disambiguates the overloaded core.CurrentPID == 0
// sentinel. The legacy meaning is "no proc assigned"; with idle enabled, the
// same value can mean "swapper bound to this core". Callers that need the
// distinction route through this helper instead of duplicating the check.
func (k *Kernel) coreIsRunningIdle(core *Core) bool {
	if !k.IdleTaskEnabled {
		return false
	}
	if k.Processes[0] == nil {
		return false
	}
	return core.RunningIdle && core.CurrentPID == 0
}
