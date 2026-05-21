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

func TestProducerConsumer_HaltsWithBalancedOps(t *testing.T) {
	// Producer and consumer each do 50 iterations on a buffer of size 1.
	// Whichever process the scheduler favors, the semaphores keep them in
	// step: producer cannot signal full again until consumer signals empty,
	// and vice versa. Demo halts cleanly when both finish their loops.
	demo := findDemo(t, "producer_consumer")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 4
	loadDemo(t, k, demo)
	runToHalt(t, k, 40000)

	// Both semaphores must end at their initial values: empty=1, full=0.
	// Any other pair would mean the producer or consumer leaked an op.
	if len(k.Semaphores) != 2 {
		t.Fatalf("expected 2 semaphores, got %d", len(k.Semaphores))
	}
	empty := k.Semaphores[0]
	full := k.Semaphores[1]
	if empty.Count != 1 {
		t.Errorf("empty.Count = %d, want 1 (consumer must have signaled back to initial state)", empty.Count)
	}
	if full.Count != 0 {
		t.Errorf("full.Count = %d, want 0 (no leftover items)", full.Count)
	}
	if len(empty.Waiters) != 0 || len(full.Waiters) != 0 {
		t.Errorf("sem waiters non-empty at halt: empty=%v full=%v", empty.Waiters, full.Waiters)
	}
}

func TestPhilosophersSafe_HaltsWithoutDeadlock(t *testing.T) {
	// The safe variant (philo 2 inverts lock order) guarantees all three
	// philosophers eventually halt. We pick a smallish quantum so context
	// switches happen mid-section.
	demo := findDemo(t, "philos_safe")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 4
	loadDemo(t, k, demo)
	runToHalt(t, k, 80000)

	// Every fork must be free at halt — no one died holding one.
	for _, m := range k.Mutexes {
		if m.Holder >= 0 {
			t.Errorf("fork %d still held by P%d at halt", m.ID, m.Holder)
		}
		if len(m.Waiters) > 0 {
			t.Errorf("fork %d has waiters %v at halt", m.ID, m.Waiters)
		}
	}
}

func TestFailureDemos_RegisterAndRun(t *testing.T) {
	for _, name := range []string{
		"stack_overflow",
		"syscall_kernel_pointer",
		"unknown_syscall",
		"double_unlock",
		"semaphore_leak",
		"latency_quantum",
	} {
		demo := findDemo(t, name)
		if demo.Category == "" {
			t.Errorf("%s: Category is empty", name)
		}
		if len(demo.Procs) == 0 {
			t.Errorf("%s: no procs", name)
		}
	}
}

func TestStackOverflow_PageFaultsQuickly(t *testing.T) {
	demo := findDemo(t, "stack_overflow")
	k := kernel.New(cpu.DefaultMemSize)
	loadDemo(t, k, demo)
	runToHalt(t, k, 100)

	p := k.Processes[1]
	if p == nil {
		t.Fatal("PID 1 not registered")
	}
	if p.State != kernel.StateZombie {
		t.Fatalf("state = %s, want ZOMBIE", p.State)
	}
	if p.BlockedOn != "page_fault" {
		t.Fatalf("BlockedOn = %q, want page_fault", p.BlockedOn)
	}
}

func TestSemaphoreLeak_LeavesThreeKernelObjects(t *testing.T) {
	demo := findDemo(t, "semaphore_leak")
	k := kernel.New(cpu.DefaultMemSize)
	loadDemo(t, k, demo)
	runToHalt(t, k, 200)

	if got := len(k.Semaphores); got != 3 {
		t.Fatalf("len(Semaphores) = %d, want 3", got)
	}
	for i, want := range []int{1, 2, 3} {
		if k.Semaphores[i].Count != want {
			t.Errorf("sem %d count = %d, want %d", i+1, k.Semaphores[i].Count, want)
		}
	}
}

// TestShowcase_ConfigurationIsValid checks the showcase demo is registered
// with the expected shape: it requests 4 cores, ships 4 procs, and every
// proc has both bytes and a source map whose offsets fall inside the binary.
// The source map is required so the CodeAsmSplit panel can light up the C
// line matching EIP on each of the 4 PIDs.
func TestShowcase_ConfigurationIsValid(t *testing.T) {
	demo := findDemo(t, "showcase")
	if demo.Cores != 4 {
		t.Errorf("showcase.Cores = %d, want 4", demo.Cores)
	}
	if demo.Category != "showcase" {
		t.Errorf("showcase.Category = %q, want %q", demo.Category, "showcase")
	}
	if len(demo.Procs) != 4 {
		t.Fatalf("showcase has %d procs, want 4", len(demo.Procs))
	}
	for _, p := range demo.Procs {
		if len(p.Bytes) == 0 {
			t.Errorf("proc %q has empty Bytes", p.Name)
			continue
		}
		if len(p.SourceMap) == 0 {
			t.Errorf("proc %q has no SourceMap", p.Name)
		}
		for _, sl := range p.SourceMap {
			if int(sl.Offset) >= len(p.Bytes) {
				t.Errorf("proc %q SourceMap offset 0x%X >= len(Bytes)=%d", p.Name, sl.Offset, len(p.Bytes))
			}
		}
	}
}

// TestShowcase_RunOnFourCoresHitsExpectedStories drives the demo for enough
// ticks on a 4-core kernel to confirm the four narrative beats land:
//
//  1. The producer creates exactly one semaphore (SemCreate via INT 0x81).
//  2. The consumer eventually page-faults (state ZOMBIE, IRQ 0x0E fired).
//  3. memwalk completes via sys_exit (state ZOMBIE).
//  4. The reader stays BLOCKED because the test never pushes a key.
//
// We also check that each of the relevant IRQ vectors fires at least once
// (0x0E, 0x20, 0x80, 0x81). IRQ 0x21 only fires when a key arrives, which
// is the user's responsibility — out of scope for an automated test.
func TestShowcase_RunOnFourCoresHitsExpectedStories(t *testing.T) {
	demo := findDemo(t, "showcase")
	k := kernel.NewMulti(cpu.DefaultMemSize, 4)
	k.TimerQuantum = 8
	loadDemo(t, k, demo)

	seenVec := make(map[uint8]bool)
	const budget = 8000
	for step := 0; step < budget && !k.AllHalted(); step++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for _, e := range evs {
			if ev, ok := e.(cpu.EventIRQFire); ok {
				seenVec[ev.Vector] = true
			}
		}
	}
	if !k.AllHalted() {
		t.Fatalf("kernel did not halt within %d steps (reader stuck non-BLOCKED?)", budget)
	}

	if len(k.Semaphores) != 1 {
		t.Errorf("Semaphores = %d, want 1 (producer should have created sem id 1)", len(k.Semaphores))
	}
	if walker, ok := k.Processes[1]; !ok {
		t.Error("PID 1 (memwalk) not registered")
	} else if walker.State != kernel.StateZombie {
		t.Errorf("memwalk state = %s, want ZOMBIE (sys_exit)", walker.State)
	}
	if reader, ok := k.Processes[2]; !ok {
		t.Error("PID 2 (reader) not registered")
	} else if reader.State != kernel.StateBlocked {
		t.Errorf("reader state = %s, want BLOCKED (no key pushed)", reader.State)
	}
	if consumer, ok := k.Processes[4]; !ok {
		t.Error("PID 4 (consumer_fault) not registered")
	} else if consumer.State != kernel.StateZombie {
		t.Errorf("consumer state = %s, want ZOMBIE (page fault)", consumer.State)
	}

	for _, v := range []uint8{0x0E, 0x20, 0x80, 0x81} {
		if !seenVec[v] {
			t.Errorf("IRQ 0x%02X never fired", v)
		}
	}
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
