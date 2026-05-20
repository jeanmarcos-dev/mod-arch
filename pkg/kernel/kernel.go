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

// Kernel wraps the CPU and adds the OS software decisions.
type Kernel struct {
	CPU *cpu.CPU

	// TimerQuantum is the number of CPU ticks between IRQ 0x20 fires.
	// 0 disables the timer. Default: 8.
	TimerQuantum int

	lastTimerFiredAt uint64

	// Processes is the process table keyed by PID.
	Processes  map[int]*PCB
	CurrentPID int
	ReadyQueue []int

	// KeyboardBuf holds pending bytes; blockedReads is the FIFO queue of
	// processes waiting in sys_read. A queue is required because several
	// processes may be waiting on the same resource (the keyboard driver here).
	KeyboardBuf  []byte
	blockedReads []*blockedReadRequest

	nextPID int
}

// New builds a Kernel with its CPU, installs the default IDT, and loads no
// processes.
func New(memSize int) *Kernel {
	k := &Kernel{
		CPU:          cpu.New(memSize),
		TimerQuantum: 8,
		Processes:    make(map[int]*PCB),
		nextPID:      1,
	}
	k.InstallDefaultIDT()
	return k
}

// Reset clears the process table and reinstalls the IDT. Useful for loading a
// fresh scenario from scratch.
func (k *Kernel) Reset() {
	k.CPU.Reset()
	k.Processes = make(map[int]*PCB)
	k.ReadyQueue = nil
	k.CurrentPID = 0
	k.nextPID = 1
	k.lastTimerFiredAt = 0
	k.KeyboardBuf = nil
	k.blockedReads = nil
	// The IDT lives in kernel memory and CPU.Reset does not touch it, but
	// reinstall defensively in case a test mutated entries.
	k.InstallDefaultIDT()
}

// AddProcess registers a new process, copies its code into the user_code
// region for that process, and returns the assigned PID. The process is left
// in the READY state.
//
// The first registered process becomes CurrentPID and is loaded into the CPU;
// any subsequent ones go onto the ready queue.
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
		k.CPU.Mem.Bytes[i] = 0
	}
	if err := k.CPU.Mem.LoadBytes(codeBase, code); err != nil {
		return 0, fmt.Errorf("load process %d code: %w", pid, err)
	}

	pcb := &PCB{
		PID:      pid,
		Name:     name,
		State:    StateReady,
		Mode:     cpu.ModeUser,
		CodeBase: codeBase,
		CodeSize: uint32(len(code)),
		StackTop: stackTop,
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

	if k.CurrentPID == 0 {
		k.CurrentPID = pid
		pcb.State = StateRun
		k.loadCPUFromPCB(pcb)
	} else {
		k.ReadyQueue = append(k.ReadyQueue, pid)
	}
	return pid, nil
}

// Step executes one instruction of the current process, possibly performing a
// context switch if the timer fired.
//
// Before stepping, it checks whether the timer should fire and, if so, pushes
// IRQ 0x20 onto CPU.PendingIRQs (the CPU picks it up on its next Step).
//
// After stepping, if the dispatch was IRQ 0x20 we run the handler logic in Go
// (scheduler): save the current PCB, pick the next one, restore it, and patch
// the kernel-stack frame so the handler's IRET jumps to the new process EIP.
func (k *Kernel) Step() ([]cpu.Event, error) {
	if k.shouldFireTimer() {
		k.CPU.PendingIRQs = append(k.CPU.PendingIRQs, IRQTimer)
		k.lastTimerFiredAt = k.CPU.Tick
	}
	evs, err := k.CPU.Step()
	if err != nil {
		return evs, err
	}
	for _, e := range evs {
		irq, ok := e.(cpu.EventIRQFire)
		if !ok {
			continue
		}
		schedEvs := k.runHandlerLogic(irq.Vector, irq.Source)
		// The handler may have called cpu.Emit (e.g. mem_read while reading
		// the user buffer in sys_write). CPU.Step has already returned its
		// own slice, so drain whatever the handler accumulated and merge.
		handlerEvs := k.CPU.DrainEvents()
		evs = append(evs, handlerEvs...)
		evs = append(evs, schedEvs...)
	}
	return evs, nil
}

// shouldFireTimer decides whether the PIT should fire on this step.
func (k *Kernel) shouldFireTimer() bool {
	if k.TimerQuantum <= 0 {
		return false
	}
	return k.CPU.Tick >= k.lastTimerFiredAt+uint64(k.TimerQuantum) && k.CPU.Tick > 0
}

// runHandlerLogic executes the work a real OS would perform between handler
// entry and IRET. We model it in Go so we do not need a full scheduler written
// in hand-encoded assembly.
//
// The handler bytes in memory are still real (nop + iret); this function runs
// BEFORE the handler body executes, mutates the kernel-stack frame and CPU
// registers, and the subsequent IRET pops the patched frame to land in the
// new process.
func (k *Kernel) runHandlerLogic(vector uint8, source string) []cpu.Event {
	switch vector {
	case IRQTimer:
		// Only schedule on HW timer; ignore SW-triggered INT 0x20.
		if source != "HW" {
			return nil
		}
		return k.scheduleNext()
	case IRQSyscall:
		// Vector 0x80 is reserved for syscalls (user invokes it via `int 0x80`,
		// source SW). Arrival via HW indicates a bug.
		if source != "SW" {
			return nil
		}
		return k.handleSyscall()
	case IRQPageFault:
		// Page faults always come via "fault" (the MMU raises them on access
		// violation). A real OS could resolve some (page-in, COW); here we
		// kill the offending process.
		if source != "fault" {
			return nil
		}
		return k.killCurrent("page_fault")
	case IRQKeyboard:
		// Keyboard: HW (PushKey) injects this IRQ with source HW. Deliver any
		// pending bytes to the first process BLOCKED in sys_read.
		if source != "HW" {
			return nil
		}
		return k.handleKeyboard()
	default:
		return nil
	}
}

// killCurrent terminates the current process and schedules the next one if any
// remain. Used by sys_exit and by page-fault handling. The PCB is left as
// ZOMBIE with the cause recorded in BlockedOn.
func (k *Kernel) killCurrent(reason string) []cpu.Event {
	curPID := k.CurrentPID
	curPCB := k.Processes[curPID]
	if curPCB == nil {
		k.CPU.Halted = true
		return nil
	}
	curPCB.State = StateZombie
	curPCB.BlockedOn = reason

	out := []cpu.Event{
		cpu.EventProcessState{PID: curPID, State: string(StateZombie), Reason: reason},
	}

	// If other READY processes exist, hand the CPU to the first one. This is
	// the scheduleNext pattern WITHOUT re-enqueueing the current PID (zombie
	// processes do not return).
	if len(k.ReadyQueue) > 0 {
		nextPID := k.ReadyQueue[0]
		k.ReadyQueue = k.ReadyQueue[1:]
		nextPCB := k.Processes[nextPID]
		if nextPCB != nil {
			out = append(out, cpu.EventSchedDecision{
				FromPID:    curPID,
				ToPID:      nextPID,
				ReadyQueue: append([]int(nil), k.ReadyQueue...),
			})

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

			writeKernelDWORD(k.CPU, k.CPU.R.ESP+0, nextPCB.Regs.EIP)
			writeKernelDWORD(k.CPU, k.CPU.R.ESP+4, uint32(nextPCB.Regs.CS))
			writeKernelDWORD(k.CPU, k.CPU.R.ESP+8, nextPCB.Regs.EFLAGS)

			out = append(out, cpu.EventPCBRestore{PID: nextPID, Regs: pcbRegsSnapshot(nextPCB)})
			nextPCB.State = StateRun
			k.CurrentPID = nextPID
			out = append(out, cpu.EventProcessState{PID: nextPID, State: string(StateRun)})
			return out
		}
	}

	// No remaining processes: halt the CPU.
	k.CPU.Halted = true
	out = append(out, cpu.EventHalt{PID: curPID})
	return out
}
