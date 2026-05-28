package cpu

import (
	"encoding/binary"
	"fmt"
	"math/bits"
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
	c.Emit(EventRegWrite{CoreID: c.CoreID, Reg: regNames[i], Old: old, New: val})
}

// setEIP redirects the instruction pointer and emits reg_write so the UI sees
// the jump explicitly. Used by JMP/CALL/RET and the HW dispatch/IRET paths.
// The implicit EIP advance done by fetch8/fetch32 does NOT go through here —
// emitting reg_write per byte would drown the event stream.
func (c *CPU) setEIP(val uint32) {
	if c.R.EIP == val {
		return
	}
	old := c.R.EIP
	c.R.EIP = val
	c.Emit(EventRegWrite{CoreID: c.CoreID, Reg: "EIP", Old: old, New: val})
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

// arithOp tags how updateArithFlags must compute CF and OF (ZF/SF are always
// derived from the result). The x86 manual specifies per-instruction whether
// each flag is set, cleared, or untouched; this enum encodes that policy.
type arithOp int

const (
	opADD arithOp = iota
	opSUB // CMP shares the same flag effects as SUB.
	opAND
	opXOR
	opINC // CF preserved; OF set only when crossing the signed maximum.
	opDEC // CF preserved; OF set only when crossing the signed minimum.
)

// updateArithFlags applies the x86 flag-update rules for a 32-bit arithmetic
// or logical result. a is the destination value before the operation, b the
// second operand (ignored for INC/DEC).
func (c *CPU) updateArithFlags(op arithOp, a, b, result uint32) {
	c.FlagSet(FlagZF, result == 0)
	c.FlagSet(FlagSF, result&0x80000000 != 0)
	// PF reflects the parity of the low 8 bits of the result (even -> 1).
	c.FlagSet(FlagPF, bits.OnesCount8(uint8(result))%2 == 0)
	switch op {
	case opADD:
		c.FlagSet(FlagCF, result < a)
		c.FlagSet(FlagOF, (^(a^b)&(a^result))&0x80000000 != 0)
	case opSUB:
		c.FlagSet(FlagCF, a < b)
		c.FlagSet(FlagOF, ((a^b)&(a^result))&0x80000000 != 0)
	case opAND, opXOR:
		c.FlagSet(FlagCF, false)
		c.FlagSet(FlagOF, false)
	case opINC:
		c.FlagSet(FlagOF, a == 0x7FFFFFFF)
	case opDEC:
		c.FlagSet(FlagOF, a == 0x80000000)
	}
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
	// Record the fault site so dispatchFault can rewind EIP if a mid-instruction
	// fault fires (e.g. page fault on a memory operand, #GP from HLT in PL3).
	c.lastInstrStart = startEIP

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
		old := *c.regPtrByIdx(regI)
		c.emitInstr(startEIP, fmt.Sprintf("INC %s", regNames[regI]))
		result := old + 1
		c.setRegByIdx(regI, result)
		c.updateArithFlags(opINC, old, 0, result)

	case opcode >= 0x48 && opcode <= 0x4F:
		regI := opcode - 0x48
		old := *c.regPtrByIdx(regI)
		c.emitInstr(startEIP, fmt.Sprintf("DEC %s", regNames[regI]))
		result := old - 1
		c.setRegByIdx(regI, result)
		c.updateArithFlags(opDEC, old, 0, result)

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
		var op arithOp
		switch opcode {
		case 0x01:
			result = dst + src
			mnem = "ADD"
			op = opADD
		case 0x29:
			result = dst - src
			mnem = "SUB"
			op = opSUB
		case 0x31:
			result = dst ^ src
			mnem = "XOR"
			op = opXOR
		case 0x39:
			// CMP does not write back, only updates flags.
			result = dst - src
			mnem = "CMP"
			op = opSUB
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
			c.updateArithFlags(op, dst, src, result)
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
		var op arithOp
		switch sub {
		case 0:
			result = dst + imm
			mnem = "ADD"
			op = opADD
		case 4:
			result = dst & imm
			mnem = "AND"
			op = opAND
		case 5:
			result = dst - imm
			mnem = "SUB"
			op = opSUB
		case 6:
			result = dst ^ imm
			mnem = "XOR"
			op = opXOR
		case 7:
			result = dst - imm
			mnem = "CMP"
			op = opSUB
		default:
			return fmt.Errorf("0x81 /%d not supported", sub)
		}
		c.emitInstr(startEIP, fmt.Sprintf("%s %s, 0x%X", mnem, regNames[rm], imm))
		if sub != 7 {
			c.setRegByIdx(rm, result)
		}
		c.updateArithFlags(op, dst, imm, result)

	// Short relative jumps (1-byte signed offset)
	//   EB         JMP rel8 (unconditional)
	//   70..7F     Jcc rel8 (full Intel set: JO/JNO, JB/JAE, JE/JNE, JBE/JA,
	//              JS/JNS, JP/JNP, JL/JGE, JLE/JG)
	case opcode == 0xEB || (opcode >= 0x70 && opcode <= 0x7F):
		off, err := c.fetch8()
		if err != nil {
			return err
		}
		rel := int32(int8(off))
		mnem, take := branchCondition(opcode, c)
		target := uint32(int32(c.R.EIP) + rel)
		c.emitInstr(startEIP, fmt.Sprintf("%s 0x%X", mnem, target))
		if take {
			c.setEIP(target)
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
		c.setEIP(target)

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
		c.setEIP(target)

	case opcode == 0xC3:
		c.emitInstr(startEIP, "RET")
		retAddr, err := c.Mem.Read32(c, c.R.ESP, c.Mode)
		if err != nil {
			return err
		}
		c.setRegByIdx(4, c.R.ESP+4)
		c.setEIP(retAddr)

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
		if c.Mode == ModeUser {
			// HLT is privileged. In real x86 PL3 executing HLT raises #GP
			// (vector 0x0D, a CPU exception); the kernel-side handler kills
			// the process. dispatchFault rewinds EIP back to startEIP so the
			// saved frame points at the HLT itself — that is how x86 reports
			// faults, and the UI relies on it to highlight the offending byte.
			return c.dispatchFault(0x0D)
		}
		c.Halted = true
		c.Emit(EventHalt{CoreID: c.CoreID})

	case opcode == 0x90:
		c.emitInstr(startEIP, "NOP")

	// FA  CLI  — clear IF (privileged)
	// FB  STI  — set IF   (privileged)
	// In real x86 both raise #GP if CPL > IOPL; here, with IOPL not modeled,
	// the rule collapses to "user must not touch IF". PL3 attempt raises #GP
	// with EIP rewinded to the offending opcode, just like HLT.
	case opcode == 0xFA || opcode == 0xFB:
		mnem := "CLI"
		if opcode == 0xFB {
			mnem = "STI"
		}
		c.emitInstr(startEIP, mnem)
		if c.Mode == ModeUser {
			return c.dispatchFault(0x0D)
		}
		c.FlagSet(FlagIF, opcode == 0xFB)

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

// branchCondition decodes opcodes 0xEB / 0x70..0x7F into a mnemonic and a
// taken/not-taken decision derived from EFLAGS. The condition table follows
// Intel SDM Vol 1, Appendix B (Jcc) verbatim; pairs like JB/JNAE/JC are listed
// under the canonical name only.
func branchCondition(opcode uint8, c *CPU) (string, bool) {
	cf := c.FlagOn(FlagCF)
	pf := c.FlagOn(FlagPF)
	zf := c.FlagOn(FlagZF)
	sf := c.FlagOn(FlagSF)
	of := c.FlagOn(FlagOF)
	switch opcode {
	case 0xEB:
		return "JMP", true
	case 0x70:
		return "JO", of
	case 0x71:
		return "JNO", !of
	case 0x72:
		return "JB", cf
	case 0x73:
		return "JAE", !cf
	case 0x74:
		return "JE", zf
	case 0x75:
		return "JNE", !zf
	case 0x76:
		return "JBE", cf || zf
	case 0x77:
		return "JA", !cf && !zf
	case 0x78:
		return "JS", sf
	case 0x79:
		return "JNS", !sf
	case 0x7A:
		return "JP", pf
	case 0x7B:
		return "JNP", !pf
	case 0x7C:
		return "JL", sf != of
	case 0x7D:
		return "JGE", sf == of
	case 0x7E:
		return "JLE", zf || sf != of
	case 0x7F:
		return "JG", !zf && sf == of
	}
	return "", false
}

// emitInstr is a shortcut for EventInstruction that gathers the bytes read.
func (c *CPU) emitInstr(startEIP uint32, mnemonic string) {
	bytes := append([]byte(nil), c.Mem.Bytes[startEIP:c.R.EIP]...)
	c.Emit(EventInstruction{
		CoreID: c.CoreID,
		// PID is stamped by Kernel.stepCore (which knows the running process);
		// CPU itself has no notion of processes.
		PID:      0,
		EIP:      startEIP,
		Mnemonic: mnemonic,
		Bytes:    bytes,
	})
}
