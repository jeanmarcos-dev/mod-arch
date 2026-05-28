package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// sysFork implements sys_fork() as a thin wrapper around Clone() with no
// sharing flags: the child gets its own address space, an independent
// FDSpace (with the parent's open fds duplicated) and a fresh TGID.
//
// Return-value convention (matches Linux):
//
//	parent EAX = child PID
//	child  EAX = 0
//
// The parent's EAX lives in the live core register file; the child's is
// stamped on its PCB.Regs by Clone before the new PCB is queued. Failure
// returns ^uint32(0) and emits no EventFork.
func (k *Kernel) sysFork(core *Core) (uint32, []cpu.Event) {
	parent := k.Processes[core.CurrentPID]
	child, events, err := k.Clone(core, parent, 0, cloneOptions{})
	if err != nil {
		return ^uint32(0), nil
	}
	return uint32(child.PID), events
}
