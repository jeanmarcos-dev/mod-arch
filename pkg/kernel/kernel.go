package kernel

import (
	"fmt"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// Process memory layout constants.
//
// user_code (0x200000-0x2FFFFF) is divided into MaxProcesses slots of
// ProcCodeSize bytes; user_stack follows the same scheme. PIDs are 1-indexed
// to match the Linux convention where PID 0 is reserved for the swapper.
const (
	MaxProcesses  = 4
	ProcCodeSize  = 0x40000 // 256 KB per process
	ProcStackSize = 0x40000
)

// MaxCores caps how many simulated cores Kernel.NewMulti will spin up. The
// limit is a pedagogical choice: 1–4 covers the demos cleanly without forcing
// the UI to handle arbitrarily many CPU panels.
const MaxCores = 4

// perCoreKernelStackSize is the slice of the kernel stack reserved per core,
// carved out of the existing kernel region. Cores grow their stacks downward
// from their own top, so a 64 KB slice is roomy for the simulator's tiny
// interrupt frames while still keeping all 4 cores well above AddrKernelText.
const perCoreKernelStackSize = 0x10000

// processCodeBase returns the base address where the code of the given PID
// is loaded. PID 1 -> 0x200000, PID 2 -> 0x240000, etc.
func processCodeBase(pid int) uint32 {
	return cpu.AddrUserBase + uint32(pid-1)*ProcCodeSize
}

// processStackTop returns the initial ESP (top of stack) for the given PID.
// PID 1 -> 0x33FFF0, PID 2 -> 0x37FFF0, etc.
//
// The 0x10-byte margin keeps the first push from landing exactly on the
// region boundary (defensive).
func processStackTop(pid int) uint32 {
	return cpu.AddrUserStack + uint32(pid)*ProcStackSize - 0x10
}

// Kernel wraps one or more CPU cores and adds the OS software decisions.
type Kernel struct {
	// Cores is the source of truth for execution state. Cores[0] is the boot
	// core; later cores come up idle until the scheduler hands them work.
	Cores []*Core

	// Mem is the shared physical memory all cores read and write through.
	Mem *cpu.Memory

	// TimerQuantum is the number of CPU ticks between IRQ 0x20 fires on each
	// core. 0 disables the timer. Default: 8.
	TimerQuantum int

	// Processes is the global process table keyed by PID.
	Processes map[int]*PCB

	// Ready is the shared FIFO of READY PIDs waiting for an idle core. It is
	// guarded by its own mutex because multicore demos pop concurrently.
	Ready *ReadyQueue

	// KeyboardBuf holds pending bytes; blockedReads is the FIFO queue of
	// processes waiting in sys_read. A queue is required because several
	// processes may be waiting on the same resource (the keyboard driver here).
	KeyboardBuf  []byte
	blockedReads []*blockedReadRequest

	// Mutexes and Semaphores are 1-indexed registries of the kernel's
	// synchronization primitives, exposed through INT 0x81.
	Mutexes    []*Mutex
	Semaphores []*Semaphore

	// Pipes is the 1-indexed registry of pipe objects created by sys_pipe
	// and sys_mknod (FIFOs). Per-process FDs in PCB.FDTable point into
	// this slice via FD.PipeID.
	Pipes []*Pipe

	// FifoByName maps a FIFO path (e.g. "/tmp/q") to its PipeID. Allows
	// processes that never met (no fork ancestry) to talk through the
	// same Pipe by opening the same name.
	FifoByName map[string]int

	// InitPID is the PID of the init process (PID 1 by convention) when one
	// has been spawned via SpawnInit. Zero means no init exists yet — in
	// that mode, sys_exit leaves orphaned children with ParentPID unchanged
	// and re-parenting is skipped. The IPC demos call SpawnInit before
	// loading their programs so the process tree always has a root.
	InitPID int

	// memSpaces and fdSpaces register the AddressSpace and FDSpace objects
	// referenced by live PCBs. Sharing across tasks of the same thread group
	// goes through these maps via shareMemSpace / shareFDSpace, which bump
	// refcounts; releaseMemSpace / releaseFDSpace delete entries when no PCB
	// references them anymore.
	memSpaces      map[int]*AddressSpace
	fdSpaces       map[int]*FDSpace
	nextMemSpaceID int
	nextFDSpaceID  int

	nextPID int
}

// New builds a single-core Kernel — the original behavior. Delegates to
// NewMulti with numCores=1 so the test suite and existing call sites stay
// unchanged.
func New(memSize int) *Kernel {
	return NewMulti(memSize, 1)
}

// NewMulti builds a Kernel with the requested number of cores (clamped to
// [1, MaxCores]). All cores share the same physical memory and the same IDT;
// only the register file, pending IRQ queue, and kernel stack pointer are
// per-core.
func NewMulti(memSize, numCores int) *Kernel {
	if numCores < 1 {
		numCores = 1
	}
	if numCores > MaxCores {
		numCores = MaxCores
	}

	mem := cpu.NewMemory(memSize)
	cores := make([]*Core, numCores)
	for i := range cores {
		stackTop := cpu.KernelStackTop - uint32(i)*perCoreKernelStackSize
		var c *cpu.CPU
		if i == 0 {
			// The boot core takes ownership of the freshly-allocated memory so
			// the resulting CPU is exactly equivalent to cpu.New(memSize).
			c = cpu.New(memSize)
			c.Mem = mem
			c.KernelStackTop = stackTop
		} else {
			c = cpu.NewWithMemory(mem, stackTop)
		}
		// Every core starts halted. With no process assigned there is nothing
		// to advance, so HasWork() must report false until DispatchIdleCores
		// wakes a core via loadCPUFromPCB. This mirrors Reset() and prevents
		// the run loop from spinning on an empty kernel.
		c.Halted = true
		// Tag the CPU with its core index so every event it emits carries
		// CoreID — the frontend uses it to keep per-core register files
		// separate instead of overwriting a global one.
		c.CoreID = i
		cores[i] = &Core{ID: i, CPU: c}
	}

	k := &Kernel{
		Cores:        cores,
		Mem:          mem,
		TimerQuantum: 8,
		Processes:    make(map[int]*PCB),
		Ready:        NewReadyQueue(),
		memSpaces:    make(map[int]*AddressSpace),
		fdSpaces:     make(map[int]*FDSpace),
		nextPID:      1,
	}
	k.InstallDefaultIDT()
	return k
}

// BootCPU returns the CPU of the boot core (Cores[0]). Convenience accessor
// for callers that only care about single-core state: the CLI tracer, the
// snapshot helpers, and the IDT installer.
func (k *Kernel) BootCPU() *cpu.CPU {
	return k.Cores[0].CPU
}

// BootCore returns Cores[0] — the boot core.
func (k *Kernel) BootCore() *Core {
	return k.Cores[0]
}

// NumCores reports how many cores this kernel owns.
func (k *Kernel) NumCores() int {
	return len(k.Cores)
}

// AllHalted is true when every core has Halted=true. The run loop in sim
// uses it as the user-visible "machine is halted" signal it broadcasts to
// the frontend; do not use it to decide whether the simulator still has
// work to advance — see HasWork below.
func (k *Kernel) AllHalted() bool {
	for _, c := range k.Cores {
		if !c.CPU.Halted {
			return false
		}
	}
	return true
}

// HasWork reports whether any core can still make progress on the next
// Step: either it is actively running, has an interrupt waiting to be
// dispatched, or there is a process queued in READY waiting for a core.
// Halted-with-pending-IRQ counts as work because dispatchIRQ clears the
// halt as soon as the CPU runs one more cycle. Halted-with-ready-queue
// counts because Simulator.StepOnce calls DispatchIdleCores first, which
// wakes a core via loadCPUFromPCB.
//
// The run loop polls this instead of AllHalted so that an asynchronous IRQ
// arriving while every core is halted (e.g. PushKey on a process blocked in
// sys_read) does not get stuck waiting for a ticker that already shut down.
func (k *Kernel) HasWork() bool {
	for _, c := range k.Cores {
		if !c.CPU.Halted {
			return true
		}
		if len(c.CPU.PendingIRQs) > 0 {
			return true
		}
	}
	return k.Ready.Len() > 0
}

// Reset clears the process table and reinstalls the IDT. Useful for loading a
// fresh scenario from scratch. All cores go back to their initial state.
func (k *Kernel) Reset() {
	for _, c := range k.Cores {
		c.CPU.Reset()
		c.CurrentPID = 0
		c.lastTimerFiredAt = 0
		// CPU.Reset clears Halted; with no process loaded every core has
		// nothing to run, so park them all halted. loadCPUFromPCB clears
		// Halted on the core that receives the next AddProcess call.
		// Without this Reset, the run loop would spin forever after a
		// Reset on an idle kernel because AllHalted() would stay false.
		c.CPU.Halted = true
	}
	k.Processes = make(map[int]*PCB)
	k.Ready.Reset()
	k.nextPID = 1
	k.KeyboardBuf = nil
	k.blockedReads = nil
	k.Mutexes = nil
	k.Semaphores = nil
	// IPC and init bookkeeping must also be wiped: without this, a second
	// LoadDemo sees k.InitPID still pointing at a now-deleted PCB, so
	// SpawnInit's idempotency check short-circuits and the demo's first
	// AddProcess takes PID 1, leaving the tree rootless. Same hazard for
	// stale Pipes/FifoByName pointing into a freed process table.
	k.InitPID = 0
	k.Pipes = nil
	k.FifoByName = nil
	// AddressSpace and FDSpace registries must be cleared too: a leftover
	// pointer in the old map would survive into the next demo and break
	// refcount accounting for fork/vfork/clone.
	k.memSpaces = make(map[int]*AddressSpace)
	k.fdSpaces = make(map[int]*FDSpace)
	k.nextMemSpaceID = 0
	k.nextFDSpaceID = 0
	// The IDT lives in kernel memory and CPU.Reset does not touch it, but
	// reinstall defensively in case a test mutated entries.
	k.InstallDefaultIDT()
}

// AddProcess registers a new process, copies its code into the user_code
// region for that process, and returns the assigned PID. The process is left
// in the READY state, queued for dispatch — actual core assignment happens
// later via DispatchIdleCores (invoked by Simulator.StartRun / StepOnce).
//
// The first registered process becomes CurrentPID on the boot core and is
// loaded into its CPU; any subsequent ones either fill remaining idle cores
// (in multicore mode) or go onto the ready queue.
func (k *Kernel) AddProcess(name string, code []byte) (int, error) {
	if k.nextPID > MaxProcesses {
		return 0, fmt.Errorf("maximum of %d processes supported", MaxProcesses)
	}
	if uint32(len(code)) > ProcCodeSize {
		return 0, fmt.Errorf("code of %d bytes exceeds ProcCodeSize=%d", len(code), ProcCodeSize)
	}

	pid := k.nextPID
	k.nextPID++

	codeBase := processCodeBase(pid)
	stackTop := processStackTop(pid)

	// Zero the region before loading to avoid leftovers from a prior program.
	for i := codeBase; i < codeBase+ProcCodeSize; i++ {
		k.Mem.Bytes[i] = 0
	}
	if err := k.Mem.LoadBytes(codeBase, code); err != nil {
		return 0, fmt.Errorf("load process %d code: %w", pid, err)
	}

	pcb := &PCB{
		PID:      pid,
		Name:     name,
		State:    StateReady,
		Mode:     cpu.ModeUser,
		StackTop: stackTop,
		Core:     -1,
		TGID:     pid,
		memSpace: k.newMemSpace(codeBase, uint32(len(code))),
		fdSpace:  k.newFDSpace(),
	}
	// Initial state: as if the process were saved right before its first
	// instruction. EIP at the code base, ESP at the top of its stack, EFLAGS
	// with IF=1 (interrupts enabled), and user CS/DS/SS.
	pcb.Regs.EIP = codeBase
	pcb.Regs.EFLAGS = cpu.FlagIF
	pcb.Regs.CS = cpu.SelUserCode
	pcb.Regs.DS = cpu.SelUserData
	pcb.Regs.SS = cpu.SelUserData
	pcb.Regs.EBP = stackTop
	pcb.SavedUserESP = stackTop
	pcb.SavedUserSS = cpu.SelUserData

	k.Processes[pid] = pcb

	// Load-time semantics: every process enters READY. Dispatch to a core
	// happens later via DispatchIdleCores so the READY→RUN transition is
	// observable in the event log instead of hidden inside AddProcess.
	k.Ready.Push(pid)
	return pid, nil
}

// firstIdleCore returns the lowest-ID core that has no CurrentPID, or nil if
// every core is busy.
func (k *Kernel) firstIdleCore() *Core {
	for _, c := range k.Cores {
		if c.CurrentPID == 0 {
			return c
		}
	}
	return nil
}

// popNextRunnable pops the head of the ready queue, skipping init. Any
// init PID encountered is re-queued at the tail so DispatchIdleCores can
// pick it up later and route it through runInitReapCycle. Returns
// (pid, true) when a non-init PID was found, (0, false) when only init
// remained (or the queue was empty). Used by killCurrent and scheduleNext
// — the regular dispatch paths that must never bind init to a core.
func (k *Kernel) popNextRunnable() (int, bool) {
	for i := 0; i < k.Ready.Len(); i++ {
		pid, ok := k.Ready.Pop()
		if !ok {
			return 0, false
		}
		if k.InitPID != 0 && pid == k.InitPID {
			k.Ready.Push(pid)
			continue
		}
		return pid, true
	}
	return 0, false
}

// Step advances the simulation by one tick, iterating every core in order.
// Each core runs at most one CPU.Step plus, if that step dispatched an IRQ,
// the matching handler logic. Returning all events in a single slice keeps
// the WS contract identical to the single-core model.
//
// Before iterating cores, Step pulls READY processes onto any idle core via
// DispatchIdleCores. The simulator also calls DispatchIdleCores before
// Step in StepOnce, and the call is idempotent when every core has work,
// so running it here costs nothing in the steady state. It matters for the
// deferred-reap flow: when a parent is woken to the ready queue at the end
// of one Step (e.g. its child just exited), this dispatch picks the parent
// up on the very next Step instead of leaving the boot core halted forever.
func (k *Kernel) Step() ([]cpu.Event, error) {
	out := k.DispatchIdleCores()
	for _, core := range k.Cores {
		evs, err := k.stepCore(core)
		out = append(out, evs...)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// stepCore advances a single core: timer check → CPU.Step → handler logic.
//
// A core with no current process and no pending IRQ is treated as completely
// idle: it does not tick and emits nothing. It only wakes up once the
// scheduler assigns it work or routes an interrupt its way.
func (k *Kernel) stepCore(core *Core) ([]cpu.Event, error) {
	if core.CurrentPID == 0 && len(core.CPU.PendingIRQs) == 0 {
		return nil, nil
	}
	if k.shouldFireTimer(core) {
		core.CPU.PendingIRQs = append(core.CPU.PendingIRQs, IRQTimer)
		core.lastTimerFiredAt = core.CPU.Tick
	}
	// Snapshot the running PID *before* the step so EventInstruction (emitted
	// at fetch time, inside CPU.Step) can be stamped with the process that
	// actually executed it — even if a later handler in this same step
	// switches the core to a different process.
	runningPID := core.CurrentPID
	evs, err := core.CPU.Step()
	for i, e := range evs {
		if inst, ok := e.(cpu.EventInstruction); ok {
			inst.PID = runningPID
			evs[i] = inst
		}
	}
	if err != nil {
		return evs, err
	}
	for _, e := range evs {
		irq, ok := e.(cpu.EventIRQFire)
		if !ok {
			continue
		}
		schedEvs := k.runHandlerLogic(core, irq.Vector, irq.Source)
		// The handler may have called cpu.Emit (e.g. mem_read while reading
		// the user buffer in sys_write). CPU.Step has already returned its
		// own slice, so drain whatever the handler accumulated and merge.
		handlerEvs := core.CPU.DrainEvents()
		evs = append(evs, handlerEvs...)
		evs = append(evs, schedEvs...)
	}
	return evs, nil
}

// shouldFireTimer decides whether the PIT should fire on this step of the
// given core. The quantum counts only user-mode ticks — while the core is
// executing kernel code (handlers, IRETs) or sitting idle (CurrentPID=0),
// lastFired keeps advancing so the next user slice gets its full quantum.
// Without this, timer IRQs would accumulate in PendingIRQs while IF=0 and
// all dispatch back-to-back on the next IRET, starving user instructions
// between successive fires. The idle-core check also prevents a halted core
// (post-killCurrent with no Ready left) from being woken into running stale
// user EIPs past sys_exit.
func (k *Kernel) shouldFireTimer(core *Core) bool {
	if k.TimerQuantum <= 0 {
		return false
	}
	if core.CurrentPID == 0 || core.CPU.Mode != cpu.ModeUser {
		core.lastTimerFiredAt = core.CPU.Tick
		return false
	}
	return core.CPU.Tick >= core.lastTimerFiredAt+uint64(k.TimerQuantum) && core.CPU.Tick > 0
}

// runHandlerLogic executes the work a real OS would perform between handler
// entry and IRET. We model it in Go so we do not need a full scheduler written
// in hand-encoded assembly.
//
// The handler bytes in memory are still real (nop + iret); this function runs
// BEFORE the handler body executes, mutates the kernel-stack frame and CPU
// registers, and the subsequent IRET pops the patched frame to land in the
// new process.
func (k *Kernel) runHandlerLogic(core *Core, vector uint8, source string) []cpu.Event {
	switch vector {
	case IRQTimer:
		// Only schedule on HW timer; ignore SW-triggered INT 0x20.
		if source != "HW" {
			return nil
		}
		return k.scheduleNext(core)
	case IRQSyscall:
		// Vector 0x80 is reserved for syscalls (user invokes it via `int 0x80`,
		// source SW). Arrival via HW indicates a bug.
		if source != "SW" {
			return nil
		}
		return k.handleSyscall(core)
	case IRQSync:
		// Vector 0x81 carries mutex/semaphore sub-syscalls. Same SW-only
		// convention as 0x80.
		if source != "SW" {
			return nil
		}
		return k.handleSync(core)
	case IRQPageFault:
		// Page faults always come via "fault" (the MMU raises them on access
		// violation). A real OS could resolve some (page-in, COW); here we
		// kill the offending process.
		if source != "fault" {
			return nil
		}
		return k.killCurrent(core, "page_fault")
	case IRQGeneralProt:
		// #GP is raised by the CPU when user code executes a privileged
		// instruction (HLT in PL3), uses a bad segment, etc. Kill the process.
		if source != "fault" {
			return nil
		}
		return k.killCurrent(core, "general_protection")
	case IRQKeyboard:
		// Keyboard: HW (PushKey) injects this IRQ with source HW. Deliver any
		// pending bytes to the first process BLOCKED in sys_read.
		if source != "HW" {
			return nil
		}
		return k.handleKeyboard(core)
	default:
		return nil
	}
}

// killCurrent terminates the current process on the given core and schedules
// the next one if any remain. Used by sys_exit and by page-fault handling.
// The PCB is left as ZOMBIE with the cause recorded in BlockedOn.
//
// Before flipping the state we evict the dying PID from every sync queue it
// might still appear in (mutexes, semaphores, keyboard, ready). Otherwise a
// later unlock/signal could "wake" the zombie via wakeWaiter and switchTo
// would resurrect it — IRET would land at its saved EIP past sys_exit and
// execute garbage opcodes.
func (k *Kernel) killCurrent(core *Core, reason string) []cpu.Event {
	curPID := core.CurrentPID
	curPCB := k.Processes[curPID]
	if curPCB == nil {
		core.CPU.Halted = true
		return nil
	}
	k.evictFromAllQueues(curPID)
	curPCB.State = StateZombie
	curPCB.BlockedOn = reason
	curPCB.Core = -1

	out := []cpu.Event{
		cpu.EventProcessState{PID: curPID, State: string(StateZombie), Reason: reason},
	}

	// Re-parent any surviving children to init so they remain reachable
	// in the process tree and so orphan zombies do not stick to a PCB
	// that is itself about to disappear. Live children land under init
	// as normal; orphan zombies queue on init.PendingReaps and init is
	// woken to the ready queue — the actual reap fires when the
	// dispatcher picks init on the next Step. Without this deferral the
	// zombie would vanish in the same tick it was orphaned, hiding the
	// "init wakes, cleans up, sleeps again" cycle from the alumno.
	if k.InitPID != 0 && curPID != k.InitPID && len(curPCB.Children) > 0 {
		init := k.Processes[k.InitPID]
		if init != nil {
			initWokenForReap := false
			for _, childPID := range curPCB.Children {
				child := k.Processes[childPID]
				if child == nil {
					continue
				}
				child.ParentPID = k.InitPID
				out = append(out, cpu.EventReparent{
					PID: childPID, OldParent: curPID, NewParent: k.InitPID,
				})
				init.Children = append(init.Children, childPID)
				if child.State == StateZombie {
					init.PendingReaps = append(init.PendingReaps, childPID)
					initWokenForReap = true
				}
			}
			curPCB.Children = nil
			if initWokenForReap && init.State == StateBlocked {
				out = append(out, k.wakeToQueue(k.InitPID, initBlockedReason)...)
			}
		}
	}

	// If other READY processes exist, hand the core to the first one. This is
	// the scheduleNext pattern WITHOUT re-enqueueing the current PID (zombie
	// processes do not return). Skip init here — it must never be dispatched
	// onto a core through the regular path because it has no user code worth
	// running (its slot only holds two hlt bytes). Init runs only via
	// runInitReapCycle, which DispatchIdleCores invokes on the next Step
	// when it sees init at the head of the queue.
	if nextPID, ok := k.popNextRunnable(); ok {
		nextPCB := k.Processes[nextPID]
		if nextPCB != nil {
			out = append(out, cpu.EventSchedDecision{
				FromPID:    curPID,
				ToPID:      nextPID,
				Reason:     reason,
				ReadyQueue: k.Ready.Snapshot(),
			})

			core.CPU.R.EAX = nextPCB.Regs.EAX
			core.CPU.R.EBX = nextPCB.Regs.EBX
			core.CPU.R.ECX = nextPCB.Regs.ECX
			core.CPU.R.EDX = nextPCB.Regs.EDX
			core.CPU.R.ESI = nextPCB.Regs.ESI
			core.CPU.R.EDI = nextPCB.Regs.EDI
			core.CPU.R.EBP = nextPCB.Regs.EBP
			core.CPU.R.DS = nextPCB.Regs.DS
			core.CPU.SavedUserESP = nextPCB.SavedUserESP
			core.CPU.SavedUserSS = nextPCB.SavedUserSS

			writeKernelDWORD(core.CPU, core.CPU.R.ESP+0, nextPCB.Regs.EIP)
			writeKernelDWORD(core.CPU, core.CPU.R.ESP+4, uint32(nextPCB.Regs.CS))
			writeKernelDWORD(core.CPU, core.CPU.R.ESP+8, nextPCB.Regs.EFLAGS)

			out = append(out, cpu.EventPCBRestore{CoreID: core.ID, PID: nextPID, Regs: pcbRegsSnapshot(nextPCB)})
			nextPCB.State = StateRun
			nextPCB.Core = core.ID
			core.CurrentPID = nextPID
			out = append(out,
				cpu.EventCoreRelease{CoreID: core.ID, PID: curPID, Reason: reason},
				cpu.EventCoreAssign{CoreID: core.ID, PID: nextPID},
				cpu.EventProcessState{PID: nextPID, State: string(StateRun)},
			)
			return out
		}
	}

	// No remaining processes for this core: release it and halt the CPU.
	core.CPU.Halted = true
	core.CurrentPID = 0
	out = append(out,
		cpu.EventCoreRelease{CoreID: core.ID, PID: curPID, Reason: reason},
		cpu.EventHalt{CoreID: core.ID, PID: curPID},
	)
	return out
}
