package cpu

import (
	"encoding/binary"
	"testing"
)

func TestCPU_Reset(t *testing.T) {
	c := New(DefaultMemSize)
	c.R.EAX = 0xCAFE
	c.R.EIP = 0x1234
	c.Tick = 99
	c.Halted = true
	c.PendingIRQs = []uint8{0x20}

	c.Reset()

	if c.R.EAX != 0 || c.R.EIP != 0 {
		t.Errorf("registers not cleared: EAX=0x%X EIP=0x%X", c.R.EAX, c.R.EIP)
	}
	if c.Tick != 0 {
		t.Errorf("tick not reset: %d", c.Tick)
	}
	if c.Halted {
		t.Error("still halted after reset")
	}
	if len(c.PendingIRQs) != 0 {
		t.Errorf("pending IRQs not cleared: %v", c.PendingIRQs)
	}
}

// installHandler writes an IDT entry pointing vector at handlerAddr and copies
// handler bytes to that address. The CPU must be in kernel-write-capable state.
func installHandler(t *testing.T, c *CPU, vector uint8, handlerAddr uint32, code []byte) {
	t.Helper()
	idtEntry := AddrIDT + uint32(vector)*4
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], handlerAddr)
	for i, b := range buf {
		if err := c.Mem.Write8(nil, idtEntry+uint32(i), b, ModeKernel); err != nil {
			t.Fatalf("write IDT[%d]: %v", i, err)
		}
	}
	for i, b := range code {
		if err := c.Mem.Write8(nil, handlerAddr+uint32(i), b, ModeKernel); err != nil {
			t.Fatalf("write handler[%d]: %v", i, err)
		}
	}
}

// TestStep_HaltedNoIRQ verifies that stepping a halted CPU is a no-op except
// for the tick counter, until an IRQ wakes it up.
func TestStep_HaltedNoIRQ(t *testing.T) {
	c := New(DefaultMemSize)
	c.Mode = ModeUser
	c.Halted = true
	startTick := c.Tick

	for i := range 3 {
		if _, err := c.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	if !c.Halted {
		t.Error("CPU should still be halted (no IRQ pending)")
	}
	if c.Tick != startTick+3 {
		t.Errorf("tick advanced to %d, want %d", c.Tick, startTick+3)
	}
}

// TestINT_DispatchAndIRET covers the full SW interrupt cycle:
//
//   - INT n in PL3 -> CPU jumps to handler in PL0
//   - IRET in PL0 -> CPU returns to PL3 at the next user instruction
//
// This exercises dispatchIRQ, handleIRET, pushKernel and popKernel.
func TestINT_DispatchAndIRET(t *testing.T) {
	c := New(DefaultMemSize)
	c.Mode = ModeUser
	c.R.EIP = AddrUserBase
	c.R.ESP = AddrUserStack + 0xFF000
	c.R.EBP = c.R.ESP
	c.R.CS = SelUserCode
	c.R.DS = SelUserData
	c.R.SS = SelUserData
	// Enable kernel stack growing downward from the top.
	c.SavedUserESP = c.R.ESP
	c.SavedUserSS = c.R.SS

	const handlerAddr = AddrKernelText
	// Handler: IRET (0xCF).
	installHandler(t, c, 0x80, handlerAddr, []byte{0xCF})

	// User program: INT 0x80 ; HLT
	program := []byte{0xCD, 0x80, 0xF4}
	if err := c.Mem.LoadBytes(AddrUserBase, program); err != nil {
		t.Fatalf("load: %v", err)
	}

	// Step 1: execute INT 0x80, CPU switches to PL0 at handlerAddr.
	if _, err := c.Step(); err != nil {
		t.Fatalf("step INT: %v", err)
	}
	if c.Mode != ModeKernel {
		t.Errorf("mode after INT: PL%d, want PL%d", c.Mode, ModeKernel)
	}
	if c.R.EIP != handlerAddr {
		t.Errorf("EIP after INT: 0x%X, want 0x%X", c.R.EIP, handlerAddr)
	}

	// Step 2: execute IRET, CPU switches back to PL3 at next user instruction.
	if _, err := c.Step(); err != nil {
		t.Fatalf("step IRET: %v", err)
	}
	if c.Mode != ModeUser {
		t.Errorf("mode after IRET: PL%d, want PL%d", c.Mode, ModeUser)
	}
	if c.R.EIP != AddrUserBase+2 {
		t.Errorf("EIP after IRET: 0x%X, want 0x%X (past INT 0x80)", c.R.EIP, AddrUserBase+2)
	}
}
