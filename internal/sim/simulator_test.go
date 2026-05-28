package sim

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// TestSetNumCores_RebuildsKernel covers the happy path: switching from 1 to 3
// cores must produce a kernel with 3 cores and clear any previous process
// state. The simulator broadcasts a fresh snapshot to existing clients.
func TestSetNumCores_RebuildsKernel(t *testing.T) {
	s := New(Config{NumCores: 1})
	if got := s.kernel.NumCores(); got != 1 {
		t.Fatalf("initial NumCores = %d, want 1", got)
	}

	if err := s.SetNumCores(3); err != nil {
		t.Fatalf("SetNumCores(3): %v", err)
	}
	if got := s.kernel.NumCores(); got != 3 {
		t.Errorf("after SetNumCores(3) NumCores = %d, want 3", got)
	}
	if len(s.kernel.Processes) != 0 {
		t.Errorf("process table not cleared: %d entries", len(s.kernel.Processes))
	}
}

// TestSetNumCores_RejectsOutOfRange ensures the simulator refuses invalid
// counts instead of silently clamping. The caller (the WS dispatcher) surfaces
// the error to the user so the SimBar can show a toast.
func TestSetNumCores_RejectsOutOfRange(t *testing.T) {
	s := New(Config{NumCores: 2})
	cases := []int{0, -1, kernel.MaxCores + 1, 99}
	for _, n := range cases {
		if err := s.SetNumCores(n); err == nil {
			t.Errorf("SetNumCores(%d) accepted, want error", n)
		}
	}
	if got := s.kernel.NumCores(); got != 2 {
		t.Errorf("NumCores changed after rejected SetNumCores calls: %d", got)
	}
}

// TestSetNumCores_NoOpWhenSame verifies that requesting the current count is
// idempotent — useful so the UI can fire the command without checking and not
// pay the cost of a full kernel rebuild.
func TestSetNumCores_NoOpWhenSame(t *testing.T) {
	s := New(Config{NumCores: 2})
	k := s.kernel
	if err := s.SetNumCores(2); err != nil {
		t.Fatalf("SetNumCores(2): %v", err)
	}
	if s.kernel != k {
		t.Error("kernel was rebuilt despite same NumCores")
	}
}

// TestLoadProgram_DemoCoresSwitchesLayout verifies that loading a demo whose
// Cores field is set rebuilds the kernel with that core count before procs
// are wired in. This is the mechanism the showcase demo uses to guarantee 4
// cores regardless of MODOS_CPU_CORES.
func TestLoadProgram_DemoCoresSwitchesLayout(t *testing.T) {
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "tiny_quad",
			Cores: 4,
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}}, // single HLT
		}},
	})
	if got := s.kernel.NumCores(); got != 1 {
		t.Fatalf("initial NumCores = %d, want 1", got)
	}

	if err := s.LoadProgram("tiny_quad"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	if got := s.kernel.NumCores(); got != 4 {
		t.Errorf("after LoadProgram NumCores = %d, want 4", got)
	}
	// LoadProgram now spawns init (PID 1) unconditionally on top of the
	// demo's own processes, so we expect 2 procs (init + tiny_quad's p1).
	if got := len(s.kernel.Processes); got != 2 {
		t.Errorf("Processes after load = %d, want 2 (init + 1 demo proc)", got)
	}
}

// TestLoadProgram_DemoCoresZeroPreservesLayout verifies legacy demos with
// Cores=0 do not rebuild the kernel — the existing core count is kept.
func TestLoadProgram_DemoCoresZeroPreservesLayout(t *testing.T) {
	s := New(Config{
		NumCores: 2,
		Demos: []Demo{{
			Name:  "legacy",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	k := s.kernel
	if err := s.LoadProgram("legacy"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	if s.kernel != k {
		t.Error("kernel was rebuilt despite Cores=0")
	}
	if got := s.kernel.NumCores(); got != 2 {
		t.Errorf("NumCores changed: got %d, want 2", got)
	}
}

// TestStepOnce_BroadcastsProcsOnFork verifies that a step that triggers a
// process-table mutation (sys_fork here) emits a {type:"procs"} broadcast
// alongside the individual events. Without it the frontend's local PCB list
// never learns about the child and subsequent pcb_save / process_state events
// for the new PID are silently dropped because find() returns undefined.
func TestStepOnce_BroadcastsProcsOnFork(t *testing.T) {
	// mov eax, 2 ; int 0x80 ; hlt — minimum bytes to call sys_fork and stop.
	forkProgram := []byte{
		0xB8, 0x02, 0x00, 0x00, 0x00,
		0xCD, 0x80,
		0xF4,
	}
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "fork_min",
			Procs: []Proc{{Name: "p1", Bytes: forkProgram}},
		}},
	})
	if err := s.LoadProgram("fork_min"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	client := NewClient(RoleAny)
	s.AddClient(client)
	defer s.RemoveClient(client)
	// Drain any messages enqueued by LoadProgram (loaded + initial snapshot).
	drain(client)

	// Step until either the fork happens or we give up. The exact tick count
	// depends on how the kernel interleaves boot dispatch with the demo's
	// instructions; cap generously so the test stays robust if scheduling
	// gains an extra stage.
	const maxSteps = 64
	sawProcs := false
	var procsMsg map[string]any
	for i := 0; i < maxSteps && !sawProcs; i++ {
		if err := s.StepOnce(); err != nil {
			t.Fatalf("StepOnce #%d: %v", i, err)
		}
		for {
			select {
			case msg := <-client.Send:
				if msg["type"] == "procs" {
					sawProcs = true
					procsMsg = msg
				}
			default:
				goto drained
			}
		}
	drained:
	}

	if !sawProcs {
		t.Fatalf("no {type:procs} broadcast observed after %d steps", maxSteps)
	}
	procs, ok := procsMsg["procs"].([]map[string]any)
	if !ok {
		t.Fatalf("procs broadcast missing procs slice: %#v", procsMsg)
	}
	// Demo proc plus init plus the new child created by sys_fork. We only
	// assert >= 3 so a future change that adds extra bookkeeping procs does
	// not break the test.
	if len(procs) < 3 {
		t.Errorf("procs list has %d entries, want >= 3 (init + parent + child)", len(procs))
	}
}

// TestProcTableMutated covers the per-event-type detection used by StepOnce
// to decide when a fresh procs snapshot must be broadcast. Each event type is
// listed explicitly so adding a new mutator event without updating the
// detector fails this test instead of silently regressing the panel.
func TestProcTableMutated(t *testing.T) {
	cases := []struct {
		name string
		ev   cpu.Event
		want bool
	}{
		{"fork", cpu.EventFork{ParentPID: 1, ChildPID: 2}, true},
		{"thread_create", cpu.EventThreadCreate{TID: 3, TGID: 2, LeaderPID: 2, CreatorID: 2}, true},
		{"thread_exit", cpu.EventThreadExit{TID: 3, TGID: 2}, true},
		{"reparent", cpu.EventReparent{PID: 3, OldParent: 2, NewParent: 1}, true},
		{"wait_op reap", cpu.EventWaitOp{PID: 1, Op: "reap"}, true},
		{"wait_op call", cpu.EventWaitOp{PID: 1, Op: "call"}, false},
		{"instruction", cpu.EventInstruction{}, false},
		{"halt", cpu.EventHalt{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := procTableMutated([]cpu.Event{c.ev}); got != c.want {
				t.Errorf("procTableMutated(%T) = %v, want %v", c.ev, got, c.want)
			}
		})
	}
}

// drain empties any messages already buffered on the client's Send channel.
// Used to isolate test assertions from setup-time broadcasts (loaded,
// snapshot, programs, etc.).
func drain(c *Client) {
	for {
		select {
		case <-c.Send:
		default:
			return
		}
	}
}
