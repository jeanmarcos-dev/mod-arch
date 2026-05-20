package cpu

import "fmt"

// This file implements what the CPU HARDWARE does automatically when an IRQ
// arrives (HW), when `int N` runs (SW), and when `iret` runs.
//
// Software never writes the mode bit directly. The HW (these CPU functions)
// changes the mode, switches the stack, and jumps to the handler. Software
// only CONFIGURES the IDT and writes the handlers; the invocation is done by
// this code in response to INT/IRQ. IRET is the exact inverse: the kernel
// executes `iret` and the HW undoes everything, deciding the return mode from
// the popped CS.

// dispatchIRQ is the HARDWARE routine the CPU runs when entering the kernel.
//
// Steps (in order, all automatic):
//  1. Read handler from IDT[vector].
//  2. If coming from user: switch ESP to the kernel stack (KernelStackTop).
//  3. Push the interrupt frame: EFLAGS, CS, EIP of the user.
//  4. Set Mode = PL0, CS = kernel.
//  5. Clear IF (further IRQs wait until the handler runs sti or iret).
//  6. Set EIP = handler.
//
// The handler, now running in PL0, eventually executes `iret`, which calls
// handleIRET and reverses everything.
func (c *CPU) dispatchIRQ(vector uint8, source string) error {
	// Read the handler from the IDT. The HW always reads it in kernel mode
	// (the IDT lives in kernel memory and only the CPU accesses it directly).
	idtEntryAddr := AddrIDT + uint32(vector)*4
	handler, err := c.Mem.Read32(nil, idtEntryAddr, ModeKernel)
	if err != nil {
		return fmt.Errorf("dispatch IRQ %d: read IDT @ 0x%X: %w", vector, idtEntryAddr, err)
	}
	if handler == 0 {
		return fmt.Errorf("dispatch IRQ %d: IDT[0x%02X] not installed", vector, vector)
	}

	c.Emit(EventIRQFire{Vector: vector, Source: source})

	// If the CPU was halted waiting for an IRQ, wake it up. This models the
	// x86 behavior of HLT: "sleep until any interrupt arrives".
	c.Halted = false

	// Remember the current state to push and later restore on IRET.
	oldEFLAGS := c.R.EFLAGS
	oldCS := c.R.CS
	oldEIP := c.R.EIP
	oldESP := c.R.ESP
	oldMode := c.Mode

	// If coming from user, switch to the kernel stack. In real x86 this is
	// done by consulting the TSS, which is per-CPU; here each core carries its
	// own KernelStackTop so concurrent traps land on separate frames.
	if oldMode == ModeUser {
		c.SavedUserESP = oldESP
		c.SavedUserSS = c.R.SS
		c.R.ESP = c.KernelStackTop
		c.R.SS = SelKernelData
	}

	// Mode switch — done automatically by the HW.
	c.Mode = ModeKernel
	c.R.CS = SelKernelCode
	if oldMode != ModeKernel {
		c.Emit(EventModeSwitch{From: oldMode, To: ModeKernel, Cause: causeOf(source)})
	}

	// Push the interrupt frame now that we are on the kernel stack.
	// Push order: EFLAGS, CS (zero-extended to 32 bits), EIP.
	// IRET pops them in LIFO order: EIP, CS, EFLAGS.
	if err := c.pushKernel(oldEFLAGS); err != nil {
		return fmt.Errorf("push eflags: %w", err)
	}
	if err := c.pushKernel(uint32(oldCS)); err != nil {
		return fmt.Errorf("push cs: %w", err)
	}
	if err := c.pushKernel(oldEIP); err != nil {
		return fmt.Errorf("push eip: %w", err)
	}

	// Block further IRQs until the handler runs sti or iret (which restores
	// the full EFLAGS).
	c.FlagSet(FlagIF, false)

	c.R.EIP = handler

	return nil
}

// handleIRET is the HARDWARE routine when the kernel executes `iret`.
//
// It exactly undoes dispatchIRQ:
//  1. Pop EIP, CS, EFLAGS from the kernel stack.
//  2. If returning to user: restore SavedUserESP/SS.
//  3. Switch Mode to PL3 if CS is a user selector.
//
// The kernel does NOT decide the return mode: the popped CS determines where
// execution resumes. If CS is a user selector, the HW automatically drops to PL3.
func (c *CPU) handleIRET() error {
	// Pop in the reverse order of dispatchIRQ's push.
	eip, err := c.popKernel()
	if err != nil {
		return fmt.Errorf("pop eip: %w", err)
	}
	cs, err := c.popKernel()
	if err != nil {
		return fmt.Errorf("pop cs: %w", err)
	}
	eflags, err := c.popKernel()
	if err != nil {
		return fmt.Errorf("pop eflags: %w", err)
	}

	// Bits 0-1 of an x86 selector encode the RPL; PL3 = 11b.
	returningToUser := uint8(cs)&0b11 == ModeUser

	c.R.EIP = eip
	c.R.CS = uint16(cs)
	// Restored verbatim, IF included.
	c.R.EFLAGS = eflags

	if returningToUser {
		oldMode := c.Mode
		c.Mode = ModeUser
		c.R.ESP = c.SavedUserESP
		c.R.SS = c.SavedUserSS
		c.Emit(EventModeSwitch{From: oldMode, To: ModeUser, Cause: "IRET"})
	}

	return nil
}

// pushKernel writes 4 bytes to the kernel stack and decrements ESP by 4.
// Privilege: ModeKernel (we are already in kernel when this is called).
func (c *CPU) pushKernel(v uint32) error {
	newESP := c.R.ESP - 4
	if err := c.Mem.Write32(c, newESP, v, ModeKernel); err != nil {
		return err
	}
	c.R.ESP = newESP
	return nil
}

// popKernel reads 4 bytes from the kernel stack and increments ESP by 4.
func (c *CPU) popKernel() (uint32, error) {
	v, err := c.Mem.Read32(c, c.R.ESP, ModeKernel)
	if err != nil {
		return 0, err
	}
	c.R.ESP += 4
	return v, nil
}

// causeOf maps "HW"/"SW" to the string used in EventModeSwitch.
//
//	HW IRQ -> "INT"   (CPU took the interrupt from the IRQ bus)
//	SW INT -> "INT"   (the `int N` instruction invokes the same mechanism)
//	fault  -> "fault" (CPU exception)
func causeOf(source string) string {
	if source == "fault" {
		return "fault"
	}
	return "INT"
}
