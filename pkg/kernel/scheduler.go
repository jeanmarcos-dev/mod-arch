package kernel

import (
	"encoding/binary"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// saveCurrentToPCB captures the running process state of the given core into
// its PCB by reading the kernel-stack frame (EIP/CS/EFLAGS pushed by the HW
// on entry) plus the registers that live in the core's CPU.
//
// Requires the core to be in kernel mode after having entered via dispatchIRQ
// from user.
func (k *Kernel) saveCurrentToPCB(core *Core) *PCB {
	pcb := k.Processes[core.CurrentPID]
	if pcb == nil {
		return nil
	}
	pcb.Regs = core.CPU.R
	pcb.Regs.EIP = readKernelDWORD(core.CPU, core.CPU.R.ESP+0)
	pcb.Regs.CS = uint16(readKernelDWORD(core.CPU, core.CPU.R.ESP+4))
	pcb.Regs.EFLAGS = readKernelDWORD(core.CPU, core.CPU.R.ESP+8)
	pcb.SavedUserESP = core.CPU.SavedUserESP
	pcb.SavedUserSS = core.CPU.SavedUserSS
	return pcb
}

// scheduleNext is the routine invoked every time the timer fires IRQ 0x20 on
// a given core. It implements classic Round-Robin:
//
//  1. Save the current CPU state into the PCB of the RUN process (live CPU
//     regs plus EIP/CS/EFLAGS pushed by the HW to the kernel stack).
//
//  2. Move the current process from RUN to READY and push it onto the tail of
//     the shared ready queue.
//
//  3. Pop the next process from the head of the ready queue. If the queue is
//     empty, no switch happens and the current process keeps running.
//
//  4. Restore the new process's CPU regs and OVERWRITE the kernel-stack frame
//     with its EIP/CS/EFLAGS. When the handler executes IRET, the HW pops the
//     patched frame and jumps to the new EIP unaware of the swap.
//
// Emits events so the frontend can visualize the transition (PCBSave,
// SchedDecision, PCBRestore, ProcessState).
func (k *Kernel) scheduleNext(core *Core) []cpu.Event {
	// With zero or one process there is nothing to schedule on this core.
	if core.CurrentPID == 0 || k.Ready.Len() == 0 {
		return nil
	}

	curPID := core.CurrentPID
	curPCB := k.Processes[curPID]
	if curPCB == nil {
		return nil
	}

	var out []cpu.Event

	// 1. Save outgoing PCB.
	// On dispatch, the HW pushed EFLAGS/CS/EIP to the kernel stack. We read
	// them here so the PCB captures the user state; the real handler bytes
	// (nop + iret) do not touch them.
	// Kernel stack layout at this point (ESP grows downward):
	//   ESP+0: saved EIP
	//   ESP+4: saved CS
	//   ESP+8: saved EFLAGS
	savedEIP := readKernelDWORD(core.CPU, core.CPU.R.ESP+0)
	savedCS := readKernelDWORD(core.CPU, core.CPU.R.ESP+4)
	savedEFLAGS := readKernelDWORD(core.CPU, core.CPU.R.ESP+8)

	curPCB.Regs = core.CPU.R
	curPCB.Regs.EIP = savedEIP
	curPCB.Regs.CS = uint16(savedCS)
	curPCB.Regs.EFLAGS = savedEFLAGS
	curPCB.SavedUserESP = core.CPU.SavedUserESP
	curPCB.SavedUserSS = core.CPU.SavedUserSS
	curPCB.State = StateReady
	curPCB.Core = -1

	out = append(out, cpu.EventPCBSave{CoreID: core.ID, PID: curPID, Regs: pcbRegsSnapshot(curPCB)})
	out = append(out, cpu.EventProcessState{PID: curPID, State: string(StateReady), Reason: "preempted by timer"})

	// 2. Push current to the tail of the ready queue.
	k.Ready.Push(curPID)

	// 3. Pop the next from the head. popNextRunnable skips init so the
	// timer can never accidentally bind PID 1 to a core; init only runs
	// through DispatchIdleCores's runInitReapCycle.
	nextPID, ok := k.popNextRunnable()
	if !ok {
		return out
	}
	nextPCB := k.Processes[nextPID]
	if nextPCB == nil {
		// Inconsistent state: bail out emitting only what we have.
		return out
	}

	out = append(out, cpu.EventSchedDecision{
		FromPID:    curPID,
		ToPID:      nextPID,
		Reason:     "timer",
		ReadyQueue: k.Ready.Snapshot(),
	})

	// 4. Restore the incoming process's user-visible regs into the CPU.
	// We keep ESP/CS pointing at the kernel stack: the handler has not
	// finished yet. Only regs the HW does not touch are restored (EAX-EBP
	// plus SavedUserESP/SS).
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

	// Patch the kernel-stack frame so IRET will jump to the new EIP.
	writeKernelDWORD(core.CPU, core.CPU.R.ESP+0, nextPCB.Regs.EIP)
	writeKernelDWORD(core.CPU, core.CPU.R.ESP+4, uint32(nextPCB.Regs.CS))
	writeKernelDWORD(core.CPU, core.CPU.R.ESP+8, nextPCB.Regs.EFLAGS)

	out = append(out, cpu.EventPCBRestore{CoreID: core.ID, PID: nextPID, Regs: pcbRegsSnapshot(nextPCB)})

	nextPCB.State = StateRun
	nextPCB.Core = core.ID
	core.CurrentPID = nextPID
	out = append(out,
		cpu.EventCoreRelease{CoreID: core.ID, PID: curPID, Reason: "preempted"},
		cpu.EventCoreAssign{CoreID: core.ID, PID: nextPID},
		cpu.EventProcessState{PID: nextPID, State: string(StateRun)},
	)

	return out
}

// loadCPUFromPCB initializes a core from the state stored in a PCB. Only used
// at bootstrap (the first process on each core); regular switching happens via
// scheduleNext.
func (k *Kernel) loadCPUFromPCB(core *Core, p *PCB) {
	c := core.CPU
	c.R = p.Regs
	c.R.ESP = p.SavedUserESP
	c.R.SS = p.SavedUserSS
	c.Mode = cpu.ModeUser
	c.SavedUserESP = p.SavedUserESP
	c.SavedUserSS = p.SavedUserSS
	// Non-boot cores started halted in NewMulti; wake them up now that they
	// have something to run.
	c.Halted = false
}

// switchTo prepares the given core's CPU and kernel stack so the next IRET
// lands in the given PID: regs restored from the PCB, kernel-stack frame
// patched with the target EIP/CS/EFLAGS. The caller is responsible for the
// bookkeeping of the outgoing PID (state, queue). Returns the events to
// propagate.
func (k *Kernel) switchTo(core *Core, pid int) []cpu.Event {
	pcb := k.Processes[pid]
	if pcb == nil {
		return nil
	}
	core.CPU.R.EAX = pcb.Regs.EAX
	core.CPU.R.EBX = pcb.Regs.EBX
	core.CPU.R.ECX = pcb.Regs.ECX
	core.CPU.R.EDX = pcb.Regs.EDX
	core.CPU.R.ESI = pcb.Regs.ESI
	core.CPU.R.EDI = pcb.Regs.EDI
	core.CPU.R.EBP = pcb.Regs.EBP
	core.CPU.R.DS = pcb.Regs.DS
	core.CPU.SavedUserESP = pcb.SavedUserESP
	core.CPU.SavedUserSS = pcb.SavedUserSS

	// Patch the top kernel-stack frame so IRET jumps to the user EIP.
	writeKernelDWORD(core.CPU, core.CPU.R.ESP+0, pcb.Regs.EIP)
	writeKernelDWORD(core.CPU, core.CPU.R.ESP+4, uint32(pcb.Regs.CS))
	writeKernelDWORD(core.CPU, core.CPU.R.ESP+8, pcb.Regs.EFLAGS)

	pcb.State = StateRun
	pcb.Core = core.ID
	core.CurrentPID = pid
	return []cpu.Event{
		cpu.EventPCBRestore{CoreID: core.ID, PID: pid, Regs: pcbRegsSnapshot(pcb)},
		cpu.EventCoreAssign{CoreID: core.ID, PID: pid},
		cpu.EventProcessState{PID: pid, State: string(StateRun)},
	}
}

func readKernelDWORD(c *cpu.CPU, addr uint32) uint32 {
	return binary.LittleEndian.Uint32(c.Mem.Bytes[addr : addr+4])
}

func writeKernelDWORD(c *cpu.CPU, addr uint32, v uint32) {
	binary.LittleEndian.PutUint32(c.Mem.Bytes[addr:addr+4], v)
}

// pcbRegsSnapshot serializes PCB registers for inclusion in events.
func pcbRegsSnapshot(p *PCB) map[string]uint32 {
	return map[string]uint32{
		"EAX": p.Regs.EAX, "EBX": p.Regs.EBX, "ECX": p.Regs.ECX, "EDX": p.Regs.EDX,
		"ESI": p.Regs.ESI, "EDI": p.Regs.EDI, "EBP": p.Regs.EBP,
		"EIP": p.Regs.EIP, "EFLAGS": p.Regs.EFLAGS,
		"ESP": p.SavedUserESP,
	}
}

// DispatchIdleCores assigns ready processes to idle cores in round-robin
// fashion (lowest core ID first). For each (core, pid) pairing it flips the
// PCB to RUN, loads the CPU from the PCB, and emits PCBRestore + SchedDecision
// + CoreAssign + ProcessState{RUN} so both the READY→RUN transition and the
// shrinking ready queue are visible in the event log. Idempotent: if every
// core already has a CurrentPID or the ready queue is empty, returns nil.
//
// Two extra responsibilities, both tied to the deferred-reap model:
//
//   - If the popped PCB has PendingReaps, the dispatcher drains them
//     BEFORE settling the PCB onto a core. The reap events are emitted in
//     this dispatch step so the alumno sees ZOMBIE on one tick and the
//     reap on the next.
//
//   - If that PCB is init, the dispatcher does not actually load init onto
//     the core. Init briefly transitions RUN → BLOCKED("wait()") inside the
//     dispatch (emit the two state events so the wake is visible), then
//     the loop tries to fill the same core slot with the next ready PID.
//     This mirrors a real init that wakes on SIGCHLD, runs waitpid until
//     ECHILD, and goes back to sleep without ever leaving its main loop.
func (k *Kernel) DispatchIdleCores() []cpu.Event {
	var out []cpu.Event

	// Eviction pass: any core currently bound to the swapper must yield as
	// soon as a real user proc (anything not init) joins the ready queue.
	// Done before the assignment loop so firstIdleCore picks the freshly
	// vacated core, and so the frontend gets a paired Release→Assign sequence
	// for the swap rather than a silent CurrentPID overwrite.
	if k.IdleTaskEnabled && k.hasNonInitReady() {
		for _, c := range k.Cores {
			if c.RunningIdle {
				out = append(out, cpu.EventCoreRelease{
					CoreID: c.ID, PID: 0, Reason: "idle preempted",
				})
				c.RunningIdle = false
				// Park the core in the legacy "no proc" sentinel so the
				// assignment loop below treats it as a fresh idle slot.
				c.CurrentPID = 0
				c.CPU.Halted = true
			}
		}
	}

	for {
		idle := k.firstIdleCore()
		if idle == nil {
			break
		}
		pid, ok := k.Ready.Pop()
		if !ok {
			break
		}
		pcb := k.Processes[pid]
		if pcb == nil {
			continue
		}

		if k.InitPID != 0 && pid == k.InitPID {
			out = append(out, k.runInitReapCycle(idle, pcb)...)
			// Try to fill this core with the next ready PID; init does
			// not occupy a core after reaping.
			continue
		}

		out = append(out,
			cpu.EventSchedDecision{FromPID: 0, ToPID: pid, Reason: "dispatch", ReadyQueue: k.Ready.Snapshot()},
		)
		out = append(out, k.consumePendingReaps(pcb)...)

		idle.CurrentPID = pid
		pcb.State = StateRun
		pcb.Core = idle.ID
		k.loadCPUFromPCB(idle, pcb)
		out = append(out,
			cpu.EventPCBRestore{CoreID: idle.ID, PID: pid, Regs: pcbRegsSnapshot(pcb)},
			cpu.EventCoreAssign{CoreID: idle.ID, PID: pid},
			cpu.EventProcessState{PID: pid, State: string(StateRun)},
		)
	}

	// Idle fallback pass: every core that still has no work gets the
	// swapper. Guarded by RunningIdle so successive Step calls do not
	// re-emit EventCoreIdle or replay loadCPUFromPCB — the swapper that is
	// already halted waiting for an IRQ must stay there.
	if k.IdleTaskEnabled && k.Processes[0] != nil {
		for _, c := range k.Cores {
			if c.CurrentPID == 0 && !c.RunningIdle {
				out = append(out, k.bindIdleToCore(c)...)
			}
		}
	}

	return out
}

// hasNonInitReady reports whether the ready queue has any PID other than
// init. Used by the eviction pass: the swapper must yield to a real user
// proc, but not to init (init never occupies a core, it just gets reaped
// in-kernel and goes back to sleep).
func (k *Kernel) hasNonInitReady() bool {
	for _, pid := range k.Ready.Snapshot() {
		if pid != k.InitPID {
			return true
		}
	}
	return false
}

// runInitReapCycle is the dispatch-time choreography for init: a visible
// READY → RUN, drain every PendingReap as a single EventInitReap, and back
// to BLOCKED("wait()"). Init never actually executes its own code — the
// cycle is implemented entirely in the kernel. EventInitReap is what tells
// the frontend that this whole sequence is kernel-emulated, so the alumno
// sees a label like "init reaps (kernel-emulated): P{n}" instead of
// guessing why init's registers mutate without a PCB restore.
//
// The idle core passed in is NOT bound to init at the end; the caller
// continues its dispatch loop and assigns the slot to the next eligible
// READY PID.
//
// init.Regs.EAX is stamped with the first reaped PID for consistency with
// the regular consumePendingReaps path; with N orphans in one cycle only
// the first PID is preserved — that is a known limitation flagged in the
// implementation plan and is harmless because init does not occupy a core.
func (k *Kernel) runInitReapCycle(idle *Core, init *PCB) []cpu.Event {
	out := []cpu.Event{
		cpu.EventProcessState{PID: init.PID, State: string(StateRun), Reason: "SIGCHLD"},
	}

	reaps := make([]cpu.ReapDetail, 0, len(init.PendingReaps))
	first := true
	for _, cpid := range init.PendingReaps {
		child := k.Processes[cpid]
		if child == nil {
			continue
		}
		detail := k.reapOne(init, child)
		if first {
			init.Regs.EAX = uint32(detail.ChildPID)
			first = false
		}
		reaps = append(reaps, detail)
	}
	init.PendingReaps = nil

	out = append(out, cpu.EventInitReap{
		InitPID:    init.PID,
		Reaps:      reaps,
		ReadyQueue: k.Ready.Snapshot(),
	})

	init.State = StateBlocked
	init.BlockedOn = initBlockedReason
	init.Core = -1
	out = append(out, cpu.EventProcessState{
		PID: init.PID, State: string(StateBlocked), Reason: initBlockedReason,
	})
	// Idle core stays idle — the outer loop in DispatchIdleCores will
	// reuse it for the next ready PID, if any.
	_ = idle
	return out
}
