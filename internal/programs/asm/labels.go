package asm

import (
	"encoding/binary"
	"fmt"
)

// fixupKind selects how a forward (or backward) jump is patched at Build() time.
type fixupKind uint8

const (
	fixupRel8  fixupKind = iota // 1-byte signed offset (JMP/Jcc short)
	fixupRel32                  // 4-byte signed offset (JMP/CALL long)
)

// fixup describes one pending jump whose target is a named label. `at` points
// at the first BYTE of the displacement (one past the opcode for rel8, one
// past the opcode for rel32). `nextEIP` is the offset of the instruction that
// FOLLOWS the jump — that's the reference point x86 uses for relative
// displacements.
type fixup struct {
	at      int       // byte offset where the displacement starts
	nextEIP int       // offset of the next instruction (for rel computation)
	target  string    // label name to resolve
	kind    fixupKind // rel8 or rel32
}

// resolve patches every fixup in place inside `bytes`, using the label table
// `labels` (label name → byte offset within bytes). Returns an error if a
// label is unknown or if a rel8 displacement does not fit in a signed byte.
func resolveFixups(bytes []byte, labels map[string]int, fixups []fixup) error {
	for _, fx := range fixups {
		target, ok := labels[fx.target]
		if !ok {
			return fmt.Errorf("label %q referenced by jump at 0x%X is undefined", fx.target, fx.at)
		}
		disp := int64(target) - int64(fx.nextEIP)
		switch fx.kind {
		case fixupRel8:
			if disp < -128 || disp > 127 {
				return fmt.Errorf("rel8 jump to %q from 0x%X overflows ±127 (disp=%d) — use the rel32 form",
					fx.target, fx.at, disp)
			}
			bytes[fx.at] = byte(int8(disp))
		case fixupRel32:
			binary.LittleEndian.PutUint32(bytes[fx.at:fx.at+4], uint32(int32(disp)))
		}
	}
	return nil
}
