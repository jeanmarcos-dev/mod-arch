package kernel

import (
	"fmt"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// KThreaddPID is the reserved PID for kthreadd, the parent of every kernel
// thread. Mirrors Linux's PID 2 convention (Silberschatz §3.3.2 introduces
// kernel threads; Linux assigns kthreadd the second-after-swapper PID so the
// kernel-thread tree has a single, stable ancestor).
const KThreaddPID = 2

// SpawnKThreadd creates the PID 2 PCB. Idempotent. ParentPID = 0 places
// kthreadd as a child of the swapper (PID 0) — the same layout Linux ships:
//
//	swapper (PID 0) ──┬─▶ init (PID 1)
//	                  └─▶ kthreadd (PID 2) ─▶ kernel/0, kernel/1, …
//
// The PCB is parked in BLOCKED forever and IsKThread = true so the scheduler
// rejects it before it can be bound to any core.
func (k *Kernel) SpawnKThreadd() *PCB {
	if existing, ok := k.Processes[KThreaddPID]; ok {
		return existing
	}
	if k.nextPID > KThreaddPID {
		return nil
	}
	// Bring nextPID up to the kthreadd slot. The expectation is that callers
	// invoke SpawnKThreadd after SpawnInit (which consumes PID 1), so
	// nextPID is naturally KThreaddPID at this point.
	for k.nextPID < KThreaddPID {
		k.nextPID++
	}
	pid := k.nextPID
	k.nextPID++

	pcb := &PCB{
		PID:       pid,
		Name:      "kthreadd",
		State:     StateBlocked,
		BlockedOn: "kernel",
		Mode:      cpu.ModeKernel,
		Slot:      -1,
		Core:      -1,
		TGID:      pid,
		ParentPID: 0,
		IsKThread: true,
	}
	k.Processes[pid] = pcb
	return pcb
}

// SpawnKernelThreads creates one PCB per core, hung off kthreadd. Each
// represents the kernel-thread that runs in the trapped process's context
// while its core is in PL0; the frontend reads cores[i].mode to render
// RUN/SLEEP for each. They never enter the ready queue (IsKThread guards
// every scheduler entry point).
//
// Expected to run once during NewMulti after SpawnKThreadd. Subsequent calls
// are a no-op so tests can construct kernels and re-bootstrap freely.
func (k *Kernel) SpawnKernelThreads() {
	if _, ok := k.Processes[KThreaddPID]; !ok {
		return
	}
	parent := k.Processes[KThreaddPID]
	for _, core := range k.Cores {
		if k.findKernelThreadByCore(core.ID) != nil {
			continue
		}
		pid := k.nextPID
		k.nextPID++
		pcb := &PCB{
			PID:       pid,
			Name:      fmt.Sprintf("kernel/%d", core.ID),
			State:     StateBlocked,
			BlockedOn: "kernel",
			Mode:      cpu.ModeKernel,
			Slot:      -1,
			Core:      core.ID,
			// TGID = pid: kthreads in Linux are NOT in kthreadd's thread
			// group (kernel_clone() without CLONE_THREAD); they share the
			// kernel address space (mm == NULL), not the thread group.
			TGID:      pid,
			ParentPID: KThreaddPID,
			IsKThread: true,
		}
		k.Processes[pid] = pcb
		parent.Children = append(parent.Children, pid)
	}
}

// findKernelThreadByCore returns the kernel/N PCB bound to the given core,
// or nil if it has not been spawned yet. Used by SpawnKernelThreads to stay
// idempotent across repeat calls.
func (k *Kernel) findKernelThreadByCore(coreID int) *PCB {
	for _, p := range k.Processes {
		if p != nil && p.IsKThread && p.PID != KThreaddPID && p.Core == coreID {
			return p
		}
	}
	return nil
}
