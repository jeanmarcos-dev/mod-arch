package kernel

import (
	"fmt"
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestSpawnKThreadd_TakesPID2 ensures kthreadd lands on PID 2 when invoked
// after SpawnInit, mirroring Linux's reserved layout: PID 0 swapper, PID 1
// init, PID 2 kthreadd. Idempotent across repeat calls.
func TestSpawnKThreadd_TakesPID2(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	kt := k.SpawnKThreadd()
	if kt == nil {
		t.Fatal("SpawnKThreadd returned nil")
	}
	if kt.PID != KThreaddPID {
		t.Errorf("kthreadd PID = %d, want %d", kt.PID, KThreaddPID)
	}
	if kt.Name != "kthreadd" {
		t.Errorf("kthreadd Name = %q, want \"kthreadd\"", kt.Name)
	}
	if !kt.IsKThread {
		t.Error("kthreadd IsKThread = false, want true")
	}
	if kt.ParentPID != 0 {
		t.Errorf("kthreadd ParentPID = %d, want 0 (swapper)", kt.ParentPID)
	}
	if kt.State != StateBlocked {
		t.Errorf("kthreadd State = %s, want BLOCKED", kt.State)
	}

	// Second call must return the same PCB without advancing nextPID.
	again := k.SpawnKThreadd()
	if again != kt {
		t.Error("SpawnKThreadd is not idempotent")
	}
}

// TestSpawnKernelThreads_OnePerCore creates one kernel/N PCB per core, hung
// off kthreadd. PIDs run consecutively starting at 3 (so for N cores the
// kthreads occupy PIDs 3..2+N and AddProcess afterwards gets 3+N).
func TestSpawnKernelThreads_OnePerCore(t *testing.T) {
	const cores = 3
	k := NewMulti(cpu.DefaultMemSize, cores)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	if k.SpawnKThreadd() == nil {
		t.Fatal("SpawnKThreadd returned nil")
	}
	k.SpawnKernelThreads()

	parent := k.Processes[KThreaddPID]
	if parent == nil {
		t.Fatal("kthreadd missing after SpawnKernelThreads")
	}
	if len(parent.Children) != cores {
		t.Errorf("kthreadd Children = %d, want %d", len(parent.Children), cores)
	}

	for i := 0; i < cores; i++ {
		name := fmt.Sprintf("kernel/%d", i)
		var found *PCB
		for _, p := range k.Processes {
			if p != nil && p.Name == name {
				found = p
				break
			}
		}
		if found == nil {
			t.Fatalf("kernel thread %s not registered in process table", name)
		}
		if !found.IsKThread {
			t.Errorf("%s IsKThread = false, want true", name)
		}
		if found.ParentPID != KThreaddPID {
			t.Errorf("%s ParentPID = %d, want %d", name, found.ParentPID, KThreaddPID)
		}
		if found.Core != i {
			t.Errorf("%s Core = %d, want %d", name, found.Core, i)
		}
	}

	// Second call must not create duplicates.
	before := len(k.Processes)
	k.SpawnKernelThreads()
	if len(k.Processes) != before {
		t.Errorf("SpawnKernelThreads is not idempotent: %d → %d processes", before, len(k.Processes))
	}
}

// TestPopNextRunnable_SkipsKThread guards against a kthread being scheduled
// onto a core. If a future bug pushes a kthread to the ready queue, the
// scheduler must drop it instead of binding it (their PCBs have no
// memSpace and would crash loadCPUFromPCB).
func TestPopNextRunnable_SkipsKThread(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	k.SpawnKThreadd()
	k.SpawnKernelThreads()

	// Force the kthread into the ready queue (simulates the bug).
	var kthreadPID int
	for pid, p := range k.Processes {
		if p != nil && p.IsKThread && pid != KThreaddPID {
			kthreadPID = pid
			break
		}
	}
	if kthreadPID == 0 {
		t.Fatal("no kernel/N kthread spawned")
	}
	k.Ready.Push(kthreadPID)

	if _, ok := k.popNextRunnable(); ok {
		t.Error("popNextRunnable returned the forced kthread; expected ok=false")
	}
}

// TestDispatchIdleCores_NeverAssignsKThread runs the dispatcher with a real
// user proc and a kthread in the table. The user proc must take the core;
// no core may end up with a kthread as CurrentPID.
func TestDispatchIdleCores_NeverAssignsKThread(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	k.SpawnKThreadd()
	k.SpawnKernelThreads()

	userPID, err := k.AddProcess("user", []byte{0x90, 0xF4})
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}

	_ = k.DispatchIdleCores()

	for i, c := range k.Cores {
		if c.CurrentPID == userPID {
			continue
		}
		if c.CurrentPID != 0 {
			if p := k.Processes[c.CurrentPID]; p != nil && p.IsKThread {
				t.Errorf("core %d bound to kthread PID %d (%s)", i, c.CurrentPID, p.Name)
			}
		}
	}
}
