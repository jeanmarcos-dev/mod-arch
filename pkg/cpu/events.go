package cpu

// Event is the simulator's sole output to the rest of the system (server, tests).
// The frontend reconstructs ALL state from the event stream, which means any
// observer can pause, rewind, or serialize state simply by storing the sequence
// of events.
type Event interface {
	Tag() string
}

// EventInstruction is emitted at the start of each step, before any mutation.
type EventInstruction struct {
	CoreID   int    `json:"core_id"`
	PID      int    `json:"pid"`
	EIP      uint32 `json:"eip"`
	Mnemonic string `json:"mnemonic"`
	Bytes    []byte `json:"bytes"`
}

// Tag returns the event tag.
func (EventInstruction) Tag() string { return "instruction" }

// EventRegWrite records a register change, so the frontend can flash the
// register that changed. CoreID identifies which CPU core produced the
// write; multicore demos rely on it to keep per-core state separated.
type EventRegWrite struct {
	CoreID int    `json:"core_id"`
	Reg    string `json:"reg"`
	Old    uint32 `json:"old"`
	New    uint32 `json:"new"`
}

// Tag returns the event tag.
func (EventRegWrite) Tag() string { return "reg_write" }

// EventMemRead is emitted when the CPU reads memory; the frontend highlights
// the address.
type EventMemRead struct {
	CoreID int    `json:"core_id"`
	Addr   uint32 `json:"addr"`
	Value  uint32 `json:"value"`
	Size   int    `json:"size"` // 1, 2 or 4 bytes
}

// Tag returns the event tag.
func (EventMemRead) Tag() string { return "mem_read" }

// EventMemWrite is emitted when the CPU writes memory.
type EventMemWrite struct {
	CoreID int    `json:"core_id"`
	Addr   uint32 `json:"addr"`
	Old    uint32 `json:"old"`
	New    uint32 `json:"new"`
	Size   int    `json:"size"`
}

// Tag returns the event tag.
func (EventMemWrite) Tag() string { return "mem_write" }

// EventModeSwitch records a PL3 <-> PL0 transition. The Cause field identifies
// who triggered the change:
//
//   - "INT"   software (int N instruction) or hardware (IRQ); CPU does it automatically
//   - "IRET"  software (iret instruction); CPU does it automatically on execution
//   - "fault" CPU detected a violation (page fault, divide error, etc.) — HW
//
// In all three cases the mode bit transition is performed by the CPU HARDWARE.
// Software only decides what to do AFTER the change; it never flips the bit.
type EventModeSwitch struct {
	CoreID int    `json:"core_id"`
	From   uint8  `json:"from"`  // 3 or 0
	To     uint8  `json:"to"`    // 3 or 0
	Cause  string `json:"cause"` // "INT" | "IRET" | "fault"
}

// Tag returns the event tag.
func (EventModeSwitch) Tag() string { return "mode_switch" }

// EventIRQFire is emitted when an interrupt/exception fires.
//
//   - Source "HW"  external (timer, keyboard) or CPU exception (page fault)
//   - Source "SW"  triggered by an `int N` instruction
//
// It is the companion to EventModeSwitch: IRQ_FIRE explains WHY the kernel is
// being entered, while ModeSwitch documents WHEN the HW flips the bit.
//
// Handler is the address read from IDT[vector] that the CPU is about to jump
// to. FromMode is the privilege level the CPU was running at right before the
// trap (3 = user, 0 = kernel). When FromMode == ModeUser the HW pushes a
// three-word interrupt frame to the kernel stack; SavedEIP/SavedCS/SavedEFLAGS
// carry that frame so the UI can render it without having to read kernel
// memory. For kernel-mode interrupts those fields are omitted (the HW does not
// push a frame when there is no privilege transition).
type EventIRQFire struct {
	CoreID      int    `json:"core_id"`
	Vector      uint8  `json:"vector"`
	Source      string `json:"source"` // "HW" | "SW"
	Handler     uint32 `json:"handler"`
	FromMode    uint8  `json:"from_mode"`
	SavedEIP    uint32 `json:"saved_eip,omitempty"`
	SavedCS     uint16 `json:"saved_cs,omitempty"`
	SavedEFLAGS uint32 `json:"saved_eflags,omitempty"`
}

// Tag returns the event tag.
func (EventIRQFire) Tag() string { return "irq_fire" }

// EventPageFault is emitted when a memory access violates the region's
// permissions. Unlike a failed mem_read/mem_write, this event triggers
// INT 0x0E and may cause the kernel to kill the process.
type EventPageFault struct {
	CoreID     int    `json:"core_id"`
	Addr       uint32 `json:"addr"`
	Mode       uint8  `json:"mode"`
	PermNeeded string `json:"perm_needed"` // "R", "W", "X"
}

// Tag returns the event tag.
func (EventPageFault) Tag() string { return "page_fault" }

// EventHalt indicates the CPU executed HLT. On a real OS the CPU sleeps until
// the next IRQ; in the simulator, if no process is READY, execution stops.
type EventHalt struct {
	CoreID int `json:"core_id"`
	PID    int `json:"pid"`
}

// Tag returns the event tag.
func (EventHalt) Tag() string { return "halt" }

// EventPCBSave is emitted when the kernel saves a process's state before a
// context switch. CoreID identifies which core was running the process —
// narrators use it to anchor the save animation at the right core panel.
type EventPCBSave struct {
	CoreID int               `json:"core_id"`
	PID    int               `json:"pid"`
	Regs   map[string]uint32 `json:"regs"`
}

// Tag returns the event tag.
func (EventPCBSave) Tag() string { return "pcb_save" }

// EventPCBRestore is emitted when the kernel loads a process's state into the
// CPU so execution can resume.
type EventPCBRestore struct {
	CoreID int               `json:"core_id"`
	PID    int               `json:"pid"`
	Regs   map[string]uint32 `json:"regs"`
}

// Tag returns the event tag.
func (EventPCBRestore) Tag() string { return "pcb_restore" }

// EventSchedDecision is the headline event of the demo: the scheduler chose
// to evict FromPID and run ToPID. ReadyQueue shows the queue state after the
// decision. Reason names the trigger ("timer", "block", "exit", ...) so the
// frontend can colour the entry differently in the context-switch feed.
type EventSchedDecision struct {
	FromPID    int    `json:"from_pid"`
	ToPID      int    `json:"to_pid"`
	Reason     string `json:"reason,omitempty"`
	ReadyQueue []int  `json:"ready_queue"`
}

// Tag returns the event tag.
func (EventSchedDecision) Tag() string { return "sched_decision" }

// EventProcessState reflects a process state transition (RUN/READY/BLOCKED/ZOMBIE).
type EventProcessState struct {
	PID    int    `json:"pid"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// Tag returns the event tag.
func (EventProcessState) Tag() string { return "process_state" }

// EventSyscall describes a syscall: number, name, args, and result. It is
// emitted by the kernel after the dispatch completes but before IRET.
type EventSyscall struct {
	PID    int      `json:"pid"`
	Num    uint32   `json:"num"`
	Name   string   `json:"name"`
	Args   []uint32 `json:"args"`
	Result uint32   `json:"result"`
}

// Tag returns the event tag.
func (EventSyscall) Tag() string { return "syscall" }

// EventConsoleOut delivers bytes that crossed the console boundary. Text is
// the UTF-8 representation of the buffer; the raw bytes are included so the
// frontend may hex-dump them if it wants. CoreID identifies the core whose
// activity produced the bytes — used by the console panel to tag each chunk
// with the originating core in multicore demos. Kind distinguishes keyboard
// echo emitted by the kernel ("in") from sys_write output produced by a
// process ("out", the default).
type EventConsoleOut struct {
	CoreID int    `json:"core_id"`
	PID    int    `json:"pid"`
	Kind   string `json:"kind,omitempty"`
	Bytes  []byte `json:"bytes"`
	Text   string `json:"text"`
}

// Tag returns the event tag.
func (EventConsoleOut) Tag() string { return "console_out" }

// EventCoreAssign is emitted when a core starts executing a PID, either at
// bootstrap or after a scheduling decision. The frontend can use it to draw
// which PID lives on which core in multicore demos.
type EventCoreAssign struct {
	CoreID int `json:"core_id"`
	PID    int `json:"pid"`
}

// Tag returns the event tag.
func (EventCoreAssign) Tag() string { return "core_assign" }

// EventCoreRelease is emitted when a core stops executing a PID — either
// because the process exited, blocked, or got preempted with no replacement
// available.
type EventCoreRelease struct {
	CoreID int    `json:"core_id"`
	PID    int    `json:"pid"`
	Reason string `json:"reason,omitempty"`
}

// Tag returns the event tag.
func (EventCoreRelease) Tag() string { return "core_release" }

// EventMutexOp summarizes every action on a kernel mutex so the frontend can
// animate locks, contention, and handoffs without juggling near-identical
// event types. Op values:
//
//   - "create"   — mutex_create returned a new ID.
//   - "acquired" — pid took the lock immediately (no contention).
//   - "waiting"  — pid blocked because someone else held the lock.
//   - "released" — pid released the lock and no one was waiting.
//   - "handoff"  — pid released the lock and the next waiter took it.
//   - "rejected" — pid attempted to unlock a mutex it does not hold; the
//     kernel rejects the call and leaves the mutex state unchanged.
type EventMutexOp struct {
	CoreID  int    `json:"core_id"`
	PID     int    `json:"pid"`
	MutexID int    `json:"mutex_id"`
	Op      string `json:"op"`
	NextPID int    `json:"next_pid,omitempty"`
	Waiters []int  `json:"waiters,omitempty"`
}

// Tag returns the event tag.
func (EventMutexOp) Tag() string { return "mutex_op" }

// EventFork records the creation of a new process via sys_fork or sys_vfork.
// IsVfork distinguishes the two flavours: vfork shares the parent's memory
// and blocks the parent until the child exits, whereas fork gives the child
// its own copy of the parent's address space.
type EventFork struct {
	ParentPID int  `json:"parent_pid"`
	ChildPID  int  `json:"child_pid"`
	IsVfork   bool `json:"is_vfork"`
}

// Tag returns the event tag.
func (EventFork) Tag() string { return "fork" }

// EventWaitOp narrates the sys_waitpid lifecycle.
//
//   - Op "call"  — parent invoked waitpid and blocked (no zombie ready).
//   - Op "reap"  — a zombie child was harvested. ChildPID identifies it and
//     ExitCode carries the value returned to sys_waitpid.
type EventWaitOp struct {
	PID      int    `json:"pid"`
	ChildPID int    `json:"child_pid"`
	Op       string `json:"op"`
	ExitCode uint32 `json:"exit_code,omitempty"`
}

// Tag returns the event tag.
func (EventWaitOp) Tag() string { return "wait_op" }

// EventReparent is emitted when a process dies leaving children behind: the
// orphans are transferred to init (PID 1) so they are not lost. The frontend
// uses it to slide the affected nodes under the new parent in the process
// tree without rebuilding the whole panel.
type EventReparent struct {
	PID       int `json:"pid"`
	OldParent int `json:"old_parent"`
	NewParent int `json:"new_parent"`
}

// Tag returns the event tag.
func (EventReparent) Tag() string { return "reparent" }

// EventPipeCreate marks the birth of a pipe or FIFO. IsFifo distinguishes
// the two: anonymous pipes are paired with their creator alone, FIFOs are
// registered by Name in a tiny VFS so unrelated processes can open them.
type EventPipeCreate struct {
	PipeID    int    `json:"pipe_id"`
	OwnerPID  int    `json:"owner_pid"`
	FdR       uint32 `json:"fd_r"`
	FdW       uint32 `json:"fd_w"`
	IsFifo    bool   `json:"is_fifo"`
	Name      string `json:"name,omitempty"`
}

// Tag returns the event tag.
func (EventPipeCreate) Tag() string { return "pipe_create" }

// EventPipeOp narrates a read or write through a pipe. BufLevel and BufCap
// describe the buffer fill state after the op, so the IPC panel can render
// the buffer bar without re-querying the kernel.
type EventPipeOp struct {
	PipeID   int    `json:"pipe_id"`
	PID      int    `json:"pid"`
	Op       string `json:"op"` // "read" | "write"
	Bytes    []byte `json:"bytes,omitempty"`
	BufLevel int    `json:"buf_level"`
	BufCap   int    `json:"buf_cap"`
}

// Tag returns the event tag.
func (EventPipeOp) Tag() string { return "pipe_op" }

// KbdReader identifies a process blocked in sys_read(stdin), shown in the
// console panel so the student can see which programs are waiting for input.
type KbdReader struct {
	PID  int    `json:"pid"`
	Prog string `json:"prog"`
}

// EventKbdState exposes the keyboard driver's state to observers: how many
// bytes are queued in KeyboardBuf and which processes are blocked waiting in
// sys_read(stdin). Emitted whenever either side changes.
type EventKbdState struct {
	BufUsed int         `json:"buf_used"`
	BufCap  int         `json:"buf_cap"`
	Readers []KbdReader `json:"readers"`
}

// Tag returns the event tag.
func (EventKbdState) Tag() string { return "kbd_state" }

// EventSemOp summarizes every action on a kernel semaphore. Op values:
//
//   - "create"          — sem_create returned a new ID.
//   - "wait_immediate"  — pid decremented count and continued.
//   - "wait_blocked"    — count was 0 so pid blocked.
//   - "signal"          — pid incremented count, no one was waiting.
//   - "handoff"         — pid signaled and the first waiter was unblocked.
type EventSemOp struct {
	CoreID  int    `json:"core_id"`
	PID     int    `json:"pid"`
	SemID   int    `json:"sem_id"`
	Op      string `json:"op"`
	Count   int    `json:"count"`
	NextPID int    `json:"next_pid,omitempty"`
	Waiters []int  `json:"waiters,omitempty"`
}

// Tag returns the event tag.
func (EventSemOp) Tag() string { return "sem_op" }
