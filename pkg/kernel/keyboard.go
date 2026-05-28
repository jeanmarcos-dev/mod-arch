package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// blockedReadRequest tracks a process blocked waiting for keyboard input. The
// kernel keeps the user buffer address and requested count; when bytes arrive
// they are delivered and the process is woken up.
type blockedReadRequest struct {
	PID     int
	BufAddr uint32
	Count   uint32
}

// PushKey simulates the arrival of a byte from the keyboard.
//
// The byte is enqueued in kernel.KeyboardBuf and IRQ 0x21 is pushed onto the
// boot core's PendingIRQs. The CPU will dispatch it on the next Step provided
// IF=1 (true while in user or kernel-halt with IF set). Real SMP would steer
// keyboard interrupts via the IO-APIC; here the BSP always handles them.
//
// Invoked by the server when the frontend sends cmd=push_key.
func (k *Kernel) PushKey(b byte) {
	k.KeyboardBuf = append(k.KeyboardBuf, b)
	boot := k.BootCPU()
	boot.PendingIRQs = append(boot.PendingIRQs, IRQKeyboard)
}

// blockCurrent removes the running process from the given core and marks it
// BLOCKED with the given reason. It schedules the next READY process if any,
// otherwise halts the core with IF=1 so a future IRQ can wake it up.
//
// This is the path used by sys_read when no bytes are immediately available.
func (k *Kernel) blockCurrent(core *Core, reason string) []cpu.Event {
	curPID := core.CurrentPID
	pcb := k.saveCurrentToPCB(core)
	if pcb == nil {
		return nil
	}
	pcb.State = StateBlocked
	pcb.BlockedOn = reason
	pcb.Core = -1

	out := []cpu.Event{
		cpu.EventPCBSave{CoreID: core.ID, PID: curPID, Regs: pcbRegsSnapshot(pcb)},
		cpu.EventProcessState{PID: curPID, State: string(StateBlocked), Reason: reason},
	}

	if nextPID, ok := k.Ready.Pop(); ok {
		out = append(out,
			cpu.EventSchedDecision{
				FromPID:    curPID,
				ToPID:      nextPID,
				Reason:     "block",
				ReadyQueue: k.Ready.Snapshot(),
			},
			cpu.EventCoreRelease{CoreID: core.ID, PID: curPID, Reason: reason},
		)
		out = append(out, k.switchTo(core, nextPID)...)
		return out
	}

	// No other READY processes: halt this core with IF=1, waiting for an IRQ.
	core.CPU.Halted = true
	core.CPU.R.EFLAGS |= cpu.FlagIF
	core.CurrentPID = 0
	out = append(out,
		cpu.EventCoreRelease{CoreID: core.ID, PID: curPID, Reason: reason},
		cpu.EventHalt{CoreID: core.ID, PID: curPID},
	)
	return out
}

// handleKeyboard runs after a core dispatched IRQ 0x21. If there are BLOCKED
// readers and pending bytes, it hands bytes to the first reader (FIFO) and
// wakes it. If bytes remain and other readers are blocked, the next IRQ will
// deliver to them.
//
// If nobody is blocked, the bytes stay in the buffer for future sys_reads to
// consume without blocking.
func (k *Kernel) handleKeyboard(core *Core) []cpu.Event {
	if len(k.blockedReads) == 0 || len(k.KeyboardBuf) == 0 {
		return nil
	}

	// FIFO: the first blocked reader is the first to receive bytes.
	req := k.blockedReads[0]
	pcb := k.Processes[req.PID]
	if pcb == nil {
		k.blockedReads = k.blockedReads[1:]
		return nil
	}
	k.blockedReads = k.blockedReads[1:]

	n := min(req.Count, uint32(len(k.KeyboardBuf)))
	delivered := append([]byte(nil), k.KeyboardBuf[:n]...)
	k.KeyboardBuf = k.KeyboardBuf[n:]

	for i, b := range delivered {
		// Pass core.CPU so the narrator sees the kernel copying keyboard
		// bytes into the user's buffer — mirrors the sys_write side.
		_ = k.Mem.Write8(core.CPU, req.BufAddr+uint32(i), b, cpu.ModeKernel)
	}

	// Set the pending syscall result in the PCB. When the process resumes
	// (switchTo) EAX = bytes read will be visible to the user.
	pcb.Regs.EAX = n

	pcb.State = StateReady
	pcb.BlockedOn = ""

	out := []cpu.Event{
		cpu.EventProcessState{PID: req.PID, State: string(StateReady), Reason: "keyboard delivered"},
		// Keyboard echoes belong to no specific core — pin them to C0 so
		// they always pass the visibility filter and the UI never drops
		// user input from view.
		cpu.EventConsoleOut{CoreID: 0, PID: 0, Bytes: delivered, Text: "[in] " + string(delivered)},
	}

	// If the core was idle (no running process), switch directly to the woken
	// process; otherwise enqueue it at the tail of the shared ready queue.
	if core.CurrentPID == 0 {
		out = append(out, cpu.EventSchedDecision{
			FromPID:    0,
			ToPID:      req.PID,
			Reason:     "wake",
			ReadyQueue: k.Ready.Snapshot(),
		})
		out = append(out, k.switchTo(core, req.PID)...)
	} else {
		k.Ready.Push(req.PID)
	}

	return out
}

// sysRead implements read(fd, buf, count). Only fd=0 (stdin) is supported.
//
//   - Buffer has bytes: copy min(count, available) and return immediately
//     without blocking.
//   - Buffer empty: register a blockedReadRequest, block the process and
//     schedule another. When a key arrives, handleKeyboard wakes it.
//
// Returns (bytes_read, events). When blocking, bytes_read is 0 as a
// placeholder; the real n is stored in PCB.Regs.EAX and restored on wake-up.
func (k *Kernel) sysRead(core *Core, fd, bufAddr, count uint32) (uint32, []cpu.Event) {
	if fd != FdStdin {
		if pcb := k.Processes[core.CurrentPID]; pcb != nil {
			if pipe, isWrite, ok := k.resolvePipeFD(pcb, fd); ok && !isWrite {
				return k.pipeRead(core, pipe, bufAddr, count)
			}
		}
		return ^uint32(0), nil
	}
	if count == 0 {
		return 0, nil
	}
	// Reject pointers that would let user code make the kernel scribble onto
	// the IDT, kernel handlers or another process's PCB. Also catches the
	// uint32 wraparound case.
	if bad, ok := userBufferFault(bufAddr, count); !ok {
		return ^uint32(0), []cpu.Event{cpu.EventPageFault{
			CoreID:     core.CPU.CoreID,
			Addr:       bad,
			Mode:       cpu.ModeUser,
			PermNeeded: "W",
		}}
	}

	// Fast path: bytes already available.
	if uint32(len(k.KeyboardBuf)) > 0 {
		n := min(count, uint32(len(k.KeyboardBuf)))
		for i := range n {
			_ = k.Mem.Write8(core.CPU, bufAddr+i, k.KeyboardBuf[i], cpu.ModeKernel)
		}
		k.KeyboardBuf = k.KeyboardBuf[n:]
		return n, nil
	}

	// Slow path: enqueue at the tail of the keyboard wait queue and sleep
	// the process. Subsequent sys_reads from other processes append behind
	// (FIFO).
	k.blockedReads = append(k.blockedReads, &blockedReadRequest{
		PID:     core.CurrentPID,
		BufAddr: bufAddr,
		Count:   count,
	})
	blockEvs := k.blockCurrent(core, "keyboard")
	return 0, blockEvs
}
