package cpu

import (
	"testing"
)

// newTestCPU prepares a CPU ready to run user code: default memory, EIP at
// user_code base, ESP at the top of user_stack, mode PL3.
func newTestCPU(t *testing.T, code []byte) *CPU {
	t.Helper()
	c := New(DefaultMemSize)
	c.Mode = ModeUser
	c.R.EIP = AddrUserBase
	// Stack top (grows downward).
	c.R.ESP = AddrUserStack + 0xFF000
	c.R.EBP = c.R.ESP
	c.R.CS = SelUserCode
	c.R.DS = SelUserData
	c.R.SS = SelUserData
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

func TestISA_UnknownOpcodeErrors(t *testing.T) {
	// 0xFE is not implemented.
	c := newTestCPU(t, []byte{0xFE, 0xF4})
	_, err := c.Step()
	if err == nil {
		t.Fatal("expected error for unknown opcode")
	}
}
