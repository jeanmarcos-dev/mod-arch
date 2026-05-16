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
// The byte is enqueued in kernel.KeyboardBuf and IRQ 0x21 is pushed onto
// CPU.PendingIRQs. The CPU will dispatch it on the next Step provided IF=1
// (true while in user or kernel-halt with IF set).
//
// Invoked by the server when the frontend sends cmd=push_key.
func (k *Kernel) PushKey(b byte) {
	k.KeyboardBuf = append(k.KeyboardBuf, b)
	k.CPU.PendingIRQs = append(k.CPU.PendingIRQs, IRQKeyboard)
}

// blockCurrent removes the current process from the CPU and marks it BLOCKED
// with the given reason. It schedules the next READY process if any, otherwise
// halts the CPU with IF=1 so a future IRQ can wake it up.
//
// This is the path used by sys_read when no bytes are immediately available.
func (k *Kernel) blockCurrent(reason string) []cpu.Event {
	curPID := k.CurrentPID
	pcb := k.saveCurrentToPCB()
	if pcb == nil {
		return nil
	}
	pcb.State = StateBlocked
	pcb.BlockedOn = reason

	out := []cpu.Event{
		cpu.EventPCBSave{PID: curPID, Regs: pcbRegsSnapshot(pcb)},
		cpu.EventProcessState{PID: curPID, State: string(StateBlocked), Reason: reason},
	}

	if len(k.ReadyQueue) > 0 {
		nextPID := k.ReadyQueue[0]
		k.ReadyQueue = k.ReadyQueue[1:]
		out = append(out, cpu.EventSchedDecision{
			FromPID:    curPID,
			ToPID:      nextPID,
			ReadyQueue: append([]int(nil), k.ReadyQueue...),
		})
		out = append(out, k.switchTo(nextPID)...)
		return out
	}

	// No other READY processes: halt with IF=1, waiting for an IRQ.
	k.CPU.Halted = true
	k.CPU.R.EFLAGS |= cpu.FlagIF
	k.CurrentPID = 0
	out = append(out, cpu.EventHalt{PID: curPID})
	return out
}

// handleKeyboard runs after the CPU dispatched IRQ 0x21. If there are BLOCKED
// readers and pending bytes, it hands bytes to the first reader (FIFO) and
// wakes it. If bytes remain and other readers are blocked, the next IRQ will
// deliver to them.
//
// If nobody is blocked, the bytes stay in the buffer for future sys_reads to
// consume without blocking.
func (k *Kernel) handleKeyboard() []cpu.Event {
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
		_ = k.CPU.Mem.Write8(nil, req.BufAddr+uint32(i), b, cpu.ModeKernel)
	}

	// Set the pending syscall result in the PCB. When the process resumes
	// (switchTo) EAX = bytes read will be visible to the user.
	pcb.Regs.EAX = n

	pcb.State = StateReady
	pcb.BlockedOn = ""

	out := []cpu.Event{
		cpu.EventProcessState{PID: req.PID, State: string(StateReady), Reason: "keyboard delivered"},
		cpu.EventConsoleOut{PID: 0, Bytes: delivered, Text: "[in] " + string(delivered)},
	}

	// If the CPU was HALTED (no running process), switch directly to the woken
	// process; otherwise enqueue it at the tail of the ready queue.
	if k.CurrentPID == 0 {
		out = append(out, cpu.EventSchedDecision{
			FromPID:    0,
			ToPID:      req.PID,
			ReadyQueue: append([]int(nil), k.ReadyQueue...),
		})
		out = append(out, k.switchTo(req.PID)...)
	} else {
		k.ReadyQueue = append(k.ReadyQueue, req.PID)
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
func (k *Kernel) sysRead(fd, bufAddr, count uint32) (uint32, []cpu.Event) {
	if fd != FdStdin {
		return ^uint32(0), nil
	}
	if count == 0 {
		return 0, nil
	}

	// Fast path: bytes already available.
	if uint32(len(k.KeyboardBuf)) > 0 {
		n := min(count, uint32(len(k.KeyboardBuf)))
		for i := range n {
			_ = k.CPU.Mem.Write8(nil, bufAddr+i, k.KeyboardBuf[i], cpu.ModeKernel)
		}
		k.KeyboardBuf = k.KeyboardBuf[n:]
		return n, nil
	}

	// Slow path: enqueue at the tail of the keyboard wait queue and sleep
	// the process. Subsequent sys_reads from other processes append behind
	// (FIFO).
	k.blockedReads = append(k.blockedReads, &blockedReadRequest{
		PID:     k.CurrentPID,
		BufAddr: bufAddr,
		Count:   count,
	})
	blockEvs := k.blockCurrent("keyboard")
	return 0, blockEvs
}
