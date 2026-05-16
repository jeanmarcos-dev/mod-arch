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
}

// Proc is a single binary that the kernel loads as a process.
//
// Source is the hand-written C-like equivalent shown to the user. It is NOT
// compiled into Bytes at runtime; the bytes are hand-encoded in parallel so
// the viewer sees WHAT the process does without reading assembly directly.
type Proc struct {
	Name   string
	Bytes  []byte
	Source string
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
    return a + b;            // add eax, ebx  ;  hlt
}
`,
				Bytes: []byte{
					0xB8, 0x0A, 0x00, 0x00, 0x00, // mov eax, 10
					0xBB, 0x20, 0x00, 0x00, 0x00, // mov ebx, 32
					0x01, 0xD8, // add eax, ebx
					0xF4, // hlt
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
    return i;                  // hlt
}
`,
				Bytes: []byte{
					0xB8, 0x00, 0x00, 0x00, 0x00, // mov eax, 0
					0xB9, 0x0A, 0x00, 0x00, 0x00, // mov ecx, 10
					0x40,       // inc eax
					0x39, 0xC8, // cmp eax, ecx
					0x75, 0xFB, // jne -5
					0xF4,
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
					0x58, // pop eax
					0xF4,
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
    return fn();               // call fn  ; hlt
}
`,
				Bytes: []byte{
					0xE8, 0x01, 0x00, 0x00, 0x00, // call +1
					0xF4,                         // hlt (skipped)
					0xB8, 0x99, 0x00, 0x00, 0x00, // mov eax, 0x99
					0xC3, // ret
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
    return x;                  // hlt
}
`,
				Bytes: []byte{
					0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
					0xCD, 0x80, // int 0x80
					0x40, // inc eax
					0xF4,
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
    return 0;                  // hlt
}
`,
				Bytes: []byte{
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90,
					0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90,
					0xF4,
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
    return 0;                              // hlt (nunca se alcanza)
}
`,
				Bytes: []byte{
					0xB8, 0xAD, 0xDE, 0x00, 0x00, // mov eax, 0xDEAD
					0xA3, 0x00, 0x00, 0x10, 0x00, // mov [0x100000], eax
					0xF4,
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
				},
			},
		},
	}
}
