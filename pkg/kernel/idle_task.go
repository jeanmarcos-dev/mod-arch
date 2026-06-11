package kernel

import (
	"fmt"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// idleProgram is the body of each swapper task: a HLT byte followed by a
// short jump that goes back to it. When an IRQ wakes the core, the IRET
// returns to the JMP, which bounces straight back to the HLT byte so the
// CPU parks itself again. Linux's per-CPU idle loop (`cpu_idle_poll`,
// `default_idle_call`) follows the same pattern.
//
//	idleCodeBase + 0: F4         HLT
//	idleCodeBase + 1: EB FD      JMP -3 (back to the HLT byte)
var idleProgram = []byte{0xF4, 0xEB, 0xFD}

// idleCodeBase places the swappers just past init's reserved slice. Each
// core gets its own 0x40-byte slot starting here so per-core PCBs do not
// share an AddressSpace and the HLT loops live at distinct addresses.
const idleCodeBase uint32 = initCodeBase + 0x100

// idleSlotStride is the spacing between successive per-core idle code
// slots. 0x40 is roomy enough for the 3-byte loop plus a bit of margin in
// case we later add a CLI/STI prelude for the privileged idle handler.
const idleSlotStride uint32 = 0x40

// idleCodeBaseForCore returns the address where the swapper of the given
// core lives. Each core's idle program is a private copy so a fault in
// one swapper cannot land another core's IRET on the wrong byte.
func idleCodeBaseForCore(coreID int) uint32 {
	return idleCodeBase + uint32(coreID)*idleSlotStride
}

// SpawnIdle allocates one swapper task per core. Silberschatz §5.5
// (Multi-Processor Scheduling) describes the canonical SMP model: each
// CPU has its own kernel idle thread that loops on HLT until an IRQ
// arrives, and the scheduler never finds a core "empty" — it finds
// either a runnable user task or the per-CPU idle task. Linux implements
// this with PID 0 and one task per CPU (init/0, init/1, …); we mirror
// that with one PCB per core in IdleByCore, each carrying a unique
// negative TID so the snapshot can disambiguate them even though every
// swapper officially shares PID 0. Idempotent across calls.
func (k *Kernel) SpawnIdle() error {
	if len(k.IdleByCore) == len(k.Cores) && k.IdleByCore[0] != nil {
		return nil
	}
	k.IdleByCore = make([]*PCB, len(k.Cores))
	for i := range k.Cores {
		base := idleCodeBaseForCore(i)
		for j := base; j < base+uint32(len(idleProgram)); j++ {
			k.Mem.Bytes[j] = 0
		}
		if err := k.Mem.LoadBytes(base, idleProgram); err != nil {
			return err
		}
		pcb := &PCB{
			PID:       0,
			TID:       -(i + 1), // negative sentinel; never collides with user TIDs (>=1)
			Name:      fmt.Sprintf("swapper/%d", i),
			State:     StateIdle,
			Mode:      cpu.ModeKernel,
			StackTop:  base + idleSlotStride/2,
			Slot:      -1,
			Core:      -1,
			TGID:      0,
			ParentPID: 0,
			memSpace:  k.newMemSpace(base, uint32(len(idleProgram)), base+idleSlotStride/4),
			fdSpace:   k.newFDSpace(),
		}
		pcb.Regs.EIP = base
		pcb.Regs.ESP = pcb.StackTop
		pcb.Regs.EFLAGS = cpu.FlagIF
		pcb.Regs.CS = cpu.SelKernelCode
		pcb.Regs.DS = cpu.SelKernelData
		pcb.Regs.SS = cpu.SelKernelData
		// loadCPUFromPCB rehydrates c.R.ESP/SS from SavedUser* on every bind.
		// The swapper lives entirely in ring 0, so its "saved user" stack is
		// the per-core kernel stack itself; without these two lines, ESP is
		// reset to 0 and the next IRQ push faults at 0xFFFFFFFC.
		pcb.SavedUserESP = pcb.StackTop
		pcb.SavedUserSS = cpu.SelKernelData
		k.IdleByCore[i] = pcb
	}
	k.IdlePID = -1
	return nil
}

// bindIdleToCore binds the swapper belonging to this core onto it: the PCB
// is wired up with Core/Slot bookkeeping, the CPU regs are loaded so the
// next Step decodes the HLT byte, and a paired EventCoreAssign +
// EventCoreIdle is emitted. The PCB stays in StateIdle (never RUN); idle
// is a resting state, not a runnable one.
func (k *Kernel) bindIdleToCore(core *Core) []cpu.Event {
	if core.ID >= len(k.IdleByCore) {
		return nil
	}
	pcb := k.IdleByCore[core.ID]
	if pcb == nil {
		return nil
	}
	pcb.State = StateIdle
	pcb.Core = core.ID
	core.CurrentPID = 0
	core.RunningIdle = true
	k.loadCPUFromPCB(core, pcb)
	// loadCPUFromPCB forces ModeUser for regular processes — undo that for
	// the swapper. PID 0 must run in kernel mode (ring 0) because HLT is a
	// privileged instruction; executing it in PL3 would raise #GP.
	core.CPU.Mode = cpu.ModeKernel
	core.CPU.R.CS = cpu.SelKernelCode
	core.CPU.R.SS = cpu.SelKernelData
	core.CPU.R.DS = cpu.SelKernelData
	return []cpu.Event{
		cpu.EventCoreAssign{CoreID: core.ID, PID: 0},
		cpu.EventCoreIdle{CoreID: core.ID, Tick: k.Clock},
	}
}

// idleSpawned reports whether SpawnIdle has been run for the current core
// layout. Used by HasWork and coreIsRunningIdle as the cheap "does the
// swapper exist yet" check.
func (k *Kernel) idleSpawned() bool {
	return len(k.IdleByCore) == len(k.Cores) && len(k.IdleByCore) > 0 && k.IdleByCore[0] != nil
}

// coreIsRunningIdle disambiguates the overloaded core.CurrentPID == 0
// sentinel. The legacy meaning is "no proc assigned"; with idle enabled the
// same value can mean "swapper bound to this core". Callers route through
// this helper instead of duplicating the check.
func (k *Kernel) coreIsRunningIdle(core *Core) bool {
	if !k.IdleTaskEnabled {
		return false
	}
	if !k.idleSpawned() {
		return false
	}
	return core.RunningIdle && core.CurrentPID == 0
}
