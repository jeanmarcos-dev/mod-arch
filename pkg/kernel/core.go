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
	// the kernel idle task (PID 0) is enabled. True means this core's
	// dedicated swapper task is bound; false means CurrentPID==0 has its
	// legacy meaning, "no proc". Each core has its own swapper PCB (see
	// Kernel.IdleByCore), so this flag is per-core state on top of that.
	RunningIdle bool

	// IdleTicks counts how many simulator ticks this core has spent in the
	// swapper's HLT loop. Incremented whenever stepCore is called on a
	// core that is RunningIdle. Pure observability: the frontend renders
	// this as a counter on the idle band and per-core badge so the student
	// sees the loop is alive without the event stream filling up with
	// repeated HLT entries.
	IdleTicks uint64

	// idleTicksSinceFire counts swapper HLT ticks since the last timer fire
	// on this core. Real x86 local-APIC timers keep ticking on idle cores
	// too; modeling that pedagogically requires a quantum counter that
	// advances during HLT, separate from userInstructionsSinceFire (which
	// only counts user-mode instructions and therefore never grows while a
	// core is parked in HLT). shouldFireTimerIdle uses it.
	idleTicksSinceFire uint64
}
