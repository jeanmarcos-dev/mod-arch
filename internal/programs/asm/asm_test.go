package asm

import (
	"bytes"
	"strings"
	"testing"
)

// TestEncodingSingleInstr pins the byte sequence produced by each instruction
// against the encodings the simulator's ISA expects. Each row is a 1-instr
// program; we test the encoder, not the builder framework around it.
func TestEncodingSingleInstr(t *testing.T) {
	cases := []struct {
		name string
		emit func(*Builder)
		want []byte
	}{
		{"MovImm EAX 10", func(b *Builder) { b.MovImm(EAX, 10) }, []byte{0xB8, 0x0A, 0, 0, 0}},
		{"MovImm EBX 32", func(b *Builder) { b.MovImm(EBX, 32) }, []byte{0xBB, 0x20, 0, 0, 0}},
		{"MovImm EDI 0xDEADBEEF", func(b *Builder) { b.MovImm(EDI, 0xDEADBEEF) }, []byte{0xBF, 0xEF, 0xBE, 0xAD, 0xDE}},
		{"MovReg EBX EAX", func(b *Builder) { b.MovReg(EBX, EAX) }, []byte{0x89, 0xC3}},
		{"AddReg EAX EBX", func(b *Builder) { b.AddReg(EAX, EBX) }, []byte{0x01, 0xD8}},
		{"SubReg EAX EBX", func(b *Builder) { b.SubReg(EAX, EBX) }, []byte{0x29, 0xD8}},
		{"XorReg EAX EAX", func(b *Builder) { b.XorReg(EAX, EAX) }, []byte{0x31, 0xC0}},
		{"CmpReg EAX ECX", func(b *Builder) { b.CmpReg(EAX, ECX) }, []byte{0x39, 0xC8}},
		{"CmpImm EAX 0", func(b *Builder) { b.CmpImm(EAX, 0) }, []byte{0x81, 0xF8, 0, 0, 0, 0}},
		{"Inc EAX", func(b *Builder) { b.Inc(EAX) }, []byte{0x40}},
		{"Dec EAX", func(b *Builder) { b.Dec(EAX) }, []byte{0x48}},
		{"Push EAX", func(b *Builder) { b.Push(EAX) }, []byte{0x50}},
		{"Pop EAX", func(b *Builder) { b.Pop(EAX) }, []byte{0x58}},
		{"Int 0x80", func(b *Builder) { b.Int(0x80) }, []byte{0xCD, 0x80}},
		{"Ret", func(b *Builder) { b.Ret() }, []byte{0xC3}},
		{"Nop", func(b *Builder) { b.Nop() }, []byte{0x90}},
		{"MovMemEAX 0x3FFFE0", func(b *Builder) { b.MovMemEAX(0x3FFFE0) }, []byte{0xA1, 0xE0, 0xFF, 0x3F, 0x00}},
		{"MovEAXMem 0x3FFFE0", func(b *Builder) { b.MovEAXMem(0x3FFFE0) }, []byte{0xA3, 0xE0, 0xFF, 0x3F, 0x00}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewProc("t").Source("// t\n").At(1)
			tc.emit(b)
			got := b.Build().Bytes
			if !bytes.Equal(got, tc.want) {
				t.Errorf("bytes = % X, want % X", got, tc.want)
			}
		})
	}
}

// TestJumpBackwards covers a Jne to a label declared earlier — the common
// "while/for loop" pattern.
func TestJumpBackwards(t *testing.T) {
	b := NewProc("loop").Source("// loop\n").At(1)
	b.MovImm(EAX, 0)
	b.MovImm(ECX, 10)
	b.Label("L")
	b.Inc(EAX)
	b.CmpReg(EAX, ECX)
	b.Jne("L")
	got := b.Build().Bytes
	// 5 (mov eax) + 5 (mov ecx) + 1 (inc) + 2 (cmp) + 2 (jne) = 15 bytes
	// JNE displacement: target=10, nextEIP=15, disp = 10-15 = -5 → 0xFB
	want := []byte{
		0xB8, 0, 0, 0, 0,
		0xB9, 0x0A, 0, 0, 0,
		0x40,       // inc eax
		0x39, 0xC8, // cmp eax, ecx
		0x75, 0xFB, // jne -5
	}
	if !bytes.Equal(got, want) {
		t.Errorf("bytes = % X, want % X", got, want)
	}
}

// TestJumpForwards covers a Je past a block — the "if/skip" pattern.
func TestJumpForwards(t *testing.T) {
	b := NewProc("ifskip").Source("// ifskip\n").At(1)
	b.CmpImm(EAX, 0)
	b.Je("after")
	b.MovImm(EBX, 1)
	b.Label("after")
	b.Ret()
	got := b.Build().Bytes
	// 6 (cmp eax,0) + 2 (jz placeholder) + 5 (mov ebx,1) + 1 (ret)
	// "after" at 13, JZ at offset 7, nextEIP=8, disp = 13-8 = 5
	want := []byte{
		0x81, 0xF8, 0, 0, 0, 0, // cmp eax, 0
		0x74, 0x05, // jz +5
		0xBB, 0x01, 0, 0, 0, // mov ebx, 1
		0xC3, // ret
	}
	if !bytes.Equal(got, want) {
		t.Errorf("bytes = % X, want % X", got, want)
	}
}

// TestJumpRel8Overflow exercises the safeguard: a rel8 jump whose target is
// more than 127 bytes away must panic at Build() with a useful message.
func TestJumpRel8Overflow(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		if !strings.Contains(r.(string), "overflows ±127") {
			t.Errorf("panic = %v, want message about ±127 overflow", r)
		}
	}()
	b := NewProc("toolong").Source("// long\n").At(1)
	b.Jmp("far")
	for i := 0; i < 200; i++ {
		b.Nop()
	}
	b.Label("far")
	b.Ret()
	_ = b.Build()
}

// TestPadTo emits the requested filler and refuses to go backwards.
func TestPadTo(t *testing.T) {
	b := NewProc("pad").Source("// pad\n").At(1)
	b.Nop()
	b.PadTo(0x10)
	if got := len(b.Build().Bytes); got != 0x10 {
		t.Errorf("len = 0x%X, want 0x10", got)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when PadTo target is in the past")
		}
	}()
	b2 := NewProc("padback").Source("// pad\n").At(1)
	b2.MovImm(EAX, 0) // 5 bytes
	b2.PadTo(2)       // already past
}

// TestSourceMapMonotonic verifies the SourceMap invariants the UI relies on:
// strictly monotonic offsets, all within len(Bytes), the comment ends up on
// the right mnemonic.
func TestSourceMapMonotonic(t *testing.T) {
	b := NewProc("smap").Source("// smap\n").At(2)
	b.MovImm(EAX, 1)
	b.Comment("exit code").MovReg(EBX, EAX)
	b.Inc(EAX)
	p := b.Build()
	if len(p.SourceMap) != 3 {
		t.Fatalf("len(SourceMap) = %d, want 3", len(p.SourceMap))
	}
	var prev int64 = -1
	for i, sl := range p.SourceMap {
		if int64(sl.Offset) <= prev {
			t.Errorf("entry %d: offset 0x%X not strictly > %d", i, sl.Offset, prev)
		}
		prev = int64(sl.Offset)
		if int(sl.Offset) >= len(p.Bytes) {
			t.Errorf("entry %d: offset 0x%X out of range", i, sl.Offset)
		}
	}
	if !strings.Contains(p.SourceMap[1].Mnemonic, "exit code") {
		t.Errorf("comment did not attach to MOV EBX, EAX (got %q)", p.SourceMap[1].Mnemonic)
	}
	// Comment must NOT leak to the next instruction.
	if strings.Contains(p.SourceMap[2].Mnemonic, "exit code") {
		t.Errorf("comment leaked to INC (got %q)", p.SourceMap[2].Mnemonic)
	}
}

// TestEmitWithoutAtPanics — guard against forgetting to set the C line, since
// a missing CLine quietly degrades the CodeAsmSplit panel.
func TestEmitWithoutAtPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	b := NewProc("noat").Source("// noat\n")
	b.MovImm(EAX, 0) // no .At() — should panic
	_ = b.Build()
}

// TestDuplicateLabelPanics — two Label("L") in the same builder is always a
// bug worth crashing early.
func TestDuplicateLabelPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	b := NewProc("dup").Source("// dup\n").At(1)
	b.Label("L")
	b.Nop()
	b.Label("L")
}

// TestLabelOffsetEmbedded — sanity that LabelOffset returns the offset at
// which the label was registered, which demos use to embed thread entry
// points as immediates.
func TestLabelOffsetEmbedded(t *testing.T) {
	b := NewProc("lo").Source("// lo\n").At(1)
	b.MovImm(EAX, 1)  // 5 bytes
	b.Label("entry")  // offset 5
	b.Nop()
	if got := b.LabelOffset("entry"); got != 5 {
		t.Errorf("LabelOffset = %d, want 5", got)
	}
}
