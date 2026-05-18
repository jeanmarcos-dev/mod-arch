package cpu_test

import (
	"fmt"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// ExampleCPU_Step shows the smallest useful program: load an immediate into
// EAX and halt. The CPU executes one instruction per Step call and returns the
// stream of events that describe what just happened.
func ExampleCPU_Step() {
	c := cpu.New(cpu.DefaultMemSize)
	c.Mode = cpu.ModeUser
	c.R.EIP = cpu.AddrUserBase
	c.R.CS = cpu.SelUserCode
	c.R.DS = cpu.SelUserData
	c.R.SS = cpu.SelUserData

	// MOV EAX, 0x42  ; HLT
	program := []byte{0xB8, 0x42, 0x00, 0x00, 0x00, 0xF4}
	_ = c.Mem.LoadBytes(cpu.AddrUserBase, program)

	for !c.Halted {
		if _, err := c.Step(); err != nil {
			fmt.Println("step error:", err)
			return
		}
	}

	fmt.Printf("EAX=0x%X halted=%v\n", c.R.EAX, c.Halted)
	// Output: EAX=0x42 halted=true
}

// ExamplePerm_Has illustrates the read/write/execute permission encoding
// used by Memory regions.
func ExamplePerm_Has() {
	rwx := cpu.PermRead | cpu.PermWrite | cpu.PermExec
	fmt.Println(rwx.Has(cpu.PermRead))
	fmt.Println(rwx.Has(cpu.PermWrite | cpu.PermExec))
	fmt.Println(cpu.PermRead.Has(cpu.PermWrite))
	// Output:
	// true
	// true
	// false
}
