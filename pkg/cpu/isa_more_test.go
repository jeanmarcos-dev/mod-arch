package cpu

import "testing"

// TestISA_CounterLoop runs a complete loop with JNE: counts EAX from 0 to 10.
// Exercises MOV imm32, INC, CMP r/r, JNE (short rel8 backward branch), HLT.
func TestISA_CounterLoop(t *testing.T) {
	c := newTestCPU(t, []byte{
		0xB8, 0x00, 0x00, 0x00, 0x00, // mov eax, 0
		0xB9, 0x0A, 0x00, 0x00, 0x00, // mov ecx, 10
		0x40,       // inc eax
		0x39, 0xC8, // cmp eax, ecx
		0x75, 0xFB, // jne -5 (back to inc)
		0xF4, // hlt
	})
	for steps := 0; !c.Halted && steps < 200; steps++ {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if !c.Halted {
		t.Fatal("loop did not halt")
	}
	if c.R.EAX != 10 {
		t.Errorf("EAX = %d, want 10", c.R.EAX)
	}
	if !c.FlagOn(FlagZF) {
		t.Error("ZF should be set after EAX==ECX")
	}
}

// TestISA_XOR_ZeroesAndSetsZF covers opcode 0x31 (XOR r/m32, r32). Self-XOR
// is the canonical x86 idiom to zero a register.
func TestISA_XOR_ZeroesAndSetsZF(t *testing.T) {
	c := newTestCPU(t, []byte{
		0xB8, 0xFF, 0xFF, 0xFF, 0xFF, // mov eax, 0xFFFFFFFF
		0x31, 0xC0, // xor eax, eax
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
	if !c.FlagOn(FlagZF) {
		t.Error("ZF should be set after XOR to zero")
	}
}

// TestISA_SHORT_JMP covers opcode 0xEB (unconditional short jump).
func TestISA_SHORT_JMP(t *testing.T) {
	c := newTestCPU(t, []byte{
		0xEB, 0x01, // jmp +1
		0xF4, // hlt (skipped)
		0xF4, // hlt (this one runs)
	})
	for !c.Halted {
		if _, err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.R.EIP != AddrUserBase+4 {
		t.Errorf("EIP = 0x%X, want 0x%X", c.R.EIP, AddrUserBase+4)
	}
}

// TestISA_SUB covers opcode 0x29 (SUB r/m32, r32).
func TestISA_SUB(t *testing.T) {
	c := newTestCPU(t, []byte{
		0xB8, 0x32, 0x00, 0x00, 0x00, // mov eax, 50
		0xBB, 0x08, 0x00, 0x00, 0x00, // mov ebx, 8
		0x29, 0xD8, // sub eax, ebx
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

// TestISA_HLT_KernelMode_EmitsEventHalt covers the swapper resting state:
// kernel-mode HLT must mark the CPU halted AND emit EventHalt{PID:0} so the
// frontend can render the idle HLT in the event log. PID 0 identifies the
// swapper to the UI; the user-mode branch raises #GP and never reaches this
// emission (covered separately by the fault-on-privileged-HLT tests).
func TestISA_HLT_KernelMode_EmitsEventHalt(t *testing.T) {
	c := newTestCPU(t, []byte{0xF4})
	c.CoreID = 3

	evs, err := c.Step()
	if err != nil {
		t.Fatalf("step hlt: %v", err)
	}
	if !c.Halted {
		t.Fatal("CPU did not halt after HLT in kernel mode")
	}
	var halt *EventHalt
	for i := range evs {
		if h, ok := evs[i].(EventHalt); ok {
			halt = &h
			break
		}
	}
	if halt == nil {
		t.Fatalf("Step emitted no EventHalt for kernel-mode HLT; events = %+v", evs)
	}
	if halt.PID != 0 {
		t.Errorf("EventHalt.PID = %d, want 0 (swapper)", halt.PID)
	}
	if halt.CoreID != 3 {
		t.Errorf("EventHalt.CoreID = %d, want 3", halt.CoreID)
	}
}

func TestCPU_GetSetReg32_Errors(t *testing.T) {
	c := New(DefaultMemSize)
	if _, err := c.GetReg32("BOGUS"); err == nil {
		t.Error("GetReg32 with unknown name: expected error, got nil")
	}
	if err := c.SetReg32("ALSO_BOGUS", 1); err == nil {
		t.Error("SetReg32 with unknown name: expected error, got nil")
	}
}
