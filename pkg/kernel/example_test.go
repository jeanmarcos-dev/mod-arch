package kernel_test

import (
	"fmt"

	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// ExampleKernel_AddProcess loads a single process whose program writes a
// constant to EAX and halts, then steps the kernel until the CPU is idle.
//
// This shows the smallest kernel cycle: load -> schedule -> step -> halt.
func ExampleKernel_AddProcess() {
	k := kernel.New(4 * 1024 * 1024)
	k.TimerQuantum = 0 // no preemption: single process, single quantum

	// MOV EAX, 0xC0FFEE ; HLT
	program := []byte{0xB8, 0xEE, 0xFF, 0xC0, 0x00, 0xF4}
	if _, err := k.AddProcess("demo", program); err != nil {
		fmt.Println("add:", err)
		return
	}
	// AddProcess now only enqueues into READY; the dispatch happens
	// explicitly. In the simulator this is invoked by StartRun/StepOnce.
	k.DispatchIdleCores()

	for !k.BootCPU().Halted {
		if _, err := k.Step(); err != nil {
			fmt.Println("step:", err)
			return
		}
	}

	fmt.Printf("EAX=0x%X halted=%v\n", k.BootCPU().R.EAX, k.BootCPU().Halted)
	// Output: EAX=0xC0FFEE halted=true
}
