package programs

import (
	"github.com/jeanmarcos-dev/mod-arch/internal/programs/asm"
	"github.com/jeanmarcos-dev/mod-arch/pkg/kernel"
)

// spinCounterDSL builds the canonical "infinite increment" CPU-bound process
// used by scheduler_demo, two_counters and latency_quantum. Bytes are identical
// across all three demos (5-byte MOV imm32, 1-byte INC, 2-byte JMP -3); only
// the wrapper Source string and Name differ pedagogically.
func spinCounterDSL(name string, reg asm.Reg, source string) Proc {
	b := asm.NewProc(name).Source(source)
	b.At(3).MovImm(reg, 0)
	b.At(4).Label("loop")
	b.At(4).Inc(reg)
	b.At(4).Jmp("loop")
	return dslProc(b)
}

// interactiveReaderDSL builds the read/echo loop shared by scheduler_demo's
// "reader" and latency_quantum's "interactive_reader". Both blockon sys_read,
// then echo via sys_write, then loop back. Bytes match exactly; only Name and
// Source differ.
func interactiveReaderDSL(name string, source string) Proc {
	b := asm.NewProc(name).Source(source)
	// PID 3 stack lives at 0x3BFFE0; buf is the first slot there.
	const bufAddr uint32 = 0x3BFFE0

	b.At(8).Label("loop")
	b.At(8).MovImm(asm.EAX, kernel.SysRead)
	b.At(8).MovImm(asm.EBX, 0)
	b.At(8).MovImm(asm.ECX, bufAddr)
	b.At(8).MovImm(asm.EDX, 1)
	b.At(8).Int(kernel.IRQSyscall)
	b.At(9).MovImm(asm.EAX, kernel.SysWrite)
	b.At(9).MovImm(asm.EBX, 1)
	b.At(9).MovImm(asm.ECX, bufAddr)
	b.At(9).MovImm(asm.EDX, 1)
	b.At(9).Int(kernel.IRQSyscall)
	b.At(7).Jmp("loop")
	return dslProc(b)
}

// ─── scheduler_demo ────────────────────────────────────────────────────────

func schedulerCounterEAX() Proc {
	return spinCounterDSL("counter_eax", asm.EAX,
		`// Loop infinito incrementando EAX. CPU-bound (nunca cede voluntariamente).
// Solo lo preempta el timer.
int main(void) {
    register int counter = 0;     // mov eax, 0
    while (1) counter++;          // L: inc eax ; jmp L
}
`)
}

func schedulerCounterEBX() Proc {
	return spinCounterDSL("counter_ebx", asm.EBX,
		`// Igual al de EAX pero usando EBX. Así se ven dos registros distintos
// creciendo según quién tenga el CPU.
int main(void) {
    register int counter = 0;     // mov ebx, 0
    while (1) counter++;          // L: inc ebx ; jmp L
}
`)
}

func schedulerReader() Proc {
	return interactiveReaderDSL("reader",
		`// Lee y echo del teclado. Cada sys_read BLOQUEA si no hay tecla,
// mostrando este proceso en el WAITING QUEUE mientras los otros corren.

char buf;  // en su user_stack @ 0x3BFFE0 (proc 3 → stack base distinto)

int main(void) {
    while (1) {
        read(0, &buf, 1);          // → BLOCKED hasta IRQ 0x21
        write(1, &buf, 1);         // echo a stdout
    }
}
`)
}

func buildSchedulerDemo() []Proc {
	return []Proc{schedulerCounterEAX(), schedulerCounterEBX(), schedulerReader()}
}

// ─── two_counters ──────────────────────────────────────────────────────────

func twoCountersP1() Proc {
	return spinCounterDSL("p1_count_eax", asm.EAX,
		`// Proceso 1: cuenta en EAX infinitamente.
int main(void) {
    register int n = 0;       // mov eax, 0
    while (1) n++;            // L: inc eax ; jmp L
}
`)
}

func twoCountersP2() Proc {
	return spinCounterDSL("p2_count_ebx", asm.EBX,
		`// Proceso 2: cuenta en EBX infinitamente.
int main(void) {
    register int n = 0;       // mov ebx, 0
    while (1) n++;            // L: inc ebx ; jmp L
}
`)
}

func buildTwoCounters() []Proc {
	return []Proc{twoCountersP1(), twoCountersP2()}
}

// ─── latency_quantum ───────────────────────────────────────────────────────

func latencyHogEAX() Proc {
	return spinCounterDSL("hog_eax", asm.EAX,
		"// CPU-bound: nunca bloquea, solo consume quantum.\n"+
			"int main(void) {\n"+
			"    register int n = 0;      // mov EAX, 0\n"+
			"    while (1) n++;           // inc EAX ; jmp\n"+
			"}\n")
}

func latencyHogEBX() Proc {
	return spinCounterDSL("hog_ebx", asm.EBX,
		"// CPU-bound: nunca bloquea, solo consume quantum.\n"+
			"int main(void) {\n"+
			"    register int n = 0;      // mov EBX, 0\n"+
			"    while (1) n++;           // inc EBX ; jmp\n"+
			"}\n")
}

func latencyInteractiveReader() Proc {
	return interactiveReaderDSL("interactive_reader",
		`// Proceso interactivo: bloquea esperando teclado y luego hace echo.
// Compare quantum alto vs bajo: la tecla llega por IRQ, pero la vuelta a RUN
// compite con los CPU-bound en la ready queue.
char buf;  // PID 3 → stack @ 0x3BFFE0

int main(void) {
    while (1) {
        read(0, &buf, 1);
        write(1, &buf, 1);
    }
}
`)
}

func buildLatencyQuantum() []Proc {
	return []Proc{latencyHogEAX(), latencyHogEBX(), latencyInteractiveReader()}
}

// ─── demo_fork ─────────────────────────────────────────────────────────────

func buildDemoFork() Proc {
	b := asm.NewProc("fork_basic").Source(`// fork() básico con espera del padre.
// El kernel devuelve 0 en EAX al hijo y el PID del hijo al padre.
int main(void) {
    int pid = fork();             // sys_fork
    if (pid == 0) {
        return 42;                // rama del hijo: exit(42)
    }
    int reaped = waitpid(-1, 0, 0);  // padre se duerme hasta cosechar
    return reaped;                // exit code = PID que cosechó
}
`)
	// parent path: fork, branch on result
	b.At(4).MovImm(asm.EAX, kernel.SysFork)
	b.At(4).Int(kernel.IRQSyscall)
	b.At(5).CmpImm(asm.EAX, 0)
	b.At(5).Je("child")
	// parent continues: waitpid then exit with reaped pid
	b.At(8).MovImm(asm.EAX, kernel.SysWaitpid)
	b.At(8).MovImm(asm.EBX, 0xFFFFFFFF) // -1
	b.At(8).MovImm(asm.ECX, 0)
	b.At(8).MovImm(asm.EDX, 0)
	b.At(8).Int(kernel.IRQSyscall)
	b.At(9).MovReg(asm.EBX, asm.EAX)
	b.At(9).MovImm(asm.EAX, kernel.SysExit)
	b.At(9).Int(kernel.IRQSyscall)
	// child path
	b.Label("child")
	b.At(6).MovImm(asm.EBX, 42)
	b.At(6).MovImm(asm.EAX, kernel.SysExit)
	b.At(6).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// ─── demo_vfork ────────────────────────────────────────────────────────────

func buildDemoVfork() Proc {
	b := asm.NewProc("vfork_shared_mem").Source(`// vfork(): el hijo comparte el espacio de direcciones del padre.
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
`)
	const sharedAddr uint32 = 0x310000

	b.At(6).MovImm(asm.EAX, kernel.SysVfork)
	b.At(6).Int(kernel.IRQSyscall)
	b.At(7).CmpImm(asm.EAX, 0)
	b.At(7).Je("child")
	// parent path: load shared word and exit with it
	b.At(11).MovMemEAX(sharedAddr)
	b.At(12).MovReg(asm.EBX, asm.EAX)
	b.At(12).MovImm(asm.EAX, kernel.SysExit)
	b.At(12).Int(kernel.IRQSyscall)
	// child path
	b.Label("child")
	b.At(8).MovImm(asm.EAX, 0xDEADBEEF)
	b.At(8).MovEAXMem(sharedAddr)
	b.At(9).MovImm(asm.EBX, 0)
	b.At(9).MovImm(asm.EAX, kernel.SysExit)
	b.At(9).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// ─── demo_pipe ─────────────────────────────────────────────────────────────

func buildDemoPipe() Proc {
	b := asm.NewProc("pipe_basic").Source(`// Pipe anónimo entre padre e hijo:
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
`)
	const fdsAddr uint32 = 0x310000  // fds[0]=fdR, fds[1]=fdW (4 bytes apart)
	const msgAddr uint32 = 0x310010  // "HOLA" landing slot
	const childBuf uint32 = 0x310020 // child's read buffer

	// sys_pipe(fds)
	b.At(8).MovImm(asm.EAX, kernel.SysPipe)
	b.At(8).MovImm(asm.EBX, fdsAddr)
	b.At(8).Int(kernel.IRQSyscall)
	// stash "HOLA" then write(fdW, msg, 4)
	b.At(9).MovImm(asm.EAX, 0x414C4F48) // "HOLA" little-endian
	b.At(9).MovEAXMem(msgAddr)
	b.At(9).MovMemEAX(fdsAddr + 4) // load fdW
	b.At(9).MovReg(asm.EBX, asm.EAX)
	b.At(9).MovImm(asm.EAX, kernel.SysWrite)
	b.At(9).MovImm(asm.ECX, msgAddr)
	b.At(9).MovImm(asm.EDX, 4)
	b.At(9).Int(kernel.IRQSyscall)
	// fork + branch
	b.At(10).MovImm(asm.EAX, kernel.SysFork)
	b.At(10).Int(kernel.IRQSyscall)
	b.At(11).CmpImm(asm.EAX, 0)
	b.At(11).Je("child")
	// parent: waitpid(-1) then exit with reaped pid
	b.At(17).MovImm(asm.EAX, kernel.SysWaitpid)
	b.At(17).MovImm(asm.EBX, 0xFFFFFFFF)
	b.At(17).MovImm(asm.ECX, 0)
	b.At(17).MovImm(asm.EDX, 0)
	b.At(17).Int(kernel.IRQSyscall)
	b.At(18).MovReg(asm.EBX, asm.EAX)
	b.At(18).MovImm(asm.EAX, kernel.SysExit)
	b.At(18).Int(kernel.IRQSyscall)
	// child: read(fdR, buf, 4)
	b.Label("child")
	b.At(13).MovMemEAX(fdsAddr) // load fdR
	b.At(13).MovReg(asm.EBX, asm.EAX)
	b.At(13).MovImm(asm.EAX, kernel.SysRead)
	b.At(13).MovImm(asm.ECX, childBuf)
	b.At(13).MovImm(asm.EDX, 4)
	b.At(13).Int(kernel.IRQSyscall)
	// echo: write(1, buf, 4)
	b.At(14).MovImm(asm.EBX, 1)
	b.At(14).MovImm(asm.EAX, kernel.SysWrite)
	b.At(14).MovImm(asm.ECX, childBuf)
	b.At(14).MovImm(asm.EDX, 4)
	b.At(14).Int(kernel.IRQSyscall)
	// child exit(0)
	b.At(15).MovImm(asm.EBX, 0)
	b.At(15).MovImm(asm.EAX, kernel.SysExit)
	b.At(15).Int(kernel.IRQSyscall)
	return dslProc(b)
}
