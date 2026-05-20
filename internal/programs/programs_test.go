package programs

import (
	"encoding/binary"
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

const sharedCounterAddr = 0x3FFFE0

// loadDemo wires a Demo's procs into a fresh single-core kernel.
func loadDemo(t *testing.T, k *kernel.Kernel, d Demo) {
	t.Helper()
	for _, p := range d.Procs {
		if _, err := k.AddProcess(p.Name, p.Bytes); err != nil {
			t.Fatalf("AddProcess %s: %v", p.Name, err)
		}
	}
}

// runToHalt drives the kernel until every core halts or the budget runs out.
func runToHalt(t *testing.T, k *kernel.Kernel, budget int) {
	t.Helper()
	for step := 0; step < budget && !k.AllHalted(); step++ {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
	}
	if !k.AllHalted() {
		t.Fatalf("kernel did not halt within %d steps", budget)
	}
}

// findDemo looks up a demo by name in the registry.
func findDemo(t *testing.T, name string) Demo {
	t.Helper()
	for _, d := range Demos() {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("demo %q not registered", name)
	return Demo{}
}

func TestSafeCounter_AlwaysReachesTwoHundred(t *testing.T) {
	// safe_counter has 3 processes: setup creates mutex 1, two workers each
	// do 200 mutex-protected increments. With the mutex serializing the
	// critical section, the counter must end at exactly 400.
	demo := findDemo(t, "safe_counter")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 8
	loadDemo(t, k, demo)
	runToHalt(t, k, 60000)

	got := binary.LittleEndian.Uint32(k.Mem.Bytes[sharedCounterAddr : sharedCounterAddr+4])
	if got != 400 {
		t.Errorf("counter = %d, want 400 (mutex must serialize the section)", got)
	}
}

func TestRaceCounter_RunsToCompletion(t *testing.T) {
	// race_counter is pedagogical: it illustrates the load/inc/store pattern
	// without any lock. Whether the toy simulator's deterministic scheduler
	// actually produces lost updates depends on the timing alignment between
	// quantum and iteration size. We assert only that the workers run and
	// the counter ends in [0, 400] (cannot exceed total increments).
	demo := findDemo(t, "race_counter")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 8
	loadDemo(t, k, demo)
	runToHalt(t, k, 60000)

	got := binary.LittleEndian.Uint32(k.Mem.Bytes[sharedCounterAddr : sharedCounterAddr+4])
	if got > 400 {
		t.Errorf("counter = %d, want <= 400 (cannot exceed total increments)", got)
	}
	if got == 0 {
		t.Errorf("counter = 0, workers never ran")
	}
	t.Logf("race_counter ended at %d (max=400, lost updates=%d)", got, 400-int(got))
}

func TestSafeCounter_OnTwoCoresStillReaches400(t *testing.T) {
	// Same demo but spread across two cores. The mutex still serializes the
	// section, so the result must hold.
	demo := findDemo(t, "safe_counter")
	k := kernel.NewMulti(cpu.DefaultMemSize, 2)
	k.TimerQuantum = 8
	loadDemo(t, k, demo)
	runToHalt(t, k, 60000)

	got := binary.LittleEndian.Uint32(k.Mem.Bytes[sharedCounterAddr : sharedCounterAddr+4])
	if got != 400 {
		t.Errorf("counter = %d, want 400 (mutex on 2 cores must still serialize)", got)
	}
}
