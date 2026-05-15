// Package kernel models the operating system software layer that runs on
// top of the simulated CPU.
//
// # Responsibilities
//
// The kernel installs the IDT, owns the handlers that run in PL0, keeps the
// process table (PCBs) and ready queue, decides context switches in response
// to the timer, and dispatches syscalls and page faults.
//
// # HW/SW boundary
//
// The kernel does NOT perform mode switches itself: that is the CPU's job.
// The kernel only configures the tables the HW consults and writes the
// handlers the HW invokes. The HW/SW boundary lives in pkg/cpu/interrupt.go.
//
// This split mirrors a real system: hardware enforces privilege transitions
// on interrupts; software defines what those transitions accomplish.
//
// # Scheduling
//
// The default scheduler is a round-robin between ready PCBs, driven by IRQ
// 0x20 (timer). The TimerQuantum field controls the number of CPU ticks
// between fires; setting it to zero disables the timer.
package kernel
