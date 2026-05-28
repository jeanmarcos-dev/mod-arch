package kernel

import (
	"fmt"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// BlockedOn reason for the parent of an outstanding vfork.
const blockedOnVfork = "vfork"

// sysVfork implements sys_vfork(): create a child that shares the parent's
// address space and park the parent in BLOCKED until the child exits.
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
	parentPID := core.CurrentPID
	parent := k.Processes[parentPID]
	if parent == nil {
		return ^uint32(0), nil, false
	}
	if k.nextPID > MaxProcesses {
		return ^uint32(0), nil, false
	}

	childPID := k.nextPID
	k.nextPID++

	child := &PCB{
		PID:          childPID,
		Name:         fmt.Sprintf("%s-vfork", parent.Name),
		State:        StateReady,
		Mode:         parent.Mode,
		CodeBase:     parent.CodeBase, // SAME slot — memory is shared
		CodeSize:     parent.CodeSize,
		StackTop:     parent.StackTop,
		Core:         -1,
		ParentPID:    parentPID,
		VforkParent:  parentPID,
		Regs:         parent.Regs,
		SavedUserESP: parent.SavedUserESP,
		SavedUserSS:  parent.SavedUserSS,
	}

	// Child resumes at the byte after `int 0x80` exactly where the parent
	// is about to return.
	child.Regs.EIP = readKernelDWORD(core.CPU, core.CPU.R.ESP+0)
	child.Regs.CS = uint16(readKernelDWORD(core.CPU, core.CPU.R.ESP+4))
	child.Regs.EFLAGS = readKernelDWORD(core.CPU, core.CPU.R.ESP+8) | cpu.FlagIF
	child.Regs.EAX = 0

	k.Processes[childPID] = child
	parent.Children = append(parent.Children, childPID)
	k.Ready.Push(childPID)

	// Stamp the parent's EAX with the child PID BEFORE blockOnSync runs:
	// blockOnSync calls saveCurrentToPCB which snapshots the live CPU
	// register file. If we wrote EAX after the save, the parent would
	// wake up reading the wrong value.
	core.CPU.R.EAX = uint32(childPID)

	out := []cpu.Event{
		cpu.EventFork{ParentPID: parentPID, ChildPID: childPID, IsVfork: true},
	}
	out = append(out, k.blockOnSync(core, blockedOnVfork)...)
	return uint32(childPID), out, true
}
