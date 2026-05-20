package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// Sync sub-syscall numbers dispatched via INT 0x81. The userland convention
// is:
//
//	EAX = sub-op (one of the constants below)
//	EBX = argument (mutex/sem ID for ops that target a specific object, or
//	      the initial count for sem_create)
//
// The result lands in EAX: a fresh ID for the create ops, or 0 on success
// for lock/unlock/wait/signal. ^uint32(0) means the requested object did not
// exist.
const (
	SyncMutexCreate uint32 = 0x10
	SyncMutexLock   uint32 = 0x11
	SyncMutexUnlock uint32 = 0x12
	SyncSemCreate   uint32 = 0x20
	SyncSemWait     uint32 = 0x21
	SyncSemSignal   uint32 = 0x22
)

// Mutex is a binary lock with FIFO waiters. Holder = -1 means the lock is
// free; otherwise it holds the PID of the owner. Real kernels also store the
// recursive count and priority data; we keep the demo minimal.
type Mutex struct {
	ID      int
	Holder  int
	Waiters []int
}

// Semaphore is a counting synchronization object. Count > 0 means the
// resource is available; sem_wait blocks once it hits 0.
type Semaphore struct {
	ID      int
	Count   int
	Waiters []int
}

// handleSync dispatches an INT 0x81 sync syscall on the given core. It mirrors
// handleSyscall but routes through the Mutex/Semaphore subsystem.
func (k *Kernel) handleSync(core *Core) []cpu.Event {
	op := core.CPU.R.EAX
	arg := core.CPU.R.EBX

	var out []cpu.Event
	var result uint32

	switch op {
	case SyncMutexCreate:
		id := k.mutexCreate()
		result = uint32(id)
		out = append(out, cpu.EventMutexOp{
			CoreID: core.ID, PID: core.CurrentPID, MutexID: id, Op: "create",
		})
	case SyncMutexLock:
		// Pre-set EAX so that if mutexLock has to block, blockOnSync
		// (which copies CPU regs into the PCB before switching to the
		// next core occupant) captures 0 — the value the user must
		// read once the lock eventually arrives. Modifying EAX AFTER
		// the switch would corrupt the next process's register file.
		core.CPU.R.EAX = 0
		evs, blocked, ok := k.mutexLock(core, int(arg))
		out = append(out, evs...)
		if !ok {
			result = ^uint32(0)
			break
		}
		if blocked {
			return out
		}
		result = 0
	case SyncMutexUnlock:
		evs, ok := k.mutexUnlock(core, int(arg))
		out = append(out, evs...)
		if !ok {
			result = ^uint32(0)
		}
	case SyncSemCreate:
		id := k.semCreate(int(arg))
		result = uint32(id)
		out = append(out, cpu.EventSemOp{
			CoreID: core.ID, PID: core.CurrentPID, SemID: id, Op: "create", Count: int(arg),
		})
	case SyncSemWait:
		// Same trick as SyncMutexLock: stage 0 in EAX so blockOnSync
		// snapshots the right success value into the blocking PCB.
		core.CPU.R.EAX = 0
		evs, blocked, ok := k.semWait(core, int(arg))
		out = append(out, evs...)
		if !ok {
			result = ^uint32(0)
			break
		}
		if blocked {
			return out
		}
		result = 0
	case SyncSemSignal:
		evs, ok := k.semSignal(core, int(arg))
		out = append(out, evs...)
		if !ok {
			result = ^uint32(0)
		}
	default:
		result = ^uint32(0)
	}

	core.CPU.R.EAX = result
	return out
}

// mutexCreate allocates a fresh mutex and returns its ID (1-indexed).
func (k *Kernel) mutexCreate() int {
	m := &Mutex{ID: len(k.Mutexes) + 1, Holder: -1}
	k.Mutexes = append(k.Mutexes, m)
	return m.ID
}

// mutexLock attempts to acquire the mutex on behalf of the running process.
// Returns (events, blocked, ok). ok=false means the mutex ID was invalid; the
// caller surfaces it as a ^uint32(0) result. When blocked=true the caller
// must leave EAX alone; the woken process will inherit EAX from its PCB on
// resume.
func (k *Kernel) mutexLock(core *Core, id int) ([]cpu.Event, bool, bool) {
	m := k.findMutex(id)
	if m == nil {
		return nil, false, false
	}
	pid := core.CurrentPID
	if m.Holder < 0 {
		m.Holder = pid
		return []cpu.Event{cpu.EventMutexOp{
			CoreID: core.ID, PID: pid, MutexID: id, Op: "acquired",
			Waiters: append([]int(nil), m.Waiters...),
		}}, false, true
	}

	m.Waiters = append(m.Waiters, pid)
	out := []cpu.Event{cpu.EventMutexOp{
		CoreID: core.ID, PID: pid, MutexID: id, Op: "waiting",
		Waiters: append([]int(nil), m.Waiters...),
	}}
	out = append(out, k.blockOnSync(core, mutexBlockedReason(id))...)
	return out, true, true
}

// mutexUnlock releases the mutex. If anyone was waiting, the first PID in the
// queue takes the lock and is moved to READY (handoff). Returns ok=false if
// the ID is unknown or the caller does not hold the lock.
func (k *Kernel) mutexUnlock(core *Core, id int) ([]cpu.Event, bool) {
	m := k.findMutex(id)
	if m == nil {
		return nil, false
	}
	pid := core.CurrentPID
	if m.Holder != pid {
		return nil, false
	}

	if len(m.Waiters) == 0 {
		m.Holder = -1
		return []cpu.Event{cpu.EventMutexOp{
			CoreID: core.ID, PID: pid, MutexID: id, Op: "released",
		}}, true
	}

	next := m.Waiters[0]
	m.Waiters = m.Waiters[1:]
	m.Holder = next
	wakeEvs := k.wakeWaiter(next, mutexBlockedReason(id))
	out := []cpu.Event{cpu.EventMutexOp{
		CoreID: core.ID, PID: pid, MutexID: id, Op: "handoff",
		NextPID: next, Waiters: append([]int(nil), m.Waiters...),
	}}
	out = append(out, wakeEvs...)
	return out, true
}

// semCreate allocates a fresh semaphore with the given initial count.
func (k *Kernel) semCreate(initCount int) int {
	if initCount < 0 {
		initCount = 0
	}
	s := &Semaphore{ID: len(k.Semaphores) + 1, Count: initCount}
	k.Semaphores = append(k.Semaphores, s)
	return s.ID
}

// semWait decrements the counter or blocks. Returns (events, blocked, ok).
func (k *Kernel) semWait(core *Core, id int) ([]cpu.Event, bool, bool) {
	s := k.findSem(id)
	if s == nil {
		return nil, false, false
	}
	pid := core.CurrentPID
	if s.Count > 0 {
		s.Count--
		return []cpu.Event{cpu.EventSemOp{
			CoreID: core.ID, PID: pid, SemID: id, Op: "wait_immediate",
			Count: s.Count, Waiters: append([]int(nil), s.Waiters...),
		}}, false, true
	}

	s.Waiters = append(s.Waiters, pid)
	out := []cpu.Event{cpu.EventSemOp{
		CoreID: core.ID, PID: pid, SemID: id, Op: "wait_blocked",
		Count: s.Count, Waiters: append([]int(nil), s.Waiters...),
	}}
	out = append(out, k.blockOnSync(core, semBlockedReason(id))...)
	return out, true, true
}

// semSignal increments the counter or wakes the first waiter (handoff).
func (k *Kernel) semSignal(core *Core, id int) ([]cpu.Event, bool) {
	s := k.findSem(id)
	if s == nil {
		return nil, false
	}
	pid := core.CurrentPID
	if len(s.Waiters) == 0 {
		s.Count++
		return []cpu.Event{cpu.EventSemOp{
			CoreID: core.ID, PID: pid, SemID: id, Op: "signal",
			Count: s.Count, Waiters: nil,
		}}, true
	}

	next := s.Waiters[0]
	s.Waiters = s.Waiters[1:]
	wakeEvs := k.wakeWaiter(next, semBlockedReason(id))
	out := []cpu.Event{cpu.EventSemOp{
		CoreID: core.ID, PID: pid, SemID: id, Op: "handoff",
		Count: s.Count, NextPID: next, Waiters: append([]int(nil), s.Waiters...),
	}}
	out = append(out, wakeEvs...)
	return out, true
}

// findMutex returns the mutex with the given ID or nil if it does not exist.
func (k *Kernel) findMutex(id int) *Mutex {
	if id < 1 || id > len(k.Mutexes) {
		return nil
	}
	return k.Mutexes[id-1]
}

// findSem returns the semaphore with the given ID or nil if it does not exist.
func (k *Kernel) findSem(id int) *Semaphore {
	if id < 1 || id > len(k.Semaphores) {
		return nil
	}
	return k.Semaphores[id-1]
}

// mutexBlockedReason and semBlockedReason build the BlockedOn string the
// frontend parses to know which object a process is waiting on.
func mutexBlockedReason(id int) string { return "mutex:" + itoa(id) }
func semBlockedReason(id int) string   { return "sem:" + itoa(id) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// blockOnSync parks the running process on the given reason and hands the
// core to the next READY PID if any. Mirrors blockCurrent for keyboard but
// emits no separate ConsoleOut and trusts the caller's mutex/sem event to
// document the cause.
func (k *Kernel) blockOnSync(core *Core, reason string) []cpu.Event {
	curPID := core.CurrentPID
	pcb := k.saveCurrentToPCB(core)
	if pcb == nil {
		return nil
	}
	pcb.State = StateBlocked
	pcb.BlockedOn = reason
	pcb.Core = -1

	out := []cpu.Event{
		cpu.EventPCBSave{PID: curPID, Regs: pcbRegsSnapshot(pcb)},
		cpu.EventProcessState{PID: curPID, State: string(StateBlocked), Reason: reason},
	}

	if nextPID, ok := k.Ready.Pop(); ok {
		out = append(out,
			cpu.EventSchedDecision{
				FromPID:    curPID,
				ToPID:      nextPID,
				ReadyQueue: k.Ready.Snapshot(),
			},
			cpu.EventCoreRelease{CoreID: core.ID, PID: curPID, Reason: reason},
		)
		out = append(out, k.switchTo(core, nextPID)...)
		return out
	}

	core.CPU.Halted = true
	core.CurrentPID = 0
	out = append(out,
		cpu.EventCoreRelease{CoreID: core.ID, PID: curPID, Reason: reason},
		cpu.EventHalt{PID: curPID},
	)
	return out
}

// evictFromAllQueues removes pid from every place where it might still be
// queued: mutex/sem waiters, keyboard blocked-reads, and the global ready
// queue. Called from killCurrent so a dying process never gets handed off a
// resource by a future unlock/signal.
func (k *Kernel) evictFromAllQueues(pid int) {
	for _, m := range k.Mutexes {
		m.Waiters = removeInt(m.Waiters, pid)
	}
	for _, s := range k.Semaphores {
		s.Waiters = removeInt(s.Waiters, pid)
	}
	for i, br := range k.blockedReads {
		if br.PID == pid {
			k.blockedReads = append(k.blockedReads[:i], k.blockedReads[i+1:]...)
			break
		}
	}
	k.Ready.Remove(pid)
}

func removeInt(xs []int, target int) []int {
	out := xs[:0]
	for _, v := range xs {
		if v != target {
			out = append(out, v)
		}
	}
	return out
}

// wakeWaiter moves a previously-blocked PID back to READY. If a core is sitting
// idle, the PID is handed straight to it via loadCPUFromPCB — switchTo cannot
// be used because the idle core has no kernel-stack frame to patch (the
// process state must come up fresh from the PCB). Otherwise the PID is queued
// for the scheduler to pick up later.
func (k *Kernel) wakeWaiter(pid int, reason string) []cpu.Event {
	pcb := k.Processes[pid]
	if pcb == nil {
		return nil
	}
	pcb.State = StateReady
	pcb.BlockedOn = ""

	out := []cpu.Event{
		cpu.EventProcessState{PID: pid, State: string(StateReady), Reason: "woken from " + reason},
	}

	if idle := k.firstIdleCore(); idle != nil {
		idle.CurrentPID = pid
		pcb.State = StateRun
		pcb.Core = idle.ID
		k.loadCPUFromPCB(idle, pcb)
		out = append(out,
			cpu.EventSchedDecision{FromPID: 0, ToPID: pid, ReadyQueue: k.Ready.Snapshot()},
			cpu.EventCoreAssign{CoreID: idle.ID, PID: pid},
			cpu.EventProcessState{PID: pid, State: string(StateRun)},
		)
		return out
	}
	k.Ready.Push(pid)
	return out
}
