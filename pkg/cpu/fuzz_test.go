package cpu

import (
	"testing"
)

func FuzzCPU(f *testing.F) {
	// Seed with some basic instruction bytes and a boolean flag for User mode
	f.Add([]byte{0xB8, 0x42, 0x00, 0x00, 0x00, 0xF4}, false) // MOV EAX, 0x42; HLT
	f.Add([]byte{0x90}, true)                                // NOP User
	f.Add([]byte{0x00, 0x00}, false)                         // ADD [EAX], AL
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}, true)              // Invalid User
	f.Add([]byte{0xEB, 0xFE}, false)                         // JMP EIP
	f.Add([]byte{0xCD, 0x80}, true)                          // INT 0x80 User
	f.Add([]byte{0x0F, 0x30}, true)                          // WRMSR User (should fail gracefully)

	f.Fuzz(func(t *testing.T, code []byte, isUserMode bool) {
		if len(code) == 0 || len(code) > 256 {
			return
		}

		c := New(DefaultMemSize)

		// Setup memory limits and protections based on mode
		if isUserMode {
			c.Mode = ModeUser
			c.R.CS = SelUserCode
			c.R.DS = SelUserData
			c.R.SS = SelUserData
		} else {
			c.Mode = ModeKernel
			c.R.CS = SelKernelCode
			c.R.DS = SelKernelData
			c.R.SS = SelKernelData
		}

		c.R.EIP = AddrUserBase
		c.R.ESP = AddrUserStack + 0xFF000
		c.R.EBP = c.R.ESP

		// Load code bytes into memory
		if err := c.Mem.LoadBytes(AddrUserBase, code); err != nil {
			return
		}

		// Run a limited number of steps to avoid infinite loops
		maxSteps := 50
		for i := 0; i < maxSteps; i++ {
			if c.Halted {
				break
			}
			_, err := c.Step()
			if err != nil {
				// Errors (e.g., page faults, GPFs, invalid opcodes) are expected
				// because we're injecting literal garbage into the stream.
				// The only condition is: the CPU should never panic.
				break
			}
		}
	})
}
