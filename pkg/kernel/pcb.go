package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// State is the current state of a process in the operating system.
//
// Transition diagram:
//
//	NEW ──admit──▶ READY ──schedule──▶ RUN ──preempt(timer)──▶ READY
//	                  ▲                  │
//	                  └────unblock───── BLOCKED ◀── sys_read with no data, etc.
//	                                    │
//	                                    └── exit/kill ──▶ ZOMBIE
type State string

// Process lifecycle states.
const (
	StateNew     State = "NEW"
	StateReady   State = "READY"
	StateRun     State = "RUN"
	StateBlocked State = "BLOCKED"
	StateZombie  State = "ZOMBIE"
)

// PCB (Process Control Block) is the bookkeeping the kernel maintains per
// process. While the process is running, its registers live in the actual
// CPU; when another process takes over, the values are preserved here and
// the new one's are loaded into the CPU.
//
// In real x86 this role is played by `task_struct` (Linux) or `_EPROCESS`
// (Windows). The structure here is a heavily simplified model.
type PCB struct {
	PID       int
	Name      string
	State     State
	BlockedOn string // description while State == BLOCKED ("keyboard", etc.)

	// Regs is a snapshot of the user-visible register file.
	// While the process is RUN these values are stale: the live regs are in
	// cpu.CPU.R.
	Regs cpu.Registers

	// SavedUserESP/SS mirror the same-named CPU fields. While the process is
	// RUN and has entered kernel, this holds the user ESP at entry. While the
	// process is READY/BLOCKED, it holds the stack position it left behind.
	SavedUserESP uint32
	SavedUserSS  uint16

	// Mode is the privilege level of the process. User processes are always
	// PL3; stored for completeness (e.g. a process could become ZOMBIE while
	// in kernel mode).
	Mode uint8

	// CodeBase, CodeSize and StackTop describe the in-memory layout: where
	// .text was loaded and the initial ESP (stack grows downward from
	// StackTop).
	CodeBase uint32
	CodeSize uint32
	StackTop uint32
}

// Snapshot returns a JSON-friendly representation of the PCB for the frontend.
func (p *PCB) Snapshot() map[string]any {
	return map[string]any{
		"pid":        p.PID,
		"name":       p.Name,
		"state":      string(p.State),
		"blocked_on": p.BlockedOn,
		"regs": map[string]uint32{
			"EAX": p.Regs.EAX, "EBX": p.Regs.EBX, "ECX": p.Regs.ECX, "EDX": p.Regs.EDX,
			"ESI": p.Regs.ESI, "EDI": p.Regs.EDI, "EBP": p.Regs.EBP,
			"EIP": p.Regs.EIP, "EFLAGS": p.Regs.EFLAGS,
		},
		"saved_user_esp": p.SavedUserESP,
		"code_base":      p.CodeBase,
		"code_size":      p.CodeSize,
		"stack_top":      p.StackTop,
	}
}
