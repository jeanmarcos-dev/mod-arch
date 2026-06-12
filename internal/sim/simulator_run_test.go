package sim

import (
	"testing"
	"time"
)

// TestLoadProgram_NoTick0Frame verifies load broadcasts no tick_events frame:
// loading does not step the CPU, so tick 0 must belong to the first STEP/RUN.
// A load-time boot frame would later merge with that step and make the tick-0
// row mutate (swapper bind → dispatch).
func TestLoadProgram_NoTick0Frame(t *testing.T) {
	s := New(Config{NumCores: 1, Demos: []Demo{{
		Name:  "add",
		Procs: []Proc{{Name: "add", Bytes: []byte{0xB8, 0x0A, 0x00, 0x00, 0x00, 0xF4}}},
	}}})
	c := NewClient(RoleAny)
	s.AddClient(c)
	defer s.RemoveClient(c)

	if err := s.LoadProgram("add"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	for drainLoop := true; drainLoop; {
		select {
		case msg := <-c.Send:
			if ev, ok := msg.(TickEventsEvent); ok {
				t.Fatalf("LoadProgram broadcast a tick_events frame (tick %d); load must not emit tick-0 events", ev.Tick)
			}
		default:
			drainLoop = false
		}
	}
}

// TestStartRun_Tick0BroadcastOnce guards against StartRun emitting a separate
// boot-dispatch frame for tick 0: that second frame merged with the first
// StepOnce's tick-0 frame and made the event log's tick-0 row visibly mutate
// (core-bind → HLT idle) under RUN but not under STEP. StepOnce owns tick 0.
func TestStartRun_Tick0BroadcastOnce(t *testing.T) {
	s := New(Config{NumCores: 2, Demos: []Demo{{
		Name:  "spin",
		Procs: []Proc{{Name: "p1", Bytes: []byte{0xEB, 0xFE}}}, // JMP -2 (forever)
	}}})
	if err := s.LoadProgram("spin"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	c := NewClient(RoleAny)
	s.AddClient(c)
	defer s.RemoveClient(c)
	drain(c) // discard loaded + snapshot

	s.StartRun(60)
	time.Sleep(100 * time.Millisecond)
	s.StopRun()

	framesByTick := map[uint64]int{}
	for drainLoop := true; drainLoop; {
		select {
		case msg := <-c.Send:
			if ev, ok := msg.(TickEventsEvent); ok {
				framesByTick[ev.Tick]++
			}
		default:
			drainLoop = false
		}
	}

	if framesByTick[0] != 1 {
		t.Fatalf("tick 0 broadcast %d times under RUN, want exactly 1 (StartRun must not pre-broadcast a boot frame)", framesByTick[0])
	}
	if len(framesByTick) < 2 {
		t.Fatalf("expected several ticks to run, got %d distinct ticks", len(framesByTick))
	}
}
