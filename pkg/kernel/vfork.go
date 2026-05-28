package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// BlockedOn reason for the parent of an outstanding vfork.
const blockedOnVfork = "vfork"

// sysVfork implements sys_vfork() as a Clone() with CLONE_VM|CLONE_VFORK:
// the child shares the parent's address space and the parent blocks until
// the child exits.
//
// Pedagogical simplification: real Linux vfork is paired with execve, after
// which the child gets its own address space and the parent unblocks. The
// simulator has no execve, so the child must call sys_exit while still
// sharing memory; that exit wakes the parent (see notifyParentOfExit's
// vfork branch).
//
// Returns (result, events, blocked):
//
//	parent EAX = child PID  (stamped before block so when the parent wakes
//	                         sys_vfork returns the right value)
//	child  EAX = 0
//	blocked = true (parent always parks until the child exits)
func (k *Kernel) sysVfork(core *Core) (uint32, []cpu.Event, bool) {
	parent := k.Processes[core.CurrentPID]
	child, events, err := k.Clone(core, parent, CloneVM|CloneVfork, cloneOptions{})
	if err != nil {
		return ^uint32(0), nil, false
	}

	// Stamp the parent's EAX with the child PID BEFORE blockOnSync runs:
	// blockOnSync calls saveCurrentToPCB which snapshots the live CPU
	// register file. If we wrote EAX after the save, the parent would
	// wake up reading the wrong value.
	core.CPU.R.EAX = uint32(child.PID)

	out := append(events, k.blockOnSync(core, blockedOnVfork)...)
	return uint32(child.PID), out, true
}
