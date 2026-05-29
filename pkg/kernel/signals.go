package kernel

// Linux i386 signal numbers used by the simulator to encode the cause of a
// fault-driven death into pcb.ExitCode. The convention follows POSIX
// WIFSIGNALED: WTERMSIG(status) = status & 0x7f. Higher bits (0x80 =
// WCOREDUMP) are NOT set — the simulator does not model core dumps, so the
// raw signal number is the entire status.
const (
	SIGILL  uint32 = 4  // illegal instruction or privileged op in user mode (#GP)
	SIGSEGV uint32 = 11 // invalid memory access (page fault)
)

// setFaultExitCode stores the WIFSIGNALED status on the current PCB so the
// parent's eventual reap surfaces the cause of death. Must be called BEFORE
// exitProcess; otherwise the parent reaps ExitCode=0 and the wstatus is
// indistinguishable from a clean sys_exit(0).
func (k *Kernel) setFaultExitCode(core *Core, sig uint32) {
	if pcb := k.Processes[core.CurrentPID]; pcb != nil {
		pcb.ExitCode = sig
	}
}
