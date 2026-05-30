package cpu

import (
	"errors"
	"fmt"
)

// Registers holds the program-visible register file plus control state.
// x86 names are preserved. ESI/EDI are kept even though the initial subset
// does not use them, so the register table is complete.
type Registers struct {
	EAX, EBX, ECX, EDX uint32
	ESI, EDI           uint32
	ESP, EBP           uint32
	EIP                uint32
	EFLAGS             uint32

	// CS, DS, SS are 16-bit selectors. In real x86 they index the GDT;
	// here they are kept as visual hints of which segment matches which mode.
	CS, DS, SS uint16
}

// EFLAGS bits used by the subset.
//
// AF (bit 4), DF (bit 10) and TF (bit 8) are intentionally omitted: the demo
// has no BCD adjustments (DAA/DAS need AF), no string ops (MOVSB/LODSB/STOSB
// need DF), and no single-step debugging (TF). PF is modeled because the Jcc
// pair JP/JNP reads it.
const (
	FlagCF uint32 = 1 << 0  // Carry
	FlagPF uint32 = 1 << 2  // Parity (even number of 1 bits in low byte)
	FlagZF uint32 = 1 << 6  // Zero
	FlagSF uint32 = 1 << 7  // Sign
	FlagIF uint32 = 1 << 9  // Interrupt enable
	FlagOF uint32 = 1 << 11 // Overflow
)

// Privilege levels. x86 has 4 (0-3); only the extremes are used.
const (
	ModeKernel uint8 = 0 // PL0
	ModeUser   uint8 = 3 // PL3
)

// GDT selectors (fixed in the simulator; in real x86 they would index real entries).
//
// Bits 0-1 of an x86 selector encode the RPL (Requested Privilege Level). User
// selectors carry RPL=3 (0b11) hardcoded in the low bits, kernel selectors carry
// RPL=0. That convention is what lets the CPU tell, when popping a CS from the
// kernel stack, whether it is returning to user or not.
//
// There is no actual GDT in memory. Real x86 looks up each loaded selector in
// a descriptor table to read its base (32 bits), limit (20 bits), DPL (2 bits)
// and type flags. Here selectors are constants only — the effective base is 0
// (flat model), the limit is the full address space, and DPL checks collapse
// to comparing CPL against the Mode field. See doc.go ("NOT implemented") for
// the rationale.
const (
	SelKernelCode uint16 = 0x08 // GDT idx 1, RPL 0
	SelKernelData uint16 = 0x10 // GDT idx 2, RPL 0
	SelUserCode   uint16 = 0x1B // GDT idx 3, RPL 3 (0x18 | 3)
	SelUserData   uint16 = 0x23 // GDT idx 4, RPL 3 (0x20 | 3)
)

// CPU is the simulated central unit.
//
// Mode mirrors what a real x86 stores in the CPL field (bits 0-1 of CS); it is
// modeled as an explicit field for clarity. Mode changes ONLY from inside the
// CPU: on INT/IRET, on faults, or when an external IRQ arrives. Software cannot
// change the mode directly — that would be an obvious security hole.
//
// The simulator is strictly synchronous: one Step executes exactly one
// instruction (or dispatches one pending IRQ). There is no internal concurrency.
type CPU struct {
	R    Registers
	Mode uint8 // 0=kernel, 3=user
	Mem  *Memory

	// CoreID identifies which logical CPU emitted each event. Default 0
	// matches the boot/single-core case; kernel.NewMulti sets it per core
	// so the frontend can route reg/mem/instruction events to the right
	// pane without mixing register files between cores.
	CoreID int

	// Halted is true after executing HLT. Subsequent steps do nothing until
	// an IRQ wakes the CPU.
	Halted bool

	// Tick is incremented on every Step. The kernel uses it to decide when
	// to fire the timer (every N ticks).
	Tick uint64

	// PendingIRQs is the queue of external IRQs waiting for dispatch. They
	// are serviced before the next fetch-decode-execute cycle, if IF=1.
	PendingIRQs []uint8

	// SavedUserESP/SS hold the user stack when entering kernel from user, so
	// IRET can restore it. In real x86 this lives in the TSS; here it is
	// simplified to plain CPU fields.
	SavedUserESP uint32
	SavedUserSS  uint16

	// KernelStackTop is the initial ESP loaded on dispatchIRQ from user mode.
	// In real x86 the value comes from the TSS, which is per-CPU; the kernel
	// gives each core its own region so concurrent traps do not stomp each
	// other's interrupt frames.
	KernelStackTop uint32

	// lastInstrStart records the EIP at which the most recent execInstruction
	// began (set before any fetch). Faults that occur mid-instruction restore
	// EIP to this value before dispatching, so the kernel-stack frame points
	// at the offending instruction — matching the x86 rule that faults save
	// the offending EIP while traps save the next one.
	lastInstrStart uint32

	// events buffers events emitted in the current step. Cleared at the start
	// of each Step and consumed by the caller at the end.
	events []Event
}

// New builds a CPU with initialized memory and zeroed registers, ready for the
// caller to configure (CS/DS/SS, EIP, ESP, initial mode).
func New(memSize int) *CPU {
	return &CPU{
		Mem:            NewMemory(memSize),
		KernelStackTop: KernelStackTop,
	}
}

// NewWithMemory builds a CPU that shares a Memory instance and uses a
// caller-supplied kernel stack region. Used by the kernel to spin up
// additional cores after the boot CPU: every core sees the same physical RAM
// but lands on its own kernel stack on trap entry.
func NewWithMemory(mem *Memory, kernelStackTop uint32) *CPU {
	return &CPU{
		Mem:            mem,
		KernelStackTop: kernelStackTop,
	}
}

// Reset clears the CPU to its initial state while preserving memory.
func (c *CPU) Reset() {
	c.R = Registers{}
	c.Mode = ModeUser
	c.Halted = false
	c.Tick = 0
	c.PendingIRQs = nil
	c.events = nil
}

// Emit appends an event to the current step's stream.
func (c *CPU) Emit(e Event) {
	c.events = append(c.events, e)
}

// DrainEvents returns and clears the pending event slice. Used by the kernel
// after it runs handler logic (sys_write, sys_read, scheduler) so the events
// the handler emitted via Emit make it out of the step boundary — Step has
// already returned its own slice by the time the handler runs.
func (c *CPU) DrainEvents() []Event {
	out := c.events
	c.events = nil
	return out
}

// SetReg32 writes a 32-bit register by name and emits the matching event.
func (c *CPU) SetReg32(name string, val uint32) error {
	old, p, err := c.regPtr(name)
	if err != nil {
		return err
	}
	if *p == val {
		return nil
	}
	*p = val
	c.Emit(EventRegWrite{CoreID: c.CoreID, Tick: c.Tick, Reg: name, Old: old, New: val})
	return nil
}

// GetReg32 returns the value of a register by name.
func (c *CPU) GetReg32(name string) (uint32, error) {
	_, p, err := c.regPtr(name)
	if err != nil {
		return 0, err
	}
	return *p, nil
}

func (c *CPU) regPtr(name string) (uint32, *uint32, error) {
	var p *uint32
	switch name {
	case "EAX":
		p = &c.R.EAX
	case "EBX":
		p = &c.R.EBX
	case "ECX":
		p = &c.R.ECX
	case "EDX":
		p = &c.R.EDX
	case "ESI":
		p = &c.R.ESI
	case "EDI":
		p = &c.R.EDI
	case "ESP":
		p = &c.R.ESP
	case "EBP":
		p = &c.R.EBP
	case "EIP":
		p = &c.R.EIP
	case "EFLAGS":
		p = &c.R.EFLAGS
	default:
		return 0, nil, fmt.Errorf("unknown register: %s", name)
	}
	return *p, p, nil
}

// FlagSet sets or clears a bit of EFLAGS and emits reg_write if it changed.
func (c *CPU) FlagSet(flag uint32, on bool) {
	old := c.R.EFLAGS
	if on {
		c.R.EFLAGS |= flag
	} else {
		c.R.EFLAGS &^= flag
	}
	if old != c.R.EFLAGS {
		c.Emit(EventRegWrite{CoreID: c.CoreID, Tick: c.Tick, Reg: "EFLAGS", Old: old, New: c.R.EFLAGS})
	}
}

// FlagOn reports whether the given flag bit is set.
func (c *CPU) FlagOn(flag uint32) bool {
	return c.R.EFLAGS&flag != 0
}

// Step executes exactly one instruction (or dispatches one pending IRQ) and
// returns the events emitted.
//
// Priority order (matches real x86):
//  1. Pending IRQs are dispatched if IF=1 (CPU enters kernel).
//  2. If the CPU is halted, nothing runs (waiting for an IRQ to wake it).
//  3. Otherwise, fetch-decode-execute the instruction at EIP.
//
// Tick is incremented in every case.
//
// The returned slice is detached from c.events so callers can safely run more
// code that emits additional events (e.g. the kernel running a syscall handler
// that calls Mem.Read8) without the new emissions appearing twice — once in
// the returned slice and once in a later DrainEvents call.
func (c *CPU) Step() ([]Event, error) {
	c.events = nil

	if len(c.PendingIRQs) > 0 && c.FlagOn(FlagIF) {
		vec := c.PendingIRQs[0]
		c.PendingIRQs = c.PendingIRQs[1:]
		if err := c.dispatchIRQ(vec, "HW"); err != nil {
			return c.DrainEvents(), err
		}
		c.Tick++
		return c.DrainEvents(), nil
	}

	if c.Halted {
		c.Tick++
		return c.DrainEvents(), nil
	}

	if err := c.execNext(); err != nil {
		// A memory violation MUST be converted by the HW (MMU) into a page
		// fault: synthesize the x86 #PF error code, expose it (and CR2)
		// through the mirror addresses, emit the event, dispatch IRQ 0x0E
		// with source "fault", and return without error (the kernel handles
		// cleanup).
		if mf, ok := errors.AsType[*MemFault](err); ok {
			errorCode := faultErrorCode(mf)
			writeFaultMirror(c.Mem, mf.Addr, errorCode)
			c.Emit(EventPageFault{
				CoreID:     c.CoreID,
				Tick:       c.Tick,
				Addr:       mf.Addr,
				Mode:       mf.Mode,
				PermNeeded: mf.PermNeeded,
				ErrorCode:  errorCode,
			})
			if disp := c.dispatchFault(0x0E); disp != nil {
				return c.DrainEvents(), disp
			}
			c.Tick++
			return c.DrainEvents(), nil
		}
		return c.DrainEvents(), err
	}

	c.Tick++
	return c.DrainEvents(), nil
}

// execNext is the CPU inner loop: fetch the opcode at EIP, decode, execute.
// The implementation lives in isa.go.
func (c *CPU) execNext() error {
	return execNextImpl(c)
}

// execNextImpl is a variable so isa.go can inject the real implementation.
// This lets cpu.go be tested in isolation without pulling in the full decoder.
var execNextImpl = func(c *CPU) error {
	return errors.New("ISA not implemented")
}
