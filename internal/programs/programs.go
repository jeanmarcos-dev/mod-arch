// Package programs holds the embedded x86 demo binaries.
//
// Each Demo is made up of one or more Procs. Single-process demos are the
// common case; multi-process demos illustrate context switching.
package programs

// Demo is a scenario loaded into the simulator. It has one or more processes.
type Demo struct {
	Name        string
	Description string
	Procs       []Proc
	// Category groups demos in the UI ("concurrencia", "interrupciones",
	// etc.). Empty means "uncategorized" and the demo appears in the
	// default picker.
	Category string
	// Cores requests a specific number of active CPU cores when this demo is
	// loaded. Zero (the default) leaves the current kernel layout untouched,
	// preserving single-core behavior for legacy demos. Values are clamped to
	// [1, kernel.MaxCores] by sim.SetNumCores.
	Cores int
}

// Proc is a single binary that the kernel loads as a process.
//
// Source is the hand-written C-like equivalent shown to the user. It is NOT
// compiled into Bytes at runtime; the bytes are hand-encoded in parallel so
// the viewer sees WHAT the process does without reading assembly directly.
//
// SourceMap is the optional bridge between machine instructions and the
// C lines that motivated them. Each entry pins one instruction to its C
// line and offers a human-readable mnemonic for the side-by-side panel
// that the UI renders. An empty SourceMap means "no bridge available" —
// the UI falls back to showing Source as plain text without highlighting.
type Proc struct {
	Name      string
	Bytes     []byte
	Source    string
	SourceMap []SourceLine
}

// SourceLine pins one machine instruction to its C line of origin.
//
//   - Offset is relative to the start of Proc.Bytes. The first instruction
//     is always at Offset 0.
//   - Mnemonic is the assembler text displayed in the ASM panel. It is the
//     author's pedagogical wording and need not be byte-for-byte identical
//     to what cpu.execInstruction emits (which contains absolute addresses
//     that vary with code_base across processes).
//   - CLine is the 1-based line number into Source. Multiple instructions
//     may share a CLine (a single C statement often expands to several
//     machine instructions).
type SourceLine struct {
	Offset   uint32
	Mnemonic string
	CLine    int
}

// Demos returns the embedded demo scenarios.
func Demos() []Demo {
	return []Demo{
		{
			Name:        "add",
			Description: "Suma simple: 10 + 32 → EAX. Demuestra MOV imm32 + ADD r,r.",
			Procs: []Proc{{
				Name: "add",
				Source: `// Suma dos números en registros.
int main(void) {
    register int a = 10;     // mov eax, 10
    register int b = 32;     // mov ebx, 32
    return a + b;            // add eax, ebx ; sys_exit(EAX)
}
`,
				Bytes: []byte{
					0xB8, 0x0A, 0x00, 0x00, 0x00, // mov eax, 10
					0xBB, 0x20, 0x00, 0x00, 0x00, // mov ebx, 32
					0x01, 0xD8, // add eax, ebx
					0x89, 0xC3, // mov ebx, eax  (exit code = result)
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1   (sys_exit)
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 10", CLine: 3},
					{Offset: 0x05, Mnemonic: "MOV EBX, 32", CLine: 4},
					{Offset: 0x0A, Mnemonic: "ADD EAX, EBX", CLine: 5},
					{Offset: 0x0C, Mnemonic: "MOV EBX, EAX     ; exit code", CLine: 5},
					{Offset: 0x0E, Mnemonic: "MOV EAX, 1       ; sys_exit", CLine: 5},
					{Offset: 0x13, Mnemonic: "INT 0x80", CLine: 5},
				},
			}},
		},
		{
			Name:        "counter",
			Description: "Cuenta de 0 a 10 en EAX usando INC + CMP + JNE.",
			Procs: []Proc{{
				Name: "counter",
				Source: `// Loop clásico: cuenta de 0 a 10.
int main(void) {
    register int i = 0;        // mov eax, 0
    register int max = 10;     // mov ecx, 10
    while (i != max) {         // L: cmp eax, ecx ; jne L
        i++;                   //    inc eax
    }
    return i;                  // sys_exit(EAX)
}
`,
				Bytes: []byte{
					0xB8, 0x00, 0x00, 0x00, 0x00, // mov eax, 0
					0xB9, 0x0A, 0x00, 0x00, 0x00, // mov ecx, 10
					0x40,       // inc eax
					0x39, 0xC8, // cmp eax, ecx
					0x75, 0xFB, // jne -5
					0x89, 0xC3, // mov ebx, eax  (exit code = count)
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1   (sys_exit)
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 0", CLine: 3},
					{Offset: 0x05, Mnemonic: "MOV ECX, 10", CLine: 4},
					{Offset: 0x0A, Mnemonic: "INC EAX", CLine: 6},
					{Offset: 0x0B, Mnemonic: "CMP EAX, ECX", CLine: 5},
					{Offset: 0x0D, Mnemonic: "JNE -5", CLine: 5},
					{Offset: 0x0F, Mnemonic: "MOV EBX, EAX     ; exit code", CLine: 8},
					{Offset: 0x11, Mnemonic: "MOV EAX, 1       ; sys_exit", CLine: 8},
					{Offset: 0x16, Mnemonic: "INT 0x80", CLine: 8},
				},
			}},
		},
		{
			Name:        "pushpop",
			Description: "Push/pop al stack del usuario. Demuestra ESP cambiando y memoria persistente.",
			Procs: []Proc{{
				Name: "pushpop",
				Source: `// Guarda un valor en el stack y lo recupera.
int main(void) {
    register int x = 0xCAFE;   // mov eax, 0xCAFE
    push_to_stack(x);          // push eax  (ESP -= 4, mem[ESP] = x)
    x = 0;                     // mov eax, 0
    x = pop_from_stack();      // pop eax   (x = mem[ESP], ESP += 4)
    return x;                  // → 0xCAFE
}
`,
				Bytes: []byte{
					0xB8, 0xFE, 0xCA, 0x00, 0x00, // mov eax, 0xCAFE
					0x50,                         // push eax
					0xB8, 0x00, 0x00, 0x00, 0x00, // mov eax, 0
					0x58,       // pop eax
					0x89, 0xC3, // mov ebx, eax  (exit code = popped value)
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 0xCAFE", CLine: 3},
					{Offset: 0x05, Mnemonic: "PUSH EAX", CLine: 4},
					{Offset: 0x06, Mnemonic: "MOV EAX, 0", CLine: 5},
					{Offset: 0x0B, Mnemonic: "POP EAX", CLine: 6},
					{Offset: 0x0C, Mnemonic: "MOV EBX, EAX     ; exit code", CLine: 7},
					{Offset: 0x0E, Mnemonic: "MOV EAX, 1       ; sys_exit", CLine: 7},
					{Offset: 0x13, Mnemonic: "INT 0x80", CLine: 7},
				},
			}},
		},
		{
			Name:        "callret",
			Description: "Llama a una función que pone 0x99 en EAX y retorna.",
			Procs: []Proc{{
				Name: "callret",
				Source: `// Demuestra call/ret: el stack guarda la dirección de retorno.
int fn(void) {
    return 0x99;               // mov eax, 0x99  ;  ret
}

int main(void) {
    return fn();               // call fn ; sys_exit(EAX)
}
`,
				Bytes: []byte{
					0xE8, 0x09, 0x00, 0x00, 0x00, // call +9  (jump past exit sequence)
					0x89, 0xC3,                   // mov ebx, eax  (exit code = fn result)
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0xCD, 0x80,                   // int 0x80
					0xB8, 0x99, 0x00, 0x00, 0x00, // mov eax, 0x99
					0xC3, // ret
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "CALL fn", CLine: 7},
					{Offset: 0x05, Mnemonic: "MOV EBX, EAX     ; exit code", CLine: 7},
					{Offset: 0x07, Mnemonic: "MOV EAX, 1       ; sys_exit", CLine: 7},
					{Offset: 0x0C, Mnemonic: "INT 0x80", CLine: 7},
					{Offset: 0x0E, Mnemonic: "MOV EAX, 0x99", CLine: 3},
					{Offset: 0x13, Mnemonic: "RET", CLine: 3},
				},
			}},
		},
		{
			Name:        "soft_int",
			Description: "Ejecuta int 0x80 (syscall). HW entra a kernel, kernel hace IRET, vuelve a user.",
			Procs: []Proc{{
				Name: "soft_int",
				Source: `// SW interrupt sin "syscall real". Solo para mostrar el mode switch
// HW que dispara int 0x80.
int main(void) {
    register int x = 1;        // mov eax, 1
    asm("int 0x80");           // HW → PL0, salta a IDT[0x80], handler iret
    x++;                       // inc eax  (ejecuta después del IRET)
    return x;                  // sys_exit(EAX)
}
`,
				Bytes: []byte{
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0xCD, 0x80, // int 0x80
					0x40,       // inc eax
					0x89, 0xC3, // mov ebx, eax  (exit code)
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 1", CLine: 4},
					{Offset: 0x05, Mnemonic: "INT 0x80", CLine: 5},
					{Offset: 0x07, Mnemonic: "INC EAX", CLine: 6},
					{Offset: 0x08, Mnemonic: "MOV EBX, EAX     ; exit code", CLine: 7},
					{Offset: 0x0A, Mnemonic: "MOV EAX, 1       ; sys_exit", CLine: 7},
					{Offset: 0x0F, Mnemonic: "INT 0x80", CLine: 7},
				},
			}},
		},
		{
			Name:        "timer_loop",
			Description: "NOPs interrumpidos por timer HW. Observe el flash USER→KERNEL→USER de la pill.",
			Procs: []Proc{{
				Name: "timer_loop",
				Source: `// Programa "tonto" que solo hace nada (NOPs).
// Útil para ver el timer HW preemptar la ejecución cada quantum ticks.
int main(void) {
    for (int i = 0; i < 16; i++)
        asm("nop");            // 16 × nop
    return 0;                  // sys_exit(0)
}
`,
				Bytes: []byte{
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90,
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90,
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
					0xCD, 0x80, // int 0x80 (sys_exit)
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x01, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x02, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x03, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x04, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x05, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x06, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x07, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x08, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x09, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x0A, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x0B, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x0C, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x0D, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x0E, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x0F, Mnemonic: "NOP", CLine: 5},
					{Offset: 0x10, Mnemonic: "MOV EAX, 1       ; sys_exit", CLine: 6},
					{Offset: 0x15, Mnemonic: "MOV EBX, 0       ; status", CLine: 6},
					{Offset: 0x1A, Mnemonic: "INT 0x80", CLine: 6},
				},
			}},
		},
		{
			Name:        "hello",
			Description: "Escribe \"hola SO\\n\" con sys_write y termina con sys_exit. Demuestra el flow completo de syscall.",
			Procs: []Proc{{
				Name: "hello",
				Source: `// Programa "Hello World" con syscalls Linux i386.
const char msg[8] = "hola SO\n";

int main(void) {
    write(1, msg, 8);          // EAX=4, EBX=1, ECX=&msg, EDX=8 ; int 0x80
    exit(0);                   // EAX=1, EBX=0 ; int 0x80
}

// La mini-libc traduce write/exit a las convenciones del kernel:
//   write(fd, buf, n)  →  EAX=4, EBX=fd, ECX=buf, EDX=n, int 0x80
//   exit(status)       →  EAX=1, EBX=status, int 0x80
`,
				Bytes: []byte{
					0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4
					0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
					0xB9, 0x22, 0x00, 0x20, 0x00, // mov ecx, 0x200022
					0xBA, 0x08, 0x00, 0x00, 0x00, // mov edx, 8
					0xCD, 0x80, // int 0x80 (sys_write)
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
					0xCD, 0x80, // int 0x80 (sys_exit)
					0x68, 0x6F, 0x6C, 0x61, 0x20, 0x53, 0x4F, 0x0A, // "hola SO\n"
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 4    ; sys_write", CLine: 5},
					{Offset: 0x05, Mnemonic: "MOV EBX, 1    ; fd=stdout", CLine: 5},
					{Offset: 0x0A, Mnemonic: "MOV ECX, 0x200022  ; &msg", CLine: 5},
					{Offset: 0x0F, Mnemonic: "MOV EDX, 8    ; len", CLine: 5},
					{Offset: 0x14, Mnemonic: "INT 0x80", CLine: 5},
					{Offset: 0x16, Mnemonic: "MOV EAX, 1    ; sys_exit", CLine: 6},
					{Offset: 0x1B, Mnemonic: "MOV EBX, 0    ; status", CLine: 6},
					{Offset: 0x20, Mnemonic: "INT 0x80", CLine: 6},
					// Bytes 0x22..0x29 are the "hola SO\n" data; not executable.
				},
			}},
		},
		{
			Name:        "readkey",
			Description: "Lee 1 byte del teclado y lo escribe a stdout, en loop. Bloquea con sys_read; el frontend le deja escribir para despertarlo.",
			Procs: []Proc{{
				Name: "readkey",
				Source: `// Echo del teclado. Loop infinito: leer 1 byte, escribir 1 byte.
// sys_read BLOQUEA si no hay teclas pendientes — el proceso pasa a estado
// BLOCKED y el kernel se lo dará a otro READY (o HALTará si no hay).
// Cuando llegue una tecla → IRQ 0x21 → kernel despierta a este proceso.

char buf;  // en user_stack @ 0x33FFE0

int main(void) {
    while (1) {
        read(0, &buf, 1);      // BLOQUEA hasta que haya tecla
        write(1, &buf, 1);     // echo
    }
}
`,
				Bytes: []byte{
					0xB8, 0x03, 0x00, 0x00, 0x00, // mov eax, 3 (sys_read)
					0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
					0xB9, 0xE0, 0xFF, 0x33, 0x00, // mov ecx, 0x33FFE0
					0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
					0xCD, 0x80, // int 0x80
					0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4 (sys_write)
					0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
					0xB9, 0xE0, 0xFF, 0x33, 0x00, // mov ecx, 0x33FFE0
					0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
					0xCD, 0x80, // int 0x80
					0xEB, 0xD2, // jmp -46
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 3    ; sys_read", CLine: 10},
					{Offset: 0x05, Mnemonic: "MOV EBX, 0    ; fd=stdin", CLine: 10},
					{Offset: 0x0A, Mnemonic: "MOV ECX, 0x33FFE0", CLine: 10},
					{Offset: 0x0F, Mnemonic: "MOV EDX, 1", CLine: 10},
					{Offset: 0x14, Mnemonic: "INT 0x80", CLine: 10},
					{Offset: 0x16, Mnemonic: "MOV EAX, 4    ; sys_write", CLine: 11},
					{Offset: 0x1B, Mnemonic: "MOV EBX, 1    ; fd=stdout", CLine: 11},
					{Offset: 0x20, Mnemonic: "MOV ECX, 0x33FFE0", CLine: 11},
					{Offset: 0x25, Mnemonic: "MOV EDX, 1", CLine: 11},
					{Offset: 0x2A, Mnemonic: "INT 0x80", CLine: 11},
					{Offset: 0x2C, Mnemonic: "JMP -46  ; while(1)", CLine: 9},
				},
			}},
		},

		{
			Name:        "badmem",
			Description: "Intenta escribir a 0x100000 (memoria kernel). MMU detecta y dispara page fault.",
			Procs: []Proc{{
				Name: "badmem",
				Source: `// Proceso "malicioso" o buggeado. Intenta escribir a memoria kernel.
// La MMU del CPU detecta la violación de permisos → page fault (IRQ 0x0E)
// → kernel mata el proceso (ZOMBIE con razón = "page_fault").
int main(void) {
    int *kernel_addr = (int*)0x100000;     // mov eax, 0xDEAD
    *kernel_addr = 0xDEAD;                 // mov [0x100000], eax  ← ¡fault!
    return 0;                              // sys_exit(0) (nunca se alcanza)
}
`,
				Bytes: []byte{
					0xB8, 0xAD, 0xDE, 0x00, 0x00, // mov eax, 0xDEAD
					0xA3, 0x00, 0x00, 0x10, 0x00, // mov [0x100000], eax
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1       (unreachable)
					0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 0xDEAD", CLine: 5},
					{Offset: 0x05, Mnemonic: "MOV [0x100000], EAX  ; ¡fault!", CLine: 6},
					{Offset: 0x0A, Mnemonic: "MOV EAX, 1       ; sys_exit (unreachable)", CLine: 7},
					{Offset: 0x0F, Mnemonic: "MOV EBX, 0", CLine: 7},
					{Offset: 0x14, Mnemonic: "INT 0x80", CLine: 7},
				},
			}},
		},

		{
			Name:        "scheduler_demo",
			Description: "TRES procesos: 2 contadores + 1 reader. Cuando el reader hace sys_read sin teclas, va al WAITING. Los otros 2 siguen alternando. Escriba para despertarlo.",
			Procs: []Proc{
				{
					Name: "counter_eax",
					Source: `// Loop infinito incrementando EAX. CPU-bound (nunca cede voluntariamente).
// Solo lo preempta el timer.
int main(void) {
    register int counter = 0;     // mov eax, 0
    while (1) counter++;          // L: inc eax ; jmp L
}
`,
					Bytes: []byte{
						0xB8, 0x00, 0x00, 0x00, 0x00, // mov eax, 0
						0x40,       // inc eax
						0xEB, 0xFD, // jmp -3
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EAX, 0", CLine: 4},
						{Offset: 0x05, Mnemonic: "INC EAX", CLine: 5},
						{Offset: 0x06, Mnemonic: "JMP -3  ; while(1)", CLine: 5},
					},
				},
				{
					Name: "counter_ebx",
					Source: `// Igual al de EAX pero usando EBX. Así se ven dos registros distintos
// creciendo según quién tenga el CPU.
int main(void) {
    register int counter = 0;     // mov ebx, 0
    while (1) counter++;          // L: inc ebx ; jmp L
}
`,
					Bytes: []byte{
						0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
						0x43,       // inc ebx
						0xEB, 0xFD, // jmp -3
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EBX, 0", CLine: 4},
						{Offset: 0x05, Mnemonic: "INC EBX", CLine: 5},
						{Offset: 0x06, Mnemonic: "JMP -3  ; while(1)", CLine: 5},
					},
				},
				{
					Name: "reader",
					Source: `// Lee y echo del teclado. Cada sys_read BLOQUEA si no hay tecla,
// mostrando este proceso en el WAITING QUEUE mientras los otros corren.

char buf;  // en su user_stack @ 0x3BFFE0 (proc 3 → stack base distinto)

int main(void) {
    while (1) {
        read(0, &buf, 1);          // → BLOCKED hasta IRQ 0x21
        write(1, &buf, 1);         // echo a stdout
    }
}
`,
					Bytes: []byte{
						0xB8, 0x03, 0x00, 0x00, 0x00, // mov eax, 3 (sys_read)
						0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
						0xB9, 0xE0, 0xFF, 0x3B, 0x00, // mov ecx, 0x3BFFE0
						0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
						0xCD, 0x80, // int 0x80
						0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4 (sys_write)
						0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
						0xB9, 0xE0, 0xFF, 0x3B, 0x00, // mov ecx, 0x3BFFE0
						0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
						0xCD, 0x80, // int 0x80
						0xEB, 0xD2, // jmp -46
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EAX, 3    ; sys_read", CLine: 8},
						{Offset: 0x05, Mnemonic: "MOV EBX, 0    ; fd=stdin", CLine: 8},
						{Offset: 0x0A, Mnemonic: "MOV ECX, 0x3BFFE0", CLine: 8},
						{Offset: 0x0F, Mnemonic: "MOV EDX, 1", CLine: 8},
						{Offset: 0x14, Mnemonic: "INT 0x80", CLine: 8},
						{Offset: 0x16, Mnemonic: "MOV EAX, 4    ; sys_write", CLine: 9},
						{Offset: 0x1B, Mnemonic: "MOV EBX, 1    ; fd=stdout", CLine: 9},
						{Offset: 0x20, Mnemonic: "MOV ECX, 0x3BFFE0", CLine: 9},
						{Offset: 0x25, Mnemonic: "MOV EDX, 1", CLine: 9},
						{Offset: 0x2A, Mnemonic: "INT 0x80", CLine: 9},
						{Offset: 0x2C, Mnemonic: "JMP -46  ; while(1)", CLine: 7},
					},
				},
			},
		},

		{
			Name:        "two_counters",
			Description: "Dos procesos: P1 incrementa EAX, P2 incrementa EBX. El scheduler RR los intercala con cada timer tick.",
			Procs: []Proc{
				{
					Name: "p1_count_eax",
					Source: `// Proceso 1: cuenta en EAX infinitamente.
int main(void) {
    register int n = 0;       // mov eax, 0
    while (1) n++;            // L: inc eax ; jmp L
}
`,
					Bytes: []byte{
						0xB8, 0x00, 0x00, 0x00, 0x00,
						0x40,
						0xEB, 0xFD,
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EAX, 0", CLine: 3},
						{Offset: 0x05, Mnemonic: "INC EAX", CLine: 4},
						{Offset: 0x06, Mnemonic: "JMP -3", CLine: 4},
					},
				},
				{
					Name: "p2_count_ebx",
					Source: `// Proceso 2: cuenta en EBX infinitamente.
int main(void) {
    register int n = 0;       // mov ebx, 0
    while (1) n++;            // L: inc ebx ; jmp L
}
`,
					Bytes: []byte{
						0xBB, 0x00, 0x00, 0x00, 0x00,
						0x43,
						0xEB, 0xFD,
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EBX, 0", CLine: 3},
						{Offset: 0x05, Mnemonic: "INC EBX", CLine: 4},
						{Offset: 0x06, Mnemonic: "JMP -3", CLine: 4},
					},
				},
			},
		},

		// ─── Concurrency demos ────────────────────────────────────────────
		// Shared counter at 0x3FFFE0 (inside user_stack region, well above any
		// process stack). Each worker iterates 200 times doing
		// load → inc → store.
		{
			Name:        "race_counter",
			Category:    "concurrencia",
			Description: "Tres procesos: el setup pone el contador en 0, dos workers lo incrementan 200 veces cada uno SIN lock. El resultado final raramente es 400 porque el timer preempt el load/store y se pierden updates.",
			Procs: []Proc{
				{
					Name: "setup_zero_counter",
					Source: `// Setup: pone el contador compartido en 0 y termina.
int main(void) {
    *(int *)0x3FFFE0 = 0;
    exit(0);
}
`,
					Bytes: []byte{
						0x31, 0xC0, // xor eax, eax
						0xA3, 0xE0, 0xFF, 0x3F, 0x00, // mov [0x3FFFE0], eax
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "XOR EAX, EAX", CLine: 3},
						{Offset: 0x02, Mnemonic: "MOV [0x3FFFE0], EAX", CLine: 3},
						{Offset: 0x07, Mnemonic: "MOV EAX, 1    ; sys_exit", CLine: 4},
						{Offset: 0x0C, Mnemonic: "XOR EBX, EBX  ; status=0", CLine: 4},
						{Offset: 0x0E, Mnemonic: "INT 0x80", CLine: 4},
					},
				},
				{
					Name: "racer_a",
					Source: `// Worker A: incrementa el contador compartido 200 veces sin lock.
int main(void) {
    for (int i = 0; i < 200; i++) {
        int n = *(int *)0x3FFFE0;   // load
        n++;                          // increment
        *(int *)0x3FFFE0 = n;         // store ← preemption window between load and store
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xB9, 0xC8, 0x00, 0x00, 0x00, // mov ecx, 200
						0xA1, 0xE0, 0xFF, 0x3F, 0x00, // L: mov eax, [counter]
						0x40,                         // inc eax
						0xA3, 0xE0, 0xFF, 0x3F, 0x00, // mov [counter], eax
						0x49,       // dec ecx
						0x75, 0xF2, // jne L
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV ECX, 200  ; for init", CLine: 3},
						{Offset: 0x05, Mnemonic: "MOV EAX, [0x3FFFE0]  ; load", CLine: 4},
						{Offset: 0x0A, Mnemonic: "INC EAX       ; n++", CLine: 5},
						{Offset: 0x0B, Mnemonic: "MOV [0x3FFFE0], EAX  ; store", CLine: 6},
						{Offset: 0x10, Mnemonic: "DEC ECX", CLine: 3},
						{Offset: 0x11, Mnemonic: "JNE -14       ; for loop", CLine: 3},
						{Offset: 0x13, Mnemonic: "MOV EAX, 1    ; sys_exit", CLine: 8},
						{Offset: 0x18, Mnemonic: "XOR EBX, EBX", CLine: 8},
						{Offset: 0x1A, Mnemonic: "INT 0x80", CLine: 8},
					},
				},
				{
					Name: "racer_b",
					Source: `// Worker B: mismo loop que A. Sin lock entre ambos.
int main(void) {
    for (int i = 0; i < 200; i++) {
        int n = *(int *)0x3FFFE0;
        n++;
        *(int *)0x3FFFE0 = n;
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xB9, 0xC8, 0x00, 0x00, 0x00,
						0xA1, 0xE0, 0xFF, 0x3F, 0x00,
						0x40,
						0xA3, 0xE0, 0xFF, 0x3F, 0x00,
						0x49,
						0x75, 0xF2,
						0xB8, 0x01, 0x00, 0x00, 0x00,
						0x31, 0xDB,
						0xCD, 0x80,
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV ECX, 200", CLine: 3},
						{Offset: 0x05, Mnemonic: "MOV EAX, [0x3FFFE0]", CLine: 4},
						{Offset: 0x0A, Mnemonic: "INC EAX", CLine: 5},
						{Offset: 0x0B, Mnemonic: "MOV [0x3FFFE0], EAX", CLine: 6},
						{Offset: 0x10, Mnemonic: "DEC ECX", CLine: 3},
						{Offset: 0x11, Mnemonic: "JNE -14", CLine: 3},
						{Offset: 0x13, Mnemonic: "MOV EAX, 1    ; sys_exit", CLine: 8},
						{Offset: 0x18, Mnemonic: "XOR EBX, EBX", CLine: 8},
						{Offset: 0x1A, Mnemonic: "INT 0x80", CLine: 8},
					},
				},
			},
		},

		{
			Name:        "safe_counter",
			Category:    "concurrencia",
			Description: "Mismo loop que race_counter, pero los workers piden un mutex (id=1) antes de modificar el contador. Resultado final SIEMPRE 400 porque el lock serializa la sección crítica.",
			Procs: []Proc{
				{
					Name: "setup_zero_and_mutex",
					Source: `// Setup: pone el contador en 0, crea el mutex (id=1) y termina.
int main(void) {
    *(int *)0x3FFFE0 = 0;
    mutex_create();   // ID 1 (primer mutex del kernel)
    exit(0);
}
`,
					Bytes: []byte{
						0x31, 0xC0, // xor eax, eax
						0xA3, 0xE0, 0xFF, 0x3F, 0x00, // mov [counter], eax
						0xB8, 0x10, 0x00, 0x00, 0x00, // mov eax, 0x10 (SyncMutexCreate)
						0xCD, 0x81, // int 0x81 → eax = mutex id
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "XOR EAX, EAX", CLine: 3},
						{Offset: 0x02, Mnemonic: "MOV [0x3FFFE0], EAX", CLine: 3},
						{Offset: 0x07, Mnemonic: "MOV EAX, 0x10  ; MutexCreate", CLine: 4},
						{Offset: 0x0C, Mnemonic: "INT 0x81", CLine: 4},
						{Offset: 0x0E, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 5},
						{Offset: 0x13, Mnemonic: "XOR EBX, EBX", CLine: 5},
						{Offset: 0x15, Mnemonic: "INT 0x80", CLine: 5},
					},
				},
				{
					Name: "locker_a",
					Source: `// Worker A: incrementa con mutex id=1.
int main(void) {
    for (int i = 0; i < 200; i++) {
        mutex_lock(1);
        int n = *(int *)0x3FFFE0;
        n++;
        *(int *)0x3FFFE0 = n;
        mutex_unlock(1);
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xB9, 0xC8, 0x00, 0x00, 0x00, // mov ecx, 200
						0xBB, 0x01, 0x00, 0x00, 0x00, // L: mov ebx, 1 (mutex id)
						0xB8, 0x11, 0x00, 0x00, 0x00, // mov eax, 0x11 (SyncMutexLock)
						0xCD, 0x81, // int 0x81
						0xA1, 0xE0, 0xFF, 0x3F, 0x00, // mov eax, [counter]
						0x40,                         // inc eax
						0xA3, 0xE0, 0xFF, 0x3F, 0x00, // mov [counter], eax
						0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
						0xB8, 0x12, 0x00, 0x00, 0x00, // mov eax, 0x12 (SyncMutexUnlock)
						0xCD, 0x81, // int 0x81
						0x49,       // dec ecx
						0x75, 0xDA, // jne L
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV ECX, 200   ; for init", CLine: 3},
						{Offset: 0x05, Mnemonic: "MOV EBX, 1     ; mutex id", CLine: 4},
						{Offset: 0x0A, Mnemonic: "MOV EAX, 0x11  ; MutexLock", CLine: 4},
						{Offset: 0x0F, Mnemonic: "INT 0x81", CLine: 4},
						{Offset: 0x11, Mnemonic: "MOV EAX, [0x3FFFE0]", CLine: 5},
						{Offset: 0x16, Mnemonic: "INC EAX", CLine: 6},
						{Offset: 0x17, Mnemonic: "MOV [0x3FFFE0], EAX", CLine: 7},
						{Offset: 0x1C, Mnemonic: "MOV EBX, 1     ; mutex id", CLine: 8},
						{Offset: 0x21, Mnemonic: "MOV EAX, 0x12  ; MutexUnlock", CLine: 8},
						{Offset: 0x26, Mnemonic: "INT 0x81", CLine: 8},
						{Offset: 0x28, Mnemonic: "DEC ECX", CLine: 3},
						{Offset: 0x29, Mnemonic: "JNE -38        ; for loop", CLine: 3},
						{Offset: 0x2B, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 10},
						{Offset: 0x30, Mnemonic: "XOR EBX, EBX", CLine: 10},
						{Offset: 0x32, Mnemonic: "INT 0x80", CLine: 10},
					},
				},
				{
					Name: "locker_b",
					Source: `// Worker B: mismo loop que A, ambos serializan vía mutex id=1.
int main(void) {
    for (int i = 0; i < 200; i++) {
        mutex_lock(1);
        int n = *(int *)0x3FFFE0;
        n++;
        *(int *)0x3FFFE0 = n;
        mutex_unlock(1);
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xB9, 0xC8, 0x00, 0x00, 0x00,
						0xBB, 0x01, 0x00, 0x00, 0x00,
						0xB8, 0x11, 0x00, 0x00, 0x00,
						0xCD, 0x81,
						0xA1, 0xE0, 0xFF, 0x3F, 0x00,
						0x40,
						0xA3, 0xE0, 0xFF, 0x3F, 0x00,
						0xBB, 0x01, 0x00, 0x00, 0x00,
						0xB8, 0x12, 0x00, 0x00, 0x00,
						0xCD, 0x81,
						0x49,
						0x75, 0xDA,
						0xB8, 0x01, 0x00, 0x00, 0x00,
						0x31, 0xDB,
						0xCD, 0x80,
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV ECX, 200", CLine: 3},
						{Offset: 0x05, Mnemonic: "MOV EBX, 1", CLine: 4},
						{Offset: 0x0A, Mnemonic: "MOV EAX, 0x11  ; MutexLock", CLine: 4},
						{Offset: 0x0F, Mnemonic: "INT 0x81", CLine: 4},
						{Offset: 0x11, Mnemonic: "MOV EAX, [0x3FFFE0]", CLine: 5},
						{Offset: 0x16, Mnemonic: "INC EAX", CLine: 6},
						{Offset: 0x17, Mnemonic: "MOV [0x3FFFE0], EAX", CLine: 7},
						{Offset: 0x1C, Mnemonic: "MOV EBX, 1", CLine: 8},
						{Offset: 0x21, Mnemonic: "MOV EAX, 0x12  ; MutexUnlock", CLine: 8},
						{Offset: 0x26, Mnemonic: "INT 0x81", CLine: 8},
						{Offset: 0x28, Mnemonic: "DEC ECX", CLine: 3},
						{Offset: 0x29, Mnemonic: "JNE -38", CLine: 3},
						{Offset: 0x2B, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 10},
						{Offset: 0x30, Mnemonic: "XOR EBX, EBX", CLine: 10},
						{Offset: 0x32, Mnemonic: "INT 0x80", CLine: 10},
					},
				},
			},
		},

		// producer_consumer: classic synchronization via two semaphores. The
		// "buffer" is conceptually one slot; sem 1 (empty) starts at 1, sem 2
		// (full) at 0. Producer waits on empty / signals full; consumer does
		// the opposite. Whether or not there is a real data field, the
		// invariant "producer ≤ consumer + 1" is what the semaphore enforces.
		{
			Name:        "producer_consumer",
			Category:    "concurrencia",
			Description: "Productor y consumidor con un buffer de un slot, sincronizados por dos semáforos. Demuestra el patrón sem_wait/sem_signal sin necesidad de mutex porque solo hay un productor y un consumidor.",
			Procs: []Proc{
				{
					Name: "setup_two_sems",
					Source: `// Setup: crea sem(empty=1) y sem(full=0); luego termina.
int main(void) {
    sem_create(1);   // sem id 1 — "empty", buffer starts available
    sem_create(0);   // sem id 2 — "full",  no items pending
    exit(0);
}
`,
					Bytes: []byte{
						0xB8, 0x20, 0x00, 0x00, 0x00, // mov eax, 0x20 (SyncSemCreate)
						0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1   (init=1 → empty)
						0xCD, 0x81, // int 0x81 → eax=1
						0xB8, 0x20, 0x00, 0x00, 0x00, // mov eax, 0x20
						0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0   (init=0 → full)
						0xCD, 0x81, // int 0x81 → eax=2
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EAX, 0x20  ; SemCreate", CLine: 3},
						{Offset: 0x05, Mnemonic: "MOV EBX, 1     ; init=1", CLine: 3},
						{Offset: 0x0A, Mnemonic: "INT 0x81       ; → eax=1", CLine: 3},
						{Offset: 0x0C, Mnemonic: "MOV EAX, 0x20  ; SemCreate", CLine: 4},
						{Offset: 0x11, Mnemonic: "MOV EBX, 0     ; init=0", CLine: 4},
						{Offset: 0x16, Mnemonic: "INT 0x81       ; → eax=2", CLine: 4},
						{Offset: 0x18, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 5},
						{Offset: 0x1D, Mnemonic: "XOR EBX, EBX", CLine: 5},
						{Offset: 0x1F, Mnemonic: "INT 0x80", CLine: 5},
					},
				},
				{
					Name: "producer",
					Source: `// Producer: 50 veces, espera buffer libre y luego avisa item disponible.
int main(void) {
    for (int i = 0; i < 50; i++) {
        sem_wait(1);   // wait empty
        // producir un item (en este demo no se escribe payload: el sync es lo
        // pedagógico, no la data).
        sem_signal(2); // signal full
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xB9, 0x32, 0x00, 0x00, 0x00, // mov ecx, 50
						0xBB, 0x01, 0x00, 0x00, 0x00, // L: mov ebx, 1 (empty)
						0xB8, 0x21, 0x00, 0x00, 0x00, // mov eax, 0x21 (SemWait)
						0xCD, 0x81, // int 0x81
						0xBB, 0x02, 0x00, 0x00, 0x00, // mov ebx, 2 (full)
						0xB8, 0x22, 0x00, 0x00, 0x00, // mov eax, 0x22 (SemSignal)
						0xCD, 0x81, // int 0x81
						0x49,       // dec ecx
						0x75, 0xE5, // jne L
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV ECX, 50    ; for init", CLine: 3},
						{Offset: 0x05, Mnemonic: "MOV EBX, 1     ; sem=empty", CLine: 4},
						{Offset: 0x0A, Mnemonic: "MOV EAX, 0x21  ; SemWait", CLine: 4},
						{Offset: 0x0F, Mnemonic: "INT 0x81", CLine: 4},
						{Offset: 0x11, Mnemonic: "MOV EBX, 2     ; sem=full", CLine: 7},
						{Offset: 0x16, Mnemonic: "MOV EAX, 0x22  ; SemSignal", CLine: 7},
						{Offset: 0x1B, Mnemonic: "INT 0x81", CLine: 7},
						{Offset: 0x1D, Mnemonic: "DEC ECX", CLine: 3},
						{Offset: 0x1E, Mnemonic: "JNE -27        ; for loop", CLine: 3},
						{Offset: 0x20, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 9},
						{Offset: 0x25, Mnemonic: "XOR EBX, EBX", CLine: 9},
						{Offset: 0x27, Mnemonic: "INT 0x80", CLine: 9},
					},
				},
				{
					Name: "consumer",
					Source: `// Consumer: 50 veces, espera item disponible y luego libera el buffer.
int main(void) {
    for (int i = 0; i < 50; i++) {
        sem_wait(2);   // wait full
        // consumir el item (idéntica omisión de payload).
        sem_signal(1); // signal empty
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xB9, 0x32, 0x00, 0x00, 0x00,
						0xBB, 0x02, 0x00, 0x00, 0x00, // L: mov ebx, 2 (full)
						0xB8, 0x21, 0x00, 0x00, 0x00, // mov eax, 0x21 (SemWait)
						0xCD, 0x81,
						0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1 (empty)
						0xB8, 0x22, 0x00, 0x00, 0x00, // mov eax, 0x22 (SemSignal)
						0xCD, 0x81,
						0x49,
						0x75, 0xE5,
						0xB8, 0x01, 0x00, 0x00, 0x00,
						0x31, 0xDB,
						0xCD, 0x80,
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV ECX, 50", CLine: 3},
						{Offset: 0x05, Mnemonic: "MOV EBX, 2     ; sem=full", CLine: 4},
						{Offset: 0x0A, Mnemonic: "MOV EAX, 0x21  ; SemWait", CLine: 4},
						{Offset: 0x0F, Mnemonic: "INT 0x81", CLine: 4},
						{Offset: 0x11, Mnemonic: "MOV EBX, 1     ; sem=empty", CLine: 6},
						{Offset: 0x16, Mnemonic: "MOV EAX, 0x22  ; SemSignal", CLine: 6},
						{Offset: 0x1B, Mnemonic: "INT 0x81", CLine: 6},
						{Offset: 0x1D, Mnemonic: "DEC ECX", CLine: 3},
						{Offset: 0x1E, Mnemonic: "JNE -27", CLine: 3},
						{Offset: 0x20, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 8},
						{Offset: 0x25, Mnemonic: "XOR EBX, EBX", CLine: 8},
						{Offset: 0x27, Mnemonic: "INT 0x80", CLine: 8},
					},
				},
			},
		},

		// dining_philosophers_deadlock: 3 philos seated at a round table with
		// 3 forks (mutex 1, 2, 3). Each philo picks LEFT then RIGHT, in the
		// same order: 0→(1,2), 1→(2,3), 2→(3,1). Classic circular wait. When
		// they grab their lefts in lock-step the system stalls: every fork
		// is held, every philo waits for the next.
		{
			Name:        "philos_deadlock",
			Category:    "concurrencia",
			Description: "Cena de los filósofos en versión naïve (3 comensales). Cada uno toma su tenedor IZQUIERDO primero, luego el DERECHO. Si todos consiguen su izquierdo simultáneamente, nadie puede tomar el derecho → deadlock visible en la cola de los mutexes.",
			Procs: []Proc{
				{
					Name: "setup_three_forks",
					Source: `// Setup: tres tenedores (mutex 1, 2, 3) y exit.
int main(void) {
    mutex_create();  // fork 1
    mutex_create();  // fork 2
    mutex_create();  // fork 3
    exit(0);
}
`,
					Bytes: []byte{
						0xB8, 0x10, 0x00, 0x00, 0x00, // mov eax, 0x10
						0xCD, 0x81, // int 0x81 → mutex 1
						0xB8, 0x10, 0x00, 0x00, 0x00,
						0xCD, 0x81, // → mutex 2
						0xB8, 0x10, 0x00, 0x00, 0x00,
						0xCD, 0x81, // → mutex 3
						0xB8, 0x01, 0x00, 0x00, 0x00,
						0x31, 0xDB,
						0xCD, 0x80,
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EAX, 0x10  ; MutexCreate", CLine: 3},
						{Offset: 0x05, Mnemonic: "INT 0x81       ; → fork 1", CLine: 3},
						{Offset: 0x07, Mnemonic: "MOV EAX, 0x10", CLine: 4},
						{Offset: 0x0C, Mnemonic: "INT 0x81       ; → fork 2", CLine: 4},
						{Offset: 0x0E, Mnemonic: "MOV EAX, 0x10", CLine: 5},
						{Offset: 0x13, Mnemonic: "INT 0x81       ; → fork 3", CLine: 5},
						{Offset: 0x15, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 6},
						{Offset: 0x1A, Mnemonic: "XOR EBX, EBX", CLine: 6},
						{Offset: 0x1C, Mnemonic: "INT 0x80", CLine: 6},
					},
				},
				philoBytes("philo_0_naive", 0, 1, 2, "filo 0: izq=1, der=2"),
				philoBytes("philo_1_naive", 1, 2, 3, "filo 1: izq=2, der=3"),
				philoBytes("philo_2_naive", 2, 3, 1, "filo 2: izq=3, der=1 (cierra el ciclo → deadlock potencial)"),
			},
		},

		// dining_philosophers_safe: same three philos but the last one
		// (philo 2) inverts its lock order: lower-id fork first, then higher.
		// Breaking the symmetric "always-left-first" rule removes the cycle.
		{
			Name:        "philos_safe",
			Category:    "concurrencia",
			Description: "Misma cena de filósofos pero el último comensal invierte su orden de toma de tenedores. La asimetría rompe el ciclo de espera → no hay deadlock, el sistema progresa.",
			Procs: []Proc{
				{
					Name: "setup_three_forks_safe",
					Source: `// Setup idéntico al naïve: tres mutexes y exit.
int main(void) {
    mutex_create();
    mutex_create();
    mutex_create();
    exit(0);
}
`,
					Bytes: []byte{
						0xB8, 0x10, 0x00, 0x00, 0x00,
						0xCD, 0x81,
						0xB8, 0x10, 0x00, 0x00, 0x00,
						0xCD, 0x81,
						0xB8, 0x10, 0x00, 0x00, 0x00,
						0xCD, 0x81,
						0xB8, 0x01, 0x00, 0x00, 0x00,
						0x31, 0xDB,
						0xCD, 0x80,
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EAX, 0x10  ; MutexCreate", CLine: 3},
						{Offset: 0x05, Mnemonic: "INT 0x81", CLine: 3},
						{Offset: 0x07, Mnemonic: "MOV EAX, 0x10", CLine: 4},
						{Offset: 0x0C, Mnemonic: "INT 0x81", CLine: 4},
						{Offset: 0x0E, Mnemonic: "MOV EAX, 0x10", CLine: 5},
						{Offset: 0x13, Mnemonic: "INT 0x81", CLine: 5},
						{Offset: 0x15, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 6},
						{Offset: 0x1A, Mnemonic: "XOR EBX, EBX", CLine: 6},
						{Offset: 0x1C, Mnemonic: "INT 0x80", CLine: 6},
					},
				},
				philoBytes("philo_0_safe", 0, 1, 2, "filo 0: 1 → 2"),
				philoBytes("philo_1_safe", 1, 2, 3, "filo 1: 2 → 3"),
				// Asimetría: philo 2 toma 1 primero (lower) y luego 3.
				philoBytes("philo_2_safe", 2, 1, 3, "filo 2: 1 → 3 (asimétrico, rompe el ciclo)"),
			},
		},

		// ─── Failure-pattern demos ────────────────────────────────────────
		{
			Name:        "stack_overflow",
			Category:    "fallos",
			Description: "Fuerza un stack overflow en pocos pasos: acerca ESP al borde inferior del user_stack y hace CALL recursivo infinito. El segundo CALL intenta escribir el return address en user_code (R-X) → page fault.",
			Procs: []Proc{{
				Name: "stack_overflow",
				Source: `// Stack overflow deliberado. En x86 real, cada CALL pushea la dirección
// de retorno. Aquí acercamos ESP al borde de user_stack para provocar el
// fault rápido y ver la MMU detener el proceso.
int recurse(void) {
    return recurse();       // call recurse
}

int main(void) {
    asm("mov esp, 0x300004");  // borde inferior del user_stack
    return recurse();          // segundo CALL escribe en user_code → fault
}
`,
				Bytes: []byte{
					0xBC, 0x04, 0x00, 0x30, 0x00, // mov esp, 0x300004
					0xE8, 0xFB, 0xFF, 0xFF, 0xFF, // call -5 (target = same CALL)
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1       (unreachable)
					0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV ESP, 0x300004  ; near stack floor", CLine: 9},
					{Offset: 0x05, Mnemonic: "CALL recurse       ; pushes return address", CLine: 10},
					{Offset: 0x0A, Mnemonic: "MOV EAX, 1         ; sys_exit (unreachable)", CLine: 10},
					{Offset: 0x0F, Mnemonic: "MOV EBX, 0", CLine: 10},
					{Offset: 0x14, Mnemonic: "INT 0x80", CLine: 10},
				},
			}},
		},

		{
			Name:        "syscall_kernel_pointer",
			Category:    "fallos",
			Description: "Pasa un puntero kernel a sys_write. Un kernel real debe rechazarlo con -EFAULT; esta demo existe para contrastar esa regla con la simplificación actual del simulador.",
			Procs: []Proc{{
				Name: "syscall_kernel_pointer",
				Source: `// Puntero inválido en syscall. En un OS real copy_from_user validaría
// que el buffer sea accesible desde user mode antes de leerlo.
int main(void) {
    write(1, (char *)0x100000, 8);  // kernel pointer: debería ser -EFAULT
    exit(0);
}
`,
				Bytes: []byte{
					0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4 (sys_write)
					0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1 (stdout)
					0xB9, 0x00, 0x00, 0x10, 0x00, // mov ecx, 0x100000 (kernel)
					0xBA, 0x08, 0x00, 0x00, 0x00, // mov edx, 8
					0xCD, 0x80, // int 0x80
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
					0x31, 0xDB, // xor ebx, ebx
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 4     ; sys_write", CLine: 4},
					{Offset: 0x05, Mnemonic: "MOV EBX, 1     ; stdout", CLine: 4},
					{Offset: 0x0A, Mnemonic: "MOV ECX, 0x100000  ; kernel ptr", CLine: 4},
					{Offset: 0x0F, Mnemonic: "MOV EDX, 8", CLine: 4},
					{Offset: 0x14, Mnemonic: "INT 0x80       ; should be -EFAULT", CLine: 4},
					{Offset: 0x16, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 5},
					{Offset: 0x1B, Mnemonic: "XOR EBX, EBX", CLine: 5},
					{Offset: 0x1D, Mnemonic: "INT 0x80", CLine: 5},
				},
			}},
		},

		{
			Name:        "unknown_syscall",
			Category:    "fallos",
			Description: "Invoca un número de syscall no implementado. El kernel devuelve 0xFFFFFFFF en EAX, equivalente pedagógico de un error tipo -ENOSYS.",
			Procs: []Proc{{
				Name: "unknown_syscall",
				Source: `// Syscall inexistente: el kernel no conoce EAX=999.
int main(void) {
    long r = syscall(999);   // EAX queda en 0xFFFFFFFF
    exit(0);
}
`,
				Bytes: []byte{
					0xB8, 0xE7, 0x03, 0x00, 0x00, // mov eax, 999
					0xCD, 0x80, // int 0x80
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0x31, 0xDB, // xor ebx, ebx
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 999   ; unknown syscall", CLine: 3},
					{Offset: 0x05, Mnemonic: "INT 0x80       ; EAX=0xFFFFFFFF", CLine: 3},
					{Offset: 0x07, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 4},
					{Offset: 0x0C, Mnemonic: "XOR EBX, EBX", CLine: 4},
					{Offset: 0x0E, Mnemonic: "INT 0x80", CLine: 4},
				},
			}},
		},

		{
			Name:        "double_unlock",
			Category:    "fallos",
			Description: "Crea un mutex e intenta desbloquearlo sin ser holder. El kernel rechaza la operación con EAX=0xFFFFFFFF; útil para ver errores de sincronización que no son page faults.",
			Procs: []Proc{{
				Name: "double_unlock",
				Source: `// Bug clásico: unlock sin ownership. El mutex existe, pero nadie lo tomó.
int main(void) {
    int m = mutex_create();  // id=1
    mutex_unlock(m);         // error: caller no es holder
    exit(0);
}
`,
				Bytes: []byte{
					0xB8, 0x10, 0x00, 0x00, 0x00, // mov eax, 0x10 (MutexCreate)
					0xCD, 0x81, // int 0x81
					0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
					0xB8, 0x12, 0x00, 0x00, 0x00, // mov eax, 0x12 (MutexUnlock)
					0xCD, 0x81, // int 0x81
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0x31, 0xDB, // xor ebx, ebx
					0xCD, 0x80, // int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 0x10  ; MutexCreate", CLine: 3},
					{Offset: 0x05, Mnemonic: "INT 0x81       ; m=1", CLine: 3},
					{Offset: 0x07, Mnemonic: "MOV EBX, 1     ; mutex id", CLine: 4},
					{Offset: 0x0C, Mnemonic: "MOV EAX, 0x12  ; MutexUnlock", CLine: 4},
					{Offset: 0x11, Mnemonic: "INT 0x81       ; EAX=0xFFFFFFFF", CLine: 4},
					{Offset: 0x13, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 5},
					{Offset: 0x18, Mnemonic: "XOR EBX, EBX", CLine: 5},
					{Offset: 0x1A, Mnemonic: "INT 0x80", CLine: 5},
				},
			}},
		},

		{
			Name:        "semaphore_leak",
			Category:    "fallos",
			Description: "Crea tres semáforos y termina sin usarlos ni liberarlos. No rompe el kernel, pero deja recursos vivos: una demo compacta de resource leak.",
			Procs: []Proc{{
				Name: "semaphore_leak",
				Source: `// Resource leak: objetos kernel creados y nunca liberados.
int main(void) {
    sem_create(1);
    sem_create(2);
    sem_create(3);
    exit(0);
}
`,
				Bytes: []byte{
					0xB8, 0x20, 0x00, 0x00, 0x00, // mov eax, 0x20
					0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
					0xCD, 0x81,
					0xB8, 0x20, 0x00, 0x00, 0x00,
					0xBB, 0x02, 0x00, 0x00, 0x00,
					0xCD, 0x81,
					0xB8, 0x20, 0x00, 0x00, 0x00,
					0xBB, 0x03, 0x00, 0x00, 0x00,
					0xCD, 0x81,
					0xB8, 0x01, 0x00, 0x00, 0x00,
					0x31, 0xDB,
					0xCD, 0x80,
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 0x20  ; SemCreate", CLine: 3},
					{Offset: 0x05, Mnemonic: "MOV EBX, 1     ; init=1", CLine: 3},
					{Offset: 0x0A, Mnemonic: "INT 0x81       ; sem 1", CLine: 3},
					{Offset: 0x0C, Mnemonic: "MOV EAX, 0x20  ; SemCreate", CLine: 4},
					{Offset: 0x11, Mnemonic: "MOV EBX, 2     ; init=2", CLine: 4},
					{Offset: 0x16, Mnemonic: "INT 0x81       ; sem 2", CLine: 4},
					{Offset: 0x18, Mnemonic: "MOV EAX, 0x20  ; SemCreate", CLine: 5},
					{Offset: 0x1D, Mnemonic: "MOV EBX, 3     ; init=3", CLine: 5},
					{Offset: 0x22, Mnemonic: "INT 0x81       ; sem 3", CLine: 5},
					{Offset: 0x24, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 6},
					{Offset: 0x29, Mnemonic: "XOR EBX, EBX", CLine: 6},
					{Offset: 0x2B, Mnemonic: "INT 0x80", CLine: 6},
				},
			}},
		},

		{
			Name:        "latency_quantum",
			Category:    "scheduler",
			Description: "Dos procesos CPU-bound y uno interactivo. Con quantum alto, el reader tarda más en volver a correr después de una tecla; con quantum bajo, la latencia baja pero suben los context switches.",
			Procs: []Proc{
				spinCounterProc("hog_eax", "EAX", 0x40),
				spinCounterProc("hog_ebx", "EBX", 0x43),
				{
					Name: "interactive_reader",
					Source: `// Proceso interactivo: bloquea esperando teclado y luego hace echo.
// Compare quantum alto vs bajo: la tecla llega por IRQ, pero la vuelta a RUN
// compite con los CPU-bound en la ready queue.
char buf;  // PID 3 → stack @ 0x3BFFE0

int main(void) {
    while (1) {
        read(0, &buf, 1);
        write(1, &buf, 1);
    }
}
`,
					Bytes: []byte{
						0xB8, 0x03, 0x00, 0x00, 0x00,
						0xBB, 0x00, 0x00, 0x00, 0x00,
						0xB9, 0xE0, 0xFF, 0x3B, 0x00,
						0xBA, 0x01, 0x00, 0x00, 0x00,
						0xCD, 0x80,
						0xB8, 0x04, 0x00, 0x00, 0x00,
						0xBB, 0x01, 0x00, 0x00, 0x00,
						0xB9, 0xE0, 0xFF, 0x3B, 0x00,
						0xBA, 0x01, 0x00, 0x00, 0x00,
						0xCD, 0x80,
						0xEB, 0xD2,
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EAX, 3    ; sys_read", CLine: 8},
						{Offset: 0x05, Mnemonic: "MOV EBX, 0    ; stdin", CLine: 8},
						{Offset: 0x0A, Mnemonic: "MOV ECX, 0x3BFFE0", CLine: 8},
						{Offset: 0x0F, Mnemonic: "MOV EDX, 1", CLine: 8},
						{Offset: 0x14, Mnemonic: "INT 0x80      ; BLOCKED", CLine: 8},
						{Offset: 0x16, Mnemonic: "MOV EAX, 4    ; sys_write", CLine: 9},
						{Offset: 0x1B, Mnemonic: "MOV EBX, 1    ; stdout", CLine: 9},
						{Offset: 0x20, Mnemonic: "MOV ECX, 0x3BFFE0", CLine: 9},
						{Offset: 0x25, Mnemonic: "MOV EDX, 1", CLine: 9},
						{Offset: 0x2A, Mnemonic: "INT 0x80      ; echo", CLine: 9},
						{Offset: 0x2C, Mnemonic: "JMP -46       ; while(1)", CLine: 7},
					},
				},
			},
		},

		// ─── Showcase ─────────────────────────────────────────────────────
		// Single demo that exercises every CPU-driven animation in one run.
		// Cuatro procesos repartidos automáticamente uno por core: P1 camina
		// memoria compartida (ByteFlight + heat grid + minimap pins), P2
		// bloquea esperando teclado (BLOCKED + IRQ 0x21), P3 crea un
		// semáforo y emite signals (INT 0x81 + ModeFlip C2), P4 espera el
		// semáforo y termina con un page fault deliberado en kernel space.
		{
			Name:        "showcase",
			Category:    "showcase",
			Cores:       4,
			Description: "Paseo completo en 4 cores. Toca byte flights, heat de memoria, semáforos, sys_read bloqueante y page fault — exhibe casi todas las animaciones del simulador en una sola corrida. Apriete una tecla en la consola para desbloquear al reader (C1). HandRaiseBadge, ConnectionBanner y el glosario inline se prueban aparte (no son disparables desde un programa).",
			Procs: []Proc{
				{
					Name: "memwalk",
					Source: `// memwalk (Core 0) — recorre cuatro contadores compartidos en USER_STACK.
// Cinco iteraciones × (load + inc + store) sobre cuatro direcciones
// distintas. Resultado visible: byte flights repetidos hacia el HexGrid,
// celdas con heat, pins en el minimap a 4 direcciones contiguas, eventos
// con tag C0 en el ActivityFeed.

int *counters = (int *)0x3FFE00;   // 4 ints contiguos

int main(void) {
    for (int i = 0; i < 5; i++) {
        counters[0]++;
        counters[1]++;
        counters[2]++;
        counters[3]++;
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xB9, 0x05, 0x00, 0x00, 0x00, // mov ecx, 5
						0xA1, 0x00, 0xFE, 0x3F, 0x00, // L: mov eax, [0x3FFE00]
						0x40,                         // inc eax
						0xA3, 0x00, 0xFE, 0x3F, 0x00, // mov [0x3FFE00], eax
						0xA1, 0x04, 0xFE, 0x3F, 0x00, // mov eax, [0x3FFE04]
						0x40,                         // inc eax
						0xA3, 0x04, 0xFE, 0x3F, 0x00, // mov [0x3FFE04], eax
						0xA1, 0x08, 0xFE, 0x3F, 0x00, // mov eax, [0x3FFE08]
						0x40,                         // inc eax
						0xA3, 0x08, 0xFE, 0x3F, 0x00, // mov [0x3FFE08], eax
						0xA1, 0x0C, 0xFE, 0x3F, 0x00, // mov eax, [0x3FFE0C]
						0x40,                         // inc eax
						0xA3, 0x0C, 0xFE, 0x3F, 0x00, // mov [0x3FFE0C], eax
						0x49,       // dec ecx
						0x75, 0xD1, // jne L (target 0x05, after 0x34 → -0x2F)
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV ECX, 5    ; for init", CLine: 10},
						{Offset: 0x05, Mnemonic: "MOV EAX, [0x3FFE00]", CLine: 11},
						{Offset: 0x0A, Mnemonic: "INC EAX", CLine: 11},
						{Offset: 0x0B, Mnemonic: "MOV [0x3FFE00], EAX", CLine: 11},
						{Offset: 0x10, Mnemonic: "MOV EAX, [0x3FFE04]", CLine: 12},
						{Offset: 0x15, Mnemonic: "INC EAX", CLine: 12},
						{Offset: 0x16, Mnemonic: "MOV [0x3FFE04], EAX", CLine: 12},
						{Offset: 0x1B, Mnemonic: "MOV EAX, [0x3FFE08]", CLine: 13},
						{Offset: 0x20, Mnemonic: "INC EAX", CLine: 13},
						{Offset: 0x21, Mnemonic: "MOV [0x3FFE08], EAX", CLine: 13},
						{Offset: 0x26, Mnemonic: "MOV EAX, [0x3FFE0C]", CLine: 14},
						{Offset: 0x2B, Mnemonic: "INC EAX", CLine: 14},
						{Offset: 0x2C, Mnemonic: "MOV [0x3FFE0C], EAX", CLine: 14},
						{Offset: 0x31, Mnemonic: "DEC ECX", CLine: 10},
						{Offset: 0x32, Mnemonic: "JNE -47       ; for loop", CLine: 10},
						{Offset: 0x34, Mnemonic: "MOV EAX, 1    ; sys_exit", CLine: 17},
						{Offset: 0x39, Mnemonic: "XOR EBX, EBX", CLine: 17},
						{Offset: 0x3B, Mnemonic: "INT 0x80", CLine: 17},
					},
				},
				{
					Name: "reader",
					Source: `// reader (Core 1) — eco de teclado limitado a 3 lecturas.
// Cada sys_read BLOQUEA al proc (estado BLOCKED visible en /procesos
// y en el WaitQueue) hasta que llega IRQ 0x21. Al recibir la tecla:
// IRET visible en InterruptNarrator, PCBCopy restore al volver a user.

char buf;  // en su stack @ 0x37FFE0 (PID 2 → stack base 0x340000)

int main(void) {
    for (int i = 0; i < 3; i++) {
        read(0, &buf, 1);     // → BLOCKED hasta IRQ 0x21
        write(1, &buf, 1);    // echo a stdout
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xBF, 0x03, 0x00, 0x00, 0x00, // mov edi, 3 (loop counter)
						0xB8, 0x03, 0x00, 0x00, 0x00, // L: mov eax, 3 (sys_read)
						0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
						0xB9, 0xE0, 0xFF, 0x37, 0x00, // mov ecx, 0x37FFE0
						0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
						0xCD, 0x80, // int 0x80
						0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4 (sys_write)
						0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
						0xB9, 0xE0, 0xFF, 0x37, 0x00, // mov ecx, 0x37FFE0
						0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
						0xCD, 0x80, // int 0x80
						0x4F,       // dec edi
						0x75, 0xD1, // jne L (target 0x05, after 0x34 → -0x2F)
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EDI, 3    ; for init", CLine: 8},
						{Offset: 0x05, Mnemonic: "MOV EAX, 3    ; sys_read", CLine: 9},
						{Offset: 0x0A, Mnemonic: "MOV EBX, 0    ; fd=stdin", CLine: 9},
						{Offset: 0x0F, Mnemonic: "MOV ECX, 0x37FFE0", CLine: 9},
						{Offset: 0x14, Mnemonic: "MOV EDX, 1", CLine: 9},
						{Offset: 0x19, Mnemonic: "INT 0x80      ; BLOCKED → IRQ 0x21", CLine: 9},
						{Offset: 0x1B, Mnemonic: "MOV EAX, 4    ; sys_write", CLine: 10},
						{Offset: 0x20, Mnemonic: "MOV EBX, 1    ; fd=stdout", CLine: 10},
						{Offset: 0x25, Mnemonic: "MOV ECX, 0x37FFE0", CLine: 10},
						{Offset: 0x2A, Mnemonic: "MOV EDX, 1", CLine: 10},
						{Offset: 0x2F, Mnemonic: "INT 0x80      ; echo", CLine: 10},
						{Offset: 0x31, Mnemonic: "DEC EDI", CLine: 8},
						{Offset: 0x32, Mnemonic: "JNE -47       ; for loop", CLine: 8},
						{Offset: 0x34, Mnemonic: "MOV EAX, 1    ; sys_exit", CLine: 13},
						{Offset: 0x39, Mnemonic: "XOR EBX, EBX", CLine: 13},
						{Offset: 0x3B, Mnemonic: "INT 0x80", CLine: 13},
					},
				},
				{
					Name: "producer",
					Source: `// producer (Core 2) — crea un semáforo y emite cinco signals.
// El consumer (Core 3) espera ese mismo semáforo. INT 0x81 dispara el arc
// del InterruptNarrator y un ModeFlip user→kernel→user en cada signal.
// El kernel render del semáforo es visible en /concurrencia.

int main(void) {
    sem_create(0);                  // sem id 1 (primer sem del kernel)
    for (int i = 0; i < 5; i++) {
        sem_signal(1);
    }
    exit(0);
}
`,
					Bytes: []byte{
						0xB8, 0x20, 0x00, 0x00, 0x00, // mov eax, 0x20 (SemCreate)
						0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0    (init=0)
						0xCD, 0x81, // int 0x81 → eax=1
						0xB9, 0x05, 0x00, 0x00, 0x00, // mov ecx, 5
						0xBB, 0x01, 0x00, 0x00, 0x00, // L: mov ebx, 1 (sem id)
						0xB8, 0x22, 0x00, 0x00, 0x00, // mov eax, 0x22 (SemSignal)
						0xCD, 0x81, // int 0x81
						0x49,       // dec ecx
						0x75, 0xF1, // jne L (target 0x11, after 0x20 → -0x0F)
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
						0x31, 0xDB, // xor ebx, ebx
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV EAX, 0x20  ; SemCreate", CLine: 6},
						{Offset: 0x05, Mnemonic: "MOV EBX, 0     ; init=0", CLine: 6},
						{Offset: 0x0A, Mnemonic: "INT 0x81       ; → eax=1", CLine: 6},
						{Offset: 0x0C, Mnemonic: "MOV ECX, 5     ; for init", CLine: 7},
						{Offset: 0x11, Mnemonic: "MOV EBX, 1     ; sem id", CLine: 8},
						{Offset: 0x16, Mnemonic: "MOV EAX, 0x22  ; SemSignal", CLine: 8},
						{Offset: 0x1B, Mnemonic: "INT 0x81", CLine: 8},
						{Offset: 0x1D, Mnemonic: "DEC ECX", CLine: 7},
						{Offset: 0x1E, Mnemonic: "JNE -15        ; for loop", CLine: 7},
						{Offset: 0x20, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 10},
						{Offset: 0x25, Mnemonic: "XOR EBX, EBX", CLine: 10},
						{Offset: 0x27, Mnemonic: "INT 0x80", CLine: 10},
					},
				},
				{
					Name: "consumer_fault",
					Source: `// consumer_fault (Core 3) — espera el semáforo del producer y muere
// deliberadamente con page fault para exhibir IRQ 0x0E.
//
// Arranca con un delay (20 ticks) para darle margen al producer a crear
// el sem id 1. Después hace tres sem_wait (consume signals del producer)
// y termina escribiendo a 0x100000 — memoria del kernel, prohibida en
// modo usuario. La MMU dispara IRQ 0x0E → kernel mata el proc con state
// ZOMBIE y causa "page_fault", animado con el shake del PCB.

int main(void) {
    for (volatile int i = 0; i < 20; i++);   // espera al productor

    sem_wait(1);
    sem_wait(1);
    sem_wait(1);

    *(int *)0x100000 = 0xDEAD;   // ← PAGE FAULT
    return 0;                     // sys_exit(0) (unreachable)
}
`,
					Bytes: []byte{
						0xB9, 0x14, 0x00, 0x00, 0x00, // mov ecx, 20
						0x49,       // DL: dec ecx
						0x75, 0xFD, // jne DL (target 0x05, after 0x08 → -3)
						0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
						0xB8, 0x21, 0x00, 0x00, 0x00, // mov eax, 0x21 (SemWait)
						0xCD, 0x81, // int 0x81  ← 1st wait
						0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
						0xB8, 0x21, 0x00, 0x00, 0x00,
						0xCD, 0x81, // int 0x81  ← 2nd wait
						0xBB, 0x01, 0x00, 0x00, 0x00,
						0xB8, 0x21, 0x00, 0x00, 0x00,
						0xCD, 0x81, // int 0x81  ← 3rd wait
						0xB8, 0xAD, 0xDE, 0x00, 0x00, // mov eax, 0xDEAD
						0xA3, 0x00, 0x00, 0x10, 0x00, // mov [0x100000], eax ← ¡fault!
						0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1       (unreachable)
						0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
						0xCD, 0x80, // int 0x80
					},
					SourceMap: []SourceLine{
						{Offset: 0x00, Mnemonic: "MOV ECX, 20    ; delay init", CLine: 10},
						{Offset: 0x05, Mnemonic: "DEC ECX", CLine: 10},
						{Offset: 0x06, Mnemonic: "JNE -3         ; spin wait", CLine: 10},
						{Offset: 0x08, Mnemonic: "MOV EBX, 1     ; sem id", CLine: 12},
						{Offset: 0x0D, Mnemonic: "MOV EAX, 0x21  ; SemWait #1", CLine: 12},
						{Offset: 0x12, Mnemonic: "INT 0x81", CLine: 12},
						{Offset: 0x14, Mnemonic: "MOV EBX, 1", CLine: 13},
						{Offset: 0x19, Mnemonic: "MOV EAX, 0x21  ; SemWait #2", CLine: 13},
						{Offset: 0x1E, Mnemonic: "INT 0x81", CLine: 13},
						{Offset: 0x20, Mnemonic: "MOV EBX, 1", CLine: 14},
						{Offset: 0x25, Mnemonic: "MOV EAX, 0x21  ; SemWait #3", CLine: 14},
						{Offset: 0x2A, Mnemonic: "INT 0x81", CLine: 14},
						{Offset: 0x2C, Mnemonic: "MOV EAX, 0xDEAD", CLine: 16},
						{Offset: 0x31, Mnemonic: "MOV [0x100000], EAX  ; ¡fault!", CLine: 16},
						{Offset: 0x36, Mnemonic: "MOV EAX, 1       ; sys_exit (unreachable)", CLine: 17},
						{Offset: 0x3B, Mnemonic: "MOV EBX, 0", CLine: 17},
						{Offset: 0x40, Mnemonic: "INT 0x80", CLine: 17},
					},
				},
			},
		},

		{
			Name:        "demo_fork",
			Category:    "ipc",
			Description: "El padre crea un hijo con fork(): el hijo recibe EAX=0 y el padre el PID del hijo. El padre espera con waitpid() hasta que el hijo termine, y cosecha su exit_code. Se carga junto a init (PID 1), que ancla el árbol.",
			Procs: []Proc{{
				Name: "fork_basic",
				Source: `// fork() básico con espera del padre.
// El kernel devuelve 0 en EAX al hijo y el PID del hijo al padre.
int main(void) {
    int pid = fork();             // sys_fork
    if (pid == 0) {
        return 42;                // rama del hijo: exit(42)
    }
    int reaped = waitpid(-1, 0, 0);  // padre se duerme hasta cosechar
    return reaped;                // exit code = PID que cosechó
}
`,
				Bytes: []byte{
					0xB8, 0x02, 0x00, 0x00, 0x00, // 0x00: mov eax, 2  (SysFork)
					0xCD, 0x80, // 0x05: int 0x80
					0x81, 0xF8, 0x00, 0x00, 0x00, 0x00, // 0x07: cmp eax, 0
					0x74, 0x1F, // 0x0D: jz +31 -> child at 0x2E
					0xB8, 0x07, 0x00, 0x00, 0x00, // 0x0F: mov eax, 7  (SysWaitpid)
					0xBB, 0xFF, 0xFF, 0xFF, 0xFF, // 0x14: mov ebx, -1
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x19: mov ecx, 0
					0xBA, 0x00, 0x00, 0x00, 0x00, // 0x1E: mov edx, 0
					0xCD, 0x80, // 0x23: int 0x80
					0x89, 0xC3, // 0x25: mov ebx, eax (exit code = reaped pid)
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x27: mov eax, 1  (SysExit)
					0xCD, 0x80, // 0x2C: int 0x80
					0xBB, 0x2A, 0x00, 0x00, 0x00, // 0x2E: mov ebx, 42 (child exit code)
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x33: mov eax, 1  (SysExit)
					0xCD, 0x80, // 0x38: int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 2       ; SysFork", CLine: 4},
					{Offset: 0x05, Mnemonic: "INT 0x80", CLine: 4},
					{Offset: 0x07, Mnemonic: "CMP EAX, 0", CLine: 5},
					{Offset: 0x0D, Mnemonic: "JZ child", CLine: 5},
					{Offset: 0x0F, Mnemonic: "MOV EAX, 7       ; SysWaitpid", CLine: 8},
					{Offset: 0x14, Mnemonic: "MOV EBX, -1", CLine: 8},
					{Offset: 0x19, Mnemonic: "MOV ECX, 0", CLine: 8},
					{Offset: 0x1E, Mnemonic: "MOV EDX, 0", CLine: 8},
					{Offset: 0x23, Mnemonic: "INT 0x80", CLine: 8},
					{Offset: 0x25, Mnemonic: "MOV EBX, EAX     ; exit = reaped", CLine: 9},
					{Offset: 0x27, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 9},
					{Offset: 0x2C, Mnemonic: "INT 0x80", CLine: 9},
					{Offset: 0x2E, Mnemonic: "MOV EBX, 42      ; child exit", CLine: 6},
					{Offset: 0x33, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 6},
					{Offset: 0x38, Mnemonic: "INT 0x80", CLine: 6},
				},
			}},
		},

		{
			Name:        "demo_pipe",
			Category:    "ipc",
			Description: "El padre crea un pipe, escribe \"HOLA\", forkea un hijo, y espera. El hijo lee los 4 bytes del pipe y los manda a stdout. La consola muestra HOLA aunque ningún proceso lo haya tipeado en su .data — el dato cruzó un canal IPC.",
			Procs: []Proc{{
				Name: "pipe_basic",
				Source: `// Pipe anónimo entre padre e hijo:
//   1) sys_pipe → fdR/fdW.
//   2) padre escribe "HOLA" en fdW (los bytes quedan en el buffer del pipe).
//   3) fork. El hijo hereda los fds.
//   4) hijo lee 4 bytes de fdR y los manda a stdout.
//   5) padre llama waitpid y termina.
int main(void) {
    int fds[2];
    pipe(fds);
    write(fds[1], "HOLA", 4);     // antes del fork — sin race
    int pid = fork();
    if (pid == 0) {
        char buf[4];
        read(fds[0], buf, 4);
        write(1, buf, 4);          // echo a stdout
        return 0;
    }
    int reaped = waitpid(-1, 0, 0);
    return reaped;
}
`,
				Bytes: []byte{
					0xB8, 0x2A, 0x00, 0x00, 0x00, // 0x00: mov eax, 42 (SysPipe)
					0xBB, 0x00, 0x00, 0x31, 0x00, // 0x05: mov ebx, 0x310000
					0xCD, 0x80, // 0x0A: int 0x80
					0xB8, 0x48, 0x4F, 0x4C, 0x41, // 0x0C: mov eax, 0x414C4F48 ("HOLA")
					0xA3, 0x10, 0x00, 0x31, 0x00, // 0x11: mov [0x310010], eax
					0xA1, 0x04, 0x00, 0x31, 0x00, // 0x16: mov eax, [0x350004] (fdW)
					0x89, 0xC3, // 0x1B: mov ebx, eax
					0xB8, 0x04, 0x00, 0x00, 0x00, // 0x1D: mov eax, 4 (SysWrite)
					0xB9, 0x10, 0x00, 0x31, 0x00, // 0x22: mov ecx, 0x310010
					0xBA, 0x04, 0x00, 0x00, 0x00, // 0x27: mov edx, 4
					0xCD, 0x80, // 0x2C: int 0x80
					0xB8, 0x02, 0x00, 0x00, 0x00, // 0x2E: mov eax, 2 (SysFork)
					0xCD, 0x80, // 0x33: int 0x80
					0x81, 0xF8, 0x00, 0x00, 0x00, 0x00, // 0x35: cmp eax, 0
					0x74, 0x1F, // 0x3B: jz +31 -> child at 0x5C
					// PARENT path: waitpid + exit
					0xB8, 0x07, 0x00, 0x00, 0x00, // 0x3D: mov eax, 7 (SysWaitpid)
					0xBB, 0xFF, 0xFF, 0xFF, 0xFF, // 0x42: mov ebx, -1
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x47: mov ecx, 0
					0xBA, 0x00, 0x00, 0x00, 0x00, // 0x4C: mov edx, 0
					0xCD, 0x80, // 0x51: int 0x80
					0x89, 0xC3, // 0x53: mov ebx, eax (exit = reaped)
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x55: mov eax, 1 (SysExit)
					0xCD, 0x80, // 0x5A: int 0x80
					// CHILD path at 0x5C
					0xA1, 0x00, 0x00, 0x31, 0x00, // 0x5C: mov eax, [0x310000] (fdR)
					0x89, 0xC3, // 0x61: mov ebx, eax
					0xB8, 0x03, 0x00, 0x00, 0x00, // 0x63: mov eax, 3 (SysRead)
					0xB9, 0x20, 0x00, 0x31, 0x00, // 0x68: mov ecx, 0x310020
					0xBA, 0x04, 0x00, 0x00, 0x00, // 0x6D: mov edx, 4
					0xCD, 0x80, // 0x72: int 0x80
					// echo to stdout
					0xBB, 0x01, 0x00, 0x00, 0x00, // 0x74: mov ebx, 1
					0xB8, 0x04, 0x00, 0x00, 0x00, // 0x79: mov eax, 4 (SysWrite)
					0xB9, 0x20, 0x00, 0x31, 0x00, // 0x7E: mov ecx, 0x310020
					0xBA, 0x04, 0x00, 0x00, 0x00, // 0x83: mov edx, 4
					0xCD, 0x80, // 0x88: int 0x80
					0xBB, 0x00, 0x00, 0x00, 0x00, // 0x8A: mov ebx, 0
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x8F: mov eax, 1 (SysExit)
					0xCD, 0x80, // 0x94: int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 42      ; SysPipe", CLine: 8},
					{Offset: 0x05, Mnemonic: "MOV EBX, 0x310000", CLine: 8},
					{Offset: 0x0A, Mnemonic: "INT 0x80", CLine: 8},
					{Offset: 0x0C, Mnemonic: "MOV EAX, 0x414C4F48 ; \"HOLA\"", CLine: 9},
					{Offset: 0x11, Mnemonic: "MOV [0x310010], EAX", CLine: 9},
					{Offset: 0x16, Mnemonic: "MOV EAX, [0x350004] ; fdW", CLine: 9},
					{Offset: 0x1B, Mnemonic: "MOV EBX, EAX", CLine: 9},
					{Offset: 0x1D, Mnemonic: "MOV EAX, 4       ; SysWrite", CLine: 9},
					{Offset: 0x22, Mnemonic: "MOV ECX, 0x310010", CLine: 9},
					{Offset: 0x27, Mnemonic: "MOV EDX, 4", CLine: 9},
					{Offset: 0x2C, Mnemonic: "INT 0x80", CLine: 9},
					{Offset: 0x2E, Mnemonic: "MOV EAX, 2       ; SysFork", CLine: 10},
					{Offset: 0x33, Mnemonic: "INT 0x80", CLine: 10},
					{Offset: 0x35, Mnemonic: "CMP EAX, 0", CLine: 11},
					{Offset: 0x3B, Mnemonic: "JZ child", CLine: 11},
					{Offset: 0x3D, Mnemonic: "MOV EAX, 7       ; SysWaitpid", CLine: 17},
					{Offset: 0x42, Mnemonic: "MOV EBX, -1", CLine: 17},
					{Offset: 0x51, Mnemonic: "INT 0x80", CLine: 17},
					{Offset: 0x53, Mnemonic: "MOV EBX, EAX     ; exit = reaped", CLine: 18},
					{Offset: 0x55, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 18},
					{Offset: 0x5A, Mnemonic: "INT 0x80", CLine: 18},
					{Offset: 0x5C, Mnemonic: "MOV EAX, [0x310000] ; fdR", CLine: 13},
					{Offset: 0x63, Mnemonic: "MOV EAX, 3       ; SysRead", CLine: 13},
					{Offset: 0x72, Mnemonic: "INT 0x80", CLine: 13},
					{Offset: 0x74, Mnemonic: "MOV EBX, 1       ; stdout", CLine: 14},
					{Offset: 0x79, Mnemonic: "MOV EAX, 4       ; SysWrite", CLine: 14},
					{Offset: 0x88, Mnemonic: "INT 0x80", CLine: 14},
					{Offset: 0x8A, Mnemonic: "MOV EBX, 0", CLine: 15},
					{Offset: 0x8F, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 15},
					{Offset: 0x94, Mnemonic: "INT 0x80", CLine: 15},
				},
			}},
		},

		{
			Name:        "demo_vfork",
			Category:    "ipc",
			Description: "vfork() comparte la memoria del padre con el hijo y deja al padre BLOCKED hasta que el hijo termina. El hijo escribe 0xDEADBEEF en una dirección y muere; el padre, al despertar, lee esa misma dirección y ve el valor cambiado.",
			Procs: []Proc{{
				Name: "vfork_shared_mem",
				Source: `// vfork(): el hijo comparte el espacio de direcciones del padre.
// Lo que escribe el hijo es visible para el padre cuando despierta.
// (Linux real combina vfork con exec; aquí mostramos solo el efecto
// pedagógico de la memoria compartida.)
int main(void) {
    int pid = vfork();
    if (pid == 0) {
        *(int *)0x310000 = 0xDEADBEEF;
        return 0;
    }
    int v = *(int *)0x310000;
    return v;                     // exit code = lo que escribió el hijo
}
`,
				Bytes: []byte{
					0xB8, 0xBE, 0x00, 0x00, 0x00, // 0x00: mov eax, 190 (SysVfork)
					0xCD, 0x80, // 0x05: int 0x80
					0x81, 0xF8, 0x00, 0x00, 0x00, 0x00, // 0x07: cmp eax, 0
					0x74, 0x0E, // 0x0D: jz +14 -> child at 0x1D
					0xA1, 0x00, 0x00, 0x31, 0x00, // 0x0F: mov eax, [0x310000]
					0x89, 0xC3, // 0x14: mov ebx, eax
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x16: mov eax, 1 (SysExit)
					0xCD, 0x80, // 0x1B: int 0x80
					0xB8, 0xEF, 0xBE, 0xAD, 0xDE, // 0x1D: mov eax, 0xDEADBEEF
					0xA3, 0x00, 0x00, 0x31, 0x00, // 0x22: mov [0x310000], eax
					0xBB, 0x00, 0x00, 0x00, 0x00, // 0x27: mov ebx, 0 (child exit)
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x2C: mov eax, 1
					0xCD, 0x80, // 0x31: int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 190     ; SysVfork", CLine: 6},
					{Offset: 0x05, Mnemonic: "INT 0x80", CLine: 6},
					{Offset: 0x07, Mnemonic: "CMP EAX, 0", CLine: 7},
					{Offset: 0x0D, Mnemonic: "JZ child", CLine: 7},
					{Offset: 0x0F, Mnemonic: "MOV EAX, [0x310000]", CLine: 11},
					{Offset: 0x14, Mnemonic: "MOV EBX, EAX     ; exit = valor leído", CLine: 12},
					{Offset: 0x16, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 12},
					{Offset: 0x1B, Mnemonic: "INT 0x80", CLine: 12},
					{Offset: 0x1D, Mnemonic: "MOV EAX, 0xDEADBEEF", CLine: 8},
					{Offset: 0x22, Mnemonic: "MOV [0x310000], EAX  ; shared write", CLine: 8},
					{Offset: 0x27, Mnemonic: "MOV EBX, 0       ; child exit", CLine: 9},
					{Offset: 0x2C, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 9},
					{Offset: 0x31, Mnemonic: "INT 0x80", CLine: 9},
				},
			}},
		},

		{
			Name:        "threads_join",
			Category:    "concurrencia",
			Description: "clone(CLONE_VM|CLONE_FILES|CLONE_THREAD) crea un hilo hermano con memoria, file descriptors y TGID compartidos con el creador. El padre llama waitpid() para esperar que el hilo termine, lee su retval y sale. El hilo escribe en stdout antes de morir para que el flujo sea visible en la BITACORA.",
			Procs: []Proc{{
				Name: "thread_join",
				Source: `// pthread_create + pthread_join, modelado directo sobre sys_clone (120)
// y sys_waitpid (7). El hilo es child del creador en este simulador, por
// eso waitpid funciona; en Linux real glibc usa futex sobre el TID.
int main(void) {
    int tid = clone(0x10500, 0, thread_fn);
    int ret = waitpid(tid, 0, 0);
    return ret;
}

void thread_fn(void) {
    write(1, "T2 done\n", 8);
    exit(123);
}
`,
				Bytes: []byte{
					// main at 0x00
					0xB8, 0x78, 0x00, 0x00, 0x00, // 0x00: mov eax, 120 (SysClone)
					0xBB, 0x00, 0x05, 0x01, 0x00, // 0x05: mov ebx, 0x10500 (CLONE_VM|CLONE_FILES|CLONE_THREAD)
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x0A: mov ecx, 0 (kernel allocates stack slot)
					0xBA, 0x80, 0x00, 0x20, 0x00, // 0x0F: mov edx, 0x200080 (thread_fn EIP)
					0xCD, 0x80, // 0x14: int 0x80
					0x89, 0xC3, // 0x16: mov ebx, eax (save thread tid in EBX for waitpid)
					0xB8, 0x07, 0x00, 0x00, 0x00, // 0x18: mov eax, 7 (SysWaitpid)
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x1D: mov ecx, 0
					0xBA, 0x00, 0x00, 0x00, 0x00, // 0x22: mov edx, 0
					0xCD, 0x80, // 0x27: int 0x80
					0x89, 0xC3, // 0x29: mov ebx, eax (exit code = reaped tid)
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x2B: mov eax, 1 (SysExit)
					0xCD, 0x80, // 0x30: int 0x80
					// pad to 0x80
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x32..0x39
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x3A..0x41
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x42..0x49
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x4A..0x51
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x52..0x59
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x5A..0x61
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x62..0x69
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x6A..0x71
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x72..0x79
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x7A..0x7F
					// thread_fn at 0x80
					0xB8, 0x04, 0x00, 0x00, 0x00, // 0x80: mov eax, 4 (SysWrite)
					0xBB, 0x01, 0x00, 0x00, 0x00, // 0x85: mov ebx, 1 (stdout)
					0xB9, 0xC0, 0x00, 0x20, 0x00, // 0x8A: mov ecx, 0x2000C0 (msg)
					0xBA, 0x08, 0x00, 0x00, 0x00, // 0x8F: mov edx, 8
					0xCD, 0x80, // 0x94: int 0x80
					0xBB, 0x7B, 0x00, 0x00, 0x00, // 0x96: mov ebx, 123 (thread retval)
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x9B: mov eax, 1 (SysExit)
					0xCD, 0x80, // 0xA0: int 0x80
					// pad to 0xC0 (28 bytes of NOP)
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0xA2..0xA9
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0xAA..0xB1
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0xB2..0xB9
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0xBA..0xBF
					// msg at 0xC0: "T2 done\n"
					0x54, 0x32, 0x20, 0x64, 0x6F, 0x6E, 0x65, 0x0A, // 0xC0..0xC7
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 120     ; SysClone", CLine: 5},
					{Offset: 0x05, Mnemonic: "MOV EBX, 0x10500 ; CLONE_VM|CLONE_FILES|CLONE_THREAD", CLine: 5},
					{Offset: 0x0A, Mnemonic: "MOV ECX, 0       ; stack=auto", CLine: 5},
					{Offset: 0x0F, Mnemonic: "MOV EDX, 0x200080; thread_fn EIP", CLine: 5},
					{Offset: 0x14, Mnemonic: "INT 0x80", CLine: 5},
					{Offset: 0x16, Mnemonic: "MOV EBX, EAX     ; tid -> waitpid arg", CLine: 6},
					{Offset: 0x18, Mnemonic: "MOV EAX, 7       ; SysWaitpid", CLine: 6},
					{Offset: 0x1D, Mnemonic: "MOV ECX, 0", CLine: 6},
					{Offset: 0x22, Mnemonic: "MOV EDX, 0", CLine: 6},
					{Offset: 0x27, Mnemonic: "INT 0x80", CLine: 6},
					{Offset: 0x29, Mnemonic: "MOV EBX, EAX     ; exit = reaped tid", CLine: 7},
					{Offset: 0x2B, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 7},
					{Offset: 0x30, Mnemonic: "INT 0x80", CLine: 7},
					{Offset: 0x80, Mnemonic: "MOV EAX, 4       ; SysWrite", CLine: 11},
					{Offset: 0x85, Mnemonic: "MOV EBX, 1       ; stdout", CLine: 11},
					{Offset: 0x8A, Mnemonic: "MOV ECX, 0x2000C0; \"T2 done\\n\"", CLine: 11},
					{Offset: 0x8F, Mnemonic: "MOV EDX, 8", CLine: 11},
					{Offset: 0x94, Mnemonic: "INT 0x80", CLine: 11},
					{Offset: 0x96, Mnemonic: "MOV EBX, 123     ; retval", CLine: 12},
					{Offset: 0x9B, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 12},
					{Offset: 0xA0, Mnemonic: "INT 0x80", CLine: 12},
				},
			}},
		},

		{
			Name:        "threads_race",
			Category:    "concurrencia",
			Description: "Dos hilos hermanos comparten un contador en 0x3F0000 e incrementan 200 veces cada uno. Sin sincronizacion, el entrelazado de los context switches deja race conditions visibles: el contador final queda muy por debajo de 400. El exit code del proceso reporta el valor final.",
			Procs: []Proc{{
				Name: "thread_race",
				Source: `// Race condition clasica: load-inc-store no atomico bajo timer
// preemption. Cada hilo hace counter++ 200 veces; sin mutex el
// resultado < 400 demuestra que el incremento NO es atomico.
volatile int *counter = (int *)0x3F0000;

int main(void) {
    *counter = 0;
    int t1 = clone(0x10500, 0, thread_fn);
    int t2 = clone(0x10500, 0, thread_fn);
    waitpid(-1, 0, 0);
    waitpid(-1, 0, 0);
    return *counter;
}

void thread_fn(void) {
    for (int i = 0; i < 200; i++) {
        int v = *counter;
        v = v + 1;
        *counter = v;
    }
    exit(0);
}
`,
				Bytes: []byte{
					// main at 0x00 — zero counter, clone twice, wait twice, exit with final value
					0xB8, 0x00, 0x00, 0x00, 0x00, // 0x00: mov eax, 0
					0xA3, 0x00, 0x00, 0x3F, 0x00, // 0x05: mov [0x3F0000], eax
					0xB8, 0x78, 0x00, 0x00, 0x00, // 0x0A: mov eax, 120 (SysClone)
					0xBB, 0x00, 0x05, 0x01, 0x00, // 0x0F: mov ebx, 0x10500
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x14: mov ecx, 0
					0xBA, 0x80, 0x00, 0x20, 0x00, // 0x19: mov edx, 0x200080
					0xCD, 0x80, // 0x1E: int 0x80
					0xB8, 0x78, 0x00, 0x00, 0x00, // 0x20: mov eax, 120 (second clone)
					0xBB, 0x00, 0x05, 0x01, 0x00, // 0x25: mov ebx, 0x10500
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x2A: mov ecx, 0
					0xBA, 0x80, 0x00, 0x20, 0x00, // 0x2F: mov edx, 0x200080
					0xCD, 0x80, // 0x34: int 0x80
					0xB8, 0x07, 0x00, 0x00, 0x00, // 0x36: mov eax, 7 (wait #1)
					0xBB, 0xFF, 0xFF, 0xFF, 0xFF, // 0x3B: mov ebx, -1
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x40: mov ecx, 0
					0xBA, 0x00, 0x00, 0x00, 0x00, // 0x45: mov edx, 0
					0xCD, 0x80, // 0x4A: int 0x80
					0xB8, 0x07, 0x00, 0x00, 0x00, // 0x4C: mov eax, 7 (wait #2)
					0xBB, 0xFF, 0xFF, 0xFF, 0xFF, // 0x51: mov ebx, -1
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x56: mov ecx, 0
					0xBA, 0x00, 0x00, 0x00, 0x00, // 0x5B: mov edx, 0
					0xCD, 0x80, // 0x60: int 0x80
					0xA1, 0x00, 0x00, 0x3F, 0x00, // 0x62: mov eax, [0x3F0000]
					0x89, 0xC3, // 0x67: mov ebx, eax (exit code = final counter)
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x69: mov eax, 1 (SysExit)
					0xCD, 0x80, // 0x6E: int 0x80
					// pad to 0x80
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x70..0x77
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // 0x78..0x7F
					// thread_fn at 0x80 — racy 200-iter loop on shared counter
					0xBB, 0x00, 0x00, 0x00, 0x00, // 0x80: mov ebx, 0 (loop counter)
					// loop_start at 0x85
					0xA1, 0x00, 0x00, 0x3F, 0x00, // 0x85: mov eax, [0x3F0000] (load)
					0x40,                         // 0x8A: inc eax
					0xA3, 0x00, 0x00, 0x3F, 0x00, // 0x8B: mov [0x3F0000], eax (store)
					0x43,                                     // 0x90: inc ebx
					0x81, 0xFB, 0xC8, 0x00, 0x00, 0x00,       // 0x91: cmp ebx, 200
					0x75, 0xEC,                               // 0x97: jne -20 -> 0x85
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x99: mov eax, 1 (SysExit)
					0xBB, 0x00, 0x00, 0x00, 0x00, // 0x9E: mov ebx, 0
					0xCD, 0x80, // 0xA3: int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 0       ; counter = 0", CLine: 7},
					{Offset: 0x05, Mnemonic: "MOV [0x3F0000], EAX", CLine: 7},
					{Offset: 0x0A, Mnemonic: "MOV EAX, 120     ; SysClone", CLine: 8},
					{Offset: 0x0F, Mnemonic: "MOV EBX, 0x10500", CLine: 8},
					{Offset: 0x14, Mnemonic: "MOV ECX, 0", CLine: 8},
					{Offset: 0x19, Mnemonic: "MOV EDX, 0x200080", CLine: 8},
					{Offset: 0x1E, Mnemonic: "INT 0x80         ; spawn t1", CLine: 8},
					{Offset: 0x20, Mnemonic: "MOV EAX, 120", CLine: 9},
					{Offset: 0x25, Mnemonic: "MOV EBX, 0x10500", CLine: 9},
					{Offset: 0x2A, Mnemonic: "MOV ECX, 0", CLine: 9},
					{Offset: 0x2F, Mnemonic: "MOV EDX, 0x200080", CLine: 9},
					{Offset: 0x34, Mnemonic: "INT 0x80         ; spawn t2", CLine: 9},
					{Offset: 0x36, Mnemonic: "MOV EAX, 7       ; SysWaitpid", CLine: 10},
					{Offset: 0x3B, Mnemonic: "MOV EBX, -1      ; any child", CLine: 10},
					{Offset: 0x40, Mnemonic: "MOV ECX, 0", CLine: 10},
					{Offset: 0x45, Mnemonic: "MOV EDX, 0", CLine: 10},
					{Offset: 0x4A, Mnemonic: "INT 0x80         ; reap #1", CLine: 10},
					{Offset: 0x4C, Mnemonic: "MOV EAX, 7", CLine: 11},
					{Offset: 0x51, Mnemonic: "MOV EBX, -1", CLine: 11},
					{Offset: 0x56, Mnemonic: "MOV ECX, 0", CLine: 11},
					{Offset: 0x5B, Mnemonic: "MOV EDX, 0", CLine: 11},
					{Offset: 0x60, Mnemonic: "INT 0x80         ; reap #2", CLine: 11},
					{Offset: 0x62, Mnemonic: "MOV EAX, [0x3F0000]", CLine: 12},
					{Offset: 0x67, Mnemonic: "MOV EBX, EAX     ; exit = final counter", CLine: 12},
					{Offset: 0x69, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 12},
					{Offset: 0x6E, Mnemonic: "INT 0x80", CLine: 12},
					{Offset: 0x80, Mnemonic: "MOV EBX, 0       ; i = 0", CLine: 16},
					{Offset: 0x85, Mnemonic: "MOV EAX, [0x3F0000] ; LOAD", CLine: 17},
					{Offset: 0x8A, Mnemonic: "INC EAX          ; +1", CLine: 18},
					{Offset: 0x8B, Mnemonic: "MOV [0x3F0000], EAX ; STORE (race window)", CLine: 19},
					{Offset: 0x90, Mnemonic: "INC EBX          ; i++", CLine: 16},
					{Offset: 0x91, Mnemonic: "CMP EBX, 200", CLine: 16},
					{Offset: 0x97, Mnemonic: "JNE loop_start", CLine: 16},
					{Offset: 0x99, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 22},
					{Offset: 0x9E, Mnemonic: "MOV EBX, 0", CLine: 22},
					{Offset: 0xA3, Mnemonic: "INT 0x80", CLine: 22},
				},
			}},
		},

		{
			Name:        "threads_safe",
			Category:    "concurrencia",
			Description: "Misma carrera de threads_race pero protegida por un mutex (INT 0x81). El leader crea el mutex y guarda el ID en 0x3F0010; cada hilo lock-ea antes del load-inc-store y unlock-ea despues. Resultado siempre = 400, sin importar el entrelazado.",
			Procs: []Proc{{
				Name: "thread_safe",
				Source: `// Race controlada por mutex_lock/unlock (INT 0x81 ops 0x11/0x12).
// El mutex serializa el critical section (load-inc-store) asi que el
// resultado final es deterministicamente 2 * 200 = 400.
volatile int *counter = (int *)0x3F0000;
int mutex_id;

int main(void) {
    *counter = 0;
    mutex_id = mutex_create();
    int t1 = clone(0x10500, 0, thread_fn);
    int t2 = clone(0x10500, 0, thread_fn);
    waitpid(-1, 0, 0);
    waitpid(-1, 0, 0);
    return *counter;
}

void thread_fn(void) {
    for (int i = 0; i < 200; i++) {
        mutex_lock(mutex_id);
        int v = *counter;
        v = v + 1;
        *counter = v;
        mutex_unlock(mutex_id);
    }
    exit(0);
}
`,
				Bytes: []byte{
					// main at 0x00
					0xB8, 0x00, 0x00, 0x00, 0x00, // 0x00: mov eax, 0
					0xA3, 0x00, 0x00, 0x3F, 0x00, // 0x05: mov [0x3F0000], eax (counter = 0)
					0xB8, 0x10, 0x00, 0x00, 0x00, // 0x0A: mov eax, 0x10 (SyncMutexCreate)
					0xCD, 0x81, // 0x0F: int 0x81
					0xA3, 0x10, 0x00, 0x3F, 0x00, // 0x11: mov [0x3F0010], eax (save mutex id)
					0xB8, 0x78, 0x00, 0x00, 0x00, // 0x16: mov eax, 120 (clone 1)
					0xBB, 0x00, 0x05, 0x01, 0x00, // 0x1B: mov ebx, 0x10500
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x20: mov ecx, 0
					0xBA, 0x80, 0x00, 0x20, 0x00, // 0x25: mov edx, 0x200080
					0xCD, 0x80, // 0x2A: int 0x80
					0xB8, 0x78, 0x00, 0x00, 0x00, // 0x2C: mov eax, 120 (clone 2)
					0xBB, 0x00, 0x05, 0x01, 0x00, // 0x31: mov ebx, 0x10500
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x36: mov ecx, 0
					0xBA, 0x80, 0x00, 0x20, 0x00, // 0x3B: mov edx, 0x200080
					0xCD, 0x80, // 0x40: int 0x80
					0xB8, 0x07, 0x00, 0x00, 0x00, // 0x42: mov eax, 7 (wait #1)
					0xBB, 0xFF, 0xFF, 0xFF, 0xFF, // 0x47: mov ebx, -1
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x4C: mov ecx, 0
					0xBA, 0x00, 0x00, 0x00, 0x00, // 0x51: mov edx, 0
					0xCD, 0x80, // 0x56: int 0x80
					0xB8, 0x07, 0x00, 0x00, 0x00, // 0x58: mov eax, 7 (wait #2)
					0xBB, 0xFF, 0xFF, 0xFF, 0xFF, // 0x5D: mov ebx, -1
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x62: mov ecx, 0
					0xBA, 0x00, 0x00, 0x00, 0x00, // 0x67: mov edx, 0
					0xCD, 0x80, // 0x6C: int 0x80
					0xA1, 0x00, 0x00, 0x3F, 0x00, // 0x6E: mov eax, [0x3F0000] (final counter)
					0x89, 0xC3, // 0x73: mov ebx, eax
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0x75: mov eax, 1 (SysExit)
					0xCD, 0x80, // 0x7A: int 0x80
					// pad to 0x80 (4 bytes of NOP)
					0x90, 0x90, 0x90, 0x90, // 0x7C..0x7F
					// thread_fn at 0x80
					0xB9, 0x00, 0x00, 0x00, 0x00, // 0x80: mov ecx, 0 (loop counter i)
					// loop_start at 0x85
					0xA1, 0x10, 0x00, 0x3F, 0x00, // 0x85: mov eax, [0x3F0010] (mutex id)
					0x89, 0xC3, // 0x8A: mov ebx, eax
					0xB8, 0x11, 0x00, 0x00, 0x00, // 0x8C: mov eax, 0x11 (SyncMutexLock)
					0xCD, 0x81, // 0x91: int 0x81  (blocks if held)
					0xA1, 0x00, 0x00, 0x3F, 0x00, // 0x93: mov eax, [0x3F0000] (load counter)
					0x40,                         // 0x98: inc eax
					0xA3, 0x00, 0x00, 0x3F, 0x00, // 0x99: mov [0x3F0000], eax (store)
					0xA1, 0x10, 0x00, 0x3F, 0x00, // 0x9E: mov eax, [0x3F0010] (mutex id again)
					0x89, 0xC3, // 0xA3: mov ebx, eax
					0xB8, 0x12, 0x00, 0x00, 0x00, // 0xA5: mov eax, 0x12 (SyncMutexUnlock)
					0xCD, 0x81, // 0xAA: int 0x81
					0x41,                                     // 0xAC: inc ecx
					0x81, 0xF9, 0xC8, 0x00, 0x00, 0x00,       // 0xAD: cmp ecx, 200
					0x75, 0xD0,                               // 0xB3: jne -48 -> 0x85
					0xB8, 0x01, 0x00, 0x00, 0x00, // 0xB5: mov eax, 1 (SysExit)
					0xBB, 0x00, 0x00, 0x00, 0x00, // 0xBA: mov ebx, 0
					0xCD, 0x80, // 0xBF: int 0x80
				},
				SourceMap: []SourceLine{
					{Offset: 0x00, Mnemonic: "MOV EAX, 0       ; counter = 0", CLine: 8},
					{Offset: 0x05, Mnemonic: "MOV [0x3F0000], EAX", CLine: 8},
					{Offset: 0x0A, Mnemonic: "MOV EAX, 0x10    ; SyncMutexCreate", CLine: 9},
					{Offset: 0x0F, Mnemonic: "INT 0x81", CLine: 9},
					{Offset: 0x11, Mnemonic: "MOV [0x3F0010], EAX ; mutex_id", CLine: 9},
					{Offset: 0x16, Mnemonic: "MOV EAX, 120     ; SysClone", CLine: 10},
					{Offset: 0x1B, Mnemonic: "MOV EBX, 0x10500", CLine: 10},
					{Offset: 0x20, Mnemonic: "MOV ECX, 0", CLine: 10},
					{Offset: 0x25, Mnemonic: "MOV EDX, 0x200080", CLine: 10},
					{Offset: 0x2A, Mnemonic: "INT 0x80         ; spawn t1", CLine: 10},
					{Offset: 0x2C, Mnemonic: "MOV EAX, 120", CLine: 11},
					{Offset: 0x31, Mnemonic: "MOV EBX, 0x10500", CLine: 11},
					{Offset: 0x36, Mnemonic: "MOV ECX, 0", CLine: 11},
					{Offset: 0x3B, Mnemonic: "MOV EDX, 0x200080", CLine: 11},
					{Offset: 0x40, Mnemonic: "INT 0x80         ; spawn t2", CLine: 11},
					{Offset: 0x42, Mnemonic: "MOV EAX, 7       ; SysWaitpid", CLine: 12},
					{Offset: 0x47, Mnemonic: "MOV EBX, -1", CLine: 12},
					{Offset: 0x4C, Mnemonic: "MOV ECX, 0", CLine: 12},
					{Offset: 0x51, Mnemonic: "MOV EDX, 0", CLine: 12},
					{Offset: 0x56, Mnemonic: "INT 0x80         ; reap #1", CLine: 12},
					{Offset: 0x58, Mnemonic: "MOV EAX, 7", CLine: 13},
					{Offset: 0x5D, Mnemonic: "MOV EBX, -1", CLine: 13},
					{Offset: 0x62, Mnemonic: "MOV ECX, 0", CLine: 13},
					{Offset: 0x67, Mnemonic: "MOV EDX, 0", CLine: 13},
					{Offset: 0x6C, Mnemonic: "INT 0x80         ; reap #2", CLine: 13},
					{Offset: 0x6E, Mnemonic: "MOV EAX, [0x3F0000]", CLine: 14},
					{Offset: 0x73, Mnemonic: "MOV EBX, EAX", CLine: 14},
					{Offset: 0x75, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 14},
					{Offset: 0x7A, Mnemonic: "INT 0x80", CLine: 14},
					{Offset: 0x80, Mnemonic: "MOV ECX, 0       ; i = 0", CLine: 18},
					{Offset: 0x85, Mnemonic: "MOV EAX, [0x3F0010] ; mutex_id", CLine: 19},
					{Offset: 0x8A, Mnemonic: "MOV EBX, EAX", CLine: 19},
					{Offset: 0x8C, Mnemonic: "MOV EAX, 0x11    ; SyncMutexLock", CLine: 19},
					{Offset: 0x91, Mnemonic: "INT 0x81         ; LOCK (may block)", CLine: 19},
					{Offset: 0x93, Mnemonic: "MOV EAX, [0x3F0000] ; critical section", CLine: 20},
					{Offset: 0x98, Mnemonic: "INC EAX", CLine: 21},
					{Offset: 0x99, Mnemonic: "MOV [0x3F0000], EAX", CLine: 22},
					{Offset: 0x9E, Mnemonic: "MOV EAX, [0x3F0010]", CLine: 23},
					{Offset: 0xA3, Mnemonic: "MOV EBX, EAX", CLine: 23},
					{Offset: 0xA5, Mnemonic: "MOV EAX, 0x12    ; SyncMutexUnlock", CLine: 23},
					{Offset: 0xAA, Mnemonic: "INT 0x81         ; UNLOCK", CLine: 23},
					{Offset: 0xAC, Mnemonic: "INC ECX", CLine: 18},
					{Offset: 0xAD, Mnemonic: "CMP ECX, 200", CLine: 18},
					{Offset: 0xB3, Mnemonic: "JNE loop_start", CLine: 18},
					{Offset: 0xB5, Mnemonic: "MOV EAX, 1       ; SysExit", CLine: 26},
					{Offset: 0xBA, Mnemonic: "MOV EBX, 0", CLine: 26},
					{Offset: 0xBF, Mnemonic: "INT 0x80", CLine: 26},
				},
			}},
		},
	}
}

// DemoNames returns the stable identifier of every demo, in the same order
// as Demos(). Useful for whitelisting input that references a demo by name.
func DemoNames() []string {
	all := Demos()
	out := make([]string, len(all))
	for i, d := range all {
		out[i] = d.Name
	}
	return out
}

// philoBytes emits a hand-encoded philosopher loop. Each philo locks its first
// fork, then its second, "eats" (two nops), and unlocks in reverse order. The
// shape is identical across philos; only the fork IDs vary, so the helper keeps
// the demo table readable.
func philoBytes(name string, idx, firstFork, secondFork int, label string) Proc {
	first := itoa(firstFork)
	second := itoa(secondFork)
	return Proc{
		Name: name,
		Source: "// Filósofo #" + itoa(idx) + " — " + label + "\n" +
			"int main(void) {\n" +
			"    for (int i = 0; i < 10; i++) {\n" +
			"        mutex_lock(" + first + ");\n" +
			"        mutex_lock(" + second + ");\n" +
			"        /* eat */\n" +
			"        mutex_unlock(" + second + ");\n" +
			"        mutex_unlock(" + first + ");\n" +
			"    }\n" +
			"    exit(0);\n" +
			"}\n",
		Bytes: []byte{
			0xB9, 0x0A, 0x00, 0x00, 0x00, // mov ecx, 10
			0xBB, byte(firstFork), 0x00, 0x00, 0x00, // L: mov ebx, firstFork
			0xB8, 0x11, 0x00, 0x00, 0x00, // mov eax, 0x11 (MutexLock)
			0xCD, 0x81, // int 0x81
			0xBB, byte(secondFork), 0x00, 0x00, 0x00, // mov ebx, secondFork
			0xB8, 0x11, 0x00, 0x00, 0x00, // mov eax, 0x11
			0xCD, 0x81,
			0x90, 0x90, // nop nop (eat)
			0xBB, byte(secondFork), 0x00, 0x00, 0x00, // mov ebx, secondFork
			0xB8, 0x12, 0x00, 0x00, 0x00, // mov eax, 0x12 (MutexUnlock)
			0xCD, 0x81,
			0xBB, byte(firstFork), 0x00, 0x00, 0x00, // mov ebx, firstFork
			0xB8, 0x12, 0x00, 0x00, 0x00,
			0xCD, 0x81,
			0x49,       // dec ecx
			0x75, 0xCB, // jne L (target 0x05, after 0x3A → delta -0x35)
			0xB8, 0x01, 0x00, 0x00, 0x00,
			0x31, 0xDB,
			0xCD, 0x80,
		},
		// Source layout (1-based):
		//   1  // Filósofo #N — <label>
		//   2  int main(void) {
		//   3      for (int i = 0; i < 10; i++) {
		//   4          mutex_lock(first);
		//   5          mutex_lock(second);
		//   6          /* eat */
		//   7          mutex_unlock(second);
		//   8          mutex_unlock(first);
		//   9      }
		//  10      exit(0);
		//  11  }
		SourceMap: []SourceLine{
			{Offset: 0x00, Mnemonic: "MOV ECX, 10    ; for init", CLine: 3},
			{Offset: 0x05, Mnemonic: "MOV EBX, " + first + "      ; first fork", CLine: 4},
			{Offset: 0x0A, Mnemonic: "MOV EAX, 0x11  ; MutexLock", CLine: 4},
			{Offset: 0x0F, Mnemonic: "INT 0x81", CLine: 4},
			{Offset: 0x11, Mnemonic: "MOV EBX, " + second + "      ; second fork", CLine: 5},
			{Offset: 0x16, Mnemonic: "MOV EAX, 0x11  ; MutexLock", CLine: 5},
			{Offset: 0x1B, Mnemonic: "INT 0x81", CLine: 5},
			{Offset: 0x1D, Mnemonic: "NOP            ; eat", CLine: 6},
			{Offset: 0x1E, Mnemonic: "NOP", CLine: 6},
			{Offset: 0x1F, Mnemonic: "MOV EBX, " + second, CLine: 7},
			{Offset: 0x24, Mnemonic: "MOV EAX, 0x12  ; MutexUnlock", CLine: 7},
			{Offset: 0x29, Mnemonic: "INT 0x81", CLine: 7},
			{Offset: 0x2B, Mnemonic: "MOV EBX, " + first, CLine: 8},
			{Offset: 0x30, Mnemonic: "MOV EAX, 0x12  ; MutexUnlock", CLine: 8},
			{Offset: 0x35, Mnemonic: "INT 0x81", CLine: 8},
			{Offset: 0x37, Mnemonic: "DEC ECX", CLine: 3},
			{Offset: 0x38, Mnemonic: "JNE -53        ; for loop", CLine: 3},
			{Offset: 0x3A, Mnemonic: "MOV EAX, 1     ; sys_exit", CLine: 10},
			{Offset: 0x3F, Mnemonic: "XOR EBX, EBX", CLine: 10},
			{Offset: 0x41, Mnemonic: "INT 0x80", CLine: 10},
		},
	}
}

// spinCounterProc is a tiny infinite CPU-bound process for scheduler demos.
// incOpcode is one of INC r32 (0x40..0x47), matching the register named in reg.
func spinCounterProc(name, reg string, incOpcode byte) Proc {
	return Proc{
		Name: name,
		Source: "// CPU-bound: nunca bloquea, solo consume quantum.\n" +
			"int main(void) {\n" +
			"    register int n = 0;      // mov " + reg + ", 0\n" +
			"    while (1) n++;           // inc " + reg + " ; jmp\n" +
			"}\n",
		Bytes: []byte{
			0xB8 + regOpcodeIndex(reg), 0x00, 0x00, 0x00, 0x00,
			incOpcode,
			0xEB, 0xFD,
		},
		SourceMap: []SourceLine{
			{Offset: 0x00, Mnemonic: "MOV " + reg + ", 0", CLine: 3},
			{Offset: 0x05, Mnemonic: "INC " + reg, CLine: 4},
			{Offset: 0x06, Mnemonic: "JMP -3        ; while(1)", CLine: 4},
		},
	}
}

func regOpcodeIndex(reg string) byte {
	switch reg {
	case "EAX":
		return 0
	case "ECX":
		return 1
	case "EDX":
		return 2
	case "EBX":
		return 3
	case "ESP":
		return 4
	case "EBP":
		return 5
	case "ESI":
		return 6
	case "EDI":
		return 7
	default:
		return 0
	}
}

// itoa is a tiny helper for the source-listing strings. Standard strconv works
// but pulling it in for one call is overkill.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
