package kernel

import (
	"encoding/binary"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// Interrupt vectors used by the demo.
//
// In real x86 the first 32 vectors (0x00-0x1F) are Intel-reserved exceptions
// and HW IRQs begin at 0x20 after PIC remap. We keep that convention.
const (
	IRQDivideError uint8 = 0x00
	IRQPageFault   uint8 = 0x0E
	IRQTimer       uint8 = 0x20 // PIT
	IRQKeyboard    uint8 = 0x21
	IRQSyscall     uint8 = 0x80 // Linux convention
)

// InstallDefaultIDT writes minimal handlers into kernel memory and registers
// them in the IDT. Each handler is essentially a stub that the HW invokes;
// the real work runs in Go via runHandlerLogic BEFORE these bytes execute.
// After that, IRET returns to user (or to the next process if the scheduler
// patched the frame).
func (k *Kernel) InstallDefaultIDT() {
	noopHandler := []byte{0xCF}                // iret
	pageFaultHandler := []byte{0x90, 0xCF}     // nop + iret
	timerHandler := []byte{0x90, 0xCF}         // nop + iret
	syscallHandler := []byte{0x90, 0x90, 0xCF} // nop nop iret
	keyboardHandler := []byte{0x90, 0xCF}

	// Each handler starts on a 0x10-aligned address so they are easy to
	// distinguish in hex dumps and visualizations.
	handlerAddrs := map[uint8]uint32{
		IRQDivideError: cpu.AddrKernelText + 0x00,
		IRQPageFault:   cpu.AddrKernelText + 0x10,
		IRQTimer:       cpu.AddrKernelText + 0x20,
		IRQKeyboard:    cpu.AddrKernelText + 0x30,
		IRQSyscall:     cpu.AddrKernelText + 0x40,
	}

	writeHandler(k.CPU, handlerAddrs[IRQDivideError], noopHandler)
	writeHandler(k.CPU, handlerAddrs[IRQPageFault], pageFaultHandler)
	writeHandler(k.CPU, handlerAddrs[IRQTimer], timerHandler)
	writeHandler(k.CPU, handlerAddrs[IRQKeyboard], keyboardHandler)
	writeHandler(k.CPU, handlerAddrs[IRQSyscall], syscallHandler)

	for vec, addr := range handlerAddrs {
		setIDTEntry(k.CPU, vec, addr)
	}
}

// SetHandler replaces the handler address for a given vector. Useful for tests
// and for future, more sophisticated handlers.
func (k *Kernel) SetHandler(vector uint8, addr uint32) {
	setIDTEntry(k.CPU, vector, addr)
}

// HandlerAddr returns the address installed for a given vector.
func (k *Kernel) HandlerAddr(vector uint8) uint32 {
	idtAddr := cpu.AddrIDT + uint32(vector)*4
	return binary.LittleEndian.Uint32(k.CPU.Mem.Bytes[idtAddr : idtAddr+4])
}

// writeHandler copies handler bytes to a kernel address. The permission check
// is bypassed because this is pre-boot setup (equivalent to a bootloader
// writing memory before handing control to the CPU).
func writeHandler(c *cpu.CPU, addr uint32, bytes []byte) {
	copy(c.Mem.Bytes[addr:], bytes)
}

// setIDTEntry writes the handler address into IDT[vector]. The IDT lives in
// kernel memory (AddrIDT) and is read by the CPU in response to INT/IRQ.
func setIDTEntry(c *cpu.CPU, vector uint8, handlerAddr uint32) {
	idtAddr := cpu.AddrIDT + uint32(vector)*4
	binary.LittleEndian.PutUint32(c.Mem.Bytes[idtAddr:], handlerAddr)
}
