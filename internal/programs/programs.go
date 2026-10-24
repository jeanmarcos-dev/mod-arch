// Package programs holds the embedded x86 demo binaries.
//
// Each Demo is made up of one or more Procs. Single-process demos are the
// common case; multi-process demos illustrate context switching.
package programs

// Difficulty rates the conceptual load a demo puts on the alumno. The picker
// surfaces it as a colored badge so the catalogue can be skimmed by level
// before reading any description.
type Difficulty string

const (
	DifficultyBasic        Difficulty = "basic"
	DifficultyIntermediate Difficulty = "intermediate"
	DifficultyAdvanced     Difficulty = "advanced"
)

// Demo is a scenario loaded into the simulator. It has one or more processes.
type Demo struct {
	Name        string
	Description string
	Procs       []Proc
	// Category groups demos in the UI. Must be one of the Category* slugs
	// declared in categories.go. Empty means "uncategorized" and the demo
	// appears under a fallback section in the picker.
	Category string
	// Cores requests a specific number of active CPU cores when this demo is
	// loaded. Zero (the default) leaves the current kernel layout untouched,
	// preserving single-core behavior for legacy demos. Values are clamped to
	// [1, kernel.MaxCores] by sim.SetNumCores.
	Cores int
	// Difficulty drives the colored badge in the picker. Empty falls back to
	// "basic" client-side; declare explicitly to avoid surprises.
	Difficulty Difficulty
	// Topics are short slugs (e.g. "mutex", "race-condition") the picker
	// indexes for search and tooltips. Optional but recommended.
	Topics []string
	// SyscallsUsed lists the syscall names a demo exercises (e.g. "fork",
	// "waitpid"). Surfaced as a tooltip line so the alumno can preview the
	// surface area before loading.
	SyscallsUsed []string
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
//
// Globals, Locals and Funcs are optional variable metadata that the VARS
// panel uses to display live values for each declared C variable. Empty
// slices mean "no variables declared" and the panel renders nothing.
type Proc struct {
	Name      string
	Bytes     []byte
	Source    string
	SourceMap []SourceLine
	Globals   []Global
	Locals    []Local
	Funcs     []Func
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

// VarType, VarLocKind, VarLoc, Global, Local and Func mirror the types in
// internal/programs/asm. We redeclare them so this package — which the
// simulator and snapshot builders depend on — does not need to import the
// asm DSL package. dslProc() copies the values across the boundary.
type VarType string

const (
	VarInt  VarType = "int"
	VarUint VarType = "uint"
	VarPtr  VarType = "ptr"
	VarChar VarType = "char"
)

type VarLocKind string

const (
	VarLocMem   VarLocKind = "mem"
	VarLocReg   VarLocKind = "reg"
	VarLocStack VarLocKind = "stack"
)

type VarLoc struct {
	Kind   VarLocKind
	Addr   uint32
	Reg    string
	Offset int32
}

type Global struct {
	Name string
	Type VarType
	Addr uint32
	Len  uint32
}

type Local struct {
	Name string
	Type VarType
	Loc  VarLoc
	Func string
}

type Func struct {
	Name        string
	StartOffset uint32
	EndOffset   uint32
}

// Demos returns the embedded demo scenarios.
func Demos() []Demo {
	return []Demo{
		// ─── Fundamentos CPU ────────────────────────────────────────────
		{
			Name:         "add",
			Description:  "Suma simple: 10 + 32 → EAX. Demuestra MOV imm32 + ADD r,r.",
			Category:     CategoryFundamentosCPU,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"mov", "add", "registers"},
			SyscallsUsed: []string{"sys_exit"},
			Procs:        []Proc{buildAdd()},
		},
		{
			Name:         "counter",
			Description:  "Cuenta de 0 a 10 en EAX usando INC + CMP + JNE.",
			Category:     CategoryFundamentosCPU,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"loops", "inc", "cmp", "jne"},
			SyscallsUsed: []string{"sys_exit"},
			Procs:        []Proc{buildCounter()},
		},
		{
			Name:         "pushpop",
			Description:  "Push/pop al stack del usuario. Demuestra ESP cambiando y memoria persistente.",
			Category:     CategoryFundamentosCPU,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"stack", "push", "pop", "esp"},
			SyscallsUsed: []string{"sys_exit"},
			Procs:        []Proc{buildPushpop()},
		},
		{
			Name:         "callret",
			Description:  "Llama a una función que pone 0x99 en EAX y retorna.",
			Category:     CategoryFundamentosCPU,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"call", "ret", "return-address", "stack-frame"},
			SyscallsUsed: []string{"sys_exit"},
			Procs:        []Proc{buildCallret()},
		},

		// ─── Modo dual y syscalls ───────────────────────────────────────
		{
			Name:         "soft_int",
			Description:  "Ejecuta int 0x80 (syscall). HW entra a kernel, kernel hace IRET, vuelve a user.",
			Category:     CategoryModoDual,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"mode-switch", "int-0x80", "iret"},
			SyscallsUsed: []string{"sys_exit"},
			Procs:        []Proc{buildSoftInt()},
		},
		{
			Name:         "hello",
			Description:  "Escribe \"hola SO\\n\" con sys_write y termina con sys_exit. Demuestra el flow completo de syscall.",
			Category:     CategoryModoDual,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"sys_write", "stdout", "syscall-abi"},
			SyscallsUsed: []string{"sys_write", "sys_exit"},
			Procs:        []Proc{buildHello()},
		},
		{
			Name:         "readkey",
			Description:  "Lee 1 byte del teclado y lo escribe a stdout, en loop. Bloquea con sys_read; el frontend le deja escribir para despertarlo.",
			Category:     CategoryModoDual,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"sys_read", "blocking-io", "keyboard-irq"},
			SyscallsUsed: []string{"sys_read", "sys_write"},
			Procs:        []Proc{buildReadkey()},
		},
		{
			Name:         "idle_demo",
			Description:  "Bloquea en sys_read y termina. Compare lado a lado con la tarea ociosa apagada (autostop) y encendida (PID 0 ocupa el núcleo en HLT).",
			Category:     CategoryScheduling,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"idle-task", "swapper", "blocking-io", "halt-loop"},
			SyscallsUsed: []string{"sys_read", "sys_exit"},
			Procs:        []Proc{buildIdleDemo()},
		},

		// ─── Scheduling ────────────────────────────────────────────────
		{
			Name:         "timer_loop",
			Description:  "NOPs interrumpidos por timer HW. Observe el flash USER→KERNEL→USER de la pill.",
			Category:     CategoryScheduling,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"timer-irq", "preemption", "nop"},
			SyscallsUsed: []string{"sys_exit"},
			Procs:        []Proc{buildTimerLoop()},
		},
		{
			Name:         "two_counters",
			Description:  "Dos procesos: P1 incrementa EAX, P2 incrementa EBX. El scheduler RR los intercala con cada timer tick.",
			Category:     CategoryScheduling,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"round-robin", "timer-preemption", "context-switch"},
			SyscallsUsed: []string{},
			Procs:        buildTwoCounters(),
		},
		{
			Name:         "scheduler_demo",
			Description:  "TRES procesos: 2 contadores + 1 reader. Cuando el reader hace sys_read sin teclas, pasa a BLOCKED. Los otros 2 siguen alternando. Escriba para despertarlo.",
			Category:     CategoryScheduling,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"round-robin", "blocking-io", "wait-queue"},
			SyscallsUsed: []string{"sys_read", "sys_write"},
			Procs:        buildSchedulerDemo(),
		},
		{
			Name:         "latency_quantum",
			Description:  "Dos procesos CPU-bound y uno interactivo. Con quantum alto, el reader tarda más en volver a correr después de una tecla; con quantum bajo, la latencia baja pero suben los context switches.",
			Category:     CategoryScheduling,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"quantum", "latency", "interactive", "tradeoff"},
			SyscallsUsed: []string{"sys_read", "sys_write"},
			Procs:        buildLatencyQuantum(),
		},

		// ─── Procesos e IPC ────────────────────────────────────────────
		{
			Name:         "demo_fork",
			Description:  "El padre crea un hijo con fork(): el hijo recibe EAX=0 y el padre el PID del hijo. El padre espera con waitpid() hasta que el hijo termine, y cosecha su exit_code. Se carga junto a init (PID 1), que ancla el árbol.",
			Category:     CategoryProcesosIPC,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"fork", "waitpid", "exit-code", "process-tree"},
			SyscallsUsed: []string{"sys_fork", "sys_waitpid", "sys_exit"},
			Procs:        []Proc{buildDemoFork()},
		},
		{
			Name:         "demo_vfork",
			Description:  "vfork() comparte la memoria del padre con el hijo y deja al padre BLOCKED hasta que el hijo termina. El hijo escribe 0xDEADBEEF en una dirección y muere; el padre, al despertar, lee esa misma dirección y ve el valor cambiado.",
			Category:     CategoryProcesosIPC,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"vfork", "shared-memory", "copy-on-write"},
			SyscallsUsed: []string{"sys_vfork", "sys_exit"},
			Procs:        []Proc{buildDemoVfork()},
		},
		{
			Name:         "demo_pipe",
			Description:  "El padre crea un pipe, escribe \"HOLA\", forkea un hijo, y espera. El hijo lee los 4 bytes del pipe y los manda a stdout. La consola muestra HOLA aunque ningún proceso lo haya tipeado en su .data — el dato cruzó un canal IPC.",
			Category:     CategoryProcesosIPC,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"pipe", "fork", "fd-inheritance", "ipc"},
			SyscallsUsed: []string{"sys_pipe", "sys_fork", "sys_write", "sys_read", "sys_waitpid", "sys_exit"},
			Procs:        []Proc{buildDemoPipe()},
		},

		// ─── Hilos ─────────────────────────────────────────────────────
		{
			Name:         "threads_join",
			Description:  "clone(CLONE_VM|CLONE_FILES|CLONE_THREAD) crea un hilo hermano con memoria, file descriptors y TGID compartidos con el creador. El padre llama waitpid() para esperar que el hilo termine, lee su retval y sale. El hilo escribe en stdout antes de morir para que el flujo sea visible en la BITACORA.",
			Category:     CategoryHilos,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"clone", "thread-join", "retval", "tgid"},
			SyscallsUsed: []string{"sys_clone", "sys_waitpid", "sys_write", "sys_exit"},
			Procs:        []Proc{buildThreadsJoin()},
		},
		{
			Name:         "threads_race",
			Description:  "Dos hilos hermanos comparten un contador en 0x3F0000 e incrementan 200 veces cada uno. Sin sincronizacion, el entrelazado de los context switches deja race conditions visibles: el contador final queda muy por debajo de 400. El exit code del proceso reporta el valor final.",
			Category:     CategoryHilos,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"race-condition", "load-inc-store", "atomicity"},
			SyscallsUsed: []string{"sys_clone", "sys_waitpid", "sys_exit"},
			Procs:        []Proc{buildThreadsRace()},
		},
		{
			Name:         "threads_safe",
			Description:  "Misma carrera de threads_race pero protegida por un mutex (INT 0x81). El leader crea el mutex y guarda el ID en 0x3F0010; cada hilo lock-ea antes del load-inc-store y unlock-ea despues. Resultado siempre = 400, sin importar el entrelazado.",
			Category:     CategoryHilos,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"mutex", "critical-section", "atomic-increment"},
			SyscallsUsed: []string{"sys_clone", "sys_waitpid", "mutex_create", "mutex_lock", "mutex_unlock", "sys_exit"},
			Procs:        []Proc{buildThreadsSafe()},
		},

		// ─── Sincronización ────────────────────────────────────────────
		// Shared counter at SharedCounterAddr() (= 0x3FFFE0), sitting 16 bytes
		// below the very top of physical memory. Slots 0..2 stacks top at
		// 0x33FFF0 / 0x37FFF0 / 0x3BFFF0, so the workers (loaded into those
		// slots) cannot clobber the counter via PUSH. Each worker iterates 200
		// times doing load → inc → store.
		{
			Name:         "race_counter",
			Description:  "Tres procesos: el setup pone el contador en 0, dos workers lo incrementan 200 veces cada uno SIN lock. El resultado final puede quedar por debajo de 400 cuando el timer interrumpe entre el load y el store; cuántos updates se pierden depende del quantum. En el simulador los workers comparten un único espacio plano de memoria de usuario; en un SO real eso equivaldría a memoria compartida (shm/mmap) o a hilos.",
			Category:     CategorySincronizacion,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"race-condition", "load-inc-store", "lost-updates"},
			SyscallsUsed: []string{"sys_exit"},
			Procs: []Proc{
				buildRaceSetupZeroCounter(),
				buildRaceRacerA(),
				buildRaceRacerB(),
			},
		},
		{
			Name:         "safe_counter",
			Description:  "Mismo loop que race_counter, pero los workers piden un mutex (id=1) antes de modificar el contador. Resultado final SIEMPRE 400 porque el lock serializa la sección crítica. Igual que en race_counter, los workers comparten un espacio plano de memoria de usuario para la demo.",
			Category:     CategorySincronizacion,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"mutex", "critical-section", "serialization"},
			SyscallsUsed: []string{"mutex_create", "mutex_lock", "mutex_unlock", "sys_exit"},
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
			Name:         "producer_consumer",
			Description:  "Productor y consumidor con un buffer de un slot, sincronizados por dos semáforos. Demuestra el patrón sem_wait/sem_signal sin necesidad de mutex porque solo hay un productor y un consumidor. La demo solo muestra la sincronización: no transfiere datos reales por el buffer.",
			Category:     CategorySincronizacion,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"semaphores", "producer-consumer", "bounded-buffer"},
			SyscallsUsed: []string{"sem_create", "sem_wait", "sem_signal", "sys_exit"},
			Cores:        3,
			Procs: []Proc{
				buildPCSetupTwoSems(),
				buildPCProducer(),
				buildPCConsumer(),
			},
		},
		// dining_philosophers_deadlock: 3 philos seated at a round table with
		// 3 forks (mutex 1, 2, 3). Each philo picks LEFT then RIGHT, in the
		// same order: 0→(1,2), 1→(2,3), 2→(3,1). Classic circular wait.
		{
			Name:         "philos_deadlock",
			Description:  "Cena de los filósofos en versión naïve (3 comensales). Cada uno toma su tenedor IZQUIERDO primero, luego el DERECHO. Si todos consiguen su izquierdo simultáneamente, nadie puede tomar el derecho → deadlock visible en la cola de los mutexes.",
			Category:     CategorySincronizacion,
			Difficulty:   DifficultyAdvanced,
			Topics:       []string{"deadlock", "dining-philosophers", "circular-wait"},
			SyscallsUsed: []string{"mutex_create", "mutex_lock", "mutex_unlock", "sys_exit"},
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
		// philos_safe: same three philos but philo 2 inverts its lock order.
		// Breaking the symmetric "always-left-first" rule removes the cycle.
		{
			Name:         "philos_safe",
			Description:  "Misma cena de filósofos pero el último comensal invierte su orden de toma de tenedores. La asimetría rompe el ciclo de espera → no hay deadlock, el sistema progresa.",
			Category:     CategorySincronizacion,
			Difficulty:   DifficultyAdvanced,
			Topics:       []string{"deadlock-prevention", "dining-philosophers", "lock-ordering"},
			SyscallsUsed: []string{"mutex_create", "mutex_lock", "mutex_unlock", "sys_exit"},
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

		// ─── Fallos y excepciones ──────────────────────────────────────
		{
			Name:         "badmem",
			Description:  "Intenta escribir a 0x100000 (memoria kernel). MMU detecta y dispara page fault.",
			Category:     CategoryFallosExcepciones,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"page-fault", "mmu", "kernel-region"},
			SyscallsUsed: []string{},
			Procs:        []Proc{buildBadmem()},
		},
		{
			Name:         "stack_overflow",
			Description:  "Fuerza un stack overflow en pocos pasos: acerca ESP al borde inferior del user_stack y hace CALL recursivo infinito. El segundo CALL intenta escribir el return address en user_code (R-X) → page fault.",
			Category:     CategoryFallosExcepciones,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"stack-overflow", "page-fault", "recursion"},
			SyscallsUsed: []string{},
			Procs:        []Proc{buildStackOverflow()},
		},
		{
			Name:         "syscall_kernel_pointer",
			Description:  "Pasa un puntero kernel a sys_write. El kernel valida el buffer (como access_ok en Linux): la syscall devuelve -1 en EAX, la bitácora muestra el intento como falla de página y el proceso sigue hasta exit. Linux real devolvería -EFAULT.",
			Category:     CategoryFallosExcepciones,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"invalid-pointer", "copy-from-user", "efault"},
			SyscallsUsed: []string{"sys_write", "sys_exit"},
			Procs:        []Proc{buildSyscallKernelPointer()},
		},
		{
			Name:         "unknown_syscall",
			Description:  "Invoca un número de syscall no implementado. El kernel devuelve 0xFFFFFFFF en EAX, equivalente pedagógico de un error tipo -ENOSYS.",
			Category:     CategoryFallosExcepciones,
			Difficulty:   DifficultyBasic,
			Topics:       []string{"enosys", "syscall-dispatch"},
			SyscallsUsed: []string{"sys_exit"},
			Procs:        []Proc{buildUnknownSyscall()},
		},
		{
			Name:         "double_unlock",
			Description:  "Crea un mutex e intenta desbloquearlo sin ser holder. El kernel rechaza la operación con EAX=0xFFFFFFFF; útil para ver errores de sincronización que no son page faults.",
			Category:     CategoryFallosExcepciones,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"mutex", "ownership", "sync-error"},
			SyscallsUsed: []string{"mutex_create", "mutex_unlock", "sys_exit"},
			Procs:        []Proc{buildDoubleUnlock()},
		},
		{
			Name:         "semaphore_leak",
			Description:  "Crea tres semáforos y termina sin usarlos ni liberarlos. No rompe el kernel, pero deja recursos vivos: una demo compacta de resource leak.",
			Category:     CategoryFallosExcepciones,
			Difficulty:   DifficultyIntermediate,
			Topics:       []string{"resource-leak", "semaphores", "cleanup"},
			SyscallsUsed: []string{"sem_create", "sys_exit"},
			Procs:        []Proc{buildSemaphoreLeak()},
		},

		// ─── Showcase ──────────────────────────────────────────────────
		// Single demo that exercises every CPU-driven animation in one run.
		// Four processes auto-distributed one per core: memwalk walks shared
		// memory, reader blocks on keyboard, producer creates a sem and signals,
		// consumer waits the sem then page-faults in kernel space.
		{
			Name:         "showcase",
			Description:  "Paseo completo en 4 cores. Toca byte flights, heat de memoria, semáforos, sys_read bloqueante y page fault — exhibe casi todas las animaciones del simulador en una sola corrida. Apriete una tecla en la consola para desbloquear al reader (C1). HandRaiseBadge, ConnectionBanner y el glosario inline se prueban aparte (no son disparables desde un programa).",
			Category:     CategoryShowcase,
			Difficulty:   DifficultyAdvanced,
			Topics:       []string{"multicore", "integration", "page-fault", "semaphores", "blocking-io"},
			SyscallsUsed: []string{"sys_read", "sys_write", "sem_create", "sem_wait", "sem_signal", "sys_exit"},
			Cores:        4,
			Procs: []Proc{
				buildShowcaseMemwalk(),
				buildShowcaseReader(),
				buildShowcaseProducer(),
				buildShowcaseConsumerFault(),
			},
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
