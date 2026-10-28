package kernel

import (
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// stepUntilWorkDrained runs Step() until PendingKernelWork is empty (or
// maxSteps is exceeded, in which case the test is failed). Returns one slice
// of events per Step — preserving the per-Step boundary lets tests assert
// both event ordering AND the number of clicks the alumno needs to make to
// complete a cascade. Returning [][]cpu.Event (not a flat slice) is what
// makes "this exit takes 5 visible Steps, not 7" expressible.
//
// Use this in any test that previously read evs from a single Step() call
// and assumed the full exit cascade landed in that batch. With deferred
// phases the cascade now spans multiple Steps.
func stepUntilWorkDrained(t *testing.T, k *Kernel, maxSteps int) [][]cpu.Event {
	t.Helper()
	var out [][]cpu.Event
	for range maxSteps {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("stepUntilWorkDrained: Step error: %v", err)
		}
		out = append(out, evs)
		if len(k.PendingKernelWork) == 0 {
			return out
		}
	}
	t.Fatalf("stepUntilWorkDrained: still %d phases pending after %d Steps", len(k.PendingKernelWork), maxSteps)
	return out
}

// flatten concatenates a [][]cpu.Event into a single slice while preserving
// order. Useful when a test only cares about the aggregate events emitted
// during a cascade and not the Step boundaries.
func flatten(stepwise [][]cpu.Event) []cpu.Event {
	var out []cpu.Event
	for _, evs := range stepwise {
		out = append(out, evs...)
	}
	return out
}
