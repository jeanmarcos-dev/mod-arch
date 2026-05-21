package cpu

import (
	"testing"
)

// newTestCPU prepares a CPU ready to exercise the ISA in isolation: default
// memory, EIP at user_code base, ESP at the top of user_stack, mode PL0
// (kernel). Tests in this file focus on opcode semantics — flag updates,
// register writes, control flow — which are independent of the privilege
// level. Running in kernel mode lets HLT terminate the test loop directly
// (since HLT is privileged: in PL3 it raises #GP, see TestISA_HLT_*).
func newTestCPU(t *testing.T, code []byte) *CPU {
	t.Helper()
	c := New(DefaultMemSize)
	c.Mode = ModeKernel
	c.R.EIP = AddrUserBase
	c.R.ESP = AddrUserStack + 0xFF000
	c.R.EBP = c.R.ESP
	c.R.CS = SelKernelCode
	c.R.DS = SelKernelData
	c.R.SS = SelKernelData
	if err := c.Mem.LoadBytes(AddrUserBase, code); err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func TestISA_MOVImm(t *testing.T) {
	// MOV EAX, 0x42  ->  B8 42 00 00 00
	c := newTestCPU(t, []byte{0xB8, 0x42, 0x00, 0x00, 0x00, 0xF4})

	if _, err := c.Step(); err != nil {
		t.Fatalf("step mov: %v", err)
	}
	if c.R.EAX != 0x42 {
		t.Errorf("EAX = 0x%X, want 0x42", c.R.EAX)
	}
	if c.R.EIP != AddrUserBase+5 {
		t.Errorf("EIP = 0x%X, want 0x%X", c.R.EIP, AddrUserBase+5)
	}
}

func TestISA_MOVAllRegs(t *testing.T) {
	tests := []struct {
		opcode byte
		want   string
		val    uint32
	}{
		{0xB8, "EAX", 0xAAAAAAAA},
		{0xB9, "ECX", 0xBBBBBBBB},
		{0xBA, "EDX", 0xCCCCCCCC},
		{0xBB, "EBX", 0xDDDDDDDD},
		{0xBE, "ESI", 0xEEEEEEEE},
		{0xBF, "EDI", 0xFFFFFFFF},
	}
	for _, tc := range tests {
		code := []byte{
			tc.opcode,
			byte(tc.val), byte(tc.val >> 8), byte(tc.val >> 16), byte(tc.val >> 24),
		}
		c := newTestCPU(t, code)
		if _, err := c.Step(); err != nil {
			t.Fatalf("MOV %s: %v", tc.want, err)
		}
		v, err := c.GetReg32(tc.want)
		if err != nil {
			t.Fatal(err)
		}
		if v != tc.val {
			t.Errorf("MOV %s: got 0x%X, want 0x%X", tc.want, v, tc.val)
		}
	}
}

func TestISA_INC(t *testing.T) {
	// MOV EAX, 41 ; INC EAX ; HLT
	c := newTestCPU(t, []byte{
		0xB8, 0x29, 0x00, 0x00, 0x00, // mov eax, 41
		0x40, // inc eax
		0xF4, // hlt
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatalf("step: %v", err)
		}
	}
	if c.R.EAX != 42 {
		t.Errorf("EAX = %d, want 42", c.R.EAX)
	}
	if c.FlagOn(FlagZF) {
		t.Error("ZF should be clear (42 != 0)")
	}
}

func TestISA_DEC_ToZeroSetsZF(t *testing.T) {
	// MOV EAX, 1 ; DEC EAX ; HLT
	c := newTestCPU(t, []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00,
		0x48,
		0xF4,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.R.EAX != 0 {
		t.Errorf("EAX = %d, want 0", c.R.EAX)
	}
	if !c.FlagOn(FlagZF) {
		t.Error("ZF should be set after DEC to zero")
	}
}

func TestISA_ADD_RegReg(t *testing.T) {
	// MOV EAX, 10 ; MOV EBX, 32 ; ADD EAX, EBX ; HLT
	// ADD r/m32, r32 with mod=11 reg=EBX(3) rm=EAX(0) -> ModR/M = 11 011 000 = 0xD8
	c := newTestCPU(t, []byte{
		0xB8, 0x0A, 0x00, 0x00, 0x00, // mov eax, 10
		0xBB, 0x20, 0x00, 0x00, 0x00, // mov ebx, 32
		0x01, 0xD8, // add eax, ebx
		0xF4,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.R.EAX != 42 {
		t.Errorf("EAX = %d, want 42", c.R.EAX)
	}
}

func TestISA_CMP_AndJE(t *testing.T) {
	// mov eax, 5 ; mov ebx, 5 ; cmp eax, ebx ; je +1 ; hlt ; hlt
	// If JE is taken, it skips the first HLT and stops on the second.
	// CMP r/m32, r32 (0x39) reg=EBX(3) rm=EAX(0) -> 11 011 000 = 0xD8
	c := newTestCPU(t, []byte{
		0xB8, 0x05, 0x00, 0x00, 0x00, // mov eax, 5
		0xBB, 0x05, 0x00, 0x00, 0x00, // mov ebx, 5
		0x39, 0xD8, // cmp eax, ebx -> ZF=1
		0x74, 0x01, // je +1
		0xF4, // hlt (should NOT run)
		0xF4, // hlt (this one runs)
	})
	steps := 0
	for !c.Halted && steps < 20 {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
		steps++
	}
	if !c.Halted {
		t.Fatal("did not reach HLT")
	}
	// Path: mov, mov, cmp, je, hlt = 5 steps.
	if steps != 5 {
		t.Errorf("steps = %d, want 5 (first HLT should be skipped)", steps)
	}
	// EIP should be just past the second HLT.
	// Layout: 5+5+2+2+1+1 = 16 bytes, EIP ends at base+0x10 after the final fetch.
	if c.R.EIP != AddrUserBase+0x10 {
		t.Errorf("EIP = 0x%X, want 0x%X (past the second HLT)", c.R.EIP, AddrUserBase+0x10)
	}
}

func TestISA_Counter_Loop(t *testing.T) {
	// "counter" program: count from 0 to 10 in EAX using INC + CMP + JNE.
	//
	//     mov eax, 0        ; B8 00 00 00 00
	//     mov ecx, 10       ; B9 0A 00 00 00
	// L:  inc eax           ; 40
	//     cmp eax, ecx      ; 39 C8   (mod=11 reg=ECX(1) rm=EAX(0) = 11 001 000)
	//     jne L             ; 75 FB   (rel8 = -5 = 0xFB, jumps back to INC)
	//     hlt               ; F4
	c := newTestCPU(t, []byte{
		0xB8, 0x00, 0x00, 0x00, 0x00,
		0xB9, 0x0A, 0x00, 0x00, 0x00,
		0x40,
		0x39, 0xC8,
		0x75, 0xFB,
		0xF4,
	})
	steps := 0
	for !c.Halted && steps < 100 {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
		steps++
	}
	if !c.Halted {
		t.Fatal("infinite loop (?), did not reach HLT")
	}
	if c.R.EAX != 10 {
		t.Errorf("EAX = %d, want 10", c.R.EAX)
	}
}

func TestISA_PushPop(t *testing.T) {
	// mov eax, 0xCAFE ; push eax ; mov eax, 0 ; pop eax ; hlt
	c := newTestCPU(t, []byte{
		0xB8, 0xFE, 0xCA, 0x00, 0x00, // mov eax, 0xCAFE
		0x50,                         // push eax
		0xB8, 0x00, 0x00, 0x00, 0x00, // mov eax, 0
		0x58, // pop eax
		0xF4,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.R.EAX != 0xCAFE {
		t.Errorf("EAX after pop = 0x%X, want 0xCAFE", c.R.EAX)
	}
}

func TestISA_CallRet(t *testing.T) {
	// Program:
	//   _start:  call fn   (rel32 = +1, skips the following HLT)
	//            hlt       (should not run)
	//   fn:      mov eax, 0x99
	//            ret       (returns to the first hlt and finishes)
	//
	// rel32 in CALL is applied AFTER the 4 displacement bytes are read, so
	// after 0xE8 + 4 bytes EIP = base+5. To reach fn @ base+6, rel = +1.
	//
	// Layout:
	//   0x200000: E8 01 00 00 00   call fn
	//   0x200005: F4               hlt
	//   0x200006: B8 99 00 00 00   mov eax, 0x99
	//   0x20000B: C3               ret
	c := newTestCPU(t, []byte{
		0xE8, 0x01, 0x00, 0x00, 0x00,
		0xF4,
		0xB8, 0x99, 0x00, 0x00, 0x00,
		0xC3,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.R.EAX != 0x99 {
		t.Errorf("EAX = 0x%X, want 0x99 (fn should have been called)", c.R.EAX)
	}
}

func TestISA_EmitsInstructionEvent(t *testing.T) {
	c := newTestCPU(t, []byte{0xB8, 0x42, 0x00, 0x00, 0x00, 0xF4})

	evs, err := c.Step()
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 {
		t.Fatal("Step should emit at least one event")
	}
	first, ok := evs[0].(EventInstruction)
	if !ok {
		t.Fatalf("first event should be EventInstruction, got %T", evs[0])
	}
	if first.Mnemonic != "MOV EAX, 0x00000042" {
		t.Errorf("Mnemonic = %q", first.Mnemonic)
	}
	if first.EIP != AddrUserBase {
		t.Errorf("event EIP = 0x%X, want 0x%X", first.EIP, AddrUserBase)
	}
}

func TestISA_ADD_SetsCFOnUnsignedOverflow(t *testing.T) {
	// MOV EAX, 0xFFFFFFFF ; MOV EBX, 1 ; ADD EAX, EBX ; HLT
	c := newTestCPU(t, []byte{
		0xB8, 0xFF, 0xFF, 0xFF, 0xFF,
		0xBB, 0x01, 0x00, 0x00, 0x00,
		0x01, 0xD8,
		0xF4,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.R.EAX != 0 {
		t.Errorf("EAX = 0x%X, want 0", c.R.EAX)
	}
	if !c.FlagOn(FlagCF) {
		t.Error("CF should be set (0xFFFFFFFF + 1 overflows)")
	}
	if !c.FlagOn(FlagZF) {
		t.Error("ZF should be set (result is 0)")
	}
	if c.FlagOn(FlagOF) {
		t.Error("OF should be clear (no signed overflow: -1 + 1 = 0)")
	}
}

func TestISA_ADD_SetsOFOnSignedOverflow(t *testing.T) {
	// MOV EAX, 0x7FFFFFFF ; MOV EBX, 1 ; ADD EAX, EBX ; HLT
	c := newTestCPU(t, []byte{
		0xB8, 0xFF, 0xFF, 0xFF, 0x7F,
		0xBB, 0x01, 0x00, 0x00, 0x00,
		0x01, 0xD8,
		0xF4,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if !c.FlagOn(FlagOF) {
		t.Error("OF should be set (INT_MAX + 1 wraps to INT_MIN)")
	}
	if !c.FlagOn(FlagSF) {
		t.Error("SF should be set (result has bit 31 = 1)")
	}
	if c.FlagOn(FlagCF) {
		t.Error("CF should be clear (no unsigned overflow)")
	}
}

func TestISA_SUB_SetsCFOnBorrow(t *testing.T) {
	// MOV EAX, 1 ; MOV EBX, 5 ; SUB EAX, EBX ; HLT
	c := newTestCPU(t, []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00,
		0xBB, 0x05, 0x00, 0x00, 0x00,
		0x29, 0xD8,
		0xF4,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if !c.FlagOn(FlagCF) {
		t.Error("CF should be set (1 - 5 borrows)")
	}
	if !c.FlagOn(FlagSF) {
		t.Error("SF should be set (result is negative)")
	}
}

func TestISA_INC_SetsOFAtSignedMax(t *testing.T) {
	// Pre-set CF so we can verify INC preserves it. STC has no opcode here,
	// so trigger CF via ADD overflow, then INC and check CF survives.
	// MOV EAX, 0xFFFFFFFF ; MOV EBX, 1 ; ADD EAX, EBX (sets CF, EAX=0)
	// MOV EAX, 0x7FFFFFFF ; INC EAX ; HLT (should set OF, leave CF)
	c := newTestCPU(t, []byte{
		0xB8, 0xFF, 0xFF, 0xFF, 0xFF,
		0xBB, 0x01, 0x00, 0x00, 0x00,
		0x01, 0xD8,
		0xB8, 0xFF, 0xFF, 0xFF, 0x7F,
		0x40,
		0xF4,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if !c.FlagOn(FlagOF) {
		t.Error("OF should be set (INC of 0x7FFFFFFF crosses signed max)")
	}
	if !c.FlagOn(FlagCF) {
		t.Error("CF should be preserved across INC (was set by previous ADD)")
	}
}

func TestISA_XOR_ClearsCFAndOF(t *testing.T) {
	// Set CF first (ADD overflow), then XOR EAX, EAX. CF and OF must be 0.
	c := newTestCPU(t, []byte{
		0xB8, 0xFF, 0xFF, 0xFF, 0xFF,
		0xBB, 0x01, 0x00, 0x00, 0x00,
		0x01, 0xD8,
		0x31, 0xC0,
		0xF4,
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.FlagOn(FlagCF) {
		t.Error("CF should be cleared by XOR")
	}
	if c.FlagOn(FlagOF) {
		t.Error("OF should be cleared by XOR")
	}
	if !c.FlagOn(FlagZF) {
		t.Error("ZF should be set (XOR EAX, EAX = 0)")
	}
}

func TestISA_JMP_EmitsEIPRegWriteWithTarget(t *testing.T) {
	// JMP rel32 to offset 0x10 from the next instruction. EIP should land at
	// AddrUserBase + 5 + 0x10, and an EventRegWrite{Reg:"EIP"} must announce it
	// with the real target — not (eip + bytes.length).
	c := newTestCPU(t, []byte{
		0xE9, 0x10, 0x00, 0x00, 0x00, // jmp +0x10
	})
	evs, err := c.Step()
	if err != nil {
		t.Fatal(err)
	}
	wantTarget := AddrUserBase + 5 + 0x10
	if c.R.EIP != wantTarget {
		t.Errorf("EIP after JMP = 0x%X, want 0x%X", c.R.EIP, wantTarget)
	}
	var sawEIPWrite bool
	for _, e := range evs {
		rw, ok := e.(EventRegWrite)
		if !ok || rw.Reg != "EIP" {
			continue
		}
		sawEIPWrite = true
		if rw.New != wantTarget {
			t.Errorf("EventRegWrite{EIP}.New = 0x%X, want 0x%X", rw.New, wantTarget)
		}
	}
	if !sawEIPWrite {
		t.Error("expected EventRegWrite for EIP after JMP, none emitted")
	}
}

func TestISA_JE_NotTaken_NoEIPRegWrite(t *testing.T) {
	// Conditional jump not taken must NOT emit reg_write for EIP. Otherwise
	// fall-through would flood the event stream.
	c := newTestCPU(t, []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (clears ZF)
		0x74, 0x05, // je +5 (not taken because ZF=0)
	})
	if _, err := c.Step(); err != nil { // MOV
		t.Fatal(err)
	}
	evs, err := c.Step() // JE not taken
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if rw, ok := e.(EventRegWrite); ok && rw.Reg == "EIP" {
			t.Errorf("JE not taken emitted EventRegWrite{EIP}=0x%X — should be silent", rw.New)
		}
	}
}

func TestISA_UnknownOpcodeErrors(t *testing.T) {
	// 0xFE is not implemented.
	c := newTestCPU(t, []byte{0xFE, 0xF4})
	_, err := c.Step()
	if err == nil {
		t.Fatal("expected error for unknown opcode")
	}
}
