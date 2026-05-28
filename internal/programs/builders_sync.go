package programs

import (
	"github.com/jeanmarcos/modos-cpu/internal/programs/asm"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// ─── race_counter ──────────────────────────────────────────────────────────

// buildRaceSetupZeroCounter zeroes the shared counter at SharedCounterAddr and
// exits. Race demo prelude.
func buildRaceSetupZeroCounter() Proc {
	b := asm.NewProc("setup_zero_counter").Source(`// Setup: pone el contador compartido en 0 y termina.
int main(void) {
    *(int *)0x3FFFE0 = 0;
    exit(0);
}
`)
	addr := SharedCounterAddr()
	b.At(3).XorReg(asm.EAX, asm.EAX)
	b.At(3).MovEAXMem(addr)
	b.At(4).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(4).Comment("status=0").XorReg(asm.EBX, asm.EBX)
	b.At(4).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// racerWorker is the unsynchronised increment loop reused by racer_a and
// racer_b. The byte image is identical; only the C source differs (worker B
// drops the "preemption window" comment).
func racerWorker(name, source string) Proc {
	b := asm.NewProc(name).Source(source)
	addr := SharedCounterAddr()
	b.At(3).MovImm(asm.ECX, 200)
	b.At(4).Label("loop").MovMemEAX(addr)
	b.At(5).Inc(asm.EAX)
	b.At(6).MovEAXMem(addr)
	b.At(3).Dec(asm.ECX)
	b.At(3).Jne("loop")
	b.At(8).MovImm(asm.EAX, kernel.SysExit)
	b.At(8).XorReg(asm.EBX, asm.EBX)
	b.At(8).Int(kernel.IRQSyscall)
	return dslProc(b)
}

func buildRaceRacerA() Proc {
	return racerWorker("racer_a", `// Worker A: incrementa el contador compartido 200 veces sin lock.
int main(void) {
    for (int i = 0; i < 200; i++) {
        int n = *(int *)0x3FFFE0;   // load
        n++;                          // increment
        *(int *)0x3FFFE0 = n;         // store ← preemption window between load and store
    }
    exit(0);
}
`)
}

func buildRaceRacerB() Proc {
	return racerWorker("racer_b", `// Worker B: mismo loop que A. Sin lock entre ambos.
int main(void) {
    for (int i = 0; i < 200; i++) {
        int n = *(int *)0x3FFFE0;
        n++;
        *(int *)0x3FFFE0 = n;
    }
    exit(0);
}
`)
}

// ─── safe_counter ──────────────────────────────────────────────────────────

func buildSafeSetupZeroAndMutex() Proc {
	b := asm.NewProc("setup_zero_and_mutex").Source(`// Setup: pone el contador en 0, crea el mutex (id=1) y termina.
int main(void) {
    *(int *)0x3FFFE0 = 0;
    mutex_create();   // ID 1 (primer mutex del kernel)
    exit(0);
}
`)
	addr := SharedCounterAddr()
	b.At(3).XorReg(asm.EAX, asm.EAX)
	b.At(3).MovEAXMem(addr)
	b.At(4).Comment("MutexCreate").MovImm(asm.EAX, kernel.SyncMutexCreate)
	b.At(4).Int(kernel.IRQSync)
	b.At(5).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(5).XorReg(asm.EBX, asm.EBX)
	b.At(5).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// lockerWorker emits the mutex-protected increment loop reused by both
// safe_counter workers. Bytes are identical; only the C source differs.
func lockerWorker(name, source string) Proc {
	b := asm.NewProc(name).Source(source)
	addr := SharedCounterAddr()
	b.At(3).MovImm(asm.ECX, 200)
	b.At(4).Label("loop").MovImm(asm.EBX, 1)
	b.At(4).MovImm(asm.EAX, kernel.SyncMutexLock)
	b.At(4).Int(kernel.IRQSync)
	b.At(5).MovMemEAX(addr)
	b.At(6).Inc(asm.EAX)
	b.At(7).MovEAXMem(addr)
	b.At(8).MovImm(asm.EBX, 1)
	b.At(8).MovImm(asm.EAX, kernel.SyncMutexUnlock)
	b.At(8).Int(kernel.IRQSync)
	b.At(3).Dec(asm.ECX)
	b.At(3).Jne("loop")
	b.At(10).MovImm(asm.EAX, kernel.SysExit)
	b.At(10).XorReg(asm.EBX, asm.EBX)
	b.At(10).Int(kernel.IRQSyscall)
	return dslProc(b)
}

func buildSafeLockerA() Proc {
	return lockerWorker("locker_a", `// Worker A: incrementa con mutex id=1.
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
`)
}

func buildSafeLockerB() Proc {
	return lockerWorker("locker_b", `// Worker B: mismo loop que A, ambos serializan vía mutex id=1.
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
`)
}

// ─── producer_consumer ─────────────────────────────────────────────────────

func buildPCSetupTwoSems() Proc {
	b := asm.NewProc("setup_two_sems").Source(`// Setup: crea sem(empty=1) y sem(full=0); luego termina.
int main(void) {
    sem_create(1);   // sem id 1 — "empty", buffer starts available
    sem_create(0);   // sem id 2 — "full",  no items pending
    exit(0);
}
`)
	b.At(3).Comment("SemCreate").MovImm(asm.EAX, kernel.SyncSemCreate)
	b.At(3).Comment("init=1").MovImm(asm.EBX, 1)
	b.At(3).Comment("→ eax=1").Int(kernel.IRQSync)
	b.At(4).Comment("SemCreate").MovImm(asm.EAX, kernel.SyncSemCreate)
	b.At(4).Comment("init=0").MovImm(asm.EBX, 0)
	b.At(4).Comment("→ eax=2").Int(kernel.IRQSync)
	b.At(5).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(5).XorReg(asm.EBX, asm.EBX)
	b.At(5).Int(kernel.IRQSyscall)
	return dslProc(b)
}

func buildPCProducer() Proc {
	b := asm.NewProc("producer").Source(`// Producer: 50 veces, espera buffer libre y luego avisa item disponible.
int main(void) {
    for (int i = 0; i < 50; i++) {
        sem_wait(1);   // wait empty
        // producir un item (en este demo no se escribe payload: el sync es lo
        // pedagógico, no la data).
        sem_signal(2); // signal full
    }
    exit(0);
}
`)
	b.At(3).Comment("for init").MovImm(asm.ECX, 50)
	b.At(4).Comment("sem=empty").Label("loop").MovImm(asm.EBX, 1)
	b.At(4).Comment("SemWait").MovImm(asm.EAX, kernel.SyncSemWait)
	b.At(4).Int(kernel.IRQSync)
	b.At(7).Comment("sem=full").MovImm(asm.EBX, 2)
	b.At(7).Comment("SemSignal").MovImm(asm.EAX, kernel.SyncSemSignal)
	b.At(7).Int(kernel.IRQSync)
	b.At(3).Dec(asm.ECX)
	b.At(3).Comment("for loop").Jne("loop")
	b.At(9).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(9).XorReg(asm.EBX, asm.EBX)
	b.At(9).Int(kernel.IRQSyscall)
	return dslProc(b)
}

func buildPCConsumer() Proc {
	b := asm.NewProc("consumer").Source(`// Consumer: 50 veces, espera item disponible y luego libera el buffer.
int main(void) {
    for (int i = 0; i < 50; i++) {
        sem_wait(2);   // wait full
        // consumir el item (idéntica omisión de payload).
        sem_signal(1); // signal empty
    }
    exit(0);
}
`)
	b.At(3).MovImm(asm.ECX, 50)
	b.At(4).Comment("sem=full").Label("loop").MovImm(asm.EBX, 2)
	b.At(4).Comment("SemWait").MovImm(asm.EAX, kernel.SyncSemWait)
	b.At(4).Int(kernel.IRQSync)
	b.At(6).Comment("sem=empty").MovImm(asm.EBX, 1)
	b.At(6).Comment("SemSignal").MovImm(asm.EAX, kernel.SyncSemSignal)
	b.At(6).Int(kernel.IRQSync)
	b.At(3).Dec(asm.ECX)
	b.At(3).Jne("loop")
	b.At(8).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(8).XorReg(asm.EBX, asm.EBX)
	b.At(8).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// ─── philosophers (deadlock + safe) ────────────────────────────────────────

// setupThreeForks emits the three-mutex-create + exit prelude shared by both
// philosopher demos. The source string differs (one references "safe"); the
// bytes are byte-identical.
func setupThreeForks(name, source string) Proc {
	b := asm.NewProc(name).Source(source)
	b.At(3).MovImm(asm.EAX, kernel.SyncMutexCreate)
	b.At(3).Int(kernel.IRQSync)
	b.At(4).MovImm(asm.EAX, kernel.SyncMutexCreate)
	b.At(4).Int(kernel.IRQSync)
	b.At(5).MovImm(asm.EAX, kernel.SyncMutexCreate)
	b.At(5).Int(kernel.IRQSync)
	b.At(6).MovImm(asm.EAX, kernel.SysExit)
	b.At(6).XorReg(asm.EBX, asm.EBX)
	b.At(6).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// philosopher emits the lock-eat-unlock loop for a single philo. firstFork and
// secondFork pick which mutex IDs to grab. The "safe" variant calls this with
// inverted ordering on philo 2 to break the circular wait.
func philosopher(name string, idx, firstFork, secondFork int, label string) Proc {
	first := itoa(firstFork)
	second := itoa(secondFork)
	source := "// Filósofo #" + itoa(idx) + " — " + label + "\n" +
		"int main(void) {\n" +
		"    for (int i = 0; i < 10; i++) {\n" +
		"        mutex_lock(" + first + ");\n" +
		"        mutex_lock(" + second + ");\n" +
		"        /* eat */\n" +
		"        mutex_unlock(" + second + ");\n" +
		"        mutex_unlock(" + first + ");\n" +
		"    }\n" +
		"    exit(0);\n" +
		"}\n"
	b := asm.NewProc(name).Source(source)
	b.At(3).MovImm(asm.ECX, 10)
	b.At(4).Label("loop").MovImm(asm.EBX, uint32(firstFork))
	b.At(4).MovImm(asm.EAX, kernel.SyncMutexLock)
	b.At(4).Int(kernel.IRQSync)
	b.At(5).MovImm(asm.EBX, uint32(secondFork))
	b.At(5).MovImm(asm.EAX, kernel.SyncMutexLock)
	b.At(5).Int(kernel.IRQSync)
	b.At(6).Nop()
	b.At(6).Nop()
	b.At(7).MovImm(asm.EBX, uint32(secondFork))
	b.At(7).MovImm(asm.EAX, kernel.SyncMutexUnlock)
	b.At(7).Int(kernel.IRQSync)
	b.At(8).MovImm(asm.EBX, uint32(firstFork))
	b.At(8).MovImm(asm.EAX, kernel.SyncMutexUnlock)
	b.At(8).Int(kernel.IRQSync)
	b.At(3).Dec(asm.ECX)
	b.At(3).Jne("loop")
	b.At(10).MovImm(asm.EAX, kernel.SysExit)
	b.At(10).XorReg(asm.EBX, asm.EBX)
	b.At(10).Int(kernel.IRQSyscall)
	return dslProc(b)
}
