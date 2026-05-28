package asm

import (
	"fmt"
)

// SourceLine mirrors programs.SourceLine. We redeclare it here so this package
// can be compiled without importing `programs` (which would cycle through tests).
// The builder's Build() helper converts back to the parent type when the demo
// wires the Proc into the catalogue.
type SourceLine struct {
	Offset   uint32
	Mnemonic string
	CLine    int
}

// Proc is what the builder hands back. It's a plain value object that demos
// embed in their programs.Proc. Build() copies these into the parent type;
// the parent never holds an asm.Builder.
type Proc struct {
	Name      string
	Bytes     []byte
	Source    string
	SourceMap []SourceLine
}

// Builder accumulates bytes, SourceMap entries and label fixups. It is the
// only stateful object the DSL exposes. Builders are NOT safe for concurrent
// use; one builder per Proc, no sharing.
type Builder struct {
	name      string
	source    string
	bytes     []byte
	sourceMap []SourceLine
	labels    map[string]int
	fixups    []fixup

	// curLine is the C source line every subsequent instruction will map to
	// until .At(n) changes it. 0 means "no line context set" — emitting an
	// instruction in that state panics, because a missing CLine breaks the
	// CodeAsmSplit panel and we want the failure to be obvious during
	// catalogue build, not silently in the UI.
	curLine int
	// curComment is the suffix appended to the auto-mnemonic of the NEXT
	// instruction emitted. Cleared after each instruction so it does not
	// leak. ".Comment()" sets it.
	curComment string
}

// NewProc starts a fresh builder under the given Proc name.
func NewProc(name string) *Builder {
	return &Builder{
		name:   name,
		labels: map[string]int{},
	}
}

// Source attaches the C-like text the alumno will see in the simulator. It is
// stored verbatim — no parsing, no transformation.
func (b *Builder) Source(text string) *Builder {
	b.source = text
	return b
}

// At sets the CLine that subsequent instructions map to. The setting is sticky
// until another .At() call. Line numbers are 1-based, matching the SourceMap
// invariant.
func (b *Builder) At(cLine int) *Builder {
	if cLine < 1 {
		panic(fmt.Sprintf("asm: At(%d) must be 1-based", cLine))
	}
	b.curLine = cLine
	return b
}

// Comment sets a suffix that will be appended to the next instruction's
// mnemonic, separated by " ; ". Cleared after that one instruction emits.
// Use to add pedagogical context (e.g. "; load", "; exit code").
func (b *Builder) Comment(text string) *Builder {
	b.curComment = text
	return b
}

// emit appends `enc` to the byte stream and adds one SourceMap entry whose
// mnemonic is `mnem` (plus the pending Comment, if any). Helpers like .Data()
// that produce raw payload bytes bypass this and call b.bytes directly.
func (b *Builder) emit(enc []byte, mnem string) *Builder {
	if b.curLine == 0 {
		panic(fmt.Sprintf("asm: emit %q at offset 0x%X without .At() — set the C line first",
			mnem, len(b.bytes)))
	}
	full := mnem
	if b.curComment != "" {
		full = mnem + " ; " + b.curComment
		b.curComment = ""
	}
	b.sourceMap = append(b.sourceMap, SourceLine{
		Offset:   uint32(len(b.bytes)),
		Mnemonic: full,
		CLine:    b.curLine,
	})
	b.bytes = append(b.bytes, enc...)
	return b
}

// ─── Instructions ──────────────────────────────────────────────────────────

// MovImm emits MOV r32, imm32 (B8+rd ib32). Always the 5-byte form.
func (b *Builder) MovImm(dst Reg, imm uint32) *Builder {
	return b.emit(encMovImm32(dst, imm), fmt.Sprintf("MOV %s, 0x%X", dst, imm))
}

// MovReg emits MOV dst, src (89 /r, mod=11).
func (b *Builder) MovReg(dst, src Reg) *Builder {
	return b.emit(encMovRegReg(dst, src), fmt.Sprintf("MOV %s, %s", dst, src))
}

// MovEAXMem emits MOV [addr], EAX (A3). Only EAX is supported as source by
// the underlying ISA.
func (b *Builder) MovEAXMem(addr uint32) *Builder {
	return b.emit(encMovEAXMem(addr), fmt.Sprintf("MOV [0x%X], EAX", addr))
}

// MovMemEAX emits MOV EAX, [addr] (A1). Only EAX is supported as destination.
func (b *Builder) MovMemEAX(addr uint32) *Builder {
	return b.emit(encMovMemEAX(addr), fmt.Sprintf("MOV EAX, [0x%X]", addr))
}

// AddReg emits ADD dst, src.
func (b *Builder) AddReg(dst, src Reg) *Builder {
	return b.emit(encAddRegReg(dst, src), fmt.Sprintf("ADD %s, %s", dst, src))
}

// SubReg emits SUB dst, src.
func (b *Builder) SubReg(dst, src Reg) *Builder {
	return b.emit(encSubRegReg(dst, src), fmt.Sprintf("SUB %s, %s", dst, src))
}

// CmpReg emits CMP a, b (flags only, no write).
func (b *Builder) CmpReg(a, c Reg) *Builder {
	return b.emit(encCmpRegReg(a, c), fmt.Sprintf("CMP %s, %s", a, c))
}

// XorReg emits XOR dst, src.
func (b *Builder) XorReg(dst, src Reg) *Builder {
	return b.emit(encXorRegReg(dst, src), fmt.Sprintf("XOR %s, %s", dst, src))
}

// CmpImm emits CMP r, imm32 (81 /7).
func (b *Builder) CmpImm(r Reg, imm uint32) *Builder {
	return b.emit(encArithImm32(7, r, imm), fmt.Sprintf("CMP %s, 0x%X", r, imm))
}

// AddImm emits ADD r, imm32 (81 /0).
func (b *Builder) AddImm(r Reg, imm uint32) *Builder {
	return b.emit(encArithImm32(0, r, imm), fmt.Sprintf("ADD %s, 0x%X", r, imm))
}

// SubImm emits SUB r, imm32 (81 /5).
func (b *Builder) SubImm(r Reg, imm uint32) *Builder {
	return b.emit(encArithImm32(5, r, imm), fmt.Sprintf("SUB %s, 0x%X", r, imm))
}

// AndImm emits AND r, imm32 (81 /4).
func (b *Builder) AndImm(r Reg, imm uint32) *Builder {
	return b.emit(encArithImm32(4, r, imm), fmt.Sprintf("AND %s, 0x%X", r, imm))
}

// XorImm emits XOR r, imm32 (81 /6).
func (b *Builder) XorImm(r Reg, imm uint32) *Builder {
	return b.emit(encArithImm32(6, r, imm), fmt.Sprintf("XOR %s, 0x%X", r, imm))
}

// Inc emits INC r32 (40+rd).
func (b *Builder) Inc(r Reg) *Builder {
	return b.emit(encIncReg(r), fmt.Sprintf("INC %s", r))
}

// Dec emits DEC r32 (48+rd).
func (b *Builder) Dec(r Reg) *Builder {
	return b.emit(encDecReg(r), fmt.Sprintf("DEC %s", r))
}

// Push emits PUSH r32 (50+rd).
func (b *Builder) Push(r Reg) *Builder {
	return b.emit(encPushReg(r), fmt.Sprintf("PUSH %s", r))
}

// Pop emits POP r32 (58+rd).
func (b *Builder) Pop(r Reg) *Builder {
	return b.emit(encPopReg(r), fmt.Sprintf("POP %s", r))
}

// Int emits INT imm8 (CD ib).
func (b *Builder) Int(vec uint8) *Builder {
	return b.emit(encInt(vec), fmt.Sprintf("INT 0x%X", vec))
}

// Ret emits RET (C3).
func (b *Builder) Ret() *Builder { return b.emit(encRet, "RET") }

// Iret emits IRET (CF).
func (b *Builder) Iret() *Builder { return b.emit(encIret, "IRET") }

// Hlt emits HLT (F4). Privileged — user-mode programs page-fault on it.
func (b *Builder) Hlt() *Builder { return b.emit(encHlt, "HLT") }

// Nop emits NOP (90). Useful for padding loops to align targets.
func (b *Builder) Nop() *Builder { return b.emit(encNop, "NOP") }

// Cli emits CLI (FA). Privileged.
func (b *Builder) Cli() *Builder { return b.emit(encCli, "CLI") }

// Sti emits STI (FB). Privileged.
func (b *Builder) Sti() *Builder { return b.emit(encSti, "STI") }

// ─── Control flow ──────────────────────────────────────────────────────────

// Jmp emits JMP rel8 (EB cb). The label may be defined before (backward) or
// after (forward) this call; resolution happens at Build(). Panics at Build()
// if the displacement does not fit in ±127.
func (b *Builder) Jmp(label string) *Builder {
	mnem := fmt.Sprintf("JMP %s", label)
	b.addRel8Jump(0xEB, label, mnem)
	return b
}

// Je emits JE/JZ rel8 (74 cb).
func (b *Builder) Je(label string) *Builder {
	b.addRel8Jump(0x74, label, fmt.Sprintf("JE %s", label))
	return b
}

// Jne emits JNE/JNZ rel8 (75 cb).
func (b *Builder) Jne(label string) *Builder {
	b.addRel8Jump(0x75, label, fmt.Sprintf("JNE %s", label))
	return b
}

// JmpFar emits JMP rel32 (E9 cd). Use when the target may be more than ±127
// bytes away. There is no Jcc rel32 in this ISA — long conditional jumps must
// be split (Jcc short to a JMP rel32 trampoline) by the demo author.
func (b *Builder) JmpFar(label string) *Builder {
	b.addRel32Jump(0xE9, label, fmt.Sprintf("JMP_FAR %s", label))
	return b
}

// Call emits CALL rel32 (E8 cd).
func (b *Builder) Call(label string) *Builder {
	b.addRel32Jump(0xE8, label, fmt.Sprintf("CALL %s", label))
	return b
}

func (b *Builder) addRel8Jump(opcode byte, label, mnem string) {
	if b.curLine == 0 {
		panic(fmt.Sprintf("asm: jump %q at offset 0x%X without .At()", mnem, len(b.bytes)))
	}
	full := mnem
	if b.curComment != "" {
		full = mnem + " ; " + b.curComment
		b.curComment = ""
	}
	b.sourceMap = append(b.sourceMap, SourceLine{
		Offset:   uint32(len(b.bytes)),
		Mnemonic: full,
		CLine:    b.curLine,
	})
	b.bytes = append(b.bytes, opcode, 0x00) // placeholder
	b.fixups = append(b.fixups, fixup{
		at:      len(b.bytes) - 1,
		nextEIP: len(b.bytes),
		target:  label,
		kind:    fixupRel8,
	})
}

func (b *Builder) addRel32Jump(opcode byte, label, mnem string) {
	if b.curLine == 0 {
		panic(fmt.Sprintf("asm: jump %q at offset 0x%X without .At()", mnem, len(b.bytes)))
	}
	full := mnem
	if b.curComment != "" {
		full = mnem + " ; " + b.curComment
		b.curComment = ""
	}
	b.sourceMap = append(b.sourceMap, SourceLine{
		Offset:   uint32(len(b.bytes)),
		Mnemonic: full,
		CLine:    b.curLine,
	})
	b.bytes = append(b.bytes, opcode, 0x00, 0x00, 0x00, 0x00) // placeholder
	b.fixups = append(b.fixups, fixup{
		at:      len(b.bytes) - 4,
		nextEIP: len(b.bytes),
		target:  label,
		kind:    fixupRel32,
	})
}

// ─── Structure ─────────────────────────────────────────────────────────────

// Label registers the current byte offset under `name`. Labels are scoped to
// the builder; collisions panic immediately (catching a duplicate label is
// cheap, debugging a wrong jump silently is not).
func (b *Builder) Label(name string) *Builder {
	if _, exists := b.labels[name]; exists {
		panic(fmt.Sprintf("asm: duplicate label %q at offset 0x%X", name, len(b.bytes)))
	}
	b.labels[name] = len(b.bytes)
	return b
}

// LabelOffset returns the byte offset previously registered for `name`,
// panicking if the label is not yet defined. Use to embed in-binary offsets
// as immediates (e.g. the absolute EIP of a thread entry point passed to
// sys_clone).
func (b *Builder) LabelOffset(name string) uint32 {
	off, ok := b.labels[name]
	if !ok {
		panic(fmt.Sprintf("asm: LabelOffset(%q) called before label is defined", name))
	}
	return uint32(off)
}

// PadTo emits NOPs until the current offset reaches `off`. Panics if the
// current offset is already past `off` (we cannot undo emitted bytes).
// PadTo entries do NOT add SourceMap lines — they are alignment, not code
// the alumno is meant to read.
func (b *Builder) PadTo(off uint32) *Builder {
	cur := uint32(len(b.bytes))
	if cur > off {
		panic(fmt.Sprintf("asm: PadTo(0x%X) but current offset is already 0x%X", off, cur))
	}
	for cur < off {
		b.bytes = append(b.bytes, 0x90)
		cur++
	}
	return b
}

// Data appends raw bytes. Use for in-binary literals like message strings
// embedded next to the code. Data does not add to SourceMap.
func (b *Builder) Data(bs ...byte) *Builder {
	b.bytes = append(b.bytes, bs...)
	return b
}

// DataStr appends the bytes of s. Use for in-binary message strings.
func (b *Builder) DataStr(s string) *Builder {
	b.bytes = append(b.bytes, []byte(s)...)
	return b
}

// Build resolves all label fixups and returns the finished Proc. Panics on
// unresolved labels or rel8 overflow — both indicate a bug in the demo
// declaration that we want to surface at catalogue construction (which
// happens once at startup), not silently at runtime.
func (b *Builder) Build() Proc {
	if err := resolveFixups(b.bytes, b.labels, b.fixups); err != nil {
		panic(fmt.Sprintf("asm[%s]: %v", b.name, err))
	}
	return Proc{
		Name:      b.name,
		Bytes:     append([]byte(nil), b.bytes...),
		Source:    b.source,
		SourceMap: append([]SourceLine(nil), b.sourceMap...),
	}
}
