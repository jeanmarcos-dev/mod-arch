package programs

import (
	"strings"
	"testing"
)

// TestSourceMapInvariants walks every Proc in every Demo and validates the
// invariants that the UI relies on:
//
//   - The first SourceLine is at offset 0 (the entry point).
//   - Offsets are strictly monotonically increasing (each instruction maps to
//     exactly one entry).
//   - All offsets fit within len(Bytes).
//   - CLine is 1-based and never exceeds the number of lines in Source.
//   - Mnemonic is non-empty.
//
// Procs without a SourceMap are allowed (the UI degrades gracefully); they
// just skip the check.
func TestSourceMapInvariants(t *testing.T) {
	t.Parallel()

	for _, demo := range Demos() {
		t.Run(demo.Name, func(t *testing.T) {
			t.Parallel()

			for _, p := range demo.Procs {
				t.Run(p.Name, func(t *testing.T) {
					if len(p.SourceMap) == 0 {
						t.Skipf("no SourceMap declared for %q (legacy demo or data-only proc)", p.Name)
						return
					}
					if p.Source == "" {
						t.Fatalf("SourceMap declared but Source is empty — they only make sense together")
					}

					sourceLines := strings.Count(p.Source, "\n")
					// Source typically ends with a trailing newline; the last
					// instruction may map to the line just before it, so we
					// count all non-empty + the trailing newline.
					if !strings.HasSuffix(p.Source, "\n") {
						sourceLines++
					}

					if p.SourceMap[0].Offset != 0 {
						t.Errorf("first SourceLine.Offset = 0x%x, want 0", p.SourceMap[0].Offset)
					}

					var prev int64 = -1
					for i, sl := range p.SourceMap {
						if sl.Mnemonic == "" {
							t.Errorf("SourceMap[%d]: Mnemonic is empty", i)
						}
						if int(sl.Offset) >= len(p.Bytes) {
							t.Errorf("SourceMap[%d]: Offset 0x%x out of range (len(Bytes)=%d)",
								i, sl.Offset, len(p.Bytes))
						}
						if int64(sl.Offset) <= prev {
							t.Errorf("SourceMap[%d]: Offset 0x%x not strictly > previous 0x%x",
								i, sl.Offset, prev)
						}
						prev = int64(sl.Offset)
						if sl.CLine < 1 {
							t.Errorf("SourceMap[%d]: CLine=%d, must be 1-based", i, sl.CLine)
						}
						if sl.CLine > sourceLines {
							t.Errorf("SourceMap[%d]: CLine=%d exceeds Source line count (%d)",
								i, sl.CLine, sourceLines)
						}
					}
				})
			}
		})
	}
}
