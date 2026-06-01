package sim

import (
	"testing"
)

// TestSetIdleTaskEnabled_TogglesFlag is the primary contract: the WS command
// flips a kernel-level switch, and any subsequent LoadProgram observes it.
// The simulator must guard the kernel mutation with s.mu so concurrent
// callers (the run-loop goroutine and the WS handler) cannot race. Default
// is ON to match the real-OS model (Silberschatz §5.5 SMP idle thread).
func TestSetIdleTaskEnabled_TogglesFlag(t *testing.T) {
	s := New(Config{NumCores: 1})
	if !s.kernel.IdleTaskEnabled {
		t.Fatal("fresh simulator has IdleTaskEnabled=false; default must be true so the swapper is wired up out of the box")
	}

	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}
	if s.kernel.IdleTaskEnabled {
		t.Error("SetIdleTaskEnabled(false) left the kernel flag true")
	}

	if err := s.SetIdleTaskEnabled(true); err != nil {
		t.Fatalf("SetIdleTaskEnabled(true): %v", err)
	}
	if !s.kernel.IdleTaskEnabled {
		t.Error("SetIdleTaskEnabled(true) left the kernel flag false")
	}
}

// TestSetIdleTaskEnabled_SpawnsImmediatelyWhenLoaded covers mid-simulation
// activation: if a demo is already running and the user flips the toggle ON,
// PID 0 must appear right away so the next Step can bind it to idle cores.
// Otherwise the user would have to reload the demo to see the swapper.
func TestSetIdleTaskEnabled_SpawnsImmediatelyWhenLoaded(t *testing.T) {
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	// Force OFF first so the test exercises the "flip ON mid-run" path.
	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}
	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	if len(s.kernel.IdleByCore) != 0 {
		t.Fatal("setup: IdleByCore already populated before SetIdleTaskEnabled")
	}

	if err := s.SetIdleTaskEnabled(true); err != nil {
		t.Fatalf("SetIdleTaskEnabled: %v", err)
	}

	if len(s.kernel.IdleByCore) == 0 || s.kernel.IdleByCore[0] == nil {
		t.Error("IdleByCore still empty after SetIdleTaskEnabled(true) on a loaded demo")
	}
}

// TestSetIdleTaskEnabled_EvictsButKeepsPCB covers turning idle off mid-run.
// PID 0 must be unbound from every core (RunningIdle=false) and the cores
// parked halted so the legacy auto-stop semantics resume. The PCB itself
// stays in the table so flipping back ON is a no-op spawn instead of a
// memory allocation thrash.
func TestSetIdleTaskEnabled_EvictsButKeepsPCB(t *testing.T) {
	s := New(Config{
		NumCores: 2,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	if err := s.SetIdleTaskEnabled(true); err != nil {
		t.Fatalf("SetIdleTaskEnabled(true): %v", err)
	}
	_ = s.kernel.DispatchIdleCores() // bind PID 0 to whatever cores remain idle

	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}

	for _, c := range s.kernel.Cores {
		if c.RunningIdle {
			t.Errorf("Core %d still RunningIdle after disabling the toggle", c.ID)
		}
	}
	if len(s.kernel.IdleByCore) == 0 || s.kernel.IdleByCore[0] == nil {
		t.Error("IdleByCore cleared after SetIdleTaskEnabled(false); per-core PCBs should stay dormant to avoid spawn thrash")
	}
}

// TestLoadProgram_SpawnsIdleWhenEnabled covers the standard wiring: with the
// kernel flag already on (e.g. the user pre-flipped it via WS, or it survived
// a Reset from the previous demo), LoadProgram must spawn PID 0 alongside
// init so dispatching can bind it on the first tick.
func TestLoadProgram_SpawnsIdleWhenEnabled(t *testing.T) {
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	s.kernel.IdleTaskEnabled = true

	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	if len(s.kernel.IdleByCore) == 0 || s.kernel.IdleByCore[0] == nil {
		t.Error("IdleByCore not populated by LoadProgram when IdleTaskEnabled was true")
	}
}

// TestLoadProgram_BindsSwapperToEveryCore covers the boot-bind contract: with
// the default-on toggle, every core must already have the swapper bound after
// LoadProgram returns — without the user having pressed RUN or stepped once.
// The student sees swappers occupying the cores at load time instead of empty
// slots that only fill on the first tick; the swapper→user transition that
// follows is then observable in the event log when RUN dispatches.
func TestLoadProgram_BindsSwapperToEveryCore(t *testing.T) {
	s := New(Config{
		NumCores: 2,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	for _, c := range s.kernel.Cores {
		if c.CurrentPID != 0 {
			t.Errorf("core %d CurrentPID = %d, want 0 (swapper) after LoadProgram", c.ID, c.CurrentPID)
		}
		if !c.RunningIdle {
			t.Errorf("core %d RunningIdle = false, want true after LoadProgram", c.ID)
		}
	}
}

// TestLoadProgram_NoBootBindWhenIdleDisabled is the regression guard for the
// legacy path: with the toggle off, LoadProgram must not bind anything to the
// cores — they stay in the "no proc" sentinel so the first Step/StartRun
// dispatches the demo's procs directly into freshly empty cores.
func TestLoadProgram_NoBootBindWhenIdleDisabled(t *testing.T) {
	s := New(Config{
		NumCores: 2,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}
	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	for _, c := range s.kernel.Cores {
		if c.RunningIdle {
			t.Errorf("core %d RunningIdle = true with idle disabled; the legacy path must leave cores unbound", c.ID)
		}
	}
}

// TestLoadProgram_NoIdleWhenDisabled guards the default-off regression: every
// existing demo's process table size must stay unchanged when the toggle is
// not flipped. Adding PID 0 unconditionally would break demos that count
// procs as a sanity check.
func TestLoadProgram_NoIdleWhenDisabled(t *testing.T) {
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	// Default is ON; the test asserts the OFF branch, so flip first.
	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}
	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	if len(s.kernel.IdleByCore) != 0 {
		t.Error("IdleByCore populated after LoadProgram with idle disabled")
	}
}

// TestProcessListLocked_IncludesPID0 verifies the wire payload exposes the
// swapper when idle is enabled. The frontend's ProcessesPanel renders the
// compact PID-0 band off this list; without is_idle in the snapshot, the
// band can't be told apart from a regular user proc that happens to have
// no parent.
func TestProcessListLocked_IncludesPID0(t *testing.T) {
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	s.kernel.IdleTaskEnabled = true
	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	procs := s.processListLocked()

	var idle map[string]any
	for _, p := range procs {
		if pid, _ := p["pid"].(int); pid == 0 {
			idle = p
			break
		}
	}
	if idle == nil {
		t.Fatal("processListLocked omitted PID 0 even though IdleTaskEnabled is true")
	}
	if got, _ := idle["is_idle"].(bool); !got {
		t.Errorf("PID 0 snapshot is_idle = %v, want true", idle["is_idle"])
	}
}

// TestProcessListLocked_OmitsPID0WhenDisabled covers the default-off
// regression. With the toggle off, the snapshot must stay byte-for-byte
// compatible with the legacy schema.
func TestProcessListLocked_OmitsPID0WhenDisabled(t *testing.T) {
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}
	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	for _, p := range s.processListLocked() {
		if pid, _ := p["pid"].(int); pid == 0 {
			t.Error("processListLocked contained PID 0 with idle disabled")
		}
	}
}

// TestStepOnce_NoHaltedBroadcastWithIdle covers the "freezes the UI" gotcha:
// with idle ON, every core is Halted while the swapper waits for an IRQ, so
// AllHalted() naturally returns true on most ticks. Emitting the legacy
// {type:"state", halted:true} broadcast in that situation would push the
// frontend into its terminal state. The gate must suppress that event when
// idle is enabled — the run loop is still alive, just sleeping.
func TestStepOnce_NoHaltedBroadcastWithIdle(t *testing.T) {
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "hlt",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}}, // immediate HLT
		}},
	})
	s.kernel.IdleTaskEnabled = true
	if err := s.LoadProgram("hlt"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	// Drain a few steps so the user proc HLTs and the swapper takes over.
	client := NewClient(RoleAny)
	s.AddClient(client)
	defer s.RemoveClient(client)
	for range 30 {
		if err := s.StepOnce(); err != nil {
			t.Fatalf("StepOnce: %v", err)
		}
	}

	// Scan the client's queue for the "machine halted" broadcast, either
	// as a top-level {type:"state"} or piggybacked inside tick_events.
	for drain := true; drain; {
		select {
		case msg := <-client.Send:
			kind, _ := msg["type"].(string)
			halted, _ := msg["halted"].(bool)
			if kind == "state" && halted {
				t.Error("StepOnce emitted {type:state, halted:true} with idle task enabled; this freezes the UI even though the run loop is alive")
			}
			if kind == "tick_events" {
				if state, ok := msg["state"].(map[string]any); ok {
					if h, _ := state["halted"].(bool); h {
						t.Error("StepOnce piggybacked state.halted=true on tick_events with idle task enabled")
					}
				}
			}
		default:
			drain = false
		}
	}
}

// TestCoreSnapshot_ExposesRunningIdle verifies the per-core wire payload
// carries the new flag. The frontend's CpuPanel keys the dedicated HLT
// badge animation off it; without it, the panel could not tell a core
// running the swapper apart from one with no proc at all.
func TestCoreSnapshot_ExposesRunningIdle(t *testing.T) {
	s := New(Config{NumCores: 1})
	if err := s.kernel.SpawnIdle(); err != nil {
		t.Fatalf("SpawnIdle: %v", err)
	}
	s.kernel.IdleTaskEnabled = true
	core := s.kernel.Cores[0]
	s.kernel.DispatchIdleCores() // bind PID 0

	got := coreSnapshot(core)
	if v, _ := got["running_idle"].(bool); !v {
		t.Errorf("coreSnapshot.running_idle = %v after binding PID 0, want true", got["running_idle"])
	}
}
