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
			Procs:       []Proc{buildAdd()},
		},
		{
			Name:        "counter",
			Description: "Cuenta de 0 a 10 en EAX usando INC + CMP + JNE.",
			Procs:       []Proc{buildCounter()},
		},
		{
			Name:        "pushpop",
			Description: "Push/pop al stack del usuario. Demuestra ESP cambiando y memoria persistente.",
			Procs:       []Proc{buildPushpop()},
		},
		{
			Name:        "callret",
			Description: "Llama a una función que pone 0x99 en EAX y retorna.",
			Procs:       []Proc{buildCallret()},
		},
		{
			Name:        "soft_int",
			Description: "Ejecuta int 0x80 (syscall). HW entra a kernel, kernel hace IRET, vuelve a user.",
			Procs:       []Proc{buildSoftInt()},
		},
		{
			Name:        "timer_loop",
			Description: "NOPs interrumpidos por timer HW. Observe el flash USER→KERNEL→USER de la pill.",
			Procs:       []Proc{buildTimerLoop()},
		},
		{
			Name:        "hello",
			Description: "Escribe \"hola SO\\n\" con sys_write y termina con sys_exit. Demuestra el flow completo de syscall.",
			Procs:       []Proc{buildHello()},
		},
		{
			Name:        "readkey",
			Description: "Lee 1 byte del teclado y lo escribe a stdout, en loop. Bloquea con sys_read; el frontend le deja escribir para despertarlo.",
			Procs:       []Proc{buildReadkey()},
		},

		{
			Name:        "badmem",
			Description: "Intenta escribir a 0x100000 (memoria kernel). MMU detecta y dispara page fault.",
			Procs:       []Proc{buildBadmem()},
		},

		{
			Name:        "scheduler_demo",
			Description: "TRES procesos: 2 contadores + 1 reader. Cuando el reader hace sys_read sin teclas, va al WAITING. Los otros 2 siguen alternando. Escriba para despertarlo.",
			Procs:       buildSchedulerDemo(),
		},

		{
			Name:        "two_counters",
			Description: "Dos procesos: P1 incrementa EAX, P2 incrementa EBX. El scheduler RR los intercala con cada timer tick.",
			Procs:       buildTwoCounters(),
		},

		// ─── Concurrency demos ────────────────────────────────────────────
		// Shared counter at SharedCounterAddr() (= 0x3FFFE0), sitting 16 bytes
		// below the very top of physical memory. Slots 0..2 stacks top at
		// 0x33FFF0 / 0x37FFF0 / 0x3BFFF0, so the workers (loaded into those
		// slots) cannot clobber the counter via PUSH. Each worker iterates 200
		// times doing load → inc → store.
		{
			Name:        "race_counter",
			Category:    "concurrencia",
			Description: "Tres procesos: el setup pone el contador en 0, dos workers lo incrementan 200 veces cada uno SIN lock. El resultado final raramente es 400 porque el timer preempt el load/store y se pierden updates.",
			Procs: []Proc{
				buildRaceSetupZeroCounter(),
				buildRaceRacerA(),
				buildRaceRacerB(),
			},
		},

		{
			Name:        "safe_counter",
			Category:    "concurrencia",
			Description: "Mismo loop que race_counter, pero los workers piden un mutex (id=1) antes de modificar el contador. Resultado final SIEMPRE 400 porque el lock serializa la sección crítica.",
			Procs: []Proc{
				buildSafeSetupZeroAndMutex(),
				buildSafeLockerA(),
				buildSafeLockerB(),
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
			Cores:       3,
			Procs: []Proc{
				buildPCSetupTwoSems(),
				buildPCProducer(),
				buildPCConsumer(),
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
				setupThreeForks("setup_three_forks", `// Setup: tres tenedores (mutex 1, 2, 3) y exit.
int main(void) {
    mutex_create();  // fork 1
    mutex_create();  // fork 2
    mutex_create();  // fork 3
    exit(0);
}
`),
				philosopher("philo_0_naive", 0, 1, 2, "filo 0: izq=1, der=2"),
				philosopher("philo_1_naive", 1, 2, 3, "filo 1: izq=2, der=3"),
				philosopher("philo_2_naive", 2, 3, 1, "filo 2: izq=3, der=1 (cierra el ciclo → deadlock potencial)"),
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
				setupThreeForks("setup_three_forks_safe", `// Setup idéntico al naïve: tres mutexes y exit.
int main(void) {
    mutex_create();
    mutex_create();
    mutex_create();
    exit(0);
}
`),
				philosopher("philo_0_safe", 0, 1, 2, "filo 0: 1 → 2"),
				philosopher("philo_1_safe", 1, 2, 3, "filo 1: 2 → 3"),
				philosopher("philo_2_safe", 2, 1, 3, "filo 2: 1 → 3 (asimétrico, rompe el ciclo)"),
			},
		},

		// ─── Failure-pattern demos ────────────────────────────────────────
		{
			Name:        "stack_overflow",
			Category:    "fallos",
			Description: "Fuerza un stack overflow en pocos pasos: acerca ESP al borde inferior del user_stack y hace CALL recursivo infinito. El segundo CALL intenta escribir el return address en user_code (R-X) → page fault.",
			Procs:       []Proc{buildStackOverflow()},
		},

		{
			Name:        "syscall_kernel_pointer",
			Category:    "fallos",
			Description: "Pasa un puntero kernel a sys_write. Un kernel real debe rechazarlo con -EFAULT; esta demo existe para contrastar esa regla con la simplificación actual del simulador.",
			Procs:       []Proc{buildSyscallKernelPointer()},
		},

		{
			Name:        "unknown_syscall",
			Category:    "fallos",
			Description: "Invoca un número de syscall no implementado. El kernel devuelve 0xFFFFFFFF en EAX, equivalente pedagógico de un error tipo -ENOSYS.",
			Procs:       []Proc{buildUnknownSyscall()},
		},

		{
			Name:        "double_unlock",
			Category:    "fallos",
			Description: "Crea un mutex e intenta desbloquearlo sin ser holder. El kernel rechaza la operación con EAX=0xFFFFFFFF; útil para ver errores de sincronización que no son page faults.",
			Procs:       []Proc{buildDoubleUnlock()},
		},

		{
			Name:        "semaphore_leak",
			Category:    "fallos",
			Description: "Crea tres semáforos y termina sin usarlos ni liberarlos. No rompe el kernel, pero deja recursos vivos: una demo compacta de resource leak.",
			Procs:       []Proc{buildSemaphoreLeak()},
		},

		{
			Name:        "latency_quantum",
			Category:    "scheduler",
			Description: "Dos procesos CPU-bound y uno interactivo. Con quantum alto, el reader tarda más en volver a correr después de una tecla; con quantum bajo, la latencia baja pero suben los context switches.",
			Procs:       buildLatencyQuantum(),
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
				buildShowcaseMemwalk(),
				buildShowcaseReader(),
				buildShowcaseProducer(),
				buildShowcaseConsumerFault(),
			},
		},
		{
			Name:        "demo_fork",
			Category:    "ipc",
			Description: "El padre crea un hijo con fork(): el hijo recibe EAX=0 y el padre el PID del hijo. El padre espera con waitpid() hasta que el hijo termine, y cosecha su exit_code. Se carga junto a init (PID 1), que ancla el árbol.",
			Procs:       []Proc{buildDemoFork()},
		},

		{
			Name:        "demo_pipe",
			Category:    "ipc",
			Description: "El padre crea un pipe, escribe \"HOLA\", forkea un hijo, y espera. El hijo lee los 4 bytes del pipe y los manda a stdout. La consola muestra HOLA aunque ningún proceso lo haya tipeado en su .data — el dato cruzó un canal IPC.",
			Procs:       []Proc{buildDemoPipe()},
		},

		{
			Name:        "demo_vfork",
			Category:    "ipc",
			Description: "vfork() comparte la memoria del padre con el hijo y deja al padre BLOCKED hasta que el hijo termina. El hijo escribe 0xDEADBEEF en una dirección y muere; el padre, al despertar, lee esa misma dirección y ve el valor cambiado.",
			Procs:       []Proc{buildDemoVfork()},
		},

		{
			Name:        "threads_join",
			Category:    "concurrencia",
			Description: "clone(CLONE_VM|CLONE_FILES|CLONE_THREAD) crea un hilo hermano con memoria, file descriptors y TGID compartidos con el creador. El padre llama waitpid() para esperar que el hilo termine, lee su retval y sale. El hilo escribe en stdout antes de morir para que el flujo sea visible en la BITACORA.",
			Procs:       []Proc{buildThreadsJoin()},
		},

		{
			Name:        "threads_race",
			Category:    "concurrencia",
			Description: "Dos hilos hermanos comparten un contador en 0x3F0000 e incrementan 200 veces cada uno. Sin sincronizacion, el entrelazado de los context switches deja race conditions visibles: el contador final queda muy por debajo de 400. El exit code del proceso reporta el valor final.",
			Procs:       []Proc{buildThreadsRace()},
		},
		{
			Name:        "threads_safe",
			Category:    "concurrencia",
			Description: "Misma carrera de threads_race pero protegida por un mutex (INT 0x81). El leader crea el mutex y guarda el ID en 0x3F0010; cada hilo lock-ea antes del load-inc-store y unlock-ea despues. Resultado siempre = 400, sin importar el entrelazado.",
			Procs:       []Proc{buildThreadsSafe()},
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

// itoa is a tiny helper for source-listing strings the philosopher builder
// stitches together. Standard strconv works but pulling it in for one call
// is overkill.
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
