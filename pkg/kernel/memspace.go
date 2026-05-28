package kernel

// AddressSpace is the per-thread-group memory layout. Every PCB carries a
// pointer to one. With fork the child gets a fresh AddressSpace (own physical
// region copied byte-by-byte by sysFork). With vfork the child shares the
// parent's AddressSpace until it exits. clone(CLONE_VM) uses the same
// shareMemSpace path as vfork so threads of the same process see the same
// code and globals.
//
// CodeBase / CodeSize describe the .text region in physical memory. StackTop
// stays on the PCB because each task has its own stack within the shared
// space. The stackBase / stackSlots fields sub-divide the stack region into
// ThreadStackSlots fixed-size slots so multiple threads can coexist without
// stomping each other's frames.
type AddressSpace struct {
	ID       int
	CodeBase uint32
	CodeSize uint32
	RefCount int

	// stackBase is the bottom of the stack region this address space owns
	// (parent's stack region at AddProcess / fork time). Threads sub-allocate
	// stacks from here through allocStackSlot.
	stackBase uint32

	// stackSlots is a bitmap of which sub-slots of the stack region are in
	// use. Bit i set means slot i is taken. The leader claims slot
	// ThreadStackSlots-1 at AddProcess / fork; threads claim lower slots.
	stackSlots uint8
}

// ThreadStackSlotSize bounds the stack of each thread sharing an address
// space. ProcStackSize / ThreadStackSlotSize gives ThreadStackSlots, the
// maximum number of concurrent tasks per process.
const (
	ThreadStackSlotSize = 0x10000
	ThreadStackSlots    = 4
)

// allocStackSlot reserves the next free stack slot inside this address space
// and returns the initial ESP for it. Slots are taken from high to low so
// the leader (slot ThreadStackSlots-1, claimed at AddProcess time) keeps its
// original StackTop. Returns (0, false) when all slots are in use.
func (a *AddressSpace) allocStackSlot() (uint32, bool) {
	for i := ThreadStackSlots - 1; i >= 0; i-- {
		bit := uint8(1) << uint(i)
		if a.stackSlots&bit != 0 {
			continue
		}
		a.stackSlots |= bit
		return a.stackBase + uint32(i+1)*ThreadStackSlotSize - 0x10, true
	}
	return 0, false
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

// newMemSpace registers a fresh AddressSpace owned by a single task. The
// leader's stack slot (the highest, slot ThreadStackSlots-1) is pre-claimed
// — that slot's top equals processStackTop(pid), the StackTop AddProcess
// stamps on the leader PCB.
func (k *Kernel) newMemSpace(codeBase, codeSize, stackBase uint32) *AddressSpace {
	k.nextMemSpaceID++
	a := &AddressSpace{
		ID:         k.nextMemSpaceID,
		CodeBase:   codeBase,
		CodeSize:   codeSize,
		RefCount:   1,
		stackBase:  stackBase,
		stackSlots: 1 << uint(ThreadStackSlots-1),
	}
	k.memSpaces[a.ID] = a
	return a
}

// shareMemSpace bumps the refcount and returns the same AddressSpace. Used
// by vfork and clone(CLONE_VM) when the new task must see the parent's
// memory.
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

// shareFDSpace bumps the refcount and returns the same FDSpace. Used by
// clone(CLONE_FILES) so threads see each other's open fds.
func (k *Kernel) shareFDSpace(f *FDSpace) *FDSpace {
	f.RefCount++
	return f
}

// releaseMemSpace and releaseFDSpace are deferred to the next phase: the
// kernel still tears down maps en masse through Reset(), matching the
// pre-refactor behavior where the fields lived on the PCB itself. Per-exit
// refcount release lands together with sys_thread_exit, when freeing a
// shared space prematurely would actually corrupt sibling threads.
