package kernel

// AddressSpace is the per-thread-group memory layout. Every PCB carries a
// pointer to one. With fork the child gets a fresh AddressSpace (own physical
// region copied byte-by-byte by sysFork). With vfork the child shares the
// parent's AddressSpace until it exits. clone(CLONE_VM) — added by the
// threads phase — uses the same shareMemSpace path as vfork so threads of
// the same process see the same code and globals.
//
// CodeBase / CodeSize describe the .text region in physical memory. StackTop
// stays on the PCB because each task has its own stack within the shared
// space.
type AddressSpace struct {
	ID       int
	CodeBase uint32
	CodeSize uint32
	RefCount int
}

// FDSpace is the file-descriptor table shared by every PCB that holds the
// same pointer. fork copies it (each task gets independent fds);
// clone(CLONE_FILES) shares it so dup/close in one thread affects siblings.
//
// Slots 0/1/2 are reserved for stdin/stdout/stderr — sys_read/sys_write
// dispatch them by number without consulting the table. Entries >= 3 are
// populated by sys_pipe / sys_open with FD{Used: true, Kind: ..., PipeID}.
type FDSpace struct {
	ID       int
	Table    []FD
	RefCount int
}

// newMemSpace registers a fresh AddressSpace owned by a single task
// (RefCount=1).
func (k *Kernel) newMemSpace(codeBase, codeSize uint32) *AddressSpace {
	k.nextMemSpaceID++
	a := &AddressSpace{
		ID:       k.nextMemSpaceID,
		CodeBase: codeBase,
		CodeSize: codeSize,
		RefCount: 1,
	}
	k.memSpaces[a.ID] = a
	return a
}

// shareMemSpace bumps the refcount and returns the same AddressSpace. Used
// by vfork (and clone(CLONE_VM) once threads land) when the new task must
// see the parent's memory.
func (k *Kernel) shareMemSpace(a *AddressSpace) *AddressSpace {
	a.RefCount++
	return a
}

// newFDSpace registers a fresh FDSpace owned by a single task.
func (k *Kernel) newFDSpace() *FDSpace {
	k.nextFDSpaceID++
	f := &FDSpace{
		ID:       k.nextFDSpaceID,
		RefCount: 1,
	}
	k.fdSpaces[f.ID] = f
	return f
}

// cloneFDSpace allocates an independent FDSpace with a copy of the source
// table. Used by fork — child and parent each get their own table so a
// close in one does not affect the other.
func (k *Kernel) cloneFDSpace(src *FDSpace) *FDSpace {
	k.nextFDSpaceID++
	dup := &FDSpace{
		ID:       k.nextFDSpaceID,
		Table:    append([]FD(nil), src.Table...),
		RefCount: 1,
	}
	k.fdSpaces[dup.ID] = dup
	return dup
}

// releaseMemSpace / releaseFDSpace / shareFDSpace are intentionally not
// defined yet: the kernel still cleans up AddressSpace and FDSpace en
// masse through Reset(), matching the pre-refactor behavior where the
// fields lived on the PCB itself. clone() and the per-exit refcount
// release land together in the threads phase, when proper teardown
// becomes load-bearing for shared spaces.
