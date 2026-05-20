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

	// lastTimerFiredAt remembers the CPU.Tick at which the PIT last fired on
	// this core, so shouldFireTimer can quantize per-core preemption.
	lastTimerFiredAt uint64
}
