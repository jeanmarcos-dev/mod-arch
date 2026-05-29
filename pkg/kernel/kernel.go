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

// processCodeBase returns the base address where the code of a user-space
// process is loaded, indexed by its user-code slot (not by PID). Slot 0 →
// 0x200000, slot 1 → 0x240000, etc. init carries Slot=-1 and lives in its
// own reserved zone (initCodeBase), not in this range.
func processCodeBase(slot int) uint32 {
	return cpu.AddrUserBase + uint32(slot)*ProcCodeSize
}

// processStackTop returns the initial ESP (top of stack) for the given
// user-code slot. Slot 0 → 0x37FFF0, slot 1 → 0x3BFFF0, etc.
//
// The 0x10-byte margin keeps the first push from landing exactly on the
// region boundary (defensive). The +1 offset preserves the original
// per-PID layout: slot 0 owns the second 256 KB band so a single demo's
// stack still lands at 0x37FFF0.
func processStackTop(slot int) uint32 {
	return cpu.AddrUserStack + uint32(slot+1)*ProcStackSize - 0x10
}

// initCodeBase is where the init (PID 1) program lives. It sits in the
// kernel-text region just past the IDT and default handlers so it does
// NOT consume a user-code slot and demos keep loading at 0x200000.
const initCodeBase uint32 = cpu.AddrKernelText + 0x1F000

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
	// nextUserSlot is the next index in the user-code region that AddProcess
	// will hand out. It is independent from nextPID so init can reserve
	// PID 1 without consuming slot 0 — that way demo bodies always land at
	// 0x200000 and any hardcoded jump targets they carry stay valid.
	nextUserSlot int

	// PendingKernelWork is a FIFO of deferred kernel phases consumed one per
	// Step. Today only the five do_exit phases land here (sys_exit / page
	// fault / #GP all converge on enqueueExitPhases); the design is
	// intentionally generic so fork/vfork/waitpid can be split into phases
	// later without renaming the queue. Step() drains one non-empty phase
	// before falling through to the normal core-tick loop, which is what
	// gives the alumno a click-per-do_exit-step view of process death.
	PendingKernelWork []pendingKernelWork
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
	if k.Ready.Len() > 0 {
		return true
	}
	// A mid-do_exit cascade leaves cores halted with nothing in the ready
	// queue but with phases still queued. Without this branch the run loop
	// would auto-stop between phases, freezing the cascade mid-way.
	return len(k.PendingKernelWork) > 0
}

// Reset clears the process table and reinstalls the IDT. Useful for loading a
// fresh scenario from scratch. All cores go back to their initial state.
func (k *Kernel) Reset() {
	for _, c := range k.Cores {
		c.CPU.Reset()
		c.CurrentPID = 0
		c.userInstructionsSinceFire = 0
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
	k.nextUserSlot = 0
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
	// Wipe any half-drained do_exit cascade. Without this, a Reset issued
	// mid-exit (e.g. user pressing LOAD between phase 2 and phase 3) would
	// leave dangling pendingKernelWork entries pointing at PIDs that no
	// longer exist; the next Step would dispatch ghost phases.
	k.PendingKernelWork = nil
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
	if k.nextUserSlot >= MaxProcesses {
		return 0, fmt.Errorf("maximum of %d processes supported", MaxProcesses)
	}
	if uint32(len(code)) > ProcCodeSize {
		return 0, fmt.Errorf("code of %d bytes exceeds ProcCodeSize=%d", len(code), ProcCodeSize)
	}

	pid := k.nextPID
	k.nextPID++
	slot := k.nextUserSlot
	k.nextUserSlot++

	codeBase := processCodeBase(slot)
	stackTop := processStackTop(slot)
	stackBase := stackTop + 0x10 - ProcStackSize

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
		Slot:     slot,
		Core:     -1,
		TGID:     pid,
		memSpace: k.newMemSpace(codeBase, uint32(len(code)), stackBase),
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
//
// Pedagogical note: when PendingKernelWork is non-empty, Step drains exactly
// ONE non-empty phase from the queue and returns — cores do not tick during
// the drain. This is the mechanism behind the click-per-do_exit-step view
// of process death: each phase (mark zombie, notify parent, schedule out,
// release_task, halt) lands on its own Step. The model is honest because
// in a real kernel do_exit runs with preemption disabled in long stretches;
// the simulator extends that invariant to all five phases for visibility.
func (k *Kernel) Step() ([]cpu.Event, error) {
	if len(k.PendingKernelWork) > 0 {
		return k.drainOneKernelWorkPhase(), nil
	}
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
		core.userInstructionsSinceFire = 0
	}
	// Capture pre-Step state AFTER any timer-IRQ push: that way the snapshot
	// reflects what CPU.Step will actually do this tick (dispatch an IRQ vs.
	// run execNext). Only ticks where execNext ran in user mode count toward
	// the next quantum.
	preMode := core.CPU.Mode
	wasHalted := core.CPU.Halted
	willDispatchIRQ := len(core.CPU.PendingIRQs) > 0 && core.CPU.FlagOn(cpu.FlagIF)
	// Snapshot the running PID *before* the step so EventInstruction (emitted
	// at fetch time, inside CPU.Step) can be stamped with the process that
	// actually executed it — even if a later handler in this same step
	// switches the core to a different process.
	runningPID := core.CurrentPID
	evs, err := core.CPU.Step()
	if !wasHalted && !willDispatchIRQ && preMode == cpu.ModeUser && runningPID != 0 {
		core.userInstructionsSinceFire++
	}
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
// given core. The quantum is measured in user-mode instructions actually
// executed since the last fire (see Core.userInstructionsSinceFire) so a
// handler that takes K kernel ticks does NOT shrink the next user slice by
// K — every fire gives the next user code TimerQuantum full instructions
// to run. The idle-core / kernel-mode short-circuit also prevents a halted
// core (post-exit with no Ready left) from being woken into running stale
// user EIPs past sys_exit.
func (k *Kernel) shouldFireTimer(core *Core) bool {
	if k.TimerQuantum <= 0 {
		return false
	}
	if core.CurrentPID == 0 || core.CPU.Mode != cpu.ModeUser {
		return false
	}
	return core.userInstructionsSinceFire >= uint64(k.TimerQuantum)
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
		// kill the offending process with a SIGSEGV-encoded status so a
		// waiting parent reaps a meaningful wstatus instead of zero.
		if source != "fault" {
			return nil
		}
		k.setFaultExitCode(core, SIGSEGV)
		return k.killAndNotify(core, "page_fault")
	case IRQGeneralProt:
		// #GP is raised by the CPU when user code executes a privileged
		// instruction (HLT in PL3), uses a bad segment, etc. Encode the
		// death as SIGILL so the parent's wstatus reflects the cause.
		if source != "fault" {
			return nil
		}
		k.setFaultExitCode(core, SIGILL)
		return k.killAndNotify(core, "general_protection")
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

// exitProcess is the shared death path used by sys_exit, page faults and #GP.
// It enqueues the five do_exit phases on PendingKernelWork and immediately
// drains phase 1 so the click that triggered the INT 0x80 (or fault) still
// shows the ZOMBIE transition. Phases 2-5 stay in the queue for the next
// Steps, which is what gives the alumno fine-grained control over the rest
// of the cascade — see Step()'s godoc for the pedagogical rationale.
//
// Phase mapping to do_exit (Robert Love, LKD chap. 3):
//
//   1. mark_zombie       → freeze CPU into PCB, ZOMBIE, reparent children
//   2. notify_parent     → SIGCHLD + wake parent (and/or init for orphans)
//   3. schedule_out      → final schedule(): context switch off the core
//   4. reap_cycle        → release_task in the parent's (or init's) context
//   5. halt              → terminal HALT if the core ends up genuinely idle
//
// PRECONDITION: the caller MUST have written pcb.ExitCode with the cause of
// death before invoking. For sys_exit that is the EBX arg; for page fault
// and #GP the handler stores the WIFSIGNALED status (raw signal number in
// the low 7 bits, e.g. SIGSEGV or SIGILL). Skipping this leaves the parent
// reaping garbage.
func (k *Kernel) exitProcess(core *Core, reason string) []cpu.Event {
	deadPID := core.CurrentPID
	if k.Processes[deadPID] == nil {
		// Degenerate case: nothing to bury. Halt the core defensively so
		// the run loop does not spin on a stale CurrentPID.
		core.CPU.Halted = true
		return nil
	}
	k.enqueueExitPhases(core, deadPID, reason)
	return k.drainOneKernelWorkPhase()
}

// killAndNotify is the death path used by fault handlers (page fault, #GP).
// Thin shim over exitProcess kept for callers that still address it by name.
// The fault handler must set pcb.ExitCode before invoking.
func (k *Kernel) killAndNotify(core *Core, reason string) []cpu.Event {
	return k.exitProcess(core, reason)
}

// markZombieAndReparent flips the running PCB to ZOMBIE and adopts its live
// children under init. Does NOT touch core.CurrentPID — the dispatch (or the
// halt) is finalizeExit's job, so notifyParentOfExit can run in between to
// wake the parent before the scheduler evaluates the ready queue.
//
// Before flipping the state we evict the dying PID from every sync queue it
// might still appear in (mutexes, semaphores, keyboard, ready). Otherwise a
// later unlock/signal could "wake" the zombie via wakeWaiter and switchTo
// would resurrect it — IRET would land at its saved EIP past sys_exit and
// execute garbage opcodes.
func (k *Kernel) markZombieAndReparent(core *Core, reason string) []cpu.Event {
	curPID := core.CurrentPID
	curPCB := k.Processes[curPID]
	if curPCB == nil {
		return nil
	}
	k.evictFromAllQueues(curPID)
	// Freeze the live CPU into the PCB before marking ZOMBIE so the frontend
	// sees the registers at the moment of death, not the stale snapshot from
	// process loading. The PCB is a faithful snapshot of core.CPU.R: after
	// sys_exit, EAX holds the syscall number (1), matching what the core
	// panel shows at the trap. The exit code lives in pcb.ExitCode and is
	// what EventProcessState/EventWaitOp carry to the frontend.
	k.saveCurrentToPCB(core)
	curPCB.State = StateZombie
	curPCB.BlockedOn = reason
	curPCB.Core = -1

	exitCode := curPCB.ExitCode
	out := []cpu.Event{
		cpu.EventPCBSave{CoreID: core.ID, PID: curPID, Regs: pcbRegsSnapshot(curPCB)},
		cpu.EventProcessState{
			PID:      curPID,
			State:    string(StateZombie),
			Reason:   reason,
			ExitCode: &exitCode,
		},
	}

	// Re-parent any surviving children to init so they remain reachable
	// in the process tree and so orphan zombies do not stick to a PCB
	// that is itself about to disappear. Live children land under init
	// as normal; orphan zombies queue on init.PendingReaps. Waking init
	// for those reaps is the job of the notify_parent phase — keeping the
	// wake out of this phase lets "the dying task finalizes" and "init is
	// woken to reap" land on separate Steps.
	if k.InitPID != 0 && curPID != k.InitPID && len(curPCB.Children) > 0 {
		init := k.Processes[k.InitPID]
		if init != nil {
			for _, childPID := range curPCB.Children {
				child := k.Processes[childPID]
				if child == nil {
					continue
				}
				child.ParentPID = k.InitPID
				child.WasReparentedToInit = true
				out = append(out, cpu.EventReparent{
					PID: childPID, OldParent: curPID, NewParent: k.InitPID,
				})
				init.Children = append(init.Children, childPID)
				if child.State == StateZombie {
					init.PendingReaps = append(init.PendingReaps, childPID)
				}
			}
			curPCB.Children = nil
		}
	}

	return out
}

// Note: the old finalizeExit was replaced by the discrete phases in
// kernel_work.go (schedule_out / reap_cycle / halt). enqueueExitPhases
// stages all five phases; Step() drains them one at a time so the alumno
// sees each transition on its own click. See kernel_work.go for the
// per-phase semantics.
