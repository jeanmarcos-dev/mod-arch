package cpu

import (
	"testing"
)

func FuzzCPU(f *testing.F) {
	// Seed with some basic instruction bytes
	f.Add([]byte{0xB8, 0x42, 0x00, 0x00, 0x00, 0xF4}) // MOV EAX, 0x42; HLT
	f.Add([]byte{0x90})                               // NOP
	f.Add([]byte{0x00, 0x00})                         // ADD [EAX], AL or similar
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})             // Invalid
	f.Add([]byte{0xEB, 0xFE})                         // JMP EIP (infinite loop but step-limited)

	f.Fuzz(func(t *testing.T, code []byte) {
		if len(code) == 0 || len(code) > 256 {
			return
		}

		c := New(DefaultMemSize)
		c.Mode = ModeKernel
		c.R.EIP = AddrUserBase
		c.R.ESP = AddrUserStack + 0xFF000
		c.R.EBP = c.R.ESP
		c.R.CS = SelKernelCode
		c.R.DS = SelKernelData
		c.R.SS = SelKernelData

		// Load code bytes into memory
		if err := c.Mem.LoadBytes(AddrUserBase, code); err != nil {
			return
		}

		// Run a limited number of steps to avoid infinite loops
		maxSteps := 20
		for i := 0; i < maxSteps; i++ {
			if c.Halted {
				break
			}
			_, err := c.Step()
			if err != nil {
				// Errors (e.g., page faults, invalid opcodes) are expected, 
				// but the CPU should not panic.
				break
			}
		}
	})
}
