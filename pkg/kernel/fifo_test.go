package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestSysMknod_RegistersFIFOByName covers the headline contract: mknod
// inserts the name into k.FifoByName, allocates a Pipe in IsFifo mode,
// and emits an EventPipeCreate carrying the name.
func TestSysMknod_RegistersFIFOByName(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	pid, err := k.AddProcess("caller", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	boot := k.BootCore()
	boot.CurrentPID = pid

	nameAddr := uint32(cpu.AddrUserStack + 0x100)
	copy(k.Mem.Bytes[nameAddr:], []byte("queue\x00"))

	result, evs := k.sysMknod(boot, nameAddr)
	if result != 0 {
		t.Errorf("sysMknod result = 0x%X, want 0", result)
	}
	if id, ok := k.FifoByName["queue"]; !ok {
		t.Error("FifoByName missing entry for \"queue\"")
	} else if id != 1 {
		t.Errorf("FifoByName[queue] = %d, want 1", id)
	}
	ev, ok := findEvent[cpu.EventPipeCreate](evs)
	if !ok {
		t.Fatal("no EventPipeCreate emitted")
	}
	if !ev.IsFifo || ev.Name != "queue" {
		t.Errorf("EventPipeCreate IsFifo=%v Name=%q, want true \"queue\"", ev.IsFifo, ev.Name)
	}
}

// TestSysOpen_TwoProcessesOneFIFO verifies that two unrelated processes
// can connect through a FIFO opened by name: A writes, B reads, and the
// bytes flow even though A and B were never parent and child.
func TestSysOpen_TwoProcessesOneFIFO(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	pidA, err := k.AddProcess("A", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess A: %v", err)
	}
	pidB, err := k.AddProcess("B", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess B: %v", err)
	}

	nameAddr := uint32(cpu.AddrUserStack + 0x100)
	copy(k.Mem.Bytes[nameAddr:], []byte("q\x00"))
	// A creates and opens write end
	boot := k.BootCore()
	boot.CurrentPID = pidA
	if r, _ := k.sysMknod(boot, nameAddr); r != 0 {
		t.Fatalf("mknod failed: 0x%X", r)
	}
	fdW, _ := k.sysOpen(boot, nameAddr, OpenWriteOnly)
	if fdW == ^uint32(0) {
		t.Fatal("A's open(write) failed")
	}

	// B opens read end (different process)
	boot.CurrentPID = pidB
	fdR, _ := k.sysOpen(boot, nameAddr, OpenReadOnly)
	if fdR == ^uint32(0) {
		t.Fatal("B's open(read) failed")
	}

	// A writes "OK"
	boot.CurrentPID = pidA
	bufAddrA := uint32(cpu.AddrUserStack + 0x200)
	copy(k.Mem.Bytes[bufAddrA:], []byte("OK"))
	if n, _ := k.sysWrite(boot, fdW, bufAddrA, 2); n != 2 {
		t.Errorf("A's write returned %d, want 2", n)
	}

	// B reads
	boot.CurrentPID = pidB
	bufAddrB := uint32(cpu.AddrUserStack + 0x300)
	if n, _ := k.sysRead(boot, fdR, bufAddrB, 2); n != 2 {
		t.Errorf("B's read returned %d, want 2", n)
	}
	got := string(k.Mem.Bytes[bufAddrB : bufAddrB+2])
	if got != "OK" {
		t.Errorf("B read %q, want %q", got, "OK")
	}
}
