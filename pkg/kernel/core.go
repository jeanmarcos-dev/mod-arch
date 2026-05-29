package kernel

import "github.com/jeanmarcos/modos-cpu/pkg/cpu"

// Core is a single execution context inside the kernel. Each core owns its
// own CPU (registers, kernel stack pointer, pending IRQs) but shares the
// physical Memory and the process table with the rest of the kernel.
//
// In single-core mode there is exactly one Core (ID=0); when N cores are
// configured, the kernel iterates them sequentially in Tick() — true HW
// parallelism is not modeled, but the visible state matches what a real SMP
// system would expose to the OS.
type Core struct {
	ID  int
	CPU *cpu.CPU

	// CurrentPID is the process currently RUN on this core, or 0 if none. A
	// core with CurrentPID == 0 sits idle until the scheduler assigns it work
	// or an IRQ wakes it up.
	CurrentPID int

	// userInstructionsSinceFire counts user-mode instructions that have
	// actually executed (execNext succeeded) since the last timer fire on
	// this core. Kernel-mode handler instructions, IRQ dispatch cycles and
	// halted noops do NOT increment it — only honest user-instruction
	// ticks count toward the quantum. shouldFireTimer fires when this
	// reaches TimerQuantum, then resets it to zero. Decoupling the quantum
	// from raw CPU.Tick eliminates the off-by-handler-cost bug where the
	// first quantum delivered N user instructions while later ones
	// delivered N - handlerTicks.
	userInstructionsSinceFire uint64

	// RunningIdle disambiguates the overloaded CurrentPID==0 sentinel when
	// the kernel idle task (PID 0) is enabled. True means the swapper is
	// bound to this core and the next Step will execute its HLT byte;
	// false means CurrentPID==0 has its legacy meaning, "no proc". Kept
	// here (per-core) rather than on the shared PCB so the same PCB can be
	// simultaneously bound to several cores in multi-core demos.
	RunningIdle bool
}
