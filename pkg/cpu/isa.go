package cpu

import (
	"encoding/binary"
	"fmt"
)

// regNames maps indices 0-7 of the ModR/M reg/rm fields (and of the +rd embedded
// in some opcodes) to their x86 register names. Note this is Intel's historic
// order, not alphabetical:
//
//	0 = EAX, 1 = ECX, 2 = EDX, 3 = EBX, 4 = ESP, 5 = EBP, 6 = ESI, 7 = EDI
var regNames = [8]string{"EAX", "ECX", "EDX", "EBX", "ESP", "EBP", "ESI", "EDI"}

func init() {
	// Inject the real execNext into the variable declared in cpu.go. This
	// indirection avoids an import cycle and lets cpu.go define a stub.
	execNextImpl = execInstruction
}

// regPtrByIdx returns a pointer to the 32-bit register at index 0-7.
func (c *CPU) regPtrByIdx(i uint8) *uint32 {
	switch i {
	case 0:
		return &c.R.EAX
	case 1:
		return &c.R.ECX
	case 2:
		return &c.R.EDX
	case 3:
		return &c.R.EBX
	case 4:
		return &c.R.ESP
	case 5:
		return &c.R.EBP
	case 6:
		return &c.R.ESI
	case 7:
		return &c.R.EDI
	}
	return nil
}

// setRegByIdx writes a register by index and emits EventRegWrite if it changed.
func (c *CPU) setRegByIdx(i uint8, val uint32) {
	p := c.regPtrByIdx(i)
	old := *p
	if old == val {
		return
	}
	*p = val
	c.Emit(EventRegWrite{Reg: regNames[i], Old: old, New: val})
}

// fetch8 reads one instruction byte at EIP, requiring execute permission, and
// advances EIP.
func (c *CPU) fetch8() (uint8, error) {
	b, err := c.Mem.ReadInstr(c, c.R.EIP, c.Mode)
	if err != nil {
		return 0, err
	}
	c.R.EIP++
	return b, nil
}

// fetch32 reads 4 little-endian instruction bytes.
func (c *CPU) fetch32() (uint32, error) {
	var buf [4]byte
	for i := range 4 {
		b, err := c.Mem.ReadInstr(c, c.R.EIP, c.Mode)
		if err != nil {
			return 0, err
		}
		buf[i] = b
		c.R.EIP++
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

// updateArithFlags updates ZF and SF from a 32-bit result. CF/OF could be
// computed more precisely; ZF and SF are enough for this subset.
func (c *CPU) updateArithFlags(result uint32) {
	c.FlagSet(FlagZF, result == 0)
	c.FlagSet(FlagSF, result&0x80000000 != 0)
}

// execInstruction is the decoder/executor. It reads EIP, dispatches by opcode,
// and emits EventInstruction as the first event of the step.
//
// The implementation is deliberately flat (one big switch) so it reads as a
// pedagogical reference. Each case documents the real x86 opcode.
//
//nolint:gocognit,gocyclo // x86 opcode dispatcher; complexity reflects ISA size and is intentional.
func execInstruction(c *CPU) error {
	startEIP := c.R.EIP

	opcode, err := c.fetch8()
	if err != nil {
		return err
	}

	switch {
	// MOV r32, imm32   — opcode B8+rd  (5 bytes total)
	// Loads a 32-bit immediate into a register.
	case opcode >= 0xB8 && opcode <= 0xBF:
		regI := opcode - 0xB8
		imm, err := c.fetch32()
		if err != nil {
			return err
		}
		c.emitInstr(startEIP, fmt.Sprintf("MOV %s, 0x%08X", regNames[regI], imm))
		c.setRegByIdx(regI, imm)

	// INC r32   — opcode 40+rd  (1 byte)
	// DEC r32   — opcode 48+rd  (1 byte)
	case opcode >= 0x40 && opcode <= 0x47:
		regI := opcode - 0x40
		p := c.regPtrByIdx(regI)
		c.emitInstr(startEIP, fmt.Sprintf("INC %s", regNames[regI]))
		result := *p + 1
		c.setRegByIdx(regI, result)
		c.updateArithFlags(result)

	case opcode >= 0x48 && opcode <= 0x4F:
		regI := opcode - 0x48
		p := c.regPtrByIdx(regI)
		c.emitInstr(startEIP, fmt.Sprintf("DEC %s", regNames[regI]))
		result := *p - 1
		c.setRegByIdx(regI, result)
		c.updateArithFlags(result)

	// PUSH r32  — opcode 50+rd  (1 byte)
	// POP  r32  — opcode 58+rd  (1 byte)
	case opcode >= 0x50 && opcode <= 0x57:
		regI := opcode - 0x50
		val := *c.regPtrByIdx(regI)
		c.emitInstr(startEIP, fmt.Sprintf("PUSH %s", regNames[regI]))
		// Stack grows downward: decrement ESP first, then write.
		newESP := c.R.ESP - 4
		if err := c.Mem.Write32(c, newESP, val, c.Mode); err != nil {
			return err
		}
		c.setRegByIdx(4, newESP) // ESP is idx 4
	case opcode >= 0x58 && opcode <= 0x5F:
		regI := opcode - 0x58
		c.emitInstr(startEIP, fmt.Sprintf("POP %s", regNames[regI]))
		val, err := c.Mem.Read32(c, c.R.ESP, c.Mode)
		if err != nil {
			return err
		}
		c.setRegByIdx(regI, val)
		c.setRegByIdx(4, c.R.ESP+4)

	// Register-to-register arithmetic with ModR/M. Mod must be 11 (reg-to-reg).
	//   01 /r  ADD r/m32, r32
	//   29 /r  SUB r/m32, r32
	//   31 /r  XOR r/m32, r32
	//   39 /r  CMP r/m32, r32   (like SUB but does not write destination)
	//   89 /r  MOV r/m32, r32
	case opcode == 0x01 || opcode == 0x29 || opcode == 0x31 || opcode == 0x39 || opcode == 0x89:
		modrm, err := c.fetch8()
		if err != nil {
			return err
		}
		mod, regSrc, rm := decodeModRM(modrm)
		if mod != 0b11 {
			return fmt.Errorf("opcode 0x%02X with mod=%b: memory operands not supported", opcode, mod)
		}
		src := *c.regPtrByIdx(regSrc)
		dst := *c.regPtrByIdx(rm)
		var result uint32
		var mnem string
		switch opcode {
		case 0x01:
			result = dst + src
			mnem = "ADD"
		case 0x29:
			result = dst - src
			mnem = "SUB"
		case 0x31:
			result = dst ^ src
			mnem = "XOR"
		case 0x39:
			// CMP does not write back, only updates flags.
			result = dst - src
			mnem = "CMP"
		case 0x89:
			result = src
			mnem = "MOV"
		}
		c.emitInstr(startEIP, fmt.Sprintf("%s %s, %s", mnem, regNames[rm], regNames[regSrc]))
		if opcode != 0x39 {
			c.setRegByIdx(rm, result)
		}
		if opcode != 0x89 {
			// MOV does not affect flags.
			c.updateArithFlags(result)
		}

	// 81 /N r/m32, imm32 — arithmetic with immediate. N goes in the ModR/M
	// reg field and selects the operation:
	//   /0 ADD, /4 AND, /5 SUB, /6 XOR, /7 CMP
	case opcode == 0x81:
		modrm, err := c.fetch8()
		if err != nil {
			return err
		}
		mod, sub, rm := decodeModRM(modrm)
		if mod != 0b11 {
			return fmt.Errorf("0x81 with mod=%b: only register operands supported", mod)
		}
		imm, err := c.fetch32()
		if err != nil {
			return err
		}
		dst := *c.regPtrByIdx(rm)
		var result uint32
		var mnem string
		switch sub {
		case 0:
			result = dst + imm
			mnem = "ADD"
		case 4:
			result = dst & imm
			mnem = "AND"
		case 5:
			result = dst - imm
			mnem = "SUB"
		case 6:
			result = dst ^ imm
			mnem = "XOR"
		case 7:
			result = dst - imm
			mnem = "CMP"
		default:
			return fmt.Errorf("0x81 /%d not supported", sub)
		}
		c.emitInstr(startEIP, fmt.Sprintf("%s %s, 0x%X", mnem, regNames[rm], imm))
		if sub != 7 {
			c.setRegByIdx(rm, result)
		}
		c.updateArithFlags(result)

	// Short relative jumps (1-byte signed offset)
	//   EB rel8   JMP
	//   74 rel8   JE / JZ   (taken when ZF=1)
	//   75 rel8   JNE / JNZ (taken when ZF=0)
	case opcode == 0xEB || opcode == 0x74 || opcode == 0x75:
		off, err := c.fetch8()
		if err != nil {
			return err
		}
		rel := int32(int8(off))
		var mnem string
		var take bool
		switch opcode {
		case 0xEB:
			mnem = "JMP"
			take = true
		case 0x74:
			mnem = "JE"
			take = c.FlagOn(FlagZF)
		case 0x75:
			mnem = "JNE"
			take = !c.FlagOn(FlagZF)
		}
		// EIP already points at the next instruction (zero offset).
		target := uint32(int32(c.R.EIP) + rel)
		c.emitInstr(startEIP, fmt.Sprintf("%s 0x%X", mnem, target))
		if take {
			c.R.EIP = target
		}

	// Long jumps (4-byte offset)
	//   E9 rel32   JMP
	case opcode == 0xE9:
		off, err := c.fetch32()
		if err != nil {
			return err
		}
		target := uint32(int32(c.R.EIP) + int32(off))
		c.emitInstr(startEIP, fmt.Sprintf("JMP 0x%X", target))
		c.R.EIP = target

	// E8 rel32   CALL (push return address, jump)
	// C3         RET   (pop EIP)
	case opcode == 0xE8:
		off, err := c.fetch32()
		if err != nil {
			return err
		}
		retAddr := c.R.EIP
		target := uint32(int32(c.R.EIP) + int32(off))
		c.emitInstr(startEIP, fmt.Sprintf("CALL 0x%X", target))
		newESP := c.R.ESP - 4
		if err := c.Mem.Write32(c, newESP, retAddr, c.Mode); err != nil {
			return err
		}
		c.setRegByIdx(4, newESP)
		c.R.EIP = target

	case opcode == 0xC3:
		c.emitInstr(startEIP, "RET")
		retAddr, err := c.Mem.Read32(c, c.R.ESP, c.Mode)
		if err != nil {
			return err
		}
		c.setRegByIdx(4, c.R.ESP+4)
		c.R.EIP = retAddr

	// MOV EAX, [moffs32]   — opcode A1  (load 32 bits from absolute address)
	// MOV [moffs32], EAX   — opcode A3  (store EAX at absolute address)
	// Compact x86 form without ModR/M, valid only for EAX.
	case opcode == 0xA1:
		addr, err := c.fetch32()
		if err != nil {
			return err
		}
		c.emitInstr(startEIP, fmt.Sprintf("MOV EAX, [0x%X]", addr))
		v, err := c.Mem.Read32(c, addr, c.Mode)
		if err != nil {
			return err
		}
		c.setRegByIdx(0, v)

	case opcode == 0xA3:
		addr, err := c.fetch32()
		if err != nil {
			return err
		}
		c.emitInstr(startEIP, fmt.Sprintf("MOV [0x%X], EAX", addr))
		if err := c.Mem.Write32(c, addr, c.R.EAX, c.Mode); err != nil {
			return err
		}

	// CD imm8   INT N  (software interrupt)
	// CF        IRET   (return from interrupt)
	// F4        HLT    (halt until IRQ)
	// 90        NOP
	case opcode == 0xCD:
		vec, err := c.fetch8()
		if err != nil {
			return err
		}
		c.emitInstr(startEIP, fmt.Sprintf("INT 0x%X", vec))
		return c.dispatchIRQ(vec, "SW")

	case opcode == 0xCF:
		c.emitInstr(startEIP, "IRET")
		return c.handleIRET()

	case opcode == 0xF4:
		c.emitInstr(startEIP, "HLT")
		c.Halted = true
		c.Emit(EventHalt{})

	case opcode == 0x90:
		c.emitInstr(startEIP, "NOP")

	default:
		return fmt.Errorf("opcode 0x%02X not implemented at EIP=0x%08X", opcode, startEIP)
	}

	return nil
}

// decodeModRM extracts the three ModR/M fields: mod (2 bits), reg (3 bits), r/m (3 bits).
func decodeModRM(b uint8) (mod, reg, rm uint8) {
	mod = (b >> 6) & 0b11
	reg = (b >> 3) & 0b111
	rm = b & 0b111
	return
}

// emitInstr is a shortcut for EventInstruction that gathers the bytes read.
func (c *CPU) emitInstr(startEIP uint32, mnemonic string) {
	bytes := append([]byte(nil), c.Mem.Bytes[startEIP:c.R.EIP]...)
	c.Emit(EventInstruction{
		// PID is overwritten by the kernel once processes exist.
		PID:      0,
		EIP:      startEIP,
		Mnemonic: mnemonic,
		Bytes:    bytes,
	})
}
