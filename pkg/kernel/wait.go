package kernel

import (
	"fmt"
	"strings"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// waitpidBlockedReason formats the BlockedOn label so the UI can tell at a
// glance whether a parent parked in sys_waitpid is waiting for any child
// ("wait(any)") or for a specific PID ("wait(P3)"). The prefix "wait(" is
// what notifyParentOfExit greps for to route reaps; init reuses the
// "wait(any)" label so all reaper-style waits read the same way.
func waitpidBlockedReason(target int) string {
	if target <= 0 {
		return "wait(any)"
	}
	return fmt.Sprintf("wait(P%d)", target)
}

// waitpidBlockedPrefix identifies any BlockedOn string produced by
// waitpidBlockedReason or initBlockedReason. notifyParentOfExit uses it to
// route reaps through the wait branch regardless of which specific target
// the parent is waiting on.
const waitpidBlockedPrefix = "wait("

// sysWaitpid implements sys_waitpid(pid, status_addr, options).
//
//	EBX = pid     (-1 = any child, >0 = specific PID)
//	ECX = status_addr (user pointer; 0 means "discard the status")
//	EDX = options (ignored — WNOHANG is not modelled)
//
// Returns (result, events, blocked):
//
//   - result  = reaped PID, or ^uint32(0) on ECHILD (no eligible children).
//   - blocked = true when no zombie is ready and the parent went BLOCKED.
//
// When blocked the caller must NOT write EAX from result; the wake path
// (sys_exit reaping the child) stamps the parent's PCB.Regs.EAX with the
// reaped PID before transitioning it to READY.
func (k *Kernel) sysWaitpid(core *Core, target, statusAddr uint32) (uint32, []cpu.Event, bool) {
	parentPID := core.CurrentPID
	parent := k.Processes[parentPID]
	if parent == nil || len(parent.Children) == 0 {
		return ^uint32(0), nil, false
	}

	want := int(int32(target))

	// Reap any zombie that matches the request.
	for _, cpid := range parent.Children {
		child := k.Processes[cpid]
		if child == nil || child.State != StateZombie {
			continue
		}
		if want > 0 && cpid != want {
			continue
		}
		reapedPID, reapEvs := k.reapZombie(parent, child, statusAddr)
		return reapedPID, reapEvs, false
	}

	// No zombie ready: park the parent until a child exits. The reason
	// string carries the wait target so the UI distinguishes
	// "wait(any)" from "wait(P3)" without us having to expose Waiter
	// separately to the frontend.
	parent.Waiter = want
	out := []cpu.Event{cpu.EventWaitOp{PID: parentPID, ChildPID: want, Op: "call"}}
	out = append(out, k.blockOnSync(core, waitpidBlockedReason(want))...)
	return 0, out, true
}

// reapZombie removes a zombie child from the process table and from the
// parent's Children list, then writes the exit status to user memory if
// statusAddr is non-zero. Returns the reaped PID plus the events that
// describe the reaping.
func (k *Kernel) reapZombie(parent, child *PCB, statusAddr uint32) (uint32, []cpu.Event) {
	exitCode := child.ExitCode

	if statusAddr != 0 {
		// Real Linux packs the exit status with WIFEXITED bits; we
		// store the raw code to keep the demo readable.
		if _, ok := userBufferFault(statusAddr, 4); ok {
			k.Mem.Bytes[statusAddr+0] = byte(exitCode)
			k.Mem.Bytes[statusAddr+1] = byte(exitCode >> 8)
			k.Mem.Bytes[statusAddr+2] = byte(exitCode >> 16)
			k.Mem.Bytes[statusAddr+3] = byte(exitCode >> 24)
		}
	}

	delete(k.Processes, child.PID)
	parent.Children = removePID(parent.Children, child.PID)

	return uint32(child.PID), []cpu.Event{cpu.EventWaitOp{
		PID: parent.PID, ChildPID: child.PID, Op: "reap", ExitCode: exitCode,
	}}
}

// removePID returns slice with the first occurrence of pid removed. If pid
// is not in the slice the original is returned unchanged.
func removePID(s []int, pid int) []int {
	for i, v := range s {
		if v == pid {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

// notifyParentOfExit is called from sys_exit AFTER the child has been marked
// ZOMBIE. The dying child does NOT get reaped synchronously here — instead
// the child PID is queued on parent.PendingReaps and the parent is moved to
// READY through the ready queue (no idle-core fast path). The reap completes
// only when the scheduler next dispatches the parent, at which point the
// dispatcher drains PendingReaps and emits the reap event. This matches the
// Unix model where SIGCHLD makes the parent runnable but the wait() syscall
// returns only when the kernel actually schedules the parent.
//
// Three cases distinguished by parent's BlockedOn:
//
//   - init (BlockedOn="wait(any)"): queue the orphan and put init back on
//     the ready queue. Init briefly transitions to RUN at dispatch time,
//     reaps every queued zombie, and re-blocks in wait(any) — the dispatcher
//     handles this loop because init never executes its own user code.
//
//   - "wait(any)" / "wait(P{n})": a regular parent parked in sys_waitpid.
//     Same deferred model: queue the child, wake parent to READY. On
//     dispatch, parent wakes with EAX = reaped PID. The label includes the
//     target so the UI shows "wait(P3)" instead of a generic "wait".
//
//   - "vfork": the parent is unblocked but the zombie is preserved so the
//     parent can still call waitpid later to read ExitCode. No PendingReap
//     is queued.
func (k *Kernel) notifyParentOfExit(child *PCB) []cpu.Event {
	if child.ParentPID == 0 {
		return nil
	}
	parent := k.Processes[child.ParentPID]
	if parent == nil {
		return nil
	}

	if k.InitPID != 0 && parent.PID == k.InitPID {
		return k.queueReapAndWake(parent, child, initBlockedReason)
	}

	if parent.State != StateBlocked {
		return nil
	}

	switch {
	case strings.HasPrefix(parent.BlockedOn, waitpidBlockedPrefix):
		if parent.Waiter > 0 && parent.Waiter != child.PID {
			return nil
		}
		reason := parent.BlockedOn
		parent.Waiter = 0
		return k.queueReapAndWake(parent, child, reason)

	case parent.BlockedOn == blockedOnVfork:
		// Only THE vforked child wakes its parent. Other children of
		// the same parent that happen to exit while we are waiting
		// must not unblock it. vfork does not reap — that is what
		// distinguishes it from wait — so we just wake.
		if child.VforkParent != parent.PID {
			return nil
		}
		return k.wakeToQueue(parent.PID, blockedOnVfork)
	}
	return nil
}

// queueReapAndWake stages a deferred reap for the given parent/child pair.
// The child PID is appended to parent.PendingReaps and the parent is moved
// from BLOCKED to READY via the ready queue (skipping wakeWaiter's idle-core
// fast path so the READY→RUN transition costs at least one scheduler
// dispatch cycle, which is what makes the zombie state visible).
//
// reason is the BlockedOn value before the wake, used so the event carries
// an honest "woken from X" string ("wait(any)", "wait(P3)", …).
func (k *Kernel) queueReapAndWake(parent, child *PCB, reason string) []cpu.Event {
	parent.PendingReaps = append(parent.PendingReaps, child.PID)
	return k.wakeToQueue(parent.PID, reason)
}

// wakeToQueue moves a BLOCKED PCB to READY and pushes it onto the ready
// queue. Unlike wakeWaiter (used by sync.go), this path never hands the
// process directly to an idle core. Wait/vfork wakes go through the queue
// so the READY state is observable for at least one dispatch cycle —
// pedagogically important so the alumno sees the parent wait its turn
// instead of teleporting from BLOCKED to RUN.
//
// Emits an EventWake alongside the state transition. EventWake carries the
// updated ready-queue snapshot so the WS client keeps its mirror in sync
// without conflating wakes (BLOCKED → READY) with scheduling decisions
// (which now travel through EventSchedDecision and EventInitReap only).
//
// Idempotent: if the PCB is already READY (woken earlier in the same Step
// and still queued), the function returns nil instead of double-queueing.
// That guards against scenarios where markZombieAndReparent has already
// enqueued init for orphan reaping and notifyParentOfExit then tries to
// enqueue it again.
func (k *Kernel) wakeToQueue(pid int, reason string) []cpu.Event {
	pcb := k.Processes[pid]
	if pcb == nil {
		return nil
	}
	if pcb.State == StateReady {
		return nil
	}
	pcb.State = StateReady
	pcb.BlockedOn = ""
	k.Ready.Push(pid)
	return []cpu.Event{
		cpu.EventProcessState{PID: pid, State: string(StateReady), Reason: "woken from " + reason},
		cpu.EventWake{
			PID:        pid,
			FromReason: reason,
			ReadyQueue: k.Ready.Snapshot(),
		},
	}
}

// reapOne removes the zombie child from the process table and from the
// parent's Children list, returning the bookkeeping detail without emitting
// any event. Callers decide how to surface the reap: regular parents wrap
// the detail in EventWaitOp{Op:"reap"}; init folds every detail of the
// cycle into a single EventInitReap.
func (k *Kernel) reapOne(parent, child *PCB) cpu.ReapDetail {
	detail := cpu.ReapDetail{
		ChildPID:   child.PID,
		ExitCode:   child.ExitCode,
		Reparented: child.WasReparentedToInit,
	}
	delete(k.Processes, child.PID)
	parent.Children = removePID(parent.Children, child.PID)
	return detail
}

// consumePendingReaps drains every queued reap on the given PCB, emitting
// one EventWaitOp{Op:"reap"} per zombie collected. The EAX of the first
// reap is stamped into Regs.EAX so the resuming sys_waitpid returns the
// reaped PID; subsequent reaps in the same dispatch leave EAX alone (a real
// parent must loop on wait() to collect multiple zombies). Used by the
// regular waitpid path; init's reaper uses runInitReapCycle which folds the
// per-zombie details into a single EventInitReap instead of emitting
// EventWaitOp.
//
// Returns the slice of events. Safe to call when PendingReaps is empty —
// returns nil.
func (k *Kernel) consumePendingReaps(parent *PCB) []cpu.Event {
	if len(parent.PendingReaps) == 0 {
		return nil
	}
	var out []cpu.Event
	first := true
	for _, cpid := range parent.PendingReaps {
		child := k.Processes[cpid]
		if child == nil {
			continue
		}
		detail := k.reapOne(parent, child)
		if first {
			parent.Regs.EAX = uint32(detail.ChildPID)
			first = false
		}
		out = append(out, cpu.EventWaitOp{
			PID:      parent.PID,
			ChildPID: detail.ChildPID,
			Op:       "reap",
			ExitCode: detail.ExitCode,
		})
	}
	parent.PendingReaps = nil
	return out
}
