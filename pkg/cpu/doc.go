// Package cpu implements a deliberately small subset of the x86 32-bit ISA
// suitable for teaching the hardware/software boundary of an operating system.
//
// # Scope
//
// The CPU emulates a curated set of i386 instructions, eight general-purpose
// registers plus EIP/EFLAGS and the three segment selectors (CS/DS/SS), two
// privilege levels (PL0 kernel, PL3 user), a flat linear memory with
// permission-checked regions, and an IDT with the vectors used by the teaching
// scenarios. The list of supported opcodes is meant to be exhaustive; if a
// program needs anything outside it, execNext returns "opcode not implemented".
//
// # Supported opcodes
//
//	MOV r32, imm32          B8+rd  imm32
//	INC r32                 40+rd
//	DEC r32                 48+rd
//	PUSH r32                50+rd
//	POP  r32                58+rd
//	ADD r/m32, r32          01 /r           (mod=11 only)
//	SUB r/m32, r32          29 /r           (mod=11 only)
//	XOR r/m32, r32          31 /r           (mod=11 only)
//	CMP r/m32, r32          39 /r           (mod=11 only)
//	MOV r/m32, r32          89 /r           (mod=11 only)
//	ADD/AND/SUB/XOR/CMP     81 /N imm32     (N = 0/4/5/6/7; mod=11 only)
//	MOV EAX, [moffs32]      A1 moffs32      (absolute load, EAX only)
//	MOV [moffs32], EAX      A3 moffs32      (absolute store, EAX only)
//	JMP rel8                EB cb
//	JMP rel32               E9 cd
//	Jcc rel8                70..7F cb       (full Intel set, see branchCondition)
//	CALL rel32              E8 cd
//	RET                     C3
//	INT imm8                CD ib
//	IRET                    CF
//	HLT                     F4              (privileged: PL3 raises #GP)
//	CLI                     FA              (privileged: PL3 raises #GP)
//	STI                     FB              (privileged: PL3 raises #GP)
//	NOP                     90
//
// # Addressing
//
// Only two memory-operand forms are supported: register-to-register via
// ModR/M with mod=11, and absolute 32-bit addresses via the A1/A3 short forms
// (EAX only). SIB encoding, displacement forms, and segment overrides are
// NOT implemented — gcc-compiled code beyond toy programs will not run.
//
// # EFLAGS
//
// Modeled bits: CF (0), PF (2), ZF (6), SF (7), IF (9), OF (11).
// Omitted on purpose: AF (4, no BCD adjustments), TF (8, no single-step), DF
// (10, no string ops). Arithmetic and logical ops update flags as per Intel
// SDM Vol 1; MOV preserves them.
//
// # IDT vectors used
//
//	0x00  #DE  Divide error            (handler installed, never raised by ISA)
//	0x0D  #GP  General protection      (HLT/CLI/STI in PL3, INT to non-user vector)
//	0x0E  #PF  Page fault              (carries P/W/U/I-D bits, CR2 via mirror)
//	0x20  IRQ0 Timer (PIT)             (HW; scheduling logic lives in pkg/kernel)
//	0x21  IRQ1 Keyboard                (HW; injected via PushKey)
//	0x80  syscall (Linux i386)         (user-invocable, DPL=3 in real x86)
//	0x81  sync subsystem               (user-invocable, mutex/semaphore syscalls)
//
// Vectors 0x80 and 0x81 are the only ones a user `INT N` can reach; everything
// else from PL3 raises #GP. HW IRQs and CPU exceptions bypass this allow-list.
//
// # NOT implemented (intentional simplifications)
//
//	GDT       Selectors are constants, not indices into a descriptor table.
//	          CS/DS/SS base is implicitly 0 (flat model); DPL/conf checks are
//	          collapsed to the Mode field. Loading a selector does not walk
//	          any table.
//
//	TSS       Per-CPU TSS does not exist. SavedUserESP/SavedUserSS replace
//	          SS0/ESP0; KernelStackTop is a plain field on each CPU.
//
//	Paging    There is no CR3/PDE/PTE walk and no TLB. Protection is enforced
//	          per Region (R/W/X by Mode), not per page.
//
//	PIC       No i8259 or APIC: the kernel pushes vectors directly into
//	          PendingIRQs. No EOI, no IMR, no priority.
//
//	NMI/#DF   Non-maskable interrupts and double faults are not modeled.
//
//	Error code on stack
//	          x86 pushes a 4-byte error code after EIP for #PF/#GP/#DF/...
//	          We expose CR2 and the error code through AddrCR2Mirror /
//	          AddrErrCodeMirror in kernel memory instead. The IRET frame is
//	          always EIP/CS/EFLAGS.
//
//	Multiplication/division
//	          MUL/IMUL/DIV/IDIV (EDX:EAX semantics) are not implemented.
//
//	Shifts/rotates
//	          SHL/SHR/SAR/ROL/ROR are not implemented.
//
//	String ops, BCD, x87, SSE, MMX, AVX, prefixes (0x66/0x67/REP/LOCK)
//	          Out of scope for the demo.
//
// # Determinism
//
// Step is purely sequential: one fetch-decode-execute per call, plus
// at-most-one interrupt dispatch when IF=1 and an IRQ is pending. Given the
// same initial state, the sequence of events is reproducible — a property
// the higher layers rely on for didactic replay.
//
// # Events
//
// The CPU does not log. It emits typed Event values describing what just
// happened (instruction, register write, memory access, IRQ fire, mode
// switch, page fault, halt). Consumers reconstruct UI state from the event
// stream.
//
// # What lives elsewhere
//
// The scheduler, syscalls, PCBs and interrupt handlers are software policy
// and live in pkg/kernel. The CPU only enforces the hardware contract:
// permission checks, mode transitions, IDT dispatch, IRET semantics.
package cpu
