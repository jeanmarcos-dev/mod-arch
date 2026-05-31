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
	SysExit    uint32 = 1
	SysFork    uint32 = 2
	SysRead    uint32 = 3
	SysWrite   uint32 = 4
	SysOpen    uint32 = 5
	SysClose   uint32 = 6
	SysWaitpid uint32 = 7
	SysMknod   uint32 = 14
	SysPipe    uint32 = 42
	SysVfork   uint32 = 190
)

// sysClone implements sys_clone(flags, child_stack, child_eip).
//
// Linux's real ABI exposes more arguments (parent_tid, tls, child_tid); the
// simulator collapses them because none are observable at this layer of the
// stack. arg3 (EDX) carries the child entry-point EIP — glibc fills it with
// the function passed to pthread_create.
//
// Returns the child PID to the parent (Clone() stamps EAX=0 on the child) and
// an error string that handleSyscall stamps onto EventSyscall.Error so the UI
// can render a failed clone explicitly instead of just "-1". Failure returns
// (^uint32(0), nil, err.Error()).
func (k *Kernel) sysClone(core *Core, flags, childStack, childEIP uint32) (uint32, []cpu.Event, string) {
	parent := k.Processes[core.CurrentPID]
	child, events, err := k.Clone(core, parent, CloneFlags(flags), cloneOptions{
		childStackTop: childStack,
		childEIP:      childEIP,
	})
	if err != nil {
		return ^uint32(0), nil, err.Error()
	}
	return uint32(child.PID), events, ""
}

// Standard file descriptors.
const (
	FdStdin  uint32 = 0
	FdStdout uint32 = 1
	FdStderr uint32 = 2
)

// syscallArgs returns the slice of argument registers actually consumed by
// the given syscall, in argv order. The event log shows only the args the
// syscall touches; trailing register values that the caller did not set
// would be noise (e.g. sys_exit(10, 0xa, 0) suggesting three arguments when
// the syscall is unary). Linux i386 calling convention passes args in
// EBX, ECX, EDX (in that order).
func syscallArgs(num uint32, ebx, ecx, edx uint32) []uint32 {
	switch num {
	case SysFork, SysVfork:
		return []uint32{}
	case SysExit, SysPipe, SysMknod:
		return []uint32{ebx}
	case SysOpen, SysWaitpid:
		return []uint32{ebx, ecx}
	default:
		// SysWrite, SysRead, SysClone, and unknown carry three args.
		return []uint32{ebx, ecx, edx}
	}
}

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
			Args:   syscallArgs(num, arg1, arg2, arg3),
			Result: 0,
		}}
		out = append(out, k.sysExit(core, arg1)...)
		return stampKernelContext(out, core.ID, core.CPU.Tick)
	}

	var out []cpu.Event
	var result uint32
	var name string
	var syscallErr string

	switch num {
	case SysFork:
		name = "fork"
		var subEvs []cpu.Event
		result, subEvs = k.sysFork(core)
		out = append(out, subEvs...)
	case SysVfork:
		name = "vfork"
		var subEvs []cpu.Event
		var blocked bool
		result, subEvs, blocked = k.sysVfork(core)
		out = append(out, subEvs...)
		if blocked {
			syscallEv := cpu.EventSyscall{
				PID: callerPID, Num: num, Name: name,
				Args: syscallArgs(num, arg1, arg2, arg3), Result: result,
			}
			return stampKernelContext(append([]cpu.Event{syscallEv}, out...), core.ID, core.CPU.Tick)
		}
	case SysClone:
		name = "clone"
		var subEvs []cpu.Event
		result, subEvs, syscallErr = k.sysClone(core, arg1, arg2, arg3)
		out = append(out, subEvs...)
	case SysPipe:
		name = "pipe"
		var subEvs []cpu.Event
		result, subEvs = k.sysPipe(core, arg1)
		out = append(out, subEvs...)
	case SysMknod:
		name = "mknod"
		var subEvs []cpu.Event
		result, subEvs = k.sysMknod(core, arg1)
		out = append(out, subEvs...)
	case SysOpen:
		name = "open"
		var subEvs []cpu.Event
		result, subEvs = k.sysOpen(core, arg1, arg2)
		out = append(out, subEvs...)
	case SysWaitpid:
		name = "waitpid"
		var subEvs []cpu.Event
		var blocked bool
		result, subEvs, blocked = k.sysWaitpid(core, arg1, arg2)
		out = append(out, subEvs...)
		if blocked {
			// Emit the syscall summary for the WS log even though the
			// caller did not receive a value yet — Result=0 is the
			// pre-stage; the eventual reap will surface the real PID.
			syscallEv := cpu.EventSyscall{
				PID: callerPID, Num: num, Name: name,
				Args: syscallArgs(num, arg1, arg2, arg3), Result: 0,
			}
			return stampKernelContext(append([]cpu.Event{syscallEv}, out...), core.ID, core.CPU.Tick)
		}
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
		Args:   syscallArgs(num, arg1, arg2, arg3),
		Result: result,
		Error:  syscallErr,
	}
	out = append([]cpu.Event{syscallEv}, out...)
	return stampKernelContext(out, core.ID, core.CPU.Tick)
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
//   - fd>=3 resolved through FDTable: pipe-write fds route to pipeWrite.
//   - Other fds: not supported, returns -1.
//
// Buffers that touch kernel memory (or wrap around uint32) are rejected with
// a page fault event and -EFAULT, matching Linux's access_ok / copy_from_user.
func (k *Kernel) sysWrite(core *Core, fd, bufAddr, count uint32) (uint32, []cpu.Event) {
	if fd != FdStdout && fd != FdStderr {
		if pcb := k.Processes[core.CurrentPID]; pcb != nil {
			if pipe, isWrite, ok := k.resolvePipeFD(pcb, fd); ok && isWrite {
				return k.pipeWrite(core, pipe, bufAddr, count)
			}
		}
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
		CoreID: core.CPU.CoreID,
		PID:    core.CurrentPID,
		Kind:   "out",
		Bytes:  append([]byte(nil), bytes...),
		Text:   string(bytes),
	}
	return count, []cpu.Event{ev}
}

// sysExit stores the exit code in the PCB and delegates to killCurrent for
// the cleanup (zombie + possible context switch or halt) on the calling
// core. Once the PCB is parked in ZOMBIE we also check if the parent is
// asleep in sys_waitpid waiting for us — if so, the zombie is reaped right
// away and the parent moves back to READY with the reaped PID in EAX.
func (k *Kernel) sysExit(core *Core, code uint32) []cpu.Event {
	pcb := k.Processes[core.CurrentPID]
	var out []cpu.Event
	if pcb != nil {
		pcb.ExitCode = code
		// A task whose TGID differs from its PID was spawned via
		// clone(CLONE_THREAD): it is a thread of its leader, not a
		// process exit. Emit the dedicated event so the UI can fold
		// the join under the leader's subtree.
		if pcb.TGID != pcb.PID {
			out = append(out, cpu.EventThreadExit{
				TID:    pcb.PID,
				TGID:   pcb.TGID,
				Retval: code,
			})
		}
	}
	out = append(out, k.exitProcess(core, "exit")...)
	return out
}
