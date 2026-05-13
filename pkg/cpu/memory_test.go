package cpu

import (
	"errors"
	"testing"
)

func TestMemory_Regions(t *testing.T) {
	m := NewMemory(DefaultMemSize)

	tests := []struct {
		addr uint32
		want string
	}{
		{0x000000, "reserved"},
		{0x0FFFFF, "reserved"},
		{0x100000, "kernel"},
		{0x1FFFFF, "kernel"},
		{0x200000, "user_code"},
		{0x2FFFFF, "user_code"},
		{0x300000, "user_stack"},
		{0x3FFFFF, "user_stack"},
	}
	for _, tc := range tests {
		got := m.regionAt(tc.addr)
		if got == nil || got.Name != tc.want {
			t.Errorf("regionAt(0x%06X) = %v, want %s", tc.addr, got, tc.want)
		}
	}

	if r := m.regionAt(0x400000); r != nil {
		t.Errorf("regionAt outside any region should be nil, got %s", r.Name)
	}
}

func TestMemory_UserCannotReadKernel(t *testing.T) {
	m := NewMemory(DefaultMemSize)
	cpu := New(DefaultMemSize)
	cpu.Mem = m

	_, err := m.Read32(cpu, 0x100000, ModeUser)
	if err == nil {
		t.Fatal("user should not be able to read kernel addresses")
	}
	var fault *MemFault
	if !errors.As(err, &fault) {
		t.Fatalf("error should be *MemFault, got %T: %v", err, err)
	}
	if fault.Region != "kernel" {
		t.Errorf("Region = %q, want kernel", fault.Region)
	}
}

func TestMemory_UserCanReadUserCode(t *testing.T) {
	m := NewMemory(DefaultMemSize)
	cpu := New(DefaultMemSize)
	cpu.Mem = m
	if err := m.LoadBytes(0x200000, []byte{0xDE, 0xAD, 0xBE, 0xEF}); err != nil {
		t.Fatal(err)
	}

	v, err := m.Read32(cpu, 0x200000, ModeUser)
	if err != nil {
		t.Fatalf("user should be able to read user_code: %v", err)
	}
	if v != 0xEFBEADDE {
		t.Errorf("Read32 little-endian = 0x%X, want 0xEFBEADDE", v)
	}
}

func TestMemory_UserCannotWriteUserCode(t *testing.T) {
	m := NewMemory(DefaultMemSize)
	cpu := New(DefaultMemSize)
	cpu.Mem = m

	err := m.Write32(cpu, 0x200000, 0xCAFE, ModeUser)
	if err == nil {
		t.Fatal("user should not be able to write user_code (it is RX)")
	}
}

func TestMemory_UserCanWriteUserStack(t *testing.T) {
	m := NewMemory(DefaultMemSize)
	cpu := New(DefaultMemSize)
	cpu.Mem = m

	if err := m.Write32(cpu, 0x300000, 0xCAFE, ModeUser); err != nil {
		t.Fatalf("user should be able to write user_stack: %v", err)
	}
	v, err := m.Read32(cpu, 0x300000, ModeUser)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0xCAFE {
		t.Errorf("read after write = 0x%X, want 0xCAFE", v)
	}
}

func TestMemory_KernelCanReadKernel(t *testing.T) {
	m := NewMemory(DefaultMemSize)
	cpu := New(DefaultMemSize)
	cpu.Mem = m
	if err := m.LoadBytes(0x100000, []byte{0xAB}); err != nil {
		t.Fatal(err)
	}

	v, err := m.Read8(cpu, 0x100000, ModeKernel)
	if err != nil {
		t.Fatalf("kernel should read kernel: %v", err)
	}
	if v != 0xAB {
		t.Errorf("Read8 = 0x%X, want 0xAB", v)
	}
}

func TestMemory_EventsEmitted(t *testing.T) {
	m := NewMemory(DefaultMemSize)
	cpu := New(DefaultMemSize)
	cpu.Mem = m

	if err := m.Write32(cpu, 0x300000, 0x1234, ModeUser); err != nil {
		t.Fatal(err)
	}
	if len(cpu.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cpu.events))
	}
	w, ok := cpu.events[0].(EventMemWrite)
	if !ok {
		t.Fatalf("expected EventMemWrite, got %T", cpu.events[0])
	}
	if w.Addr != 0x300000 || w.New != 0x1234 {
		t.Errorf("wrong event: %+v", w)
	}
}

func TestCPU_SetRegEmitsEvent(t *testing.T) {
	cpu := New(DefaultMemSize)
	cpu.events = nil

	if err := cpu.SetReg32("EAX", 42); err != nil {
		t.Fatal(err)
	}
	if cpu.R.EAX != 42 {
		t.Errorf("EAX = %d, want 42", cpu.R.EAX)
	}
	if len(cpu.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cpu.events))
	}

	// Setting the same value should not emit.
	cpu.events = nil
	if err := cpu.SetReg32("EAX", 42); err != nil {
		t.Fatal(err)
	}
	if len(cpu.events) != 0 {
		t.Errorf("no-op should not emit event, got %d", len(cpu.events))
	}
}

func TestCPU_Flags(t *testing.T) {
	cpu := New(DefaultMemSize)
	cpu.FlagSet(FlagZF, true)
	if !cpu.FlagOn(FlagZF) {
		t.Error("ZF should be set")
	}
	cpu.FlagSet(FlagZF, false)
	if cpu.FlagOn(FlagZF) {
		t.Error("ZF should be clear")
	}
}
