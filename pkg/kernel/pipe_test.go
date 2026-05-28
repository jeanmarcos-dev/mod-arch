package kernel

import (
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// TestSysPipe_AllocatesTwoDistinctFDs covers the headline contract of
// sys_pipe: two fresh fds in the caller's FDTable point at the same Pipe,
// one as the read end and one as the write end. Both must be >= 3 so they
// never collide with the stdin/stdout/stderr handles.
func TestSysPipe_AllocatesTwoDistinctFDs(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	pid, err := k.AddProcess("caller", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	boot := k.BootCore()
	boot.CurrentPID = pid

	fdPairAddr := uint32(cpu.AddrUserStack + 0x100)
	result, evs := k.sysPipe(boot, fdPairAddr)

	if result != 0 {
		t.Errorf("sys_pipe result = 0x%X, want 0 (success)", result)
	}
	if len(k.Pipes) != 1 {
		t.Fatalf("k.Pipes len=%d, want 1", len(k.Pipes))
	}
	if _, ok := findEvent[cpu.EventPipeCreate](evs); !ok {
		t.Error("no EventPipeCreate emitted")
	}

	// Read the fd pair out of user memory.
	fdR := readKernelDWORD(boot.CPU, fdPairAddr+0)
	fdW := readKernelDWORD(boot.CPU, fdPairAddr+4)
	if fdR == fdW {
		t.Errorf("fdR == fdW (%d): pipe must allocate two distinct fds", fdR)
	}
	if fdR < 3 || fdW < 3 {
		t.Errorf("pipe fds (%d, %d) collide with stdin/stdout/stderr", fdR, fdW)
	}

	// Resolving each fd must yield the same pipe but with opposite ends.
	caller := k.Processes[pid]
	table := caller.FDSpace().Table
	rEntry := table[fdR]
	wEntry := table[fdW]
	if rEntry.Kind != "pipe-read" || wEntry.Kind != "pipe-write" {
		t.Errorf("fd kinds = (%s, %s), want (pipe-read, pipe-write)", rEntry.Kind, wEntry.Kind)
	}
	if rEntry.PipeID != wEntry.PipeID {
		t.Errorf("read and write fds point at different pipes (%d vs %d)", rEntry.PipeID, wEntry.PipeID)
	}
}

// TestPipeWriteThenRead_SameProcess covers the simplest end-to-end flow:
// the caller writes a few bytes to the write end and then reads them back
// from the read end. Tests buffer accumulation and FIFO ordering without
// any blocking edge cases.
func TestPipeWriteThenRead_SameProcess(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	pid, err := k.AddProcess("caller", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	boot := k.BootCore()
	boot.CurrentPID = pid

	fdPairAddr := uint32(cpu.AddrUserStack + 0x100)
	if r, _ := k.sysPipe(boot, fdPairAddr); r != 0 {
		t.Fatalf("sys_pipe failed: 0x%X", r)
	}
	fdR := readKernelDWORD(boot.CPU, fdPairAddr+0)
	fdW := readKernelDWORD(boot.CPU, fdPairAddr+4)

	// Place "HOLA" in user-writable memory.
	bufAddr := uint32(cpu.AddrUserStack + 0x200)
	copy(k.Mem.Bytes[bufAddr:], []byte("HOLA"))

	// sys_write(fdW, bufAddr, 4)
	wResult, _ := k.sysWrite(boot, fdW, bufAddr, 4)
	if wResult != 4 {
		t.Errorf("sys_write to pipe returned %d, want 4", wResult)
	}

	// sys_read(fdR, otherAddr, 4)
	otherAddr := uint32(cpu.AddrUserStack + 0x210)
	rResult, _ := k.sysRead(boot, fdR, otherAddr, 4)
	if rResult != 4 {
		t.Errorf("sys_read from pipe returned %d, want 4", rResult)
	}
	got := string(k.Mem.Bytes[otherAddr : otherAddr+4])
	if got != "HOLA" {
		t.Errorf("read bytes = %q, want %q", got, "HOLA")
	}
}
