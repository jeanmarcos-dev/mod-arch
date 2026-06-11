package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestStep_OtherCoresTickDuringExitCascade pins the multicore invariant: while
// one core drains a do_exit phase, every OTHER core keeps ticking its own work.
// Freezing the siblings (the old behavior) left their event-log cells blank for
// the whole cascade, which read as "every CPU stalls when one process exits" —
// wrong for SMP.
func TestStep_OtherCoresTickDuringExitCascade(t *testing.T) {
	exitProg := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
		0xCD, 0x80, // int 0x80
	}
	looper := []byte{0x90, 0xEB, 0xFD} // nop ; jmp -3 (never terminates)

	k := NewMulti(cpu.DefaultMemSize, 2)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	k.TimerQuantum = 0 // no preemption — keep each proc pinned to its core

	dier, err := k.AddProcess("dier", exitProg)
	if err != nil {
		t.Fatalf("AddProcess dier: %v", err)
	}
	sibling, err := k.AddProcess("looper", looper)
	if err != nil {
		t.Fatalf("AddProcess looper: %v", err)
	}
	k.AdoptUnderInit(dier)
	k.AdoptUnderInit(sibling)
	k.DispatchIdleCores() // dier -> core 0, looper -> core 1

	siblingCore := k.Processes[sibling].Core
	if siblingCore == k.Processes[dier].Core {
		t.Fatalf("test setup: dier and looper landed on the same core (%d)", siblingCore)
	}

	// Step until the exit cascade is queued (INT 0x80 fires).
	cascadeStarted := false
	for range 30 {
		if _, err := k.Step(); err != nil {
			t.Fatalf("Step: %v", err)
		}
		if len(k.PendingKernelWork) > 0 {
			cascadeStarted = true
			break
		}
	}
	if !cascadeStarted {
		t.Fatal("exit cascade never queued")
	}

	// Drive the remaining phases. Every kernel-work tick must still carry an
	// instruction from the sibling core.
	for range 8 {
		if len(k.PendingKernelWork) == 0 {
			break
		}
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("Step: %v", err)
		}
		siblingTicked := false
		for _, e := range evs {
			if inst, ok := e.(cpu.EventInstruction); ok && inst.CoreID == siblingCore {
				siblingTicked = true
				break
			}
		}
		if !siblingTicked {
			t.Errorf("kernel-work tick (clock=%d) emitted no instruction on sibling core %d — it was frozen during the exit cascade", k.Clock-1, siblingCore)
		}
	}
}
