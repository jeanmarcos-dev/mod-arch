package kernel

import (
	"fmt"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// SysClone is the syscall number for clone() (Linux i386 ABI).
const SysClone uint32 = 120

// CloneFlags selects what the child task shares with its parent. The values
// match linux/sched.h so a program written with the real ABI in mind keeps
// working against the simulator. Only the bits the simulator actually
// models are listed; anything else is ignored.
type CloneFlags uint32

const (
	// CloneVM means "child sees the same address space as the parent" —
	// shared .text and globals. Used by vfork and pthread_create. Without
	// it, fork copies the parent's address space.
	CloneVM CloneFlags = 0x00000100

	// CloneFiles means "child sees the same FDSpace as the parent" —
	// close/dup in one task ripples to siblings. Without it the FDSpace
	// is duplicated (fork) or replaced with an empty one (legacy vfork).
	CloneFiles CloneFlags = 0x00000400

	// CloneVfork means "parent blocks until child exits". Combined with
	// CloneVM, this is the classic POSIX vfork.
	CloneVfork CloneFlags = 0x00004000

	// CloneThread means "child shares the parent's TGID". Combined with
	// CloneVM and CloneFiles this is how glibc creates a pthread: the
	// new task is a sibling thread, not a child process.
	CloneThread CloneFlags = 0x00010000
)

// cloneOptions carries the non-flag inputs to Clone — passed by sysFork,
// sysVfork and sysClone with different values, but the core machinery does
// not care which syscall is on top.
type cloneOptions struct {
	// childStackTop, when non-zero, overrides the kernel's default stack
	// pick. sys_clone uses it for pthread_create (user-allocated stack).
	// fork / vfork leave it zero so the kernel picks per the flags.
	childStackTop uint32

	// childEIP, when non-zero, sets the child's initial EIP directly —
	// the pthread entry point. Zero means "resume where the parent's
	// int 0x80 returns", which is what fork and vfork need.
	childEIP uint32

	// name is the human-readable label written to PCB.Name. Empty falls
	// back to a derived name based on flags.
	name string
}

// Clone is the unified primitive behind sys_fork, sys_vfork and sys_clone.
// It allocates a new task with the sharing semantics requested by flags
// and returns the new PCB together with the events to broadcast.
//
// Failure (PID exhaustion, no MemSpace, no free stack slot) returns
// nil PCB and an error; the dispatcher converts that into ^uint32(0) in
// EAX. The function does not block the parent — sysVfork wraps this call
// in blockOnSync because blocking depends on the syscall return path
// (saveCurrentToPCB has to happen after EAX is stamped).
func (k *Kernel) Clone(core *Core, parent *PCB, flags CloneFlags, opts cloneOptions) (*PCB, []cpu.Event, error) {
	if parent == nil {
		return nil, nil, fmt.Errorf("clone: nil parent")
	}
	if k.nextPID > MaxProcesses {
		return nil, nil, fmt.Errorf("clone: PID space exhausted")
	}
	parentMem := parent.MemSpace()
	if parentMem == nil {
		return nil, nil, fmt.Errorf("clone: parent has no MemSpace")
	}

	childPID := k.nextPID
	k.nextPID++

	// AddressSpace + stack: share or copy. Threads sub-allocate stack
	// slots inside the shared region; fork carves a fresh dedicated one.
	childMem, childStackTop, err := k.resolveCloneMemory(parent, parentMem, childPID, flags, opts.childStackTop)
	if err != nil {
		return nil, nil, err
	}

	// FDSpace: share, clone or empty depending on flags.
	childFDs := k.resolveCloneFDSpace(parent.FDSpace(), flags)

	// TGID: thread inherits the group; fork/vfork start a new group.
	childTGID := childPID
	if flags&CloneThread != 0 {
		childTGID = parent.TGID
	}

	name := opts.name
	if name == "" {
		switch {
		case flags&CloneThread != 0:
			name = fmt.Sprintf("%s-thread", parent.Name)
		case flags&CloneVfork != 0:
			name = fmt.Sprintf("%s-vfork", parent.Name)
		default:
			name = fmt.Sprintf("%s-child", parent.Name)
		}
	}

	child := &PCB{
		PID:          childPID,
		Name:         name,
		State:        StateReady,
		Mode:         parent.Mode,
		StackTop:     childStackTop,
		Core:         -1,
		TGID:         childTGID,
		ParentPID:    parent.PID,
		Regs:         parent.Regs,
		SavedUserESP: parent.SavedUserESP,
		SavedUserSS:  parent.SavedUserSS,
		memSpace:     childMem,
		fdSpace:      childFDs,
	}
	if flags&CloneVfork != 0 {
		child.VforkParent = parent.PID
	}

	stampCloneEntry(child, core, opts, childStackTop)
	if flags&CloneVM == 0 {
		translateForkStack(child, parent, childStackTop)
		bumpClonedPipeFDs(k.Pipes, childFDs.Table)
	}

	// Child returns 0 from the syscall (Linux convention).
	child.Regs.EAX = 0

	k.Processes[childPID] = child
	parent.Children = append(parent.Children, childPID)
	k.Ready.Push(childPID)

	events := []cpu.Event{
		cpu.EventFork{
			ParentPID: parent.PID,
			ChildPID:  childPID,
			IsVfork:   flags&CloneVfork != 0,
		},
		cpu.EventProcessState{
			PID:    childPID,
			State:  string(StateReady),
			Reason: cloneReason(flags),
		},
	}
	return child, events, nil
}

// resolveCloneMemory decides which AddressSpace and StackTop the child
// gets. Four cases:
//
//   - Caller supplied an explicit child stack (pthread_create with a user-
//     allocated stack): use it as-is and share the parent's MemSpace.
//   - CloneVfork (with CloneVM): classic vfork — share MemSpace and reuse
//     the parent's StackTop. The parent blocks until the child exits, so
//     no other task touches that stack in the meantime.
//   - CloneVM without explicit stack (pthread_create that delegates the
//     stack to the kernel): share MemSpace and carve a slot from the
//     shared stack region.
//   - Neither (fork): fresh MemSpace at the child's own slot, dedicated
//     stack at processStackTop(childPID).
func (k *Kernel) resolveCloneMemory(parent *PCB, parentMem *AddressSpace, childPID int, flags CloneFlags, explicitStack uint32) (*AddressSpace, uint32, error) {
	switch {
	case flags&CloneVM != 0 && explicitStack != 0:
		return k.shareMemSpace(parentMem), explicitStack, nil

	case flags&CloneVfork != 0:
		return k.shareMemSpace(parentMem), parent.StackTop, nil

	case flags&CloneVM != 0:
		mem := k.shareMemSpace(parentMem)
		top, ok := mem.allocStackSlot()
		if !ok {
			return nil, 0, fmt.Errorf("clone: no stack slots free in shared address space")
		}
		return mem, top, nil

	default:
		codeBase := processCodeBase(childPID)
		stackTop := processStackTop(childPID)
		stackBase := stackTop + 0x10 - ProcStackSize
		// Zero the child's new code region and copy the parent's bytes
		// into it. The rest stays zero so stale data from a previous
		// incarnation cannot leak in.
		for i := codeBase; i < codeBase+ProcCodeSize; i++ {
			k.Mem.Bytes[i] = 0
		}
		for i := uint32(0); i < parentMem.CodeSize; i++ {
			k.Mem.Bytes[codeBase+i] = k.Mem.Bytes[parentMem.CodeBase+i]
		}
		// Mirror the parent's stack too — the interrupt frame the kernel
		// just pushed must be present in the child so its IRET resumes
		// at the correct point.
		parentStackBase := processStackTop(parent.PID) + 0x10 - ProcStackSize
		childStackBase := stackTop + 0x10 - ProcStackSize
		for i := uint32(0); i < ProcStackSize; i++ {
			k.Mem.Bytes[childStackBase+i] = k.Mem.Bytes[parentStackBase+i]
		}
		return k.newMemSpace(codeBase, parentMem.CodeSize, stackBase), stackTop, nil
	}
}

// resolveCloneFDSpace returns the FDSpace pointer the child should carry:
// shared (CloneFiles), cloned (fork) or fresh empty (legacy vfork). The
// vfork-empty case preserves the simulator's pre-refactor behavior; real
// POSIX vfork shares the parent's fds. clone(CLONE_VM|CLONE_FILES) gets
// the more accurate "shared" path.
func (k *Kernel) resolveCloneFDSpace(parentFDs *FDSpace, flags CloneFlags) *FDSpace {
	switch {
	case flags&CloneFiles != 0:
		return k.shareFDSpace(parentFDs)
	case flags&CloneVM != 0:
		// vfork legacy: empty table, not shared, not cloned.
		return k.newFDSpace()
	default:
		return k.cloneFDSpace(parentFDs)
	}
}

// stampCloneEntry writes the initial EIP / CS / EFLAGS / EBP / ESP on the
// child PCB. With opts.childEIP non-zero (pthread_create) the child starts
// at the supplied entry point on a fresh user stack. Otherwise the kernel
// reads the int 0x80 frame to resume the parent's syscall return path —
// that is what fork and vfork need.
func stampCloneEntry(child *PCB, core *Core, opts cloneOptions, childStackTop uint32) {
	if opts.childEIP != 0 {
		child.Regs.EIP = opts.childEIP
		child.Regs.CS = cpu.SelUserCode
		child.Regs.EFLAGS = cpu.FlagIF
		child.Regs.EBP = childStackTop
		child.SavedUserESP = childStackTop
		return
	}
	child.Regs.EIP = readKernelDWORD(core.CPU, core.CPU.R.ESP+0)
	child.Regs.CS = uint16(readKernelDWORD(core.CPU, core.CPU.R.ESP+4))
	child.Regs.EFLAGS = readKernelDWORD(core.CPU, core.CPU.R.ESP+8) | cpu.FlagIF
}

// translateForkStack remaps SavedUserESP and EBP from the parent's stack
// region to the child's. Only used on the fork path: vfork children reuse
// the parent's stack, and pthread children received a fresh stack from
// user space so no translation applies.
func translateForkStack(child, parent *PCB, childStackTop uint32) {
	parentStackBase := processStackTop(parent.PID) + 0x10 - ProcStackSize
	childStackBase := childStackTop + 0x10 - ProcStackSize
	if child.SavedUserESP >= parentStackBase && child.SavedUserESP < parentStackBase+ProcStackSize {
		child.SavedUserESP = child.SavedUserESP - parentStackBase + childStackBase
	}
	if child.Regs.EBP >= parentStackBase && child.Regs.EBP < parentStackBase+ProcStackSize {
		child.Regs.EBP = child.Regs.EBP - parentStackBase + childStackBase
	}
}

// bumpClonedPipeFDs increments Readers/Writers on every pipe the duplicated
// FDSpace points at. Without this, fork's "two processes both hold a pipe
// fd" state would only register as one reference inside the Pipe object,
// and the kernel would deliver EOF / EPIPE one close too early.
func bumpClonedPipeFDs(pipes []*Pipe, table []FD) {
	for _, fd := range table {
		if !fd.Used || fd.PipeID < 1 || fd.PipeID > len(pipes) {
			continue
		}
		p := pipes[fd.PipeID-1]
		switch fd.Kind {
		case "pipe-read":
			p.Readers++
		case "pipe-write":
			p.Writers++
		}
	}
}

func cloneReason(flags CloneFlags) string {
	switch {
	case flags&CloneThread != 0:
		return "clone"
	case flags&CloneVfork != 0:
		return "vfork"
	default:
		return "fork"
	}
}
