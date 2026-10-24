package programs

import (
	"github.com/jeanmarcos/modos-cpu/internal/programs/asm"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// buildStackOverflow forces a stack overflow by relocating ESP near the user
// stack floor and recursing into itself.
func buildStackOverflow() Proc {
	b := asm.NewProc("stack_overflow").Source(`// Stack overflow deliberado. En x86 real, cada CALL pushea la dirección
// de retorno. Aquí acercamos ESP al borde de user_stack para provocar el
// fault rápido y ver la MMU detener el proceso.
int recurse(void) {
    return recurse();       // call recurse
}

int main(void) {
    asm("mov esp, 0x300004");  // borde inferior del user_stack
    return recurse();          // segundo CALL escribe en user_code → fault
}
`)
	b.At(9).Comment("near stack floor").MovImm(asm.ESP, 0x300004)
	b.At(10).Comment("pushes return address").Label("recurse").Call("recurse")
	b.At(10).Comment("sys_exit (unreachable)").MovImm(asm.EAX, kernel.SysExit)
	b.At(10).MovImm(asm.EBX, 0)
	b.At(10).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildSyscallKernelPointer passes a kernel-side pointer to sys_write. A real
// OS rejects it with -EFAULT; this demo highlights the missing copy_from_user
// check in the toy simulator.
func buildSyscallKernelPointer() Proc {
	b := asm.NewProc("syscall_kernel_pointer").Source(`// Puntero inválido en syscall. El kernel valida que el buffer sea
// accesible desde user mode antes de leerlo (como access_ok en Linux).
int main(void) {
    write(1, (char *)0x100000, 8);  // kernel pointer: devuelve -1 (Linux: -EFAULT)
    exit(0);
}
`)
	b.At(4).Comment("sys_write").MovImm(asm.EAX, kernel.SysWrite)
	b.At(4).Comment("stdout").MovImm(asm.EBX, 1)
	b.At(4).Comment("kernel ptr").MovImm(asm.ECX, 0x100000)
	b.At(4).MovImm(asm.EDX, 8)
	b.At(4).Comment("should be -EFAULT").Int(kernel.IRQSyscall)
	b.At(5).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(5).XorReg(asm.EBX, asm.EBX)
	b.At(5).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildUnknownSyscall invokes an unimplemented syscall number. The kernel
// returns 0xFFFFFFFF in EAX.
func buildUnknownSyscall() Proc {
	b := asm.NewProc("unknown_syscall").Source(`// Syscall inexistente: el kernel no conoce EAX=999.
int main(void) {
    long r = syscall(999);   // EAX queda en 0xFFFFFFFF
    exit(0);
}
`)
	b.At(3).Comment("unknown syscall").MovImm(asm.EAX, 999)
	b.At(3).Comment("EAX=0xFFFFFFFF").Int(kernel.IRQSyscall)
	b.At(4).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(4).XorReg(asm.EBX, asm.EBX)
	b.At(4).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildDoubleUnlock unlocks a mutex without holding it. Kernel rejects with
// EAX=0xFFFFFFFF.
func buildDoubleUnlock() Proc {
	b := asm.NewProc("double_unlock").Source(`// Bug clásico: unlock sin ownership. El mutex existe, pero nadie lo tomó.
int main(void) {
    int m = mutex_create();  // id=1
    mutex_unlock(m);         // error: caller no es holder
    exit(0);
}
`)
	b.At(3).Comment("MutexCreate").MovImm(asm.EAX, kernel.SyncMutexCreate)
	b.At(3).Comment("m=1").Int(kernel.IRQSync)
	b.At(4).Comment("mutex id").MovImm(asm.EBX, 1)
	b.At(4).Comment("MutexUnlock").MovImm(asm.EAX, kernel.SyncMutexUnlock)
	b.At(4).Comment("EAX=0xFFFFFFFF").Int(kernel.IRQSync)
	b.At(5).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(5).XorReg(asm.EBX, asm.EBX)
	b.At(5).Int(kernel.IRQSyscall)
	return dslProc(b)
}

// buildSemaphoreLeak creates three semaphores and exits without freeing them.
// Pedagogically demonstrates a resource leak.
func buildSemaphoreLeak() Proc {
	b := asm.NewProc("semaphore_leak").Source(`// Resource leak: objetos kernel creados y nunca liberados.
int main(void) {
    sem_create(1);
    sem_create(2);
    sem_create(3);
    exit(0);
}
`)
	b.At(3).Comment("SemCreate").MovImm(asm.EAX, kernel.SyncSemCreate)
	b.At(3).Comment("init=1").MovImm(asm.EBX, 1)
	b.At(3).Comment("sem 1").Int(kernel.IRQSync)
	b.At(4).Comment("SemCreate").MovImm(asm.EAX, kernel.SyncSemCreate)
	b.At(4).Comment("init=2").MovImm(asm.EBX, 2)
	b.At(4).Comment("sem 2").Int(kernel.IRQSync)
	b.At(5).Comment("SemCreate").MovImm(asm.EAX, kernel.SyncSemCreate)
	b.At(5).Comment("init=3").MovImm(asm.EBX, 3)
	b.At(5).Comment("sem 3").Int(kernel.IRQSync)
	b.At(6).Comment("sys_exit").MovImm(asm.EAX, kernel.SysExit)
	b.At(6).XorReg(asm.EBX, asm.EBX)
	b.At(6).Int(kernel.IRQSyscall)
	return dslProc(b)
}
