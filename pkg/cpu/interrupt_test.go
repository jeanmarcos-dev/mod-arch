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

// findIRQFire returns the first EventIRQFire in evs, or nil.
func findIRQFire(evs []Event) *EventIRQFire {
	for _, e := range evs {
		if f, ok := e.(EventIRQFire); ok {
			return &f
		}
	}
	return nil
}

// TestINT_FireCarriesHandlerAndFrame asserts that when an interrupt enters the
// kernel from user mode, the emitted EventIRQFire carries the handler address
// the CPU is about to jump to and the three-word frame (EIP/CS/EFLAGS) that
// the HW just pushed to the kernel stack.
func TestINT_FireCarriesHandlerAndFrame(t *testing.T) {
	c := New(DefaultMemSize)
	c.Mode = ModeUser
	c.R.EIP = AddrUserBase
	c.R.ESP = AddrUserStack + 0xFF000
	c.R.EBP = c.R.ESP
	c.R.CS = SelUserCode
	c.R.DS = SelUserData
	c.R.SS = SelUserData
	c.R.EFLAGS = 0x00000202 // IF=1, bit 1 always set
	c.SavedUserESP = c.R.ESP
	c.SavedUserSS = c.R.SS

	const handlerAddr = AddrKernelText
	installHandler(t, c, 0x80, handlerAddr, []byte{0xCF}) // IRET

	if err := c.Mem.LoadBytes(AddrUserBase, []byte{0xCD, 0x80}); err != nil {
		t.Fatalf("load: %v", err)
	}

	preEIP := c.R.EIP
	preCS := c.R.CS
	preEFLAGS := c.R.EFLAGS

	evs, err := c.Step()
	if err != nil {
		t.Fatalf("step INT: %v", err)
	}
	fire := findIRQFire(evs)
	if fire == nil {
		t.Fatalf("no EventIRQFire emitted; got %d events", len(evs))
	}

	if fire.Vector != 0x80 {
		t.Errorf("vector = 0x%X, want 0x80", fire.Vector)
	}
	if fire.Handler != handlerAddr {
		t.Errorf("handler = 0x%X, want 0x%X", fire.Handler, handlerAddr)
	}
	if fire.FromMode != ModeUser {
		t.Errorf("from_mode = %d, want %d (ModeUser)", fire.FromMode, ModeUser)
	}
	// The HW pushes the address of the instruction following the INT, which
	// the CPU has already advanced past while fetching the opcode + vector.
	wantSavedEIP := preEIP + 2
	if fire.SavedEIP != wantSavedEIP {
		t.Errorf("saved_eip = 0x%X, want 0x%X (next user EIP)", fire.SavedEIP, wantSavedEIP)
	}
	if fire.SavedCS != preCS {
		t.Errorf("saved_cs = 0x%X, want 0x%X", fire.SavedCS, preCS)
	}
	if fire.SavedEFLAGS != preEFLAGS {
		t.Errorf("saved_eflags = 0x%X, want 0x%X", fire.SavedEFLAGS, preEFLAGS)
	}
}

// TestINT_FireFromKernelOmitsFrame covers IRQs that fire while the CPU is
// already in kernel mode: no privilege transition happens, no frame is pushed,
// and the saved_* fields stay zero so the UI knows there is nothing to render.
func TestINT_FireFromKernelOmitsFrame(t *testing.T) {
	c := New(DefaultMemSize)
	c.Mode = ModeKernel
	c.R.EIP = AddrKernelText + 0x100
	c.R.ESP = c.KernelStackTop
	c.R.CS = SelKernelCode

	const handlerAddr = AddrKernelText + 0x200
	installHandler(t, c, 0x21, handlerAddr, []byte{0xCF})

	if err := c.dispatchIRQ(0x21, "HW"); err != nil {
		t.Fatalf("dispatchIRQ: %v", err)
	}
	fire := findIRQFire(c.DrainEvents())
	if fire == nil {
		t.Fatal("no EventIRQFire emitted")
	}
	if fire.FromMode != ModeKernel {
		t.Errorf("from_mode = %d, want %d (ModeKernel)", fire.FromMode, ModeKernel)
	}
	if fire.SavedEIP != 0 || fire.SavedCS != 0 || fire.SavedEFLAGS != 0 {
		t.Errorf("saved_* should be zero when from_mode==kernel; got EIP=0x%X CS=0x%X EFLAGS=0x%X",
			fire.SavedEIP, fire.SavedCS, fire.SavedEFLAGS)
	}
	if fire.Handler != handlerAddr {
		t.Errorf("handler = 0x%X, want 0x%X", fire.Handler, handlerAddr)
	}
}
