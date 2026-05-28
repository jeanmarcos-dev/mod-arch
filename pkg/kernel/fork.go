package kernel

import (
	"fmt"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// sysFork implements sys_fork(): allocate a fresh PID, copy the calling
// process's PCB and address space into a new slot, and leave the child READY.
//
// Return-value convention (matches Linux):
//
//	parent EAX = child PID
//	child  EAX = 0
//
// The parent's EAX lives in the live core register file (it is RUN); the
// child's lives in its saved PCB.Regs because no core owns it yet.
//
// Failure returns ^uint32(0) (-1 in two's complement) and emits no
// EventFork — fork ran out of PID slots.
func (k *Kernel) sysFork(core *Core) (uint32, []cpu.Event) {
	parentPID := core.CurrentPID
	parent := k.Processes[parentPID]
	if parent == nil {
		return ^uint32(0), nil
	}

	if k.nextPID > MaxProcesses {
		return ^uint32(0), nil
	}

	childPID := k.nextPID
	k.nextPID++

	codeBase := processCodeBase(childPID)
	stackTop := processStackTop(childPID)

	// Zero the child's code region before copying so stale bytes from a
	// previous incarnation cannot leak in.
	for i := codeBase; i < codeBase+ProcCodeSize; i++ {
		k.Mem.Bytes[i] = 0
	}
	// Copy the parent's code into the child's slot. We copy exactly
	// parent.CodeSize bytes — the rest stays zero.
	for i := uint32(0); i < parent.CodeSize; i++ {
		k.Mem.Bytes[codeBase+i] = k.Mem.Bytes[parent.CodeBase+i]
	}

	// Copy the parent's stack region too. Even though the parent has not
	// executed any push since `int 0x80`, the kernel may already have
	// pushed an interrupt frame; mirroring the bytes keeps the child's
	// resume point identical to the parent's.
	parentStackBase := processStackTop(parentPID) + 0x10 - ProcStackSize
	childStackBase := stackTop + 0x10 - ProcStackSize
	for i := uint32(0); i < ProcStackSize; i++ {
		k.Mem.Bytes[childStackBase+i] = k.Mem.Bytes[parentStackBase+i]
	}

	child := &PCB{
		PID:          childPID,
		Name:         fmt.Sprintf("%s-child", parent.Name),
		State:        StateReady,
		Mode:         parent.Mode,
		CodeBase:     codeBase,
		CodeSize:     parent.CodeSize,
		StackTop:     stackTop,
		Core:         -1,
		ParentPID:    parentPID,
		Regs:         parent.Regs,
		SavedUserESP: parent.SavedUserESP,
		SavedUserSS:  parent.SavedUserSS,
		// Inherit a copy of the parent's open fds. Pipe entries refer
		// to shared kernel Pipe objects so both processes can now
		// reach the same channel — that is how parent/child pipelines
		// work in Linux.
		FDTable: append([]FD(nil), parent.FDTable...),
	}
	for _, fd := range child.FDTable {
		if !fd.Used || fd.PipeID < 1 || fd.PipeID > len(k.Pipes) {
			continue
		}
		p := k.Pipes[fd.PipeID-1]
		switch fd.Kind {
		case "pipe-read":
			p.Readers++
		case "pipe-write":
			p.Writers++
		}
	}

	// Translate the parent's stack pointer to the child's stack slot so
	// the child resumes on its own stack rather than re-entering the
	// parent's frame.
	if child.SavedUserESP >= parentStackBase && child.SavedUserESP < parentStackBase+ProcStackSize {
		child.SavedUserESP = child.SavedUserESP - parentStackBase + childStackBase
	}
	// Same for EBP — it points into the parent's stack frame and must be
	// translated to the equivalent address inside the child's stack.
	if child.Regs.EBP >= parentStackBase && child.Regs.EBP < parentStackBase+ProcStackSize {
		child.Regs.EBP = child.Regs.EBP - parentStackBase + childStackBase
	}

	// EIP and CS for the child mirror the parent's resume point: when the
	// scheduler eventually loads this PCB, IRET will land at the byte
	// after `int 0x80` exactly like the parent. The HW pushed the frame
	// at [ESP, ESP+4, ESP+8] when int 0x80 fired.
	child.Regs.EIP = readKernelDWORD(core.CPU, core.CPU.R.ESP+0)
	child.Regs.CS = uint16(readKernelDWORD(core.CPU, core.CPU.R.ESP+4))
	child.Regs.EFLAGS = readKernelDWORD(core.CPU, core.CPU.R.ESP+8) | cpu.FlagIF

	// fork returns 0 in the child.
	child.Regs.EAX = 0

	k.Processes[childPID] = child
	parent.Children = append(parent.Children, childPID)

	k.Ready.Push(childPID)

	return uint32(childPID), []cpu.Event{
		cpu.EventFork{ParentPID: parentPID, ChildPID: childPID, IsVfork: false},
		cpu.EventProcessState{PID: childPID, State: string(StateReady), Reason: "fork"},
	}
}
