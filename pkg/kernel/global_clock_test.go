package kernel

import (
	"reflect"
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// eventTick reads the Tick field shared by every stamped event via reflection,
// so the invariant test below does not need a type switch over every event.
func eventTick(e cpu.Event) (uint64, bool) {
	v := reflect.ValueOf(e)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0, false
	}
	f := v.FieldByName("Tick")
	if !f.IsValid() || f.Kind() != reflect.Uint64 {
		return 0, false
	}
	return f.Uint(), true
}

// TestStep_GlobalTickUniformAcrossCores is the regression guard for the
// out-of-order event log bug: every event produced by a single Step must carry
// the same tick, and the global clock must advance by exactly one per Step,
// even when cores run and idle independently.
func TestStep_GlobalTickUniformAcrossCores(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	nop4Halt := []byte{0x90, 0x90, 0x90, 0xF4}
	if _, err := k.AddProcess("p1", nop4Halt); err != nil {
		t.Fatal(err)
	}
	if _, err := k.AddProcess("p2", nop4Halt); err != nil {
		t.Fatal(err)
	}

	for step := range 12 {
		clockBefore := k.Clock
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		if k.Clock != clockBefore+1 {
			t.Fatalf("step %d: Clock = %d, want %d", step, k.Clock, clockBefore+1)
		}
		for _, e := range evs {
			tick, ok := eventTick(e)
			if !ok {
				continue
			}
			if tick != clockBefore {
				t.Errorf("step %d: event %T Tick=%d, want %d", step, e, tick, clockBefore)
			}
		}
	}
}
