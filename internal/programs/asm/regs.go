// Package asm builds x86 binaries for the demo catalogue without the author
// having to hand-encode bytes. The generated Proc contains:
//
//   - Bytes:     opcode stream identical to what a hand-written demo would emit.
//   - Source:    C-like text supplied verbatim by the author (the DSL never
//     synthesizes this — pedagogical wording is human work).
//   - SourceMap: one entry per instruction, generated automatically from the
//     builder calls and the .At() / .Comment() context.
//
// The DSL targets the ISA implemented in pkg/cpu/isa.go: 32-bit register file,
// reg-reg ModR/M only, no SIB. Instructions outside that subset are not
// exposed.
package asm

// Reg is a 32-bit general-purpose register. Values match the Intel register
// index used by ModR/M encoding (the same indexing pkg/cpu uses internally).
type Reg uint8

// Register constants. The order is the Intel canonical one and MUST be kept in
// sync with pkg/cpu.regNames so disassembly mnemonics line up.
const (
	EAX Reg = 0
	ECX Reg = 1
	EDX Reg = 2
	EBX Reg = 3
	ESP Reg = 4
	EBP Reg = 5
	ESI Reg = 6
	EDI Reg = 7
)

// String returns the canonical uppercase name (EAX, ECX, …). Used by the
// builder to render mnemonics in the SourceMap.
func (r Reg) String() string {
	switch r {
	case EAX:
		return "EAX"
	case ECX:
		return "ECX"
	case EDX:
		return "EDX"
	case EBX:
		return "EBX"
	case ESP:
		return "ESP"
	case EBP:
		return "EBP"
	case ESI:
		return "ESI"
	case EDI:
		return "EDI"
	default:
		return "?"
	}
}
