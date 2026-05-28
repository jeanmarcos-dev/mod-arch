package programs

import (
	"github.com/jeanmarcos/modos-cpu/internal/programs/asm"
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// threadFnAbsoluteAddr is the EIP passed to sys_clone in the thread demos:
// AddrUserBase (0x200000) plus the local threadFnOffset (0x80) where the
// inline thread_fn body sits inside the leader's code segment.
const threadFnAbsoluteAddr uint32 = cpu.AddrUserBase + threadFnOffset

// ─── threads_race ──────────────────────────────────────────────────────────

// buildThreadsRace: leader spawns two siblings that race on a shared counter
// without synchronisation. Expected result is < 400 because INC is not atomic.
func buildThreadsRace() Proc {
	b := asm.NewProc("thread_race").Source(`// Race condition clasica: load-inc-store no atomico bajo timer
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
`)

	counterAddr := ThreadCounterAddr()

	// main: zero counter, clone twice, wait twice, exit with final value
	b.At(7).Comment("counter = 0").MovImm(asm.EAX, 0)
	b.At(7).MovEAXMem(counterAddr)
	b.At(8).Comment("SysClone").MovImm(asm.EAX, kernel.SysClone)
	b.At(8).MovImm(asm.EBX, cloneVMFilesThread)
	b.At(8).MovImm(asm.ECX, 0)
	b.At(8).MovImm(asm.EDX, threadFnAbsoluteAddr)
	b.At(8).Comment("spawn t1").Int(kernel.IRQSyscall)
	b.At(9).MovImm(asm.EAX, kernel.SysClone)
	b.At(9).MovImm(asm.EBX, cloneVMFilesThread)
	b.At(9).MovImm(asm.ECX, 0)
	b.At(9).MovImm(asm.EDX, threadFnAbsoluteAddr)
	b.At(9).Comment("spawn t2").Int(kernel.IRQSyscall)
	b.At(10).Comment("SysWaitpid").MovImm(asm.EAX, kernel.SysWaitpid)
	b.At(10).Comment("any child").MovImm(asm.EBX, 0xFFFFFFFF)
	b.At(10).MovImm(asm.ECX, 0)
	b.At(10).MovImm(asm.EDX, 0)
	b.At(10).Comment("reap #1").Int(kernel.IRQSyscall)
	b.At(11).MovImm(asm.EAX, kernel.SysWaitpid)
	b.At(11).MovImm(asm.EBX, 0xFFFFFFFF)
	b.At(11).MovImm(asm.ECX, 0)
	b.At(11).MovImm(asm.EDX, 0)
	b.At(11).Comment("reap #2").Int(kernel.IRQSyscall)
	b.At(12).MovMemEAX(counterAddr)
	b.At(12).Comment("exit = final counter").MovReg(asm.EBX, asm.EAX)
	b.At(12).Comment("SysExit").MovImm(asm.EAX, kernel.SysExit)
	b.At(12).Int(kernel.IRQSyscall)

	// thread_fn at offset 0x80
	b.PadTo(threadFnOffset)
	b.At(16).Comment("i = 0").MovImm(asm.EBX, 0)
	b.At(17).Comment("LOAD").Label("loop_start").MovMemEAX(counterAddr)
	b.At(18).Comment("+1").Inc(asm.EAX)
	b.At(19).Comment("STORE (race window)").MovEAXMem(counterAddr)
	b.At(16).Comment("i++").Inc(asm.EBX)
	b.At(16).CmpImm(asm.EBX, 200)
	b.At(16).Jne("loop_start")
	b.At(22).Comment("SysExit").MovImm(asm.EAX, kernel.SysExit)
	b.At(22).MovImm(asm.EBX, 0)
	b.At(22).Int(kernel.IRQSyscall)

	p := dslProc(b)
	// The literal renders the loop_start MOV / STORE mnemonics with bracketed
	// addresses written as " ; LOAD" / " ; STORE (race window)" — already
	// handled. The literal's JNE mnemonic is "JNE loop_start" not "JNE -20";
	// the DSL builder produces "JNE loop_start" via .Jne("loop_start"). Match.
	return p
}

// ─── threads_safe ──────────────────────────────────────────────────────────

// buildThreadsSafe: same race protected with a mutex. Final counter is 400.
func buildThreadsSafe() Proc {
	b := asm.NewProc("thread_safe").Source(`// Race controlada por mutex_lock/unlock (INT 0x81 ops 0x11/0x12).
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
`)

	counterAddr := ThreadCounterAddr()
	const mutexIDAddr uint32 = 0x3F0010

	// main at 0x00
	b.At(8).Comment("counter = 0").MovImm(asm.EAX, 0)
	b.At(8).MovEAXMem(counterAddr)
	b.At(9).Comment("SyncMutexCreate").MovImm(asm.EAX, kernel.SyncMutexCreate)
	b.At(9).Int(kernel.IRQSync)
	b.At(9).Comment("mutex_id").MovEAXMem(mutexIDAddr)
	b.At(10).Comment("SysClone").MovImm(asm.EAX, kernel.SysClone)
	b.At(10).MovImm(asm.EBX, cloneVMFilesThread)
	b.At(10).MovImm(asm.ECX, 0)
	b.At(10).MovImm(asm.EDX, threadFnAbsoluteAddr)
	b.At(10).Comment("spawn t1").Int(kernel.IRQSyscall)
	b.At(11).MovImm(asm.EAX, kernel.SysClone)
	b.At(11).MovImm(asm.EBX, cloneVMFilesThread)
	b.At(11).MovImm(asm.ECX, 0)
	b.At(11).MovImm(asm.EDX, threadFnAbsoluteAddr)
	b.At(11).Comment("spawn t2").Int(kernel.IRQSyscall)
	b.At(12).Comment("SysWaitpid").MovImm(asm.EAX, kernel.SysWaitpid)
	b.At(12).MovImm(asm.EBX, 0xFFFFFFFF)
	b.At(12).MovImm(asm.ECX, 0)
	b.At(12).MovImm(asm.EDX, 0)
	b.At(12).Comment("reap #1").Int(kernel.IRQSyscall)
	b.At(13).MovImm(asm.EAX, kernel.SysWaitpid)
	b.At(13).MovImm(asm.EBX, 0xFFFFFFFF)
	b.At(13).MovImm(asm.ECX, 0)
	b.At(13).MovImm(asm.EDX, 0)
	b.At(13).Comment("reap #2").Int(kernel.IRQSyscall)
	b.At(14).MovMemEAX(counterAddr)
	b.At(14).MovReg(asm.EBX, asm.EAX)
	b.At(14).Comment("SysExit").MovImm(asm.EAX, kernel.SysExit)
	b.At(14).Int(kernel.IRQSyscall)

	// thread_fn at 0x80
	b.PadTo(threadFnOffset)
	b.At(18).Comment("i = 0").MovImm(asm.ECX, 0)
	b.At(19).Comment("mutex_id").Label("loop_start").MovMemEAX(mutexIDAddr)
	b.At(19).MovReg(asm.EBX, asm.EAX)
	b.At(19).Comment("SyncMutexLock").MovImm(asm.EAX, kernel.SyncMutexLock)
	b.At(19).Comment("LOCK (may block)").Int(kernel.IRQSync)
	b.At(20).Comment("critical section").MovMemEAX(counterAddr)
	b.At(21).Inc(asm.EAX)
	b.At(22).MovEAXMem(counterAddr)
	b.At(23).MovMemEAX(mutexIDAddr)
	b.At(23).MovReg(asm.EBX, asm.EAX)
	b.At(23).Comment("SyncMutexUnlock").MovImm(asm.EAX, kernel.SyncMutexUnlock)
	b.At(23).Comment("UNLOCK").Int(kernel.IRQSync)
	b.At(18).Inc(asm.ECX)
	b.At(18).CmpImm(asm.ECX, 200)
	b.At(18).Jne("loop_start")
	b.At(26).Comment("SysExit").MovImm(asm.EAX, kernel.SysExit)
	b.At(26).MovImm(asm.EBX, 0)
	b.At(26).Int(kernel.IRQSyscall)

	return dslProc(b)
}

// ─── showcase ─────────────────────────────────────────────────────────────

// buildShowcaseMemwalk walks four contiguous counters in shared memory five
// times each. Produces lots of byte flights and heat-grid activity.
func buildShowcaseMemwalk() Proc {
	b := asm.NewProc("memwalk").Source(`// memwalk (Core 0) — recorre cuatro contadores compartidos en USER_STACK.
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
`)
	const base uint32 = 0x3FFE00
	b.At(10).Comment("for init").MovImm(asm.ECX, 5)
	b.At(11).Label("loop").MovMemEAX(base)
	b.At(11).Inc(asm.EAX)
	b.At(11).MovEAXMem(base)
	b.At(12).MovMemEAX(base + 4)
	b.At(12).Inc(asm.EAX)
	b.At(12).MovEAXMem(base + 4)
	b.At(13).MovMemEAX(base + 8)
	b.At(13).Inc(asm.EAX)
	b.At(13).MovEAXMem(base + 8)
	b.At(14).MovMemEAX(base + 12)
	b.At(14).Inc(asm.EAX)
	b.At(14).MovEAXMem(base + 12)
	b.At(10).Dec(asm.ECX)
	b.At(10).Comment("for loop").Jne("loop")
	b.At(17).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(17).XorReg(asm.EBX, asm.EBX)
	b.At(17).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildShowcaseReader does a bounded keyboard echo: three sys_read/sys_write
// pairs then sys_exit. Each sys_read blocks until IRQ 0x21.
func buildShowcaseReader() Proc {
	b := asm.NewProc("reader").Source(`// reader (Core 1) — eco de teclado limitado a 3 lecturas.
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
`)
	const bufAddr uint32 = 0x37FFE0
	b.At(8).Comment("for init").MovImm(asm.EDI, 3)
	b.At(9).Comment("sys_read").Label("loop").MovImm(asm.EAX, kernel.SysRead)
	b.At(9).Comment("fd=stdin").MovImm(asm.EBX, 0)
	b.At(9).MovImm(asm.ECX, bufAddr)
	b.At(9).MovImm(asm.EDX, 1)
	b.At(9).Comment("BLOCKED → IRQ 0x21").Int(kernel.IRQSyscall)
	b.At(10).Comment("sys_write").MovImm(asm.EAX, kernel.SysWrite)
	b.At(10).Comment("fd=stdout").MovImm(asm.EBX, 1)
	b.At(10).MovImm(asm.ECX, bufAddr)
	b.At(10).MovImm(asm.EDX, 1)
	b.At(10).Comment("echo").Int(kernel.IRQSyscall)
	b.At(8).Dec(asm.EDI)
	b.At(8).Comment("for loop").Jne("loop")
	b.At(13).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(13).XorReg(asm.EBX, asm.EBX)
	b.At(13).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildShowcaseProducer creates a semaphore and signals it five times.
func buildShowcaseProducer() Proc {
	b := asm.NewProc("producer").Source(`// producer (Core 2) — crea un semáforo y emite cinco signals.
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
`)
	b.At(6).Comment("SemCreate").MovImm(asm.EAX, kernel.SyncSemCreate)
	b.At(6).Comment("init=0").MovImm(asm.EBX, 0)
	b.At(6).Comment("→ eax=1").Int(kernel.IRQSync)
	b.At(7).Comment("for init").MovImm(asm.ECX, 5)
	b.At(8).Comment("sem id").Label("loop").MovImm(asm.EBX, 1)
	b.At(8).Comment("SemSignal").MovImm(asm.EAX, kernel.SyncSemSignal)
	b.At(8).Int(kernel.IRQSync)
	b.At(7).Dec(asm.ECX)
	b.At(7).Comment("for loop").Jne("loop")
	b.At(10).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(10).XorReg(asm.EBX, asm.EBX)
	b.At(10).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildShowcaseConsumerFault waits on the producer's semaphore three times,
// then writes to a kernel address to provoke a page fault.
func buildShowcaseConsumerFault() Proc {
	b := asm.NewProc("consumer_fault").Source(`// consumer_fault (Core 3) — espera el semáforo del producer y muere
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
`)
	b.At(10).Comment("delay init").MovImm(asm.ECX, 20)
	b.At(10).Label("delay").Dec(asm.ECX)
	b.At(10).Comment("spin wait").Jne("delay")
	b.At(12).Comment("sem id").MovImm(asm.EBX, 1)
	b.At(12).Comment("SemWait #1").MovImm(asm.EAX, kernel.SyncSemWait)
	b.At(12).Int(kernel.IRQSync)
	b.At(13).MovImm(asm.EBX, 1)
	b.At(13).Comment("SemWait #2").MovImm(asm.EAX, kernel.SyncSemWait)
	b.At(13).Int(kernel.IRQSync)
	b.At(14).MovImm(asm.EBX, 1)
	b.At(14).Comment("SemWait #3").MovImm(asm.EAX, kernel.SyncSemWait)
	b.At(14).Int(kernel.IRQSync)
	b.At(16).MovImm(asm.EAX, 0xDEAD)
	b.At(16).Comment("¡fault!").MovEAXMem(0x100000)
	b.At(17).Comment("sys_exit (unreachable)").MovImm(asm.EAX, kernel.SysExit)
	b.At(17).MovImm(asm.EBX, 0)
	b.At(17).Int(kernel.IRQSyscall)
	return dslProc(b)
}
