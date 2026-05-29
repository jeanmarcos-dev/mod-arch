package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestAdoptUnderInit_DoesNotMarkReparented guards the polarity of the
// WasReparentedToInit flag: AdoptUnderInit is the birth-adoption path used
// by every demo loaded with init (see internal/sim/simulator.go), so children
// it touches must NOT be marked as reparented. The flag is only set when
// markZombieAndReparent transfers an alive child to init because its
// original parent died — that is what makes the EventInitReap render
// "orphan" instead of "child".
//
// A regression here (e.g. someone routing AdoptUnderInit through the same
// path that sets the flag) would make every demo's child show up as an
// orphan in the log, even though they were children of init from birth.
func TestAdoptUnderInit_DoesNotMarkReparented(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	pid, err := k.AddProcess("demo", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}

	k.AdoptUnderInit(pid)

	child := k.Processes[pid]
	if child == nil {
		t.Fatalf("child PID %d disappeared after AdoptUnderInit", pid)
	}
	if child.ParentPID != k.InitPID {
		t.Errorf("child.ParentPID = %d, want %d (init) — adoption should have set the parent", child.ParentPID, k.InitPID)
	}
	if child.WasReparentedToInit {
		t.Error("child.WasReparentedToInit = true after AdoptUnderInit; want false (birth-adoption must not be marked as reparenting)")
	}
}
