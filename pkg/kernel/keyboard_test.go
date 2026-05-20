package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// readkeyBytes mirrors the demo readkey program literally.
var readkeyBytes = []byte{
	0xB8, 0x03, 0x00, 0x00, 0x00, // mov eax, 3
	0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
	0xB9, 0xE0, 0xFF, 0x33, 0x00, // mov ecx, 0x33FFE0
	0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
	0xCD, 0x80, // int 0x80
	0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4
	0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
	0xB9, 0xE0, 0xFF, 0x33, 0x00, // mov ecx, 0x33FFE0
	0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
	0xCD, 0x80, // int 0x80
	0xEB, 0xD2, // jmp -46
}

func TestKeyboard_BlocksProcessIfEmpty(t *testing.T) {
	k := newTestKernel(t, readkeyBytes)
	k.TimerQuantum = 0

	for i := range 30 {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if k.Processes[1].State == StateBlocked {
			break
		}
	}
	if k.Processes[1].State != StateBlocked {
		t.Fatalf("P1 state = %s, want BLOCKED", k.Processes[1].State)
	}
	if k.Processes[1].BlockedOn != "keyboard" {
		t.Errorf("P1 reason = %q, want keyboard", k.Processes[1].BlockedOn)
	}
	if !k.BootCPU().Halted {
		t.Error("CPU should be HALTED waiting for an IRQ")
	}
	if !k.BootCPU().FlagOn(cpu.FlagIF) {
		t.Error("IF should be on (HALT waiting for IRQ)")
	}
}

func TestKeyboard_PushKeyWakesProcess(t *testing.T) {
	k := newTestKernel(t, readkeyBytes)
	k.TimerQuantum = 0

	for i := range 30 {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if k.Processes[1].State == StateBlocked {
			break
		}
	}
	if k.Processes[1].State != StateBlocked {
		t.Fatal("did not enter BLOCKED")
	}

	// Push 'a' and step until the process resumes.
	k.PushKey('a')

	for range 20 {
		if _, err := k.Step(); err != nil {
			t.Fatal(err)
		}
		if k.Processes[1].State == StateRun {
			break
		}
	}
	if k.Processes[1].State != StateRun {
		t.Errorf("P1 state after IRQ = %s, want RUN", k.Processes[1].State)
	}
	if k.BootCPU().R.EAX != 1 {
		t.Errorf("EAX after read = %d, want 1 (bytes read)", k.BootCPU().R.EAX)
	}
	// Verify the byte 'a' landed at the buffer address.
	b := k.BootCPU().Mem.Bytes[0x33FFE0]
	if b != 'a' {
		t.Errorf("buf @ 0x33FFE0 = 0x%X, want 0x%X ('a')", b, 'a')
	}
}

func TestKeyboard_TwoReadersBlockInQueue(t *testing.T) {
	// Two processes both call sys_read. Both must end up in the wait queue.
	// The first key wakes the first blocked reader (FIFO); the second keeps
	// waiting. Quantum=0 keeps the block sequence deterministic.
	k := newTestKernelMulti(t, readkeyBytes, readkeyBytes)
	k.TimerQuantum = 0

	for range 100 {
		if _, err := k.Step(); err != nil {
			t.Fatal(err)
		}
		if k.Processes[1].State == StateBlocked && k.Processes[2].State == StateBlocked {
			break
		}
	}
	if k.Processes[1].State != StateBlocked || k.Processes[2].State != StateBlocked {
		t.Fatalf("both processes should be BLOCKED, got P1=%s P2=%s",
			k.Processes[1].State, k.Processes[2].State)
	}
	if len(k.blockedReads) != 2 {
		t.Errorf("blockedReads has %d entries, want 2", len(k.blockedReads))
	}

	// Push one key: only the FIRST blocked reader wakes up.
	firstBlockedPID := k.blockedReads[0].PID
	k.PushKey('z')
	for range 30 {
		if _, err := k.Step(); err != nil {
			t.Fatal(err)
		}
		if k.Processes[firstBlockedPID].State == StateRun {
			break
		}
	}

	if k.Processes[firstBlockedPID].State == StateBlocked {
		t.Errorf("P%d should have been woken (it was first in line)", firstBlockedPID)
	}
	// The other process is still BLOCKED waiting for another key.
	otherPID := 3 - firstBlockedPID // 1<->2
	if k.Processes[otherPID].State != StateBlocked {
		t.Errorf("P%d should still be BLOCKED, got %s", otherPID, k.Processes[otherPID].State)
	}
}

func TestKeyboard_EchoFlow(t *testing.T) {
	// Integration: PushKey('x'), step until 'x' appears in console_out.
	k := newTestKernel(t, readkeyBytes)
	k.TimerQuantum = 0

	for range 30 {
		if _, err := k.Step(); err != nil {
			t.Fatal(err)
		}
		if k.Processes[1].State == StateBlocked {
			break
		}
	}

	k.PushKey('x')

	var echoed bool
	for range 60 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if c, ok := e.(cpu.EventConsoleOut); ok && c.PID == 1 && c.Text == "x" {
				echoed = true
			}
		}
		if echoed {
			break
		}
	}
	if !echoed {
		t.Fatal("never saw 'x' on stdout (process should have read it and written back)")
	}
}
