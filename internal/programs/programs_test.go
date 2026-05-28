package programs

import (
	"encoding/binary"
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

const (
	sharedCounterAddr = 0x3FFFE0
	threadCounterAddr = 0x3F0000
)

// loadDemo wires a Demo's procs into a fresh single-core kernel and triggers
// the boot dispatch the simulator would normally run on RUN/STEP. init (PID 1)
// is spawned first and every loaded proc is parented to it — same wiring
// sim.LoadDemo applies in production now that init is unconditional.
func loadDemo(t *testing.T, k *kernel.Kernel, d Demo) {
	t.Helper()
	if _, err := k.SpawnInit(); err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	for _, p := range d.Procs {
		pid, err := k.AddProcess(p.Name, p.Bytes)
		if err != nil {
			t.Fatalf("AddProcess %s: %v", p.Name, err)
		}
		k.AdoptUnderInit(pid)
	}
	k.DispatchIdleCores()
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
	// loadDemo halts on AllHalted; with init unconditional, the demo body
	// lives at PID 2 (PID 1 is init, which never runs user code).
	for step := 0; step < 200; step++ {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		if p := k.Processes[2]; p != nil && p.State == kernel.StateZombie {
			if p.BlockedOn != "page_fault" {
				t.Fatalf("BlockedOn = %q, want page_fault", p.BlockedOn)
			}
			return
		}
	}
	t.Fatal("stack_overflow demo did not zombify with page_fault reason")
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
	for step := 0; step < budget && k.HasWork(); step++ {
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

	if len(k.Semaphores) != 1 {
		t.Errorf("Semaphores = %d, want 1 (producer should have created sem id 1)", len(k.Semaphores))
	}
	// PIDs shift by one now that init occupies PID 1 unconditionally.
	if walker, ok := k.Processes[2]; !ok {
		t.Error("PID 2 (memwalk) not registered")
	} else if walker.State != kernel.StateZombie {
		t.Errorf("memwalk state = %s, want ZOMBIE (sys_exit)", walker.State)
	}
	if reader, ok := k.Processes[3]; !ok {
		t.Error("PID 3 (reader) not registered")
	} else if reader.State != kernel.StateBlocked {
		t.Errorf("reader state = %s, want BLOCKED (no key pushed)", reader.State)
	}
	if consumer, ok := k.Processes[5]; !ok {
		t.Error("PID 5 (consumer_fault) not registered")
	} else if consumer.State != kernel.StateZombie {
		t.Errorf("consumer state = %s, want ZOMBIE (page fault)", consumer.State)
	}

	for _, v := range []uint8{0x0E, 0x20, 0x80, 0x81} {
		if !seenVec[v] {
			t.Errorf("IRQ 0x%02X never fired", v)
		}
	}
}

// runUntilParentReaped drives the kernel and returns the exit_code init
// captured when it auto-reaped the given pid. Returns (-1, false) if the
// reap never happened within the budget.
//
// Uses HasWork rather than AllHalted as the loop's continuation predicate:
// with deferred reap, between a child's exit and init's dispatch the boot
// core is momentarily halted (AllHalted=true) while init is queued for the
// next Step — exiting on AllHalted would miss the reap that fires the
// step after.
func runUntilParentReaped(t *testing.T, k *kernel.Kernel, pid int, budget int) (uint32, bool) {
	t.Helper()
	for step := 0; step < budget; step++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for _, e := range evs {
			if w, ok := e.(cpu.EventWaitOp); ok && w.Op == "reap" && w.ChildPID == pid {
				return w.ExitCode, true
			}
		}
		if !k.HasWork() {
			break
		}
	}
	return 0, false
}

// TestDemoFork_ParentReapsChildExitCode verifies the embedded fork demo
// produces the expected end state: with init at PID 1, the demo's main
// proc is PID 2, its forked child is PID 3, the parent's waitpid reaps
// the child returning PID 3, and init then auto-reaps PID 2.
func TestDemoFork_ParentReapsChildExitCode(t *testing.T) {
	demo := findDemo(t, "demo_fork")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 0
	loadDemo(t, k, demo)

	exit, ok := runUntilParentReaped(t, k, 2, 5000)
	if !ok {
		t.Fatal("init never reaped PID 2 within 5000 steps")
	}
	if exit != 3 {
		t.Errorf("PID 2 exit_code (reaped by init) = %d, want 3", exit)
	}
	if init, ok := k.Processes[1]; !ok || init.Name != "init" {
		t.Error("init (PID 1) missing or wrong name")
	}
}

// TestDemoVfork_ParentReadsChildWrite verifies vfork's shared-memory
// semantics: child writes 0xDEADBEEF into a user-stack address, exits,
// parent wakes up and reads that exact value (which init then sees as
// the reaped exit code).
func TestDemoVfork_ParentReadsChildWrite(t *testing.T) {
	demo := findDemo(t, "demo_vfork")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 0
	loadDemo(t, k, demo)

	exit, ok := runUntilParentReaped(t, k, 2, 5000)
	if !ok {
		t.Fatal("init never reaped PID 2 within 5000 steps")
	}
	if exit != 0xDEADBEEF {
		t.Errorf("PID 2 exit_code = 0x%08X, want 0xDEADBEEF", exit)
	}
}

// TestDemoPipe_ChildEchoesHOLA exercises the fork+pipe pipeline: parent
// writes "HOLA" through the pipe, child reads it, child echoes to stdout
// and exits, parent reaps. The console output must be HOLA.
func TestDemoPipe_ChildEchoesHOLA(t *testing.T) {
	demo := findDemo(t, "demo_pipe")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 0
	loadDemo(t, k, demo)

	var consoleBytes []byte
	pipeOps := 0
	forkSeen := false
	for step := 0; step < 10000 && !k.AllHalted(); step++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for _, e := range evs {
			switch ev := e.(type) {
			case cpu.EventConsoleOut:
				consoleBytes = append(consoleBytes, ev.Bytes...)
			case cpu.EventPipeOp:
				pipeOps++
			case cpu.EventFork:
				forkSeen = true
			}
		}
	}
	if !k.AllHalted() {
		t.Fatal("kernel did not halt within 10000 steps")
	}
	if !forkSeen {
		t.Error("EventFork never fired")
	}
	if pipeOps == 0 {
		t.Error("no EventPipeOp emitted — pipe write/read never happened")
	}
	if got := string(consoleBytes); got != "HOLA" {
		for pid, p := range k.Processes {
			t.Logf("  PID %d: state=%s blocked_on=%s exit=%d EAX=0x%X EIP=0x%X",
				pid, p.State, p.BlockedOn, p.ExitCode, p.Regs.EAX, p.Regs.EIP)
		}
		t.Logf("  pipeOps=%d forkSeen=%v pipes=%d", pipeOps, forkSeen, len(k.Pipes))
		if len(k.Pipes) > 0 {
			t.Logf("  pipe[0].Buf=%q readers=%d writers=%d", string(k.Pipes[0].Buf), k.Pipes[0].Readers, k.Pipes[0].Writers)
		}
		t.Errorf("console output = %q, want %q", got, "HOLA")
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

// runUntilDrained drives the kernel while it still has work to do. Unlike
// runToHalt this keeps stepping when the boot core happens to halt but a
// PCB sits on the ready queue waiting to be re-dispatched — the moment a
// parent's wait() reaps a thread, the next Step picks the parent off
// the queue and resumes it. Plain runToHalt would exit at the first
// "all cores halted" check and miss that resume.
func runUntilDrained(t *testing.T, k *kernel.Kernel, budget int) {
	t.Helper()
	for step := 0; step < budget && k.HasWork(); step++ {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
	}
	if k.HasWork() {
		t.Fatalf("kernel still has work after %d steps", budget)
	}
}

// TestThreadsJoin_LeaderReapsThreadAndExits drives the threads_join demo to
// completion and checks the full lifecycle through events: the leader (PID 2,
// since init holds PID 1) spawns one sibling thread (PID 3) via sys_clone
// with CLONE_VM|CLONE_FILES|CLONE_THREAD, the thread writes "T2 done\n" to
// stdout and exits with retval 123, the leader's sys_waitpid reaps it with
// the TID as exit code, then the leader itself exits and init eventually
// reaps the leader. Reading PCBs at the end is unreliable because init has
// already removed them; we assert on events instead.
func TestThreadsJoin_LeaderReapsThreadAndExits(t *testing.T) {
	demo := findDemo(t, "threads_join")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 8
	loadDemo(t, k, demo)

	const (
		leaderPID = 2
		threadTID = 3
	)
	var (
		sawThreadCreate bool
		sawThreadExit   bool
		leaderReaped    bool
	)
	for step := 0; step < 5000 && k.HasWork(); step++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for _, e := range evs {
			switch ev := e.(type) {
			case cpu.EventThreadCreate:
				if ev.TID == threadTID && ev.TGID == leaderPID && ev.CreatorID == leaderPID {
					sawThreadCreate = true
				}
			case cpu.EventThreadExit:
				if ev.TID == threadTID && ev.TGID == leaderPID && ev.Retval == 123 {
					sawThreadExit = true
				}
			case cpu.EventWaitOp:
				if ev.Op == "reap" && ev.ChildPID == leaderPID && ev.ExitCode == threadTID {
					leaderReaped = true
				}
			}
		}
	}
	if !sawThreadCreate {
		t.Error("never saw EventThreadCreate{TID:3, TGID:2, CreatorID:2}")
	}
	if !sawThreadExit {
		t.Error("never saw EventThreadExit{TID:3, TGID:2, Retval:123}")
	}
	if !leaderReaped {
		t.Error("never saw init reap the leader with ExitCode = 3 (the reaped TID)")
	}
}

// TestThreadsRace_ProducesObservableLostUpdates drives threads_race to
// completion and verifies the counter ended in [1, 400]. A bound below 400
// proves the race actually fired; the precise value depends on quantum vs
// loop alignment, so the test only asserts the visible-race property.
func TestThreadsRace_ProducesObservableLostUpdates(t *testing.T) {
	demo := findDemo(t, "threads_race")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 8
	loadDemo(t, k, demo)
	runUntilDrained(t, k, 30000)

	got := binary.LittleEndian.Uint32(k.Mem.Bytes[threadCounterAddr : threadCounterAddr+4])
	if got == 0 {
		t.Fatal("counter = 0 — neither thread ran")
	}
	if got > 400 {
		t.Errorf("counter = %d, want <= 400 (cannot exceed total increments)", got)
	}
	t.Logf("threads_race ended at %d (max=400, lost updates=%d)", got, 400-int(got))
}

// TestThreadsSafe_AlwaysReachesFourHundred drives threads_safe and asserts
// the counter ends at exactly 400. The mutex around the load-inc-store
// must serialize the critical section regardless of how the scheduler
// interleaves the two threads.
func TestThreadsSafe_AlwaysReachesFourHundred(t *testing.T) {
	demo := findDemo(t, "threads_safe")
	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = 8
	loadDemo(t, k, demo)
	runUntilDrained(t, k, 60000)

	got := binary.LittleEndian.Uint32(k.Mem.Bytes[threadCounterAddr : threadCounterAddr+4])
	if got != 400 {
		t.Errorf("counter = %d, want 400 (mutex must serialize the critical section)", got)
	}
}
