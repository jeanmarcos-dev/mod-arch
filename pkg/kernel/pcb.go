package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// State is the current state of a process in the operating system.
//
// Transition diagram:
//
//	NEW ──admit──▶ READY ──schedule──▶ RUN ──preempt(timer)──▶ READY
//	                  ▲                  │
//	                  └────unblock───── BLOCKED ◀── sys_read with no data, etc.
//	                                    │
//	                                    └── exit/kill ──▶ ZOMBIE
type State string

// Process lifecycle states.
const (
	StateNew     State = "NEW"
	StateReady   State = "READY"
	StateRun     State = "RUN"
	StateBlocked State = "BLOCKED"
	StateZombie  State = "ZOMBIE"
	// StateIdle is the resting state of the kernel idle task (PID 0). It is
	// never reached by user processes: the swapper sits in it whenever no
	// runnable process exists, executing HLT until the next IRQ. Modeled as
	// a distinct state so the frontend can render the dedicated HLT badge
	// without branching on PID inside every RUN-state code path.
	StateIdle State = "IDLE"
)

// PCB (Process Control Block) is the bookkeeping the kernel maintains per
// process. While the process is running, its registers live in the actual
// CPU; when another process takes over, the values are preserved here and
// the new one's are loaded into the CPU.
//
// In real x86 this role is played by `task_struct` (Linux) or `_EPROCESS`
// (Windows). The structure here is a heavily simplified model.
type PCB struct {
	PID       int
	Name      string
	State     State
	BlockedOn string // description while State == BLOCKED ("keyboard", etc.)

	// BlockedSince records the CPU tick at which the PCB last entered the
	// BLOCKED state. Zero means "not currently blocked" (or freshly spawned
	// init at boot). The frontend subtracts this from the live tick to show
	// how long a waiter has been parked on its resource.
	BlockedSince uint64

	// Regs is a snapshot of the user-visible register file.
	// While the process is RUN these values are stale: the live regs are in
	// cpu.CPU.R.
	Regs cpu.Registers

	// SavedUserESP/SS mirror the same-named CPU fields. While the process is
	// RUN and has entered kernel, this holds the user ESP at entry. While the
	// process is READY/BLOCKED, it holds the stack position it left behind.
	SavedUserESP uint32
	SavedUserSS  uint16

	// Mode is the privilege level of the process. User processes are always
	// PL3; stored for completeness (e.g. a process could become ZOMBIE while
	// in kernel mode).
	Mode uint8

	// StackTop is the initial ESP for this task (stack grows downward).
	// Each PCB owns its own stack region even when several PCBs share an
	// AddressSpace through CLONE_VM, so the layout stays here on the PCB
	// rather than inside AddressSpace.
	StackTop uint32

	// Slot is the index in the user-code region this PCB occupies. PIDs are
	// monotonically increasing (1, 2, 3...), but slots are allocated only for
	// processes that load user code into the 0x200000–0x2FFFFF range. init
	// (PID 1) carries Slot=-1 because its body lives in a reserved kernel
	// zone; demo processes loaded after it get Slot=0, 1, 2, … so their
	// code base stays at 0x200000 regardless of init's presence. Hardcoded
	// jump targets inside demos (e.g. clone() entry points) keep working.
	Slot int

	// memSpace points to the AddressSpace this task lives in. fork() gives
	// the child a fresh one; vfork() and clone(CLONE_VM) share with the
	// parent so threads of the same process see the same .text and globals.
	// AddressSpace carries CodeBase / CodeSize.
	memSpace *AddressSpace

	// fdSpace points to the file-descriptor table this task sees. fork()
	// copies; clone(CLONE_FILES) shares so dup/close ripple to siblings.
	// FDSpace owns the underlying []FD.
	fdSpace *FDSpace

	// TGID (Thread Group ID) is the PID of the thread-group leader. For a
	// fork()-created process, TGID == PID. clone(CLONE_THREAD) inherits the
	// parent's TGID so getpid()-style lookups can group threads under their
	// process.
	TGID int

	// TID is the per-task identity inside a thread group. For regular procs
	// this equals PID, which is the default and what the snapshot exposes
	// when this field is zero. The swappers (PID 0, one per core) override
	// it with the core index so the snapshot can disambiguate "swapper/0"
	// from "swapper/1" even though their PIDs collide.
	TID int

	// Core is the ID of the core that currently owns this PCB (RUN), or -1
	// when the process is READY/BLOCKED/ZOMBIE and no core holds it.
	Core int

	// ParentPID is the PID of the process that spawned this one through
	// sys_fork or sys_vfork. Zero means "no parent" (top-level processes
	// loaded directly through AddProcess).
	ParentPID int

	// Children is the list of child PIDs spawned by this process and not yet
	// reaped via sys_waitpid (live + zombies). Re-parenting on exit walks
	// this list to hand orphans over to init.
	Children []int

	// ExitCode is the value passed to sys_exit. Filled on exit, read by
	// sys_waitpid when the parent reaps this PCB.
	ExitCode uint32

	// Waiter is the child PID this process is waiting for in sys_waitpid:
	//
	//	-1  → any child
	//	>0  → a specific PID
	//	 0  → not currently waiting
	//
	// Used by sys_exit to decide whether to wake this PCB when a child of
	// it becomes zombie.
	Waiter int

	// VforkParent records that this PCB was created via sys_vfork and shares
	// memory with its parent. While the child is alive the parent is parked
	// in BLOCKED with BlockedOn="vfork"; the child's sys_exit wakes it.
	// Zero means "not a vfork child".
	VforkParent int

	// PendingReaps queues zombie child PIDs that this PCB must collect
	// before it can resume user code. notifyParentOfExit appends to this
	// list and wakes the parent to READY; the scheduler drains the list
	// when the parent is next dispatched, modeling the gap between a
	// child's death and the parent's wait() return. For init this queue
	// can grow when several orphans die before init is scheduled.
	PendingReaps []int

	// WasReparentedToInit is true when this PCB was adopted under init
	// because its original parent died with the PCB still alive — i.e. a
	// genuine orphan in the Unix sense. Set by markZombieAndReparent when
	// it walks the dying parent's Children list. Stays false for PCBs
	// adopted by init at load time via AdoptUnderInit (birth-adoption,
	// which is what every WithInit demo currently does). The frontend uses
	// this to tell "init reaped orphan P{n}" apart from "init reaped child
	// P{n}" without having to replay the EventReparent history.
	WasReparentedToInit bool

	// IsKThread marks PCBs that represent the kernel-thread side of the
	// Linux-style hierarchy: kthreadd (PID 2) and the per-core kernel/N
	// tasks that hang from it. These PCBs are never scheduled onto a core
	// — the scheduler skips them — and their RUN/SLEEP appearance is
	// derived in the frontend from the live mode of the bound core, not
	// from State (which stays BLOCKED). They exist so the process tree
	// can show "who executes the kernel on each core" with a real PID
	// rather than a synthetic visual band.
	IsKThread bool
}

// FD describes one entry of the per-process file-descriptor table.
//
//   - Used  → false means "free slot, available for allocation".
//   - Kind  → "pipe-read" or "pipe-write" for pipe fds; reserved strings
//     for future kinds. stdin/stdout/stderr are not entered here.
//   - PipeID → 1-indexed PipeID for pipe fds, zero otherwise.
type FD struct {
	Used   bool
	Kind   string
	PipeID int
}

// IsIdle reports whether this PCB is the kernel idle task (PID 0, the
// Linux-style swapper). Callers use it to special-case the swapper without
// having to thread the IdlePID through every helper.
func (p *PCB) IsIdle() bool { return p.PID == 0 }

// IsKernelThread reports whether this PCB belongs to the kthread hierarchy
// (kthreadd or kernel/N). Method form mirrors IsIdle/IsInit so callers can
// branch on a single predicate without poking at the underlying flag.
func (p *PCB) IsKernelThread() bool { return p.IsKThread }

// tidOrPID returns the explicit TID when set, falling back to PID for the
// majority of procs where TID is left at the zero value. The swappers use
// the explicit TID slot because they all share PID 0.
func (p *PCB) tidOrPID() int {
	if p.TID != 0 {
		return p.TID
	}
	return p.PID
}

// MemSpace returns the AddressSpace this PCB is bound to. Nil means the
// PCB has not been initialised yet (only the init PCB before SpawnInit
// finishes wiring it up).
func (p *PCB) MemSpace() *AddressSpace { return p.memSpace }

// FDSpace returns the file-descriptor table this PCB sees. Nil only on a
// half-initialised PCB.
func (p *PCB) FDSpace() *FDSpace { return p.fdSpace }

// Snapshot returns a JSON-friendly representation of the PCB for the frontend.
// CodeBase / CodeSize are sourced from the AddressSpace so threads of the same
// process (sharing one MemSpace) all report the same base.
func (p *PCB) Snapshot() map[string]any {
	children := append([]int(nil), p.Children...)
	var codeBase, codeSize uint32
	if p.memSpace != nil {
		codeBase = p.memSpace.CodeBase
		codeSize = p.memSpace.CodeSize
	}
	return map[string]any{
		"pid":           p.PID,
		"name":          p.Name,
		"state":         string(p.State),
		"blocked_on":    p.BlockedOn,
		"blocked_since": p.BlockedSince,
		"regs": map[string]uint32{
			"EAX": p.Regs.EAX, "EBX": p.Regs.EBX, "ECX": p.Regs.ECX, "EDX": p.Regs.EDX,
			"ESI": p.Regs.ESI, "EDI": p.Regs.EDI, "EBP": p.Regs.EBP,
			"EIP": p.Regs.EIP, "EFLAGS": p.Regs.EFLAGS,
			// PCB's ESP lives in SavedUserESP, not in Regs.
			"ESP": p.SavedUserESP,
		},
		"saved_user_esp": p.SavedUserESP,
		"code_base":      codeBase,
		"code_size":      codeSize,
		"stack_top":      p.StackTop,
		"parent_pid":     p.ParentPID,
		"children":       children,
		"exit_code":      p.ExitCode,
		"vfork_parent":   p.VforkParent,
		// Thread group fields: tid is just PID under a name the frontend
		// can branch on without parsing semantics, tgid groups siblings,
		// and is_leader flags the task that owns the address space — the
		// one that should render the "process" card with threads nested
		// under it.
		"tgid":      p.TGID,
		"tid":       p.tidOrPID(),
		"is_leader": p.TGID == p.PID,
		// is_idle flags the swapper (PID 0) so the frontend can pull it
		// out of the regular process tree and render it as the compact
		// kernel-idle band instead of a user-proc row.
		"is_idle": p.IsIdle(),
		// is_kthread tags kthreadd and the kernel/N tasks so the tree can
		// nest them under kthreadd instead of mixing them with user procs,
		// and the renderer can derive RUN/SLEEP from cores[N].mode.
		"is_kthread": p.IsKThread,
	}
}
