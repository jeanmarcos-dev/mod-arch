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
	}
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
