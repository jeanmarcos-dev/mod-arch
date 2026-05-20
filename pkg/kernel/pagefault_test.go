package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// badmemBytes is hand-encoded: mov eax, 0xDEAD ; mov [0x100000], eax ; hlt
var badmemBytes = []byte{
	0xB8, 0xAD, 0xDE, 0x00, 0x00,
	0xA3, 0x00, 0x00, 0x10, 0x00,
	0xF4,
}

func TestPageFault_KillsProcessSingle(t *testing.T) {
	k := newTestKernel(t, badmemBytes)
	k.TimerQuantum = 0

	var pfEv *cpu.EventPageFault
	for range 20 {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step: %v", err)
		}
		for _, e := range evs {
			if pf, ok := e.(cpu.EventPageFault); ok {
				pe := pf
				pfEv = &pe
			}
		}
		if k.BootCPU().Halted {
			break
		}
	}
	if pfEv == nil {
		t.Fatal("no EventPageFault emitted")
	}
	if pfEv.Addr != 0x100000 {
		t.Errorf("PageFault addr = 0x%X, want 0x100000", pfEv.Addr)
	}
	if !k.BootCPU().Halted {
		t.Error("CPU should be halted (only process died)")
	}
	if k.Processes[1].State != StateZombie {
		t.Errorf("P1 state = %s, want ZOMBIE", k.Processes[1].State)
	}
	if k.Processes[1].BlockedOn != "page_fault" {
		t.Errorf("P1 reason = %q, want page_fault", k.Processes[1].BlockedOn)
	}
}

func TestPageFault_KillsProcessSwitchesToNext(t *testing.T) {
	// P1 performs a bad memory access; P2 is READY looping on nops.
	loopProg := []byte{0x90, 0xEB, 0xFE} // nop + jmp -2
	k := newTestKernelMulti(t, badmemBytes, loopProg)
	k.TimerQuantum = 0

	for i := range 30 {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if k.BootCore().CurrentPID == 2 && k.Processes[1].State == StateZombie {
			break
		}
	}

	if k.Processes[1].State != StateZombie {
		t.Errorf("P1 should be ZOMBIE, got %s", k.Processes[1].State)
	}
	if k.Processes[1].BlockedOn != "page_fault" {
		t.Errorf("P1 reason = %q, want page_fault", k.Processes[1].BlockedOn)
	}
	if k.BootCore().CurrentPID != 2 {
		t.Errorf("CurrentPID = %d, want 2 (P2 takes the CPU)", k.BootCore().CurrentPID)
	}
	if k.Processes[2].State != StateRun {
		t.Errorf("P2 state = %s, want RUN", k.Processes[2].State)
	}
	if k.BootCPU().Halted {
		t.Error("CPU should NOT be halted (P2 is still alive)")
	}
}

func TestPageFault_UserCannotReadKernelEither(t *testing.T) {
	// This time the user tries to READ from kernel memory; must also fault.
	readKernel := []byte{
		0xA1, 0x00, 0x00, 0x10, 0x00, // mov eax, [0x100000]
		0xF4,
	}
	k := newTestKernel(t, readKernel)
	k.TimerQuantum = 0

	for range 10 {
		if _, err := k.Step(); err != nil {
			t.Fatal(err)
		}
		if k.BootCPU().Halted {
			break
		}
	}
	if k.Processes[1].State != StateZombie {
		t.Errorf("P1 state = %s, want ZOMBIE", k.Processes[1].State)
	}
}
