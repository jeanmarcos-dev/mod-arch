package programs

import (
	"github.com/jeanmarcos/modos-cpu/internal/programs/asm"
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// cloneVMFilesThread is the flag mask the threads demos pass to sys_clone:
// share virtual memory, file table and thread-group id with the creator.
// Matches the value glibc uses for pthread_create's flags argument.
const cloneVMFilesThread uint32 = 0x10500

// threadFnOffset is the in-process offset where the inline thread_fn body
// starts inside the leader's code segment. The demos pad with NOPs up to
// this offset so the thread entry point is at a stable, hand-picked address.
const threadFnOffset uint32 = 0x80

// threadMsgOffset is the in-process offset of the inline string the thread
// body writes via sys_write. Kept far enough from threadFnOffset to leave
// room for the syscall sequence (≤ 0x40 bytes today).
const threadMsgOffset uint32 = 0xC0

// buildThreadsJoin: leader spawns one sibling thread via sys_clone, the
// thread writes "T2 done\n" then exits with 123, leader waits and returns
// the reaped TID as its own exit code. Demonstrates CLONE_VM|CLONE_FILES|
// CLONE_THREAD and the sys_waitpid join contract.
func buildThreadsJoin() Proc {
	b := asm.NewProc("thread_join").Source(`// pthread_create + pthread_join, modelado directo sobre sys_clone (120)
// y sys_waitpid (7). El hilo es child del creador en este simulador, por
// eso waitpid funciona; en Linux real glibc usa futex sobre el TID.
int main(void) {
    int tid = clone(0x10500, 0, thread_fn);
    int ret = waitpid(tid, 0, 0);
    return ret;
}

void thread_fn(void) {
    write(1, "T2 done\n", 8);
    exit(123);
}
`)

	threadFnAddr := cpu.AddrUserBase + threadFnOffset
	threadMsgAddr := cpu.AddrUserBase + threadMsgOffset

	// main: clone + waitpid + exit
	b.At(5).Comment("SysClone").MovImm(asm.EAX, kernel.SysClone)
	b.At(5).Comment("CLONE_VM|CLONE_FILES|CLONE_THREAD").MovImm(asm.EBX, cloneVMFilesThread)
	b.At(5).Comment("stack=auto").MovImm(asm.ECX, 0)
	b.At(5).Comment("thread_fn EIP").MovImm(asm.EDX, threadFnAddr)
	b.At(5).Int(kernel.IRQSyscall)
	b.At(6).Comment("tid -> waitpid arg").MovReg(asm.EBX, asm.EAX)
	b.At(6).Comment("SysWaitpid").MovImm(asm.EAX, kernel.SysWaitpid)
	b.At(6).MovImm(asm.ECX, 0)
	b.At(6).MovImm(asm.EDX, 0)
	b.At(6).Int(kernel.IRQSyscall)
	b.At(7).Comment("exit = reaped tid").MovReg(asm.EBX, asm.EAX)
	b.At(7).Comment("SysExit").MovImm(asm.EAX, kernel.SysExit)
	b.At(7).Int(kernel.IRQSyscall)

	// thread_fn body
	b.PadTo(threadFnOffset)
	b.Label("thread_fn")
	b.At(11).Comment("SysWrite").MovImm(asm.EAX, kernel.SysWrite)
	b.At(11).Comment("stdout").MovImm(asm.EBX, 1)
	b.At(11).Comment(`"T2 done\n"`).MovImm(asm.ECX, threadMsgAddr)
	b.At(11).MovImm(asm.EDX, 8)
	b.At(11).Int(kernel.IRQSyscall)
	b.At(12).Comment("retval").MovImm(asm.EBX, 123)
	b.At(12).Comment("SysExit").MovImm(asm.EAX, kernel.SysExit)
	b.At(12).Int(kernel.IRQSyscall)

	// inline message
	b.PadTo(threadMsgOffset)
	b.DataStr("T2 done\n")

	return dslProc(b)
}
