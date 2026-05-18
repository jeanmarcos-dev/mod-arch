package cpu

import (
	"errors"
	"strings"
	"testing"
)

func TestMemory_Read16Write16(t *testing.T) {
	m := NewMemory(DefaultMemSize)
	if err := m.Write16(nil, AddrUserStack+0x100, 0xBEEF, ModeKernel); err != nil {
		t.Fatalf("write16: %v", err)
	}
	v, err := m.Read16(nil, AddrUserStack+0x100, ModeKernel)
	if err != nil {
		t.Fatalf("read16: %v", err)
	}
	if v != 0xBEEF {
		t.Errorf("read16 = 0x%X, want 0xBEEF", v)
	}
}

func TestMemFault_Error(t *testing.T) {
	cases := []struct {
		name    string
		fault   *MemFault
		wantSub string
	}{
		{
			name:    "out of range",
			fault:   &MemFault{Addr: 0xDEADBEEF, Reason: "out of physical range"},
			wantSub: "0xDEADBEEF",
		},
		{
			name:    "perm denied",
			fault:   &MemFault{Addr: 0x300000, Reason: "kernel does not allow X in PL3"},
			wantSub: "kernel does not allow",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.fault.Error()
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tc.wantSub)
			}
		})
	}
}

// TestMemory_ReadInstr_Denied verifies that fetching from a region without
// execute permission produces a MemFault. This is the MMU contract the CPU
// converts into a page fault.
func TestMemory_ReadInstr_Denied(t *testing.T) {
	m := NewMemory(DefaultMemSize)
	_, err := m.ReadInstr(nil, AddrUserStack+0x100, ModeUser)
	if err == nil {
		t.Fatal("ReadInstr in user_stack region: expected fault, got nil")
	}
	var mf *MemFault
	if !errors.As(err, &mf) {
		t.Fatalf("expected *MemFault, got %T: %v", err, err)
	}
	if mf.PermNeeded != "X" {
		t.Errorf("PermNeeded = %q, want %q", mf.PermNeeded, "X")
	}
}
