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
- Movimiento de datos: MOV r,imm32 ; MOV r,r ; MOV r,[mem] ; MOV [mem],r
- Pila: PUSH r ; PUSH imm32 ; POP r
- Aritmética: ADD r,r ; ADD r,imm32 ; SUB r,r ; SUB r,imm32 ; INC r ; DEC r
- Lógica: XOR r,r (cero común) ; CMP r,r ; CMP r,imm32
- Control de flujo: JMP rel ; JE rel ; JNE rel ; CALL rel ; RET
- Interrupciones: INT imm8 (transiciona a PL0 vía IDT) ; IRET (vuelve a PL3)
- Halt: HLT (suspende el core hasta la próxima interrupción)
No hay LEA, no hay direccionamientos indirectos complejos ni multiplicación/división.
La idea es que el assembly se lea como pseudocódigo, no como un puzzle.`,
		},
		{
			id:    "isa-registers",
			topic: "Registros disponibles",
			text: `El CPU expone 13 registros visibles:
- Generales: EAX, EBX, ECX, EDX, ESI, EDI (32 bits cada uno)
- Pila y marco: ESP (stack pointer), EBP (frame pointer)
- Control: EIP (instruction pointer), EFLAGS (banderas ZF, SF, CF, OF, IF)
- Segmento: CS, DS, SS (no se usan para segmentación real; quedan como vestigios para diagnóstico)
EFLAGS.IF habilita o deshabilita las interrupciones de hardware (timer, teclado). El estudiante puede
modificarlo con instrucciones privilegiadas que solo se ejecutan en PL0.
No hay registros XMM/YMM, ni MSR, ni TSC. La sincronización fina se logra con INT/IRET y el quantum del scheduler.`,
		},
		{
			id:    "isa-privilege",
			topic: "Modos de privilegio PL0 y PL3",
			text: `El CPU funciona en uno de dos niveles de privilegio:
- PL3 (modo usuario): donde corren los procesos. No pueden tocar memoria del kernel ni cambiar IF.
  Si intentan acceder a una dirección con permisos insuficientes el CPU dispara una excepción de página (vector 0x0E).
- PL0 (modo kernel): donde corre el código del scheduler, las rutinas de IDT y los handlers de syscalls.
La única forma legítima de pasar de PL3 a PL0 es vía interrupción (INT imm8) o porque el hardware disparó
un trap (timer, teclado, page fault, divide error). La única forma de volver es IRET. El CPU jamás cambia
de modo "por petición" del software corriendo en usuario — siempre lo orquesta el hardware, igual que en x86 real.
Esta restricción es el corazón de la separación kernel/usuario y se respeta estrictamente en el simulador.`,
		},
		{
			id:    "isa-idt",
			topic: "Tabla de descriptores de interrupciones (IDT)",
			text: `La IDT tiene 5 vectores activos:
- 0x00 — Divide error (no implementado realmente; reservado para futuras demos)
- 0x0E — Page fault: dispara cuando un proceso accede a memoria con permisos insuficientes (badmem demo).
- 0x20 — Timer: dispara cada N pasos según el quantum del scheduler. Es el motor del preemptive scheduling.
- 0x21 — Keyboard: dispara cuando hay una tecla en el buffer (demo readkey).
- 0x80 — System call: el contrato Linux i386 estándar. EAX = nº de syscall, EBX/ECX/EDX = args.
Cada vector apunta a un handler en PL0. El CPU empuja EFLAGS, CS, EIP en la pila del kernel al entrar y
los restaura con IRET. Los handlers están en pkg/kernel/ y son legibles como el resto del simulador.`,
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
mostrar ejemplos reales de man pages (syscalls(2)) y de código que un estudiante puede compilar afuera.
El kernel valida los punteros que pasa el usuario; pasar un puntero a memoria PL0 dispara una excepción
de página antes de que el handler haga nada (demo syscall_kernel_pointer).`,
		},
		{
			id:    "isa-memory-map",
			topic: "Mapa de memoria de 4 MB",
			text: `La memoria simulada es de 4 MB y está dividida en regiones con permisos diferenciados:
- Kernel code (PL0, read+exec): los handlers de IDT y el scheduler.
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
