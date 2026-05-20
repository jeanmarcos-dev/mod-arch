package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// Syscall numbers, following the Linux x86 32-bit convention.
//
// Argument convention (Linux i386):
//
//	EAX = syscall number
//	EBX = arg1
//	ECX = arg2
//	EDX = arg3
//
// The result is returned in EAX. IRET does not touch EAX, so the handler logic
// (in Go) writes the result there before the IRET runs.
const (
	SysExit  uint32 = 1
	SysRead  uint32 = 3
	SysWrite uint32 = 4
	SysOpen  uint32 = 5
	SysClose uint32 = 6
)

// Standard file descriptors.
const (
	FdStdin  uint32 = 0
	FdStdout uint32 = 1
	FdStderr uint32 = 2
)

// handleSyscall dispatches based on EAX of the calling core. Called from
// runHandlerLogic when an IRQ on vector 0x80 arrives with source SW.
//
// Returns the events to propagate. The core's CPU EAX is left holding the
// result so that when the in-memory handler runs IRET the user sees it (same
// as Linux).
func (k *Kernel) handleSyscall(core *Core) []cpu.Event {
	num := core.CPU.R.EAX
	arg1 := core.CPU.R.EBX
	arg2 := core.CPU.R.ECX
	arg3 := core.CPU.R.EDX

	var out []cpu.Event
	var result uint32
	var name string

	switch num {
	case SysExit:
		name = "exit"
		out = append(out, k.sysExit(core, arg1)...)
		result = 0
	case SysWrite:
		name = "write"
		var subEvs []cpu.Event
		result, subEvs = k.sysWrite(core, arg1, arg2, arg3)
		out = append(out, subEvs...)
	case SysRead:
		name = "read"
		var subEvs []cpu.Event
		result, subEvs = k.sysRead(core, arg1, arg2, arg3)
		out = append(out, subEvs...)
	default:
		// Real Linux returns -ENOSYS (-38) for unknown syscalls.
		name = "unknown"
		result = ^uint32(0)
	}

	core.CPU.R.EAX = result

	// Emit the main syscall event with the full result.
	syscallEv := cpu.EventSyscall{
		PID:    core.CurrentPID,
		Num:    num,
		Name:   name,
		Args:   []uint32{arg1, arg2, arg3},
		Result: result,
	}
	out = append([]cpu.Event{syscallEv}, out...)
	return out
}

// sysWrite implements write(fd, buf, count).
//
//   - fd=1 (stdout) or fd=2 (stderr): bytes flow to the console buffer and an
//     EventConsoleOut is emitted.
//   - Other fds: not supported, returns -1.
//
// The user buffer is read with PL0 permissions (kernel can read any region,
// including user_code where program .data sometimes lives). A real OS would
// validate the user is allowed to read the range to prevent a malicious
// pointer into kernel memory; we skip that check because this demo is a
// pedagogical tool, not a security sandbox.
func (k *Kernel) sysWrite(core *Core, fd, bufAddr, count uint32) (uint32, []cpu.Event) {
	if fd != FdStdout && fd != FdStderr {
		return ^uint32(0), nil
	}
	if count == 0 {
		return 0, nil
	}

	bytes := make([]byte, count)
	for i := range count {
		// Pass core.CPU (not nil) so mem_read events are emitted while the
		// kernel reads the user's buffer. The narrator layer relies on
		// these to draw bytes flying from user memory into the console.
		b, err := k.Mem.Read8(core.CPU, bufAddr+i, cpu.ModeKernel)
		if err != nil {
			// Real Linux returns -EFAULT here.
			return ^uint32(0), nil
		}
		bytes[i] = b
	}

	ev := cpu.EventConsoleOut{
		PID:   core.CurrentPID,
		Bytes: append([]byte(nil), bytes...),
		Text:  string(bytes),
	}
	return count, []cpu.Event{ev}
}

// sysExit stores the exit code in the PCB and delegates to killCurrent for
// the cleanup (zombie + possible context switch or halt) on the calling core.
func (k *Kernel) sysExit(core *Core, code uint32) []cpu.Event {
	if pcb := k.Processes[core.CurrentPID]; pcb != nil {
		pcb.Regs.EAX = code
	}
	return k.killCurrent(core, "exit")
}
