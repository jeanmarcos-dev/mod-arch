package asm

import "encoding/binary"

// modRMReg returns the ModR/M byte for a register-to-register encoding
// (mod=0b11). reg goes in the reg field (or a sub-opcode for 0x81-style
// instructions), rm in the r/m field.
func modRMReg(reg, rm Reg) byte {
	return 0b1100_0000 | (byte(reg)<<3)&0b0011_1000 | byte(rm)&0b0000_0111
}

// encImm32 returns the little-endian 4-byte encoding of imm.
func encImm32(imm uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], imm)
	return b[:]
}

// encMovImm32 encodes MOV r32, imm32 (opcode B8+rd, 5 bytes).
func encMovImm32(dst Reg, imm uint32) []byte {
	out := make([]byte, 0, 5)
	out = append(out, 0xB8+byte(dst))
	out = append(out, encImm32(imm)...)
	return out
}

// encMovRegReg encodes MOV r/m32, r32 (opcode 89 /r, 2 bytes).
func encMovRegReg(dst, src Reg) []byte {
	return []byte{0x89, modRMReg(src, dst)}
}

// encMovEAXMem encodes MOV [moffs32], EAX (opcode A3, 5 bytes).
// Only valid for EAX as the source — the ISA does not provide a generic
// mem-write form, by design.
func encMovEAXMem(addr uint32) []byte {
	out := make([]byte, 0, 5)
	out = append(out, 0xA3)
	out = append(out, encImm32(addr)...)
	return out
}

// encMovMemEAX encodes MOV EAX, [moffs32] (opcode A1, 5 bytes).
func encMovMemEAX(addr uint32) []byte {
	out := make([]byte, 0, 5)
	out = append(out, 0xA1)
	out = append(out, encImm32(addr)...)
	return out
}

// encAddRegReg encodes ADD r/m32, r32 (opcode 01 /r). dst is r/m, src is reg.
func encAddRegReg(dst, src Reg) []byte { return []byte{0x01, modRMReg(src, dst)} }

// encSubRegReg encodes SUB r/m32, r32 (opcode 29 /r).
func encSubRegReg(dst, src Reg) []byte { return []byte{0x29, modRMReg(src, dst)} }

// encXorRegReg encodes XOR r/m32, r32 (opcode 31 /r).
func encXorRegReg(dst, src Reg) []byte { return []byte{0x31, modRMReg(src, dst)} }

// encCmpRegReg encodes CMP r/m32, r32 (opcode 39 /r). CMP does not write back;
// only flags are updated.
func encCmpRegReg(dst, src Reg) []byte { return []byte{0x39, modRMReg(src, dst)} }

// encArithImm32 encodes the 0x81 /N family: ADD/AND/SUB/XOR/CMP r32, imm32.
// sub selects the operation (0=ADD, 4=AND, 5=SUB, 6=XOR, 7=CMP).
func encArithImm32(sub byte, dst Reg, imm uint32) []byte {
	out := make([]byte, 0, 6)
	out = append(out, 0x81, modRMReg(Reg(sub), dst))
	out = append(out, encImm32(imm)...)
	return out
}

// encIncReg encodes INC r32 (opcode 40+rd, 1 byte).
func encIncReg(r Reg) []byte { return []byte{0x40 + byte(r)} }

// encDecReg encodes DEC r32 (opcode 48+rd, 1 byte).
func encDecReg(r Reg) []byte { return []byte{0x48 + byte(r)} }

// encPushReg encodes PUSH r32 (opcode 50+rd, 1 byte).
func encPushReg(r Reg) []byte { return []byte{0x50 + byte(r)} }

// encPopReg encodes POP r32 (opcode 58+rd, 1 byte).
func encPopReg(r Reg) []byte { return []byte{0x58 + byte(r)} }

// encInt encodes INT imm8 (opcode CD ib, 2 bytes).
func encInt(vec uint8) []byte { return []byte{0xCD, vec} }

// Single-byte instructions.
var (
	encRet  = []byte{0xC3}
	encIret = []byte{0xCF}
	encHlt  = []byte{0xF4}
	encNop  = []byte{0x90}
	encCli  = []byte{0xFA}
	encSti  = []byte{0xFB}
)
