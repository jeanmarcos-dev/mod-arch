package programs

import "github.com/jeanmarcos-dev/mod-arch/pkg/cpu"

// procStackSize mirrors kernel.ProcStackSize. It is duplicated here (instead of
// imported) to keep `programs` free of a `pkg/kernel` dependency that would
// otherwise cycle through `internal/sim`.
const procStackSize uint32 = 0x40000

// ProcessStackTop returns the initial ESP that the kernel hands to a user
// process loaded into the given user-code slot. Slot 0 → 0x33FFF0, slot 1 →
// 0x37FFF0, etc.
//
// Keep in sync with kernel.processStackTop. The duplication is intentional —
// `programs` cannot import `pkg/kernel` (it would create an import cycle via
// `internal/sim`), and demos only need the address to embed it as an
// immediate, never to call the kernel function at runtime.
func ProcessStackTop(slot int) uint32 {
	return cpu.AddrUserStack + uint32(slot+1)*procStackSize - 0x10
}

// SharedCounterAddr is the address sync demos use for a counter visible to
// every process. It sits just below the very top of physical memory, above
// the last user-stack slot, so no process's stack can clobber it. Used by
// race_counter, safe_counter, threads_race, threads_safe and showcase.
func SharedCounterAddr() uint32 { return cpu.AddrUserStack + 0xFFFE0 } // 0x3FFFE0

// ThreadCounterAddr is the address the in-process thread demos (threads_race,
// threads_safe) use for the shared counter visible to every CLONE_VM thread.
// It deliberately differs from SharedCounterAddr so the two scenarios can
// coexist in the same memory layout when chained.
func ThreadCounterAddr() uint32 { return cpu.AddrUserStack + 0xF0000 } // 0x3F0000
