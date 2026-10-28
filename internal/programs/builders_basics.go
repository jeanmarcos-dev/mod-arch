package programs

import (
	"github.com/jeanmarcos-dev/mod-arch/internal/programs/asm"
	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
	"github.com/jeanmarcos-dev/mod-arch/pkg/kernel"
)

// buildAdd: tiny pedagogical demo — load two registers, add them, exit with
// the result as status. Showcases MOV imm32, ADD r,r and the INT 0x80 ABI.
func buildAdd() Proc {
	b := asm.NewProc("add").Source(`// Suma dos números en registros.
int main(void) {
    register int a = 10;     // mov eax, 10
    register int b = 32;     // mov ebx, 32
    return a + b;            // add eax, ebx ; sys_exit(EAX)
}
`)
	b.At(3).MovImm(asm.EAX, 10)
	b.At(4).MovImm(asm.EBX, 32)
	b.At(5).AddReg(asm.EAX, asm.EBX)
	b.At(5).Comment("exit code").MovReg(asm.EBX, asm.EAX)
	b.At(5).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(5).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildCounter: classic counter loop 0..10 using INC + CMP + JNE rel8.
func buildCounter() Proc {
	b := asm.NewProc("counter").Source(`// Loop clásico: cuenta de 0 a 10.
int main(void) {
    register int i = 0;        // mov eax, 0
    register int max = 10;     // mov ecx, 10
    while (i != max) {         // L: cmp eax, ecx ; jne L
        i++;                   //    inc eax
    }
    return i;                  // sys_exit(EAX)
}
`)
	b.At(3).MovImm(asm.EAX, 0)
	b.At(4).MovImm(asm.ECX, 10)
	b.At(6).Label("loop").Inc(asm.EAX)
	b.At(5).CmpReg(asm.EAX, asm.ECX)
	b.At(5).Jne("loop")
	b.At(8).Comment("exit code").MovReg(asm.EBX, asm.EAX)
	b.At(8).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(8).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildPushpop: push a value to the user stack and pop it back.
func buildPushpop() Proc {
	b := asm.NewProc("pushpop").Source(`// Guarda un valor en el stack y lo recupera.
int main(void) {
    register int x = 0xCAFE;   // mov eax, 0xCAFE
    push_to_stack(x);          // push eax  (ESP -= 4, mem[ESP] = x)
    x = 0;                     // mov eax, 0
    x = pop_from_stack();      // pop eax   (x = mem[ESP], ESP += 4)
    return x;                  // → 0xCAFE
}
`)
	b.At(3).MovImm(asm.EAX, 0xCAFE)
	b.At(4).Push(asm.EAX)
	b.At(5).MovImm(asm.EAX, 0)
	b.At(6).Pop(asm.EAX)
	b.At(7).Comment("exit code").MovReg(asm.EBX, asm.EAX)
	b.At(7).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(7).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildCallret: call a tiny fn that returns 0x99, then exit with EAX as
// status. The CALL is rel32 forward past the exit sequence to fn.
func buildCallret() Proc {
	b := asm.NewProc("callret").Source(`// Demuestra call/ret: el stack guarda la dirección de retorno.
int fn(void) {
    return 0x99;               // mov eax, 0x99  ;  ret
}

int main(void) {
    return fn();               // call fn ; sys_exit(EAX)
}
`)
	b.At(7).Call("fn")
	b.At(7).Comment("exit code").MovReg(asm.EBX, asm.EAX)
	b.At(7).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(7).Int(kernel.IRQSyscall)
	b.Label("fn")
	b.At(3).MovImm(asm.EAX, 0x99)
	b.At(3).Ret()
	return dslProc(b)
}

// buildSoftInt: fire int 0x80, then continue after IRET. Shows the HW
// mode-switch path without doing useful work in kernel.
func buildSoftInt() Proc {
	b := asm.NewProc("soft_int").Source(`// SW interrupt sin "syscall real". Solo para mostrar el mode switch
// HW que dispara int 0x80.
int main(void) {
    register int x = 1;        // mov eax, 1
    asm("int 0x80");           // HW → PL0, salta a IDT[0x80], handler iret
    x++;                       // inc eax  (ejecuta después del IRET)
    return x;                  // sys_exit(EAX)
}
`)
	b.At(4).MovImm(asm.EAX, 1)
	b.At(5).Int(kernel.IRQSyscall)
	b.At(6).Inc(asm.EAX)
	b.At(7).Comment("exit code").MovReg(asm.EBX, asm.EAX)
	b.At(7).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(7).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildTimerLoop: 16 NOPs followed by sys_exit(0). Used to observe the
// timer IRQ preempt user mode.
func buildTimerLoop() Proc {
	b := asm.NewProc("timer_loop").Source(`// Programa "tonto" que solo hace nada (NOPs).
// Útil para ver el timer HW preemptar la ejecución cada quantum ticks.
int main(void) {
    for (int i = 0; i < 16; i++)
        asm("nop");            // 16 × nop
    return 0;                  // sys_exit(0)
}
`)
	for i := 0; i < 16; i++ {
		b.At(5).Nop()
	}
	b.At(6).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(6).Comment("status").MovImm(asm.EBX, 0)
	b.At(6).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildHello: sys_write the inline string "hola SO\n" then sys_exit(0).
// The string lives in-binary right after the code, at AddrUserBase + 0x22.
func buildHello() Proc {
	b := asm.NewProc("hello").Source(`// Programa "Hello World" con syscalls Linux i386.
const char msg[8] = "hola SO\n";

int main(void) {
    write(1, msg, 8);          // EAX=4, EBX=1, ECX=&msg, EDX=8 ; int 0x80
    exit(0);                   // EAX=1, EBX=0 ; int 0x80
}

// La mini-libc traduce write/exit a las convenciones del kernel:
//   write(fd, buf, n)  →  EAX=4, EBX=fd, ECX=buf, EDX=n, int 0x80
//   exit(status)       →  EAX=1, EBX=status, int 0x80
`)
	msgAddr := cpu.AddrUserBase + 0x22
	b.At(5).Comment("sys_write").MovImm(asm.EAX, kernel.SysWrite)
	b.At(5).Comment("fd=stdout").MovImm(asm.EBX, 1)
	b.At(5).Comment("&msg").MovImm(asm.ECX, msgAddr)
	b.At(5).Comment("len").MovImm(asm.EDX, 8)
	b.At(5).Int(kernel.IRQSyscall)
	b.At(6).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(6).Comment("status").MovImm(asm.EBX, 0)
	b.At(6).Int(kernel.IRQSyscall)
	b.DataStr("hola SO\n")
	return dslProc(b)
}

// buildReadkey: blocking echo loop — sys_read 1 byte into the user stack
// buffer, sys_write it to stdout, jmp back to the top. JMP rel8 -46 closes
// the loop.
func buildReadkey() Proc {
	b := asm.NewProc("readkey").Source(`// Echo del teclado. Loop infinito: leer 1 byte, escribir 1 byte.
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
`)
	const bufAddr uint32 = 0x33FFE0
	b.Label("loop")
	b.At(10).Comment("sys_read").MovImm(asm.EAX, kernel.SysRead)
	b.At(10).Comment("fd=stdin").MovImm(asm.EBX, 0)
	b.At(10).MovImm(asm.ECX, bufAddr)
	b.At(10).MovImm(asm.EDX, 1)
	b.At(10).Int(kernel.IRQSyscall)
	b.At(11).Comment("sys_write").MovImm(asm.EAX, kernel.SysWrite)
	b.At(11).Comment("fd=stdout").MovImm(asm.EBX, 1)
	b.At(11).MovImm(asm.ECX, bufAddr)
	b.At(11).MovImm(asm.EDX, 1)
	b.At(11).Int(kernel.IRQSyscall)
	b.At(9).Jmp("loop")
	return dslProc(b)
}

// buildIdleDemo: blocks on a single sys_read and exits. With the idle task
// off, the simulator auto-stops the instant the proc blocks (legacy
// behavior). With the idle task on, the same instant the proc blocks the
// swapper takes the core in HLT — and the run loop keeps ticking,
// waiting for a key. Side-by-side it surfaces exactly what the swapper
// adds to the model.
func buildIdleDemo() Proc {
	b := asm.NewProc("idle_demo").Source(`// Bloquea en sys_read y termina. Demuestra el rol de la tarea ociosa.
// Con la tarea ociosa apagada: la simulación se detiene cuando el proceso
// bloquea (no hay más trabajo). Encendida: PID 0 (swapper) ocupa el núcleo
// en HLT esperando una IRQ del teclado. Pulse una tecla para reanudar.

char buf;

int main(void) {
    read(0, &buf, 1);   // BLOQUEA → con idle ON, el núcleo lo agarra el swapper
    return buf;         // exit code = byte leído
}
`)
	const bufAddr uint32 = 0x33FFE0
	b.At(7).Comment("sys_read").MovImm(asm.EAX, kernel.SysRead)
	b.At(7).Comment("fd=stdin").MovImm(asm.EBX, 0)
	b.At(7).MovImm(asm.ECX, bufAddr)
	b.At(7).MovImm(asm.EDX, 1)
	b.At(7).Int(kernel.IRQSyscall)
	b.At(8).Comment("exit code = EAX (bytes read)").MovReg(asm.EBX, asm.EAX)
	b.At(8).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(8).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildBadmem: write to a kernel address. MMU traps it as a page fault and
// the kernel kills the process before the exit sequence runs.
func buildBadmem() Proc {
	b := asm.NewProc("badmem").Source(`// Proceso "malicioso" o buggeado. Intenta escribir a memoria kernel.
// La MMU del CPU detecta la violación de permisos → page fault (IRQ 0x0E)
// → kernel mata el proceso (ZOMBIE con razón = "page_fault").
int main(void) {
    int *kernel_addr = (int*)0x100000;     // mov eax, 0xDEAD
    *kernel_addr = 0xDEAD;                 // mov [0x100000], eax  ← ¡fault!
    return 0;                              // sys_exit(0) (nunca se alcanza)
}
`)
	b.At(5).MovImm(asm.EAX, 0xDEAD)
	b.At(6).MovEAXMem(0x100000)
	b.At(7).Comment("sys_exit (unreachable)").MovImm(asm.EAX, kernel.SysExit)
	b.At(7).MovImm(asm.EBX, 0)
	b.At(7).Int(kernel.IRQSyscall)
	return dslProc(b)
}
