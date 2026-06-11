package sim

import (
	"fmt"
	"testing"
)

// addProgram: mov eax,0x0A ; mov ebx,0x20 ; add eax,ebx ; hlt. A plain
// arithmetic demo whose user ticks each emit exactly one instruction.
var addProgram = []byte{
	0xB8, 0x0A, 0x00, 0x00, 0x00,
	0xBB, 0x20, 0x00, 0x00, 0x00,
	0x01, 0xD8,
	0xF4,
}

func newAddSim(t *testing.T) *Simulator {
	t.Helper()
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "add_min",
			Procs: []Proc{{Name: "add", Bytes: addProgram}},
		}},
	})
	if err := s.LoadProgram("add_min"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	return s
}

// drainTickFrames pulls every queued message and returns only the tick_events
// frames, as wire maps.
func drainTickFrames(client *Client) []map[string]any {
	var frames []map[string]any
	for {
		select {
		case msg := <-client.Send:
			m := msgAsMap(msg)
			if m["type"] == "tick_events" {
				frames = append(frames, m)
			}
		default:
			return frames
		}
	}
}

func frameEvents(frame map[string]any) []map[string]any {
	raw, _ := frame["events"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		out = append(out, msgAsMap(e))
	}
	return out
}

// TestStepOnce_EmitsAtMostOneTickFrame locks the single-emission contract: one
// StepOnce produces one tick_events broadcast (never two). The duplicated
// event log was a frontend double-connect, but a backend regression that
// double-broadcast a tick would look identical to the user, so we guard it
// here too.
func TestStepOnce_EmitsAtMostOneTickFrame(t *testing.T) {
	s := newAddSim(t)
	client := NewClient(RoleAny)
	s.AddClient(client)
	defer s.RemoveClient(client)
	drain(client) // discard loaded + snapshot

	for i := range 16 {
		if err := s.StepOnce(); err != nil {
			t.Fatalf("StepOnce #%d: %v", i, err)
		}
		frames := drainTickFrames(client)
		if len(frames) > 1 {
			t.Fatalf("StepOnce #%d broadcast %d tick_events frames, want at most 1", i, len(frames))
		}
	}
}

// TestStepOnce_NoDuplicateEventsInFrame verifies a single frame never carries
// the same instruction twice — the exact symptom the user saw ([200005] MOV
// EBX appearing on two rows).
func TestStepOnce_NoDuplicateEventsInFrame(t *testing.T) {
	s := newAddSim(t)
	client := NewClient(RoleAny)
	s.AddClient(client)
	defer s.RemoveClient(client)
	drain(client)

	for i := range 16 {
		if err := s.StepOnce(); err != nil {
			t.Fatalf("StepOnce #%d: %v", i, err)
		}
		for _, frame := range drainTickFrames(client) {
			seen := map[string]bool{}
			for _, ev := range frameEvents(frame) {
				typ, _ := ev["type"].(string)
				if typ != "instruction" {
					continue
				}
				core, _ := ev["core_id"].(float64)
				tick, _ := ev["tick"].(float64)
				eip, _ := ev["eip"].(float64)
				key := fmt.Sprintf("%s:%v:%v:%v", typ, core, tick, eip)
				if seen[key] {
					t.Fatalf("StepOnce #%d emitted duplicate instruction in one frame: core=%v tick=%v eip=%v", i, core, tick, eip)
				}
				seen[key] = true
			}
		}
	}
}

// TestStepOnce_OneInstructionPerTickCore enforces the core atomicity
// guarantee: CPU.Step runs execNext at most once per tick, so a (tick, core)
// cell must carry at most one `instruction` event. Two instructions on the
// same cell is exactly the symptom the user reported (the same MOV on two
// rows). The timer model may legitimately bundle the quantum-expiring
// instruction with the IRQ that preempts it, so we assert specifically on
// instruction count rather than on all execution-class events.
func TestStepOnce_OneInstructionPerTickCore(t *testing.T) {
	s := newAddSim(t)
	client := NewClient(RoleAny)
	s.AddClient(client)
	defer s.RemoveClient(client)
	drain(client)

	for i := range 16 {
		if err := s.StepOnce(); err != nil {
			t.Fatalf("StepOnce #%d: %v", i, err)
		}
		for _, frame := range drainTickFrames(client) {
			instrByCore := map[float64]int{}
			for _, ev := range frameEvents(frame) {
				if typ, _ := ev["type"].(string); typ != "instruction" {
					continue
				}
				core, _ := ev["core_id"].(float64)
				instrByCore[core]++
				if instrByCore[core] > 1 {
					t.Fatalf("StepOnce #%d: core %v carries %d instruction events in one tick frame, want at most 1", i, core, instrByCore[core])
				}
			}
		}
	}
}
