package sim

import (
	"testing"
	"time"
)

// drainStateEvents pulls every state event currently in the client's send
// buffer and returns them in arrival order. Used by pause tests to inspect
// the paused/running flags broadcast by the simulator.
func drainStateEvents(c *Client, wait time.Duration) []map[string]any {
	deadline := time.Now().Add(wait)
	out := []map[string]any{}
	for {
		select {
		case msg := <-c.Send:
			if t, _ := msg["type"].(string); t == "state" {
				out = append(out, msg)
			}
		default:
			if time.Now().After(deadline) {
				return out
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
}

// TestPause_MarksPausedTrue verifies that explicit user pause (cmd:pause path)
// emits a state event whose paused flag is true. This distinguishes manual
// pause from idle autostop and is what the frontend uses to decide whether to
// freeze visual narrators mid-animation.
func TestPause_MarksPausedTrue(t *testing.T) {
	s := New(Config{NumCores: 1, Demos: []Demo{{
		Name:  "spin",
		Procs: []Proc{{Name: "p1", Bytes: []byte{0xEB, 0xFE}}}, // JMP -2 (forever)
	}}})
	if err := s.LoadProgram("spin"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	c := NewClient(RoleAny)
	s.AddClient(c)

	s.StartRun(0)
	// Drain the running:true state event so we only see what Pause emits.
	drainStateEvents(c, 50*time.Millisecond)

	s.Pause()

	states := drainStateEvents(c, 50*time.Millisecond)
	var sawPaused bool
	for _, st := range states {
		p, hasP := st["paused"]
		r, _ := st["running"].(bool)
		if hasP && p == true && r == false {
			sawPaused = true
			break
		}
	}
	if !sawPaused {
		t.Fatalf("Pause() did not emit state{running:false, paused:true}; got %+v", states)
	}
}

// TestPause_DuringIdleAutostopStillBroadcasts checks the corner case where
// the user presses pause while the simulator is already parked in idle
// autostop (e.g. readkey waiting for a key). The transition is:
//   running=true, paused=false  → autostop → running=false, paused=false
//   user presses pause          → running=false, paused=true
// The second transition must still produce a state event so the frontend
// can flip the UI from "PAUSE (will run on key)" to "RUN (manually paused)".
func TestPause_DuringIdleAutostopStillBroadcasts(t *testing.T) {
	readkey := []byte{
		0xB8, 0x03, 0x00, 0x00, 0x00,
		0xBB, 0x00, 0x00, 0x00, 0x00,
		0xB9, 0xE0, 0xFF, 0x33, 0x00,
		0xBA, 0x01, 0x00, 0x00, 0x00,
		0xCD, 0x80,
		0xEB, 0xFE,
	}
	s := New(Config{NumCores: 1, Demos: []Demo{{
		Name:  "readkey_idle",
		Procs: []Proc{{Name: "p1", Bytes: readkey}},
	}}})
	// This test exercises the legacy auto-stop path that triggers when no
	// process is runnable. The new default (idle ON) keeps the sim alive
	// forever, so disable the swapper to assert the legacy semantics.
	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}
	if err := s.LoadProgram("readkey_idle"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	c := NewClient(RoleAny)
	s.AddClient(c)

	s.StartRun(60)
	drainStateEvents(c, 500*time.Millisecond)
	if s.running {
		t.Fatalf("expected idle autostop before Pause, but s.running is still true")
	}
drain:
	for {
		select {
		case <-c.Send:
		default:
			break drain
		}
	}

	s.Pause()

	states := drainStateEvents(c, 100*time.Millisecond)
	var sawPaused bool
	for _, st := range states {
		if p, ok := st["paused"].(bool); ok && p {
			sawPaused = true
			break
		}
	}
	if !sawPaused {
		t.Fatalf("Pause() during idle autostop did not broadcast state{paused:true}; states=%+v", states)
	}
}

// TestPushKey_ResumesAfterIdleAutostop verifies that injecting a key while
// the simulator is parked in idle autostop (no user pause) brings the run
// loop back to life. The user expects "I never paused — I was just waiting
// for input"; once the input arrives, the loop must resume so the wakened
// reader can keep executing without a second click on RUN.
func TestPushKey_ResumesAfterIdleAutostop(t *testing.T) {
	// readkey: MOV EAX,3 / MOV EBX,0 / MOV ECX,0x33FFE0 / MOV EDX,1 / INT 0x80,
	// then JMP back so HasWork stays true after the wake.
	readkey := []byte{
		0xB8, 0x03, 0x00, 0x00, 0x00, // MOV EAX, 3
		0xBB, 0x00, 0x00, 0x00, 0x00, // MOV EBX, 0
		0xB9, 0xE0, 0xFF, 0x33, 0x00, // MOV ECX, 0x33FFE0
		0xBA, 0x01, 0x00, 0x00, 0x00, // MOV EDX, 1
		0xCD, 0x80, // INT 0x80
		0xEB, 0xFE, // JMP -2
	}
	s := New(Config{NumCores: 1, Demos: []Demo{{
		Name:  "readkey_test",
		Procs: []Proc{{Name: "p1", Bytes: readkey}},
	}}})
	// This test exercises the legacy auto-stop path. Disable the swapper so
	// the run loop actually parks when sys_read blocks.
	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}
	if err := s.LoadProgram("readkey_test"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	c := NewClient(RoleAny)
	s.AddClient(c)

	// Run at the max clock so the 5-instruction lead-in to INT 0x80 finishes
	// inside a tight test window. At 60 Hz the read blocks within ~100 ms.
	s.StartRun(60)
	drainStateEvents(c, 500*time.Millisecond)
	if s.running {
		t.Fatalf("expected idle autostop before PushKey, but s.running is still true")
	}

	// Drain any leftover state events so we only see what PushKey produces.
drainLoop:
	for {
		select {
		case <-c.Send:
		default:
			break drainLoop
		}
	}

	s.PushKey("x")

	states := drainStateEvents(c, 500*time.Millisecond)
	var sawResumed bool
	for _, st := range states {
		if r, ok := st["running"].(bool); ok && r {
			sawResumed = true
			break
		}
	}
	if !sawResumed {
		t.Fatalf("PushKey after idle autostop did not resume the run loop; states=%+v", states)
	}
}

// TestPushKey_DoesNotResumeWhenUserPaused verifies the inverse: when the
// user explicitly paused, a key arriving must NOT silently restart the
// loop. The byte still enters the kernel buffer (and IRQ 0x21 is queued),
// but the simulator stays frozen until the user resumes manually.
func TestPushKey_DoesNotResumeWhenUserPaused(t *testing.T) {
	s := New(Config{NumCores: 1, Demos: []Demo{{
		Name:  "spin",
		Procs: []Proc{{Name: "p1", Bytes: []byte{0xEB, 0xFE}}}, // JMP -2
	}}})
	if err := s.LoadProgram("spin"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	c := NewClient(RoleAny)
	s.AddClient(c)

	s.StartRun(0)
	drainStateEvents(c, 50*time.Millisecond)
	s.Pause()
	drainStateEvents(c, 50*time.Millisecond)
	if s.running {
		t.Fatalf("expected sim to be stopped after Pause()")
	}

	s.PushKey("y")
	time.Sleep(200 * time.Millisecond)
	if s.running {
		t.Fatalf("PushKey after user Pause must not auto-resume the run loop")
	}
}

// TestStopRun_IdleAutostopMarksPausedFalse verifies that the run loop's
// internal autostop (HasWork=false → StopRun) does NOT mark paused. The
// frontend treats this as idle wait, not user pause, so visual narrators
// keep animating naturally to completion.
func TestStopRun_IdleAutostopMarksPausedFalse(t *testing.T) {
	s := New(Config{NumCores: 1, Demos: []Demo{{
		Name:  "halt_now",
		Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}}, // HLT
	}}})
	// Legacy autostop test: disable the swapper so HasWork can actually
	// return false when the lone user proc dies.
	if err := s.SetIdleTaskEnabled(false); err != nil {
		t.Fatalf("SetIdleTaskEnabled(false): %v", err)
	}
	if err := s.LoadProgram("halt_now"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	c := NewClient(RoleAny)
	s.AddClient(c)

	s.StartRun(0)

	// Wait long enough for the run loop to tick once, execute HLT (which
	// raises #GP in user mode and routes through exitProcess), drain the
	// five-phase do_exit cascade across subsequent ticks, see HasWork=false,
	// and self-stop. At the default clock that is one tick for the HLT plus
	// roughly two-three more for the cascade phases — give plenty of slack
	// so this is not flaky under heavy CI load.
	states := drainStateEvents(c, 3000*time.Millisecond)

	// Look for the autostop state event: running:false with paused:false.
	var sawAutoStop bool
	for _, st := range states {
		r, hasR := st["running"]
		if !hasR || r != false {
			continue
		}
		p, hasP := st["paused"]
		if hasP && p == false {
			sawAutoStop = true
			break
		}
	}
	if !sawAutoStop {
		t.Fatalf("idle autostop did not emit state{running:false, paused:false}; got %+v", states)
	}
}
