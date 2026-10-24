package rag

// LoadISAAsRawDocs returns curated chunks describing the modos-cpu ISA
// subset. The content is hand-written rather than parsed from README.md so
// pedagogical wording stays tight and the chunks are sized for one concept
// each — the retriever performs best when each unit is self-contained.
//
// When the README or simulator semantics change, edit the entries below
// rather than re-deriving from prose; it keeps the corpus consistent across
// reindex runs.
func LoadISAAsRawDocs() []RawDoc {
	entries := []struct {
		id    string
		topic string
		text  string
	}{
		{
			id:    "isa-overview",
			topic: "Resumen del ISA simplificado",
			text: `modos-cpu implementa un subset simplificado de x86 de 32 bits con propósito pedagógico.
Comparte la semántica esencial de la arquitectura real (registros, flags, modo protegido vs usuario, IDT,
syscalls Linux i386) pero deja por fuera modos legacy (real mode, virtual 8086), paginación completa,
SSE/MMX, microcódigo y la mayoría de prefijos. El objetivo es que el estudiante pueda razonar acerca
de cómo un CPU real arranca un proceso, atiende interrupciones y vuelve al kernel sin lidiar con
miles de opcodes. Todo el ISA cabe en aproximadamente 25 instrucciones y 13 registros.`,
		},
		{
			id:    "isa-opcodes",
			topic: "Opcodes soportados",
			text: `El ISA admite las siguientes familias de instrucciones:
- Movimiento de datos: MOV r,imm32 ; MOV r,r ; MOV EAX,[dir] ; MOV [dir],EAX (solo direcciones absolutas y solo con EAX)
- Pila: PUSH r ; POP r
- Aritmética: ADD r,r ; ADD r,imm32 ; SUB r,r ; SUB r,imm32 ; INC r ; DEC r
- Lógica y comparación: XOR r,r ; XOR r,imm32 ; AND r,imm32 ; CMP r,r ; CMP r,imm32
- Control de flujo: JMP rel ; la familia completa de saltos condicionales Jcc (JE, JNE, JG, JGE, JL, JLE, JA, JB, etc.) ; CALL rel ; RET ; NOP
- Interrupciones: INT imm8 (entra a PL0 vía IDT) ; IRET (vuelve al nivel guardado) ; CLI y STI (privilegiadas)
- Halt: HLT (privilegiada; suspende el core hasta la próxima interrupción)
No hay LEA, ni direccionamiento con desplazamientos o SIB, ni multiplicación ni división.
La idea es que el assembly se lea como pseudocódigo, no como un puzzle.`,
		},
		{
			id:    "isa-registers",
			topic: "Registros disponibles",
			text: `El CPU expone 13 registros visibles:
- Generales: EAX, EBX, ECX, EDX, ESI, EDI (32 bits cada uno)
- Pila y marco: ESP (stack pointer), EBP (frame pointer)
- Control: EIP (instruction pointer), EFLAGS (banderas CF, PF, ZF, SF, IF, OF)
- Segmento: CS, DS, SS (no se usan para segmentación real; quedan como vestigios para diagnóstico)
EFLAGS.IF habilita o deshabilita las interrupciones de hardware (timer, teclado). Se modifica con CLI y STI,
que son privilegiadas: ejecutarlas en PL3 dispara una General Protection (#GP, vector 0x0D).
No hay registros XMM/YMM, ni MSR, ni TSC. La sincronización fina se logra con INT/IRET y el quantum del scheduler.`,
		},
		{
			id:    "isa-privilege",
			topic: "Modos de privilegio PL0 y PL3",
			text: `El CPU funciona en uno de dos niveles de privilegio:
- PL3 (modo usuario): donde corren los procesos. No pueden tocar memoria del kernel ni cambiar IF.
  Si intentan acceder a una dirección con permisos insuficientes el CPU dispara una excepción de página (vector 0x0E).
- PL0 (modo kernel): donde corren los handlers de la IDT. En este simulador esos handlers son stubs mínimos (nop; iret) y la lógica del kernel (scheduler, syscalls, sincronización) se ejecuta en el host, escrita en Go, mientras el CPU está en PL0.
La única forma legítima de pasar de PL3 a PL0 es vía interrupción (INT imm8) o porque el hardware disparó
un trap o una excepción (timer, teclado, page fault, general protection). La única forma de volver es IRET. El CPU jamás cambia
de modo "por petición" del software corriendo en usuario: siempre lo orquesta el hardware. En x86 real también existen SYSCALL/SYSENTER y call gates, que este simulador no modela.
Esta restricción es el corazón de la separación kernel/usuario y se respeta estrictamente en el simulador.`,
		},
		{
			id:    "isa-idt",
			topic: "Tabla de descriptores de interrupciones (IDT)",
			text: `La IDT tiene 7 vectores instalados:
- 0x00 — Divide error: el handler está instalado, pero el ISA no tiene instrucciones de división, así que nunca se dispara.
- 0x0D — General protection (#GP): HLT, CLI o STI ejecutadas en PL3, o INT hacia un vector no permitido para usuario.
- 0x0E — Page fault: dispara cuando un proceso accede a memoria con permisos insuficientes (demo badmem). En el simulador siempre es fatal.
- 0x20 — Timer: dispara cada N pasos según el quantum del scheduler. Es el motor del preemptive scheduling.
- 0x21 — Keyboard: dispara cuando hay una tecla en el buffer (demo readkey).
- 0x80 — System call: el contrato Linux i386 estándar. EAX = nº de syscall, EBX/ECX/EDX = args.
- 0x81 — Mutex y semáforos: convención propia del simulador, no de Linux. EAX elige la operación (0x10 mutex_create, 0x11 mutex_lock, 0x12 mutex_unlock, 0x20 sem_create, 0x21 sem_wait, 0x22 sem_signal) y EBX el id.
Cada vector apunta a un handler en PL0. Al entrar, el CPU empuja EFLAGS, CS y EIP (y además SS:ESP de usuario si viene de PL3) y
los restaura con IRET. La lógica de cada handler está en pkg/kernel/, escrita en Go.`,
		},
		{
			id:    "isa-syscalls",
			topic: "Convención de system calls",
			text: `Las syscalls siguen la convención clásica de Linux i386:
- EAX: número de syscall (1 = sys_exit, 4 = sys_write, ...)
- EBX, ECX, EDX: hasta tres argumentos enteros
- INT 0x80 dispara la llamada
- El retorno deja el valor en EAX y vuelve al espacio de usuario con IRET
Esta convención permite que código Linux clásico se vea casi idéntico en modos-cpu, lo cual facilita
mostrar ejemplos que siguen la convención de las man pages (syscalls(2)); los programas del simulador están escritos a mano.
El kernel valida los punteros que pasa el usuario (equivalente a access_ok en Linux): si el buffer apunta a memoria
del kernel, la syscall devuelve -1 en EAX y el simulador registra un evento de falla de página para que se vea en la
bitácora. A diferencia de un page fault del CPU, el proceso no muere (demo syscall_kernel_pointer). Linux real devolvería -EFAULT.`,
		},
		{
			id:    "isa-memory-map",
			topic: "Mapa de memoria de 4 MB",
			text: `La memoria simulada es de 4 MB y está dividida en regiones con permisos diferenciados:
- Kernel code (PL0, read+exec): los stubs de los handlers de la IDT. El scheduler y el resto de la lógica del kernel corren en el host (Go).
- Kernel data (PL0, read+write): tablas de procesos, IDT, GDT mínima.
- User code (PL3, read+exec): donde el kernel carga los bytes de cada Proc.
- User data/stack (PL3, read+write): pila y heap por proceso.
- I/O virtual: regiones reservadas (no MMIO real). Las "lecturas de teclado" se entregan vía interrupción, no por leer una dirección.
No hay paginación real ni MMU completa: el CPU valida en cada acceso que el modo actual (PL0/PL3) tenga
permisos para esa región. Esto basta para enseñar la diferencia entre user/kernel space y por qué un proceso
no puede leer la TCB de otro, sin entrar en page tables ni TLB.`,
		},
		{
			id:    "isa-scheduler",
			topic: "Scheduler Round Robin con quantum",
			text: `El scheduler usa Round Robin con quantum configurable. En cada interrupción de timer (vector 0x20)
el kernel saca el proceso actual al final de la cola READY y carga el siguiente. El estado guardado por
proceso es el contexto completo (EAX-EDI, ESP, EBP, EIP, EFLAGS) en una TCB en kernel data.
El estudiante puede observar el context switch en vivo (demos two_counters, scheduler_demo). Las demos de
concurrencia muestran por qué incrementar una variable compartida sin protección rompe la lógica
(race_counter), cómo arreglarlo con un mutex (safe_counter) y los riesgos del orden de adquisición
(philos_deadlock, philos_safe).`,
		},
	}
	out := make([]RawDoc, 0, len(entries))
	for _, e := range entries {
		out = append(out, RawDoc{
			Source:   SourceISA,
			SourceID: e.id,
			Text:     e.text,
			Metadata: map[string]any{"topic": e.topic},
		})
	}
	return out
}
