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
	callerPID := core.CurrentPID

	// sys_exit is special: it may switch the core to a different process via
	// killCurrent, so core.CPU.R afterwards belongs to that new occupant.
	// Touching EAX or emitting events keyed on core.CurrentPID below would
	// corrupt the next process's register file (same shape as the
	// SyncMutexLock/SemWait pre-stage trick). Handle the exit up front and
	// return.
	if num == SysExit {
		out := []cpu.Event{cpu.EventSyscall{
			PID:    callerPID,
			Num:    num,
			Name:   "exit",
			Args:   []uint32{arg1, arg2, arg3},
			Result: 0,
		}}
		return append(out, k.sysExit(core, arg1)...)
	}

	var out []cpu.Event
	var result uint32
	var name string

	switch num {
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

// userBufferFault returns the first offending byte and false if the range
// [addr, addr+count) falls outside the user-accessible window (user_code,
// user_stack). Returns 0, true when the range is fully inside it.
//
// Real kernels do something equivalent (access_ok / copy_from_user) to stop
// a process from passing a pointer into kernel memory: without this check
// sys_write with bufAddr in the kernel range would happily exfiltrate kernel
// bytes, and sys_read would let user clobber the IDT, kernel handlers, or a
// neighbouring PCB.
func userBufferFault(addr, count uint32) (uint32, bool) {
	if count == 0 {
		return 0, true
	}
	end := addr + count - 1
	if end < addr {
		// uint32 wraparound: the buffer "wraps" past the top of memory.
		return addr, false
	}
	if addr < cpu.AddrUserBase {
		return addr, false
	}
	if end > cpu.AddrUserTop {
		// The buffer starts in user space but spills past its top.
		return cpu.AddrUserTop + 1, false
	}
	return 0, true
}

// sysWrite implements write(fd, buf, count).
//
//   - fd=1 (stdout) or fd=2 (stderr): bytes flow to the console buffer and an
//     EventConsoleOut is emitted.
//   - Other fds: not supported, returns -1.
//
// Buffers that touch kernel memory (or wrap around uint32) are rejected with
// a page fault event and -EFAULT, matching Linux's access_ok / copy_from_user.
func (k *Kernel) sysWrite(core *Core, fd, bufAddr, count uint32) (uint32, []cpu.Event) {
	if fd != FdStdout && fd != FdStderr {
		return ^uint32(0), nil
	}
	if count == 0 {
		return 0, nil
	}
	if bad, ok := userBufferFault(bufAddr, count); !ok {
		return ^uint32(0), []cpu.Event{cpu.EventPageFault{
			CoreID:     core.CPU.CoreID,
			Addr:       bad,
			Mode:       cpu.ModeUser,
			PermNeeded: "R",
		}}
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
