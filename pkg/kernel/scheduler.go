package kernel

import (
	"encoding/binary"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// saveCurrentToPCB captures the current process state into its PCB by reading
// the kernel-stack frame (EIP/CS/EFLAGS pushed by the HW on entry) plus the
// registers that live in the CPU.
//
// Requires the CPU to be in kernel mode after having entered via dispatchIRQ
// from user.
func (k *Kernel) saveCurrentToPCB() *PCB {
	pcb := k.Processes[k.CurrentPID]
	if pcb == nil {
		return nil
	}
	pcb.Regs = k.CPU.R
	pcb.Regs.EIP = readKernelDWORD(k.CPU, k.CPU.R.ESP+0)
	pcb.Regs.CS = uint16(readKernelDWORD(k.CPU, k.CPU.R.ESP+4))
	pcb.Regs.EFLAGS = readKernelDWORD(k.CPU, k.CPU.R.ESP+8)
	pcb.SavedUserESP = k.CPU.SavedUserESP
	pcb.SavedUserSS = k.CPU.SavedUserSS
	return pcb
}

// scheduleNext is the routine invoked every time the timer fires IRQ 0x20 from
// a user process. It implements classic Round-Robin:
//
//  1. Save the current CPU state into the PCB of the RUN process (live CPU
//     regs plus EIP/CS/EFLAGS pushed by the HW to the kernel stack).
//
//  2. Move the current process from RUN to READY and push it onto the tail of
//     the ready queue.
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
func (k *Kernel) scheduleNext() []cpu.Event {
	// With zero or one process there is nothing to schedule.
	if k.CurrentPID == 0 || len(k.ReadyQueue) == 0 {
		return nil
	}

	curPID := k.CurrentPID
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
	savedEIP := readKernelDWORD(k.CPU, k.CPU.R.ESP+0)
	savedCS := readKernelDWORD(k.CPU, k.CPU.R.ESP+4)
	savedEFLAGS := readKernelDWORD(k.CPU, k.CPU.R.ESP+8)

	curPCB.Regs = k.CPU.R
	curPCB.Regs.EIP = savedEIP
	curPCB.Regs.CS = uint16(savedCS)
	curPCB.Regs.EFLAGS = savedEFLAGS
	curPCB.SavedUserESP = k.CPU.SavedUserESP
	curPCB.SavedUserSS = k.CPU.SavedUserSS
	curPCB.State = StateReady

	out = append(out, cpu.EventPCBSave{PID: curPID, Regs: pcbRegsSnapshot(curPCB)})
	out = append(out, cpu.EventProcessState{PID: curPID, State: string(StateReady), Reason: "preempted by timer"})

	// 2. Push current to the tail of the ready queue.
	k.ReadyQueue = append(k.ReadyQueue, curPID)

	// 3. Pop the next from the head.
	nextPID := k.ReadyQueue[0]
	k.ReadyQueue = k.ReadyQueue[1:]
	nextPCB := k.Processes[nextPID]
	if nextPCB == nil {
		// Inconsistent state: bail out emitting only what we have.
		return out
	}

	out = append(out, cpu.EventSchedDecision{
		FromPID:    curPID,
		ToPID:      nextPID,
		ReadyQueue: append([]int(nil), k.ReadyQueue...),
	})

	// 4. Restore the incoming process's user-visible regs into the CPU.
	// We keep ESP/CS pointing at the kernel stack: the handler has not
	// finished yet. Only regs the HW does not touch are restored (EAX-EBP
	// plus SavedUserESP/SS).
	k.CPU.R.EAX = nextPCB.Regs.EAX
	k.CPU.R.EBX = nextPCB.Regs.EBX
	k.CPU.R.ECX = nextPCB.Regs.ECX
	k.CPU.R.EDX = nextPCB.Regs.EDX
	k.CPU.R.ESI = nextPCB.Regs.ESI
	k.CPU.R.EDI = nextPCB.Regs.EDI
	k.CPU.R.EBP = nextPCB.Regs.EBP
	k.CPU.R.DS = nextPCB.Regs.DS
	k.CPU.SavedUserESP = nextPCB.SavedUserESP
	k.CPU.SavedUserSS = nextPCB.SavedUserSS

	// Patch the kernel-stack frame so IRET will jump to the new EIP.
	writeKernelDWORD(k.CPU, k.CPU.R.ESP+0, nextPCB.Regs.EIP)
	writeKernelDWORD(k.CPU, k.CPU.R.ESP+4, uint32(nextPCB.Regs.CS))
	writeKernelDWORD(k.CPU, k.CPU.R.ESP+8, nextPCB.Regs.EFLAGS)

	out = append(out, cpu.EventPCBRestore{PID: nextPID, Regs: pcbRegsSnapshot(nextPCB)})

	nextPCB.State = StateRun
	k.CurrentPID = nextPID
	out = append(out, cpu.EventProcessState{PID: nextPID, State: string(StateRun)})

	return out
}

// loadCPUFromPCB initializes the CPU from the state stored in a PCB. Only used
// at bootstrap (the first process); regular switching happens via scheduleNext.
func (k *Kernel) loadCPUFromPCB(p *PCB) {
	c := k.CPU
	c.R = p.Regs
	c.R.ESP = p.SavedUserESP
	c.R.SS = p.SavedUserSS
	c.Mode = cpu.ModeUser
	c.SavedUserESP = p.SavedUserESP
	c.SavedUserSS = p.SavedUserSS
}

// switchTo prepares the CPU and kernel stack so the next IRET lands in the
// given PID: regs restored from the PCB, kernel-stack frame patched with the
// target EIP/CS/EFLAGS. The caller is responsible for the bookkeeping of the
// outgoing PID (state, queue). Returns the events to propagate.
func (k *Kernel) switchTo(pid int) []cpu.Event {
	pcb := k.Processes[pid]
	if pcb == nil {
		return nil
	}
	k.CPU.R.EAX = pcb.Regs.EAX
	k.CPU.R.EBX = pcb.Regs.EBX
	k.CPU.R.ECX = pcb.Regs.ECX
	k.CPU.R.EDX = pcb.Regs.EDX
	k.CPU.R.ESI = pcb.Regs.ESI
	k.CPU.R.EDI = pcb.Regs.EDI
	k.CPU.R.EBP = pcb.Regs.EBP
	k.CPU.R.DS = pcb.Regs.DS
	k.CPU.SavedUserESP = pcb.SavedUserESP
	k.CPU.SavedUserSS = pcb.SavedUserSS

	// Patch the top kernel-stack frame so IRET jumps to the user EIP.
	writeKernelDWORD(k.CPU, k.CPU.R.ESP+0, pcb.Regs.EIP)
	writeKernelDWORD(k.CPU, k.CPU.R.ESP+4, uint32(pcb.Regs.CS))
	writeKernelDWORD(k.CPU, k.CPU.R.ESP+8, pcb.Regs.EFLAGS)

	pcb.State = StateRun
	k.CurrentPID = pid
	return []cpu.Event{
		cpu.EventPCBRestore{PID: pid, Regs: pcbRegsSnapshot(pcb)},
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
