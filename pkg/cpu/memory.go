package cpu

import (
	"encoding/binary"
	"fmt"
)

// Perm declares what is allowed in a memory region, split by mode.
//
// In real x86 the MMU performs this check by walking page tables (U/S bit of
// each PTE). Here it is simplified to contiguous regions with fixed perms.
// The important part is that the check is performed by the HARDWARE (the
// Read/Write methods), not by software.
type Perm uint8

// Memory permission bits.
const (
	PermNone  Perm = 0
	PermRead  Perm = 1 << 0
	PermWrite Perm = 1 << 1
	PermExec  Perm = 1 << 2
)

// Has reports whether p includes every bit set in want.
func (p Perm) Has(want Perm) bool { return p&want == want }

// Region is a contiguous memory range with per-mode permissions.
type Region struct {
	Name     string
	Start    uint32 // inclusive
	End      uint32 // inclusive
	UserPerm Perm   // permissions when CPU is in PL3
	KernPerm Perm   // permissions when CPU is in PL0
}

func (r Region) contains(addr uint32) bool { return addr >= r.Start && addr <= r.End }

// Memory is the flat byte array plus the region list.
//
// The CPU asks for R/W with its current mode; Memory checks permissions and,
// on failure, emits an EventPageFault and returns an error. The caller decides
// what to do with that error — typically, raise INT 0x0E.
type Memory struct {
	Bytes   []byte
	Regions []Region
}

// Base addresses of the simplified layout. 4 MB total is enough for the demo
// and keeps hex addresses readable (6 digits).
const (
	AddrReserved   uint32 = 0x000000 // BIOS/IDT/GDT
	AddrKernelBase uint32 = 0x100000 // 1 MB, where the kernel starts
	AddrUserBase   uint32 = 0x200000 // 2 MB, where user programs start
	AddrUserStack  uint32 = 0x300000 // 3 MB, user stack base (grows downward from +1 MB)
	AddrUserTop    uint32 = 0x3FFFFF // 4 MB - 1, top of physical memory

	// IDT: 256-vector table; each entry points to the handler address in
	// kernel space. In real x86 these are 8-byte descriptors; here they
	// are simplified to 4-byte pointers.
	AddrIDT        uint32 = 0x100000 // base of the IDT (256 * 4 = 1024 bytes)
	AddrKernelText uint32 = 0x101000 // where the kernel handlers start
	KernelStackTop uint32 = 0x1FFFF0 // top of the kernel stack; grows downward

	// Page-fault mirrors. Real x86 puts the faulting address in CR2 and pushes
	// the error code onto the kernel stack right after EIP. The simulator has
	// no control registers and avoids the stack push (to keep IRET frames
	// uniform across vectors); a handler reads these two words instead. They
	// live in the gap between the IDT (0x100000..0x1003FF) and the handlers
	// (0x101000+), inside the kernel-only region.
	AddrCR2Mirror     uint32 = 0x100400 // faulting linear address
	AddrErrCodeMirror uint32 = 0x100404 // P/W/U/I-D bits (see EventPageFault doc)
)

// DefaultMemSize is the default physical memory size: 4 MB.
const DefaultMemSize = 4 * 1024 * 1024

// NewMemory creates memory with the default region layout:
//
//	0x000000 - 0x0FFFFF  reserved (IDT/GDT/BIOS)         — kernel R/W
//	0x100000 - 0x1FFFFF  kernel .text + .data + stack    — kernel R/W/X
//	0x200000 - 0x2FFFFF  user .text + .data              — user RX, kernel RW
//	0x300000 - 0x3FFFFF  user stack                      — user RW, kernel RW
//
// Addresses are chosen to be easy to read in hex (6 digits) and to follow the
// classic x86 convention: low = BIOS, kernel above it, user on top.
func NewMemory(size int) *Memory {
	return &Memory{
		Bytes: make([]byte, size),
		Regions: []Region{
			{
				Name:  "reserved",
				Start: 0x000000, End: 0x0FFFFF,
				UserPerm: PermNone,
				KernPerm: PermRead | PermWrite,
			},
			{
				Name:  "kernel",
				Start: 0x100000, End: 0x1FFFFF,
				UserPerm: PermNone,
				KernPerm: PermRead | PermWrite | PermExec,
			},
			{
				Name:  "user_code",
				Start: 0x200000, End: 0x2FFFFF,
				UserPerm: PermRead | PermExec,
				KernPerm: PermRead | PermWrite | PermExec,
			},
			{
				Name:  "user_stack",
				Start: 0x300000, End: 0x3FFFFF,
				UserPerm: PermRead | PermWrite,
				KernPerm: PermRead | PermWrite,
			},
		},
	}
}

// regionAt returns the region containing the address, or nil if none does.
func (m *Memory) regionAt(addr uint32) *Region {
	for i := range m.Regions {
		if m.Regions[i].contains(addr) {
			return &m.Regions[i]
		}
	}
	return nil
}

// permForMode returns the effective permissions for mode in region r.
func permForMode(r *Region, mode uint8) Perm {
	if mode == ModeKernel {
		return r.KernPerm
	}
	return r.UserPerm
}

// check validates that access "want" is allowed for "mode" in addr..addr+size-1.
// On failure it returns a MemFault with the offending region and required perm.
func (m *Memory) check(addr uint32, size int, mode uint8, want Perm) error {
	if int(addr)+size > len(m.Bytes) {
		return &MemFault{Addr: addr, Reason: "out of physical range"}
	}
	// An access may span multiple regions; ALL touched regions must grant the
	// permission. In practice regions are large so this rarely matters.
	for off := range size {
		a := addr + uint32(off) //nolint:gosec // off < size, safe conversion
		r := m.regionAt(a)
		if r == nil {
			return &MemFault{Addr: a, Reason: "address has no region"}
		}
		if !permForMode(r, mode).Has(want) {
			return &MemFault{
				Addr:       a,
				Reason:     fmt.Sprintf("region %s does not allow %s in mode PL%d", r.Name, permString(want), mode),
				PermNeeded: permString(want),
				Region:     r.Name,
				Mode:       mode,
			}
		}
	}
	return nil
}

// MemFault is the error returned by Memory on an invalid access. The caller
// (typically the ISA) converts it into EventPageFault + INT 0x0E.
type MemFault struct {
	Addr       uint32
	Reason     string
	PermNeeded string
	Region     string
	Mode       uint8
}

// Error implements the error interface.
func (e *MemFault) Error() string {
	return fmt.Sprintf("memfault @ 0x%08X: %s", e.Addr, e.Reason)
}

func permString(p Perm) string {
	s := ""
	if p&PermRead != 0 {
		s += "R"
	}
	if p&PermWrite != 0 {
		s += "W"
	}
	if p&PermExec != 0 {
		s += "X"
	}
	if s == "" {
		s = "-"
	}
	return s
}

// Read8 reads one byte at addr, checking read permission, and emits
// EventMemRead on cpu (cpu may be nil for internal accesses without tracing).
func (m *Memory) Read8(cpu *CPU, addr uint32, mode uint8) (uint8, error) {
	if err := m.check(addr, 1, mode, PermRead); err != nil {
		return 0, err
	}
	v := m.Bytes[addr]
	if cpu != nil {
		cpu.Emit(EventMemRead{CoreID: cpu.CoreID, Addr: addr, Value: uint32(v), Size: 1})
	}
	return v, nil
}

// Read16 reads a little-endian 16-bit value at addr.
func (m *Memory) Read16(cpu *CPU, addr uint32, mode uint8) (uint16, error) {
	if err := m.check(addr, 2, mode, PermRead); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint16(m.Bytes[addr:])
	if cpu != nil {
		cpu.Emit(EventMemRead{CoreID: cpu.CoreID, Addr: addr, Value: uint32(v), Size: 2})
	}
	return v, nil
}

// Read32 reads a little-endian 32-bit value at addr.
func (m *Memory) Read32(cpu *CPU, addr uint32, mode uint8) (uint32, error) {
	if err := m.check(addr, 4, mode, PermRead); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(m.Bytes[addr:])
	if cpu != nil {
		cpu.Emit(EventMemRead{CoreID: cpu.CoreID, Addr: addr, Value: v, Size: 4})
	}
	return v, nil
}

// ReadInstr is like Read8 but requires execute permission instead of read.
// A real CPU's instruction fetch uses the instruction bus and checks the X bit.
func (m *Memory) ReadInstr(cpu *CPU, addr uint32, mode uint8) (uint8, error) {
	if err := m.check(addr, 1, mode, PermExec); err != nil {
		return 0, err
	}
	return m.Bytes[addr], nil
}

// Write8 writes one byte little-endian, checking write permission.
func (m *Memory) Write8(cpu *CPU, addr uint32, val uint8, mode uint8) error {
	if err := m.check(addr, 1, mode, PermWrite); err != nil {
		return err
	}
	old := m.Bytes[addr]
	m.Bytes[addr] = val
	if cpu != nil {
		cpu.Emit(EventMemWrite{CoreID: cpu.CoreID, Addr: addr, Old: uint32(old), New: uint32(val), Size: 1})
	}
	return nil
}

// Write16 writes a little-endian 16-bit value, checking write permission.
func (m *Memory) Write16(cpu *CPU, addr uint32, val uint16, mode uint8) error {
	if err := m.check(addr, 2, mode, PermWrite); err != nil {
		return err
	}
	old := binary.LittleEndian.Uint16(m.Bytes[addr:])
	binary.LittleEndian.PutUint16(m.Bytes[addr:], val)
	if cpu != nil {
		cpu.Emit(EventMemWrite{CoreID: cpu.CoreID, Addr: addr, Old: uint32(old), New: uint32(val), Size: 2})
	}
	return nil
}

// Write32 writes a little-endian 32-bit value, checking write permission.
func (m *Memory) Write32(cpu *CPU, addr uint32, val uint32, mode uint8) error {
	if err := m.check(addr, 4, mode, PermWrite); err != nil {
		return err
	}
	old := binary.LittleEndian.Uint32(m.Bytes[addr:])
	binary.LittleEndian.PutUint32(m.Bytes[addr:], val)
	if cpu != nil {
		cpu.Emit(EventMemWrite{CoreID: cpu.CoreID, Addr: addr, Old: old, New: val, Size: 4})
	}
	return nil
}

// faultErrorCode synthesizes the 32-bit value that real x86 would push after
// EIP on a #PF. P/W/U/I-D bits follow Intel SDM Vol 3 §6.15.
func faultErrorCode(mf *MemFault) uint32 {
	var code uint32
	if mf.PermNeeded != "" {
		code |= 0x1 // P: a region exists, this is a protection violation
	}
	if mf.PermNeeded == "W" {
		code |= 0x2 // W
	}
	if mf.Mode == ModeUser {
		code |= 0x4 // U
	}
	if mf.PermNeeded == "X" {
		code |= 0x10 // I-D
	}
	return code
}

// writeFaultMirror updates the CR2 and error-code mirror words. The write
// bypasses permission checks because the MMU itself is doing it before any
// handler runs (equivalent to the CPU loading CR2 in real x86).
func writeFaultMirror(m *Memory, faultAddr, errorCode uint32) {
	binary.LittleEndian.PutUint32(m.Bytes[AddrCR2Mirror:], faultAddr)
	binary.LittleEndian.PutUint32(m.Bytes[AddrErrCodeMirror:], errorCode)
}

// LoadBytes copies a byte slice to addr without checking permissions. It is
// intended for the kernel LOADER — equivalent to placing the binary in physical
// memory before execution. It is not called from the ISA.
func (m *Memory) LoadBytes(addr uint32, data []byte) error {
	if int(addr)+len(data) > len(m.Bytes) {
		return fmt.Errorf("load: %d bytes at 0x%08X exceed physical size", len(data), addr)
	}
	copy(m.Bytes[addr:], data)
	return nil
}
