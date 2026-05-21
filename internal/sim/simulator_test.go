package sim

import (
	"testing"

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
	if got := len(s.kernel.Processes); got != 1 {
		t.Errorf("Processes after load = %d, want 1", got)
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
