package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// kernelWorkKind identifies the discrete pieces of work the kernel defers
// across Steps to give the user fine-grained control over multi-step
// procedures. The five exit phases below map 1:1 to the canonical steps of
// do_exit (Robert Love, LKD chap. 3): mark task DEAD + reparent, notify
// parent (SIGCHLD + wake), final schedule(), release_task in the parent's
// context, and the terminal halt when nothing else is runnable. Splitting
// these into separate Steps lets the alumno see each transition on its own
// click instead of watching 7-12 events stream by atomically.
type kernelWorkKind int

const (
	workMarkZombie kernelWorkKind = iota + 1
	workNotifyParent
	workScheduleOut
	workReapCycle
	workHalt
)

func (k kernelWorkKind) String() string {
	switch k {
	case workMarkZombie:
		return "mark_zombie"
	case workNotifyParent:
		return "notify_parent"
	case workScheduleOut:
		return "schedule_out"
	case workReapCycle:
		return "reap_cycle"
	case workHalt:
		return "halt"
	}
	return "?"
}

// pendingKernelWork is a single deferred phase queued on the kernel. Plain
// data so Reset() can wipe the queue safely (no dangling closures into freed
// PCBs) and tests can inspect what is pending mid-cascade.
type pendingKernelWork struct {
	kind    kernelWorkKind
	coreID  int
	deadPID int
	reason  string
}

// enqueueExitPhases stages the five do_exit phases for the dying PID on the
// given core. Callers (sys_exit, page fault handler, #GP handler) hand off
// to this and then immediately drain phase 1 inside the same Step, so the
// click that triggered INT 0x80 still surfaces the ZOMBIE transition.
func (k *Kernel) enqueueExitPhases(core *Core, deadPID int, reason string) {
	base := pendingKernelWork{coreID: core.ID, deadPID: deadPID, reason: reason}
	for _, kind := range [...]kernelWorkKind{
		workMarkZombie,
		workNotifyParent,
		workScheduleOut,
		workReapCycle,
		workHalt,
	} {
		ph := base
		ph.kind = kind
		k.PendingKernelWork = append(k.PendingKernelWork, ph)
	}
}

// drainOneKernelWorkPhase pops phases until one emits at least one event,
// returning that phase's events. Empty phases (e.g. notify_parent when the
// dying task had no parent sleeping in wait()) are collapsed transparently
// so the user never sees a Step that emits nothing.
func (k *Kernel) drainOneKernelWorkPhase() []cpu.Event {
	for len(k.PendingKernelWork) > 0 {
		ph := k.PendingKernelWork[0]
		k.PendingKernelWork = k.PendingKernelWork[1:]
		evs := k.runKernelWorkPhase(ph)
		if len(evs) > 0 {
			core := k.Cores[ph.coreID]
			return stampKernelContext(evs, core.ID, core.CPU.Tick)
		}
	}
	return nil
}

func (k *Kernel) runKernelWorkPhase(ph pendingKernelWork) []cpu.Event {
	switch ph.kind {
	case workMarkZombie:
		return k.doMarkZombiePhase(ph)
	case workNotifyParent:
		return k.doNotifyParentPhase(ph)
	case workScheduleOut:
		return k.doScheduleOutPhase(ph)
	case workReapCycle:
		return k.doReapCyclePhase(ph)
	case workHalt:
		return k.doHaltPhase(ph)
	}
	return nil
}

// doMarkZombiePhase is phase 1: snapshot the CPU into the PCB, mark the
// process ZOMBIE, and re-parent any surviving children to init. Orphan
// zombies are appended to init.PendingReaps but init is NOT woken here —
// that wake is centralized in phase 2 so "the dying task finalizes" and
// "init is woken" land on separate Steps.
func (k *Kernel) doMarkZombiePhase(ph pendingKernelWork) []cpu.Event {
	core := k.Cores[ph.coreID]
	return k.markZombieAndReparent(core, ph.reason)
}

// doNotifyParentPhase is phase 2: wake the parent if it was sleeping in
// wait()/waitpid()/vfork, AND wake init if phase 1 added orphan zombies to
// its PendingReaps. Both wakes route through wakeToQueue, which is
// idempotent — if the dying task was a direct child of init AND brought
// orphan grandchildren along, both code paths converge on the same wake
// and only one EventWake is emitted.
func (k *Kernel) doNotifyParentPhase(ph pendingKernelWork) []cpu.Event {
	var out []cpu.Event
	if dead := k.Processes[ph.deadPID]; dead != nil {
		out = append(out, k.notifyParentOfExit(dead)...)
	}
	if k.InitPID != 0 {
		if init := k.Processes[k.InitPID]; init != nil &&
			init.State == StateBlocked && len(init.PendingReaps) > 0 {
			out = append(out, k.wakeToQueue(k.InitPID, initBlockedReason)...)
		}
	}
	return out
}

// doScheduleOutPhase is phase 3: the final schedule() of the moribundo.
// Pop a non-init runnable and switch to it. If nothing runnable, just emit
// CoreRelease and leave the core idle — phases 4/5 will pick up from there.
//
// EventCoreRelease is emitted BEFORE the PCBRestore of the incoming task
// so the log reads "moribundo leaves, sibling takes over" — matching the
// do_exit ordering where release runs before the new task resumes.
func (k *Kernel) doScheduleOutPhase(ph pendingKernelWork) []cpu.Event {
	core := k.Cores[ph.coreID]
	deadPID := ph.deadPID
	reason := ph.reason

	if nextPID, ok := k.popNextRunnable(); ok {
		if nextPCB := k.Processes[nextPID]; nextPCB != nil {
			out := []cpu.Event{
				cpu.EventSchedDecision{
					FromPID:    deadPID,
					ToPID:      nextPID,
					Reason:     reason,
					ReadyQueue: k.Ready.Snapshot(),
				},
				cpu.EventCoreRelease{CoreID: core.ID, PID: deadPID, Reason: reason},
			}
			out = append(out, k.switchTo(core, nextPID)...)
			return out
		}
	}

	core.CurrentPID = 0
	return []cpu.Event{
		cpu.EventCoreRelease{CoreID: core.ID, PID: deadPID, Reason: reason},
	}
}

// doReapCyclePhase is phase 4: release_task, in the parent's context.
//
// Path A — phase 3 dispatched a regular parent with pending reaps: drain
// them with consumePendingReaps (which stamps Regs.EAX with the reaped PID)
// and mirror the updated EAX back to the live CPU so the resuming syscall
// returns the right value.
//
// Path B — phase 3 left the core idle and init is queued with pending
// reaps: pull init out of the ready queue and run runInitReapCycle inline.
// Init re-blocks at the end, so the core stays idle for phase 5 to halt.
//
// Returns nil when neither path applies, letting the next phase run on the
// same click via the transparent-collapse loop.
func (k *Kernel) doReapCyclePhase(ph pendingKernelWork) []cpu.Event {
	core := k.Cores[ph.coreID]

	if core.CurrentPID != 0 {
		running := k.Processes[core.CurrentPID]
		if running == nil || len(running.PendingReaps) == 0 {
			return nil
		}
		evs := k.consumePendingReaps(running)
		core.CPU.R.EAX = running.Regs.EAX
		return evs
	}

	if k.InitPID != 0 {
		if init := k.Processes[k.InitPID]; init != nil &&
			init.State == StateReady && len(init.PendingReaps) > 0 {
			if k.Ready.Remove(k.InitPID) {
				return k.runInitReapCycle(core, init)
			}
		}
	}
	return nil
}

// doHaltPhase is phase 5: terminal HALT of the core that ran the moribundo.
// Only fires if the core ends up genuinely idle (phase 3 found no sibling,
// phase 4 either did the init reap and init re-blocked or had nothing to
// do). Collapses transparently when a sibling is now running on the core.
func (k *Kernel) doHaltPhase(ph pendingKernelWork) []cpu.Event {
	core := k.Cores[ph.coreID]
	if core.CurrentPID != 0 {
		return nil
	}
	if core.CPU.Halted {
		return nil
	}
	core.CPU.Halted = true
	return []cpu.Event{cpu.EventHalt{CoreID: core.ID, PID: ph.deadPID}}
}
