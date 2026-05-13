// Package cpu implements a deliberately small subset of the x86 32-bit ISA
// suitable for teaching the hardware/software boundary of an operating system.
//
// # Scope
//
// The CPU emulates ~25 instructions, 13 general-purpose and segment registers,
// two privilege levels (PL0 kernel and PL3 user), a flat 64MB linear memory
// with permission-checked regions, and an IDT with the vectors used by the
// teaching scenarios (0x00, 0x0E, 0x20, 0x21, 0x80).
//
// # Determinism
//
// Step is purely sequential: one fetch-decode-execute per call, plus
// at-most-one interrupt dispatch when IF=1 and an IRQ is pending. Given the
// same initial state, the sequence of events is reproducible — a property
// the higher layers rely on for didactic replay.
//
// # Events
//
// The CPU does not log. It emits typed Event values describing what just
// happened (instruction, register write, memory access, IRQ fire, mode
// switch, page fault, halt). Consumers reconstruct UI state from the event
// stream.
//
// # What lives elsewhere
//
// The scheduler, syscalls, PCBs and interrupt handlers are software policy
// and live in pkg/kernel. The CPU only enforces the hardware contract:
// permission checks, mode transitions, IDT dispatch, IRET semantics.
package cpu
