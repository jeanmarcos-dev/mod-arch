package kernel

import (
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// helloBytes is a hand-encoded "hello" program. We reuse literal bytes so a
// change in the programs.go definition does not affect these unit tests.
var helloBytes = []byte{
	0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4
	0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1
	0xB9, 0x22, 0x00, 0x20, 0x00, // mov ecx, 0x200022
	0xBA, 0x08, 0x00, 0x00, 0x00, // mov edx, 8
	0xCD, 0x80, // int 0x80
	0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
	0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
	0xCD, 0x80, // int 0x80
	0x68, 0x6F, 0x6C, 0x61, 0x20, 0x53, 0x4F, 0x0A, // "hola SO\n"
}

func TestSyscall_WriteEmitsConsole(t *testing.T) {
	k := newTestKernel(t, helloBytes)
	k.TimerQuantum = 0 // disable timer; isolate the syscall

	var consoleEv *cpu.EventConsoleOut
	for range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if c, ok := e.(cpu.EventConsoleOut); ok {
				ce := c
				consoleEv = &ce
				goto done
			}
		}
	}
done:
	if consoleEv == nil {
		t.Fatal("no EventConsoleOut emitted")
	}
	if consoleEv.Text != "hola SO\n" {
		t.Errorf("Text = %q, want %q", consoleEv.Text, "hola SO\n")
	}
	if len(consoleEv.Bytes) != 8 {
		t.Errorf("Bytes len = %d, want 8", len(consoleEv.Bytes))
	}
}

func TestSyscall_WriteEmitsEvent(t *testing.T) {
	k := newTestKernel(t, helloBytes)
	k.TimerQuantum = 0

	var writeEv *cpu.EventSyscall
	for range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if s, ok := e.(cpu.EventSyscall); ok && s.Name == "write" {
				se := s
				writeEv = &se
				goto done
			}
		}
	}
done:
	if writeEv == nil {
		t.Fatal("no write EventSyscall emitted")
	}
	if writeEv.Num != SysWrite || writeEv.Args[0] != 1 || writeEv.Args[2] != 8 {
		t.Errorf("EventSyscall = %+v", writeEv)
	}
	if writeEv.Result != 8 {
		t.Errorf("Result = %d, want 8 (bytes written)", writeEv.Result)
	}
}

func TestSyscall_ExitHalts(t *testing.T) {
	k := newTestKernel(t, helloBytes)
	k.TimerQuantum = 0

	maxSteps := 40
	for i := range maxSteps {
		if _, err := k.Step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if k.BootCPU().Halted {
			break
		}
	}
	if !k.BootCPU().Halted {
		t.Fatalf("CPU did not halt after sys_exit within %d steps", maxSteps)
	}
	if k.Processes[1].State != StateZombie {
		t.Errorf("P1 state = %s, want ZOMBIE", k.Processes[1].State)
	}
}

// TestSyscall_ExitEmitsPCBSaveAndExitCode covers the bug where the frontend
// saw a ZOMBIE process with stale registers and exit_code=0 after sys_exit:
// killCurrent did not snapshot the CPU into the PCB before flipping the
// state, and EventProcessState did not carry the exit code. The fix has to
// emit both an EventPCBSave with the final registers and an EventProcessState
// whose ExitCode points at the value sys_exit stored in pcb.ExitCode.
func TestSyscall_ExitEmitsPCBSaveAndExitCode(t *testing.T) {
	// Program: mov ebx, 42 ; mov eax, 1 (sys_exit) ; int 0x80
	exitProg := []byte{
		0xBB, 0x2A, 0x00, 0x00, 0x00, // mov ebx, 42
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
		0xCD, 0x80, // int 0x80
	}
	k := newTestKernel(t, exitProg)
	k.TimerQuantum = 0

	var pcbSave *cpu.EventPCBSave
	var procState *cpu.EventProcessState
	for range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("Step: %v", err)
		}
		for _, e := range evs {
			switch v := e.(type) {
			case cpu.EventPCBSave:
				if v.PID == 1 {
					ev := v
					pcbSave = &ev
				}
			case cpu.EventProcessState:
				if v.PID == 1 && v.State == string(StateZombie) {
					ev := v
					procState = &ev
				}
			}
		}
		if k.BootCPU().Halted {
			break
		}
	}

	if pcbSave == nil {
		t.Fatal("killCurrent did not emit EventPCBSave for the exiting process")
	}
	if got := pcbSave.Regs["EBX"]; got != 42 {
		t.Errorf("PCBSave EBX = %d, want 42 (the i386 exit-code arg in EBX, untouched after MOV)", got)
	}
	if got := pcbSave.Regs["EAX"]; got != SysExit {
		t.Errorf("PCBSave EAX = %d, want %d (sys_exit syscall number; PCB mirrors core.CPU.R at trap)", got, SysExit)
	}

	if procState == nil {
		t.Fatal("killCurrent did not emit EventProcessState{State:ZOMBIE} for PID 1")
	}
	if procState.ExitCode == nil {
		t.Fatal("EventProcessState{ZOMBIE} did not carry an ExitCode pointer — frontend cannot see exit_code")
	}
	if *procState.ExitCode != 42 {
		t.Errorf("EventProcessState ExitCode = %d, want 42", *procState.ExitCode)
	}
}

func TestSyscall_ExitSchedulesNext(t *testing.T) {
	// Two processes: the first calls exit, the second (READY) takes RUN as
	// the schedule_out phase drains. With the do_exit cascade split into
	// phases (mark_zombie → notify_parent → schedule_out → reap_cycle → halt)
	// the dispatch of the sibling lands a couple of Steps after the ZOMBIE,
	// not in the same Step — but the invariant the alumno cares about
	// survives: NO EventHalt is emitted on the Step that holds the ZOMBIE,
	// because finalizeExit no longer races scheduler decisions.
	exitProg := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1 (sys_exit)
		0xBB, 0x05, 0x00, 0x00, 0x00, // mov ebx, 5 (status)
		0xCD, 0x80, // int 0x80
	}
	loopProg := []byte{0x90, 0xEB, 0xFE} // nop + jmp -2 (infinite loop)

	k := newTestKernelMulti(t, exitProg, loopProg)
	k.TimerQuantum = 0 // timer disabled; we verify the exit path only

	exitStep := -1
	dispatchStep := -1
	var exitStepEvents []cpu.Event
	for i := range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		for _, e := range evs {
			if ps, ok := e.(cpu.EventProcessState); ok && ps.PID == 1 && ps.State == string(StateZombie) {
				exitStep = i
				exitStepEvents = evs
			}
			if ca, ok := e.(cpu.EventCoreAssign); ok && ca.PID == 2 {
				dispatchStep = i
			}
		}
		if k.BootCore().CurrentPID == 2 {
			break
		}
	}
	if k.BootCore().CurrentPID != 2 {
		t.Fatalf("CurrentPID = %d, want 2 (P1 exited and should yield to P2)", k.BootCore().CurrentPID)
	}
	if exitStep < 0 || dispatchStep < 0 {
		t.Fatalf("did not observe exit (%d) and dispatch (%d) events", exitStep, dispatchStep)
	}
	if dispatchStep <= exitStep {
		t.Errorf("P2 dispatched on Step %d but P1 exited on Step %d — dispatch must follow the exit (not precede or share its Step)", dispatchStep, exitStep)
	}
	for _, e := range exitStepEvents {
		if _, ok := e.(cpu.EventHalt); ok {
			t.Errorf("EventHalt emitted in the exit Step while a sibling was ready — the death cascade must not halt the core while a runnable sibling exists")
		}
	}
	if k.Processes[1].State != StateZombie {
		t.Errorf("P1 state = %s, want ZOMBIE", k.Processes[1].State)
	}
	if k.Processes[2].State != StateRun {
		t.Errorf("P2 state = %s, want RUN", k.Processes[2].State)
	}
	if k.Processes[1].ExitCode != 5 {
		t.Errorf("P1 ExitCode = %d, want 5", k.Processes[1].ExitCode)
	}
}

// TestSyscall_ExitWithInitParent_DoExitPhasesInOrder covers the init-emulated
// reaper path with the do_exit cascade split into discrete Steps. With
// AdoptUnderInit the only process exits and init is the only candidate left.
// The cascade must:
//
//   - Land each of the canonical do_exit steps on its own Step so the alumno
//     can click through them: mark_zombie → notify_parent (wake init) →
//     schedule_out (core release) → reap_cycle (init reaps inline,
//     kernel-emulated) → halt.
//   - NEVER emit an EventHalt before init has reaped — that would be the
//     "spurious HALT between exit and wake" pre-phase bug.
//   - Leave init.PendingReaps drained and init back in BLOCKED at the end.
//   - Leave the simulation truly idle: a Step after the terminal HALT emits
//     nothing.
func TestSyscall_ExitWithInitParent_DoExitPhasesInOrder(t *testing.T) {
	// Program: mov eax, 1 ; mov ebx, 42 ; int 0x80
	exitProg := []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00,
		0xBB, 0x2A, 0x00, 0x00, 0x00,
		0xCD, 0x80,
	}
	k := New(cpu.DefaultMemSize)
	initPID, err := k.SpawnInit()
	if err != nil {
		t.Fatalf("SpawnInit: %v", err)
	}
	pid, err := k.AddProcess("exit", exitProg)
	if err != nil {
		t.Fatalf("AddProcess: %v", err)
	}
	k.AdoptUnderInit(pid)
	k.TimerQuantum = 0

	// Stepwise: each entry is the events emitted by one Step(). The cascade
	// spans multiple Steps (one per do_exit phase) — we want both the
	// ordering between Steps AND the conventional ordering within them.
	var stepwise [][]cpu.Event
	for i := range 30 {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		stepwise = append(stepwise, evs)
		if k.BootCPU().Halted && len(k.PendingKernelWork) == 0 {
			break
		}
	}

	zombieStep, releaseStep, reapStep, haltStep := -1, -1, -1, -1
	for i, evs := range stepwise {
		for _, e := range evs {
			switch v := e.(type) {
			case cpu.EventProcessState:
				if v.PID == pid && v.State == string(StateZombie) && zombieStep == -1 {
					zombieStep = i
				}
			case cpu.EventCoreRelease:
				if v.PID == pid && releaseStep == -1 {
					releaseStep = i
				}
			case cpu.EventHalt:
				if haltStep == -1 {
					haltStep = i
				}
			case cpu.EventInitReap:
				if reapStep == -1 {
					reapStep = i
					if len(v.Reaps) != 1 || v.Reaps[0].ChildPID != pid {
						t.Errorf("EventInitReap.Reaps = %+v, want single reap of PID %d", v.Reaps, pid)
					}
					if v.Reaps[0].Reparented {
						t.Errorf("EventInitReap.Reaps[0].Reparented = true; want false (P%d was adopted by AdoptUnderInit at load time, not reparented after a death)", pid)
					}
				}
			case cpu.EventWaitOp:
				if v.Op == "reap" {
					t.Errorf("init emitted EventWaitOp{Op:reap} %+v — its reap must travel through EventInitReap only", v)
				}
			}
		}
	}

	if zombieStep < 0 || releaseStep < 0 || reapStep < 0 || haltStep < 0 {
		t.Fatalf("expected ZOMBIE (%d), core_release (%d), init_reap (%d) and HALT (%d) all in the cascade", zombieStep, releaseStep, reapStep, haltStep)
	}
	// Each canonical do_exit step lands on a later Step than the previous.
	// This is the click-by-click property the refactor delivers — pre-phase
	// the cascade collapsed all four into one Step.
	if zombieStep >= releaseStep || releaseStep >= reapStep || reapStep >= haltStep {
		t.Errorf("phase Steps: ZOMBIE@%d core_release@%d init_reap@%d HALT@%d — want strictly increasing (each do_exit phase its own Step)", zombieStep, releaseStep, reapStep, haltStep)
	}
	// Sibling property of the old "no spurious HALT" assertion: no HALT
	// emitted in any Step before the reap actually happened.
	for i := 0; i < reapStep; i++ {
		for _, e := range stepwise[i] {
			if _, ok := e.(cpu.EventHalt); ok {
				t.Errorf("EventHalt emitted on Step %d, before the reap on Step %d — halt must wait for init to drain", i, reapStep)
			}
		}
	}

	// 2. init.PendingReaps drained; the orphan child PCB is gone.
	if init := k.Processes[initPID]; init != nil {
		if len(init.PendingReaps) != 0 {
			t.Errorf("init.PendingReaps = %v after the cascade, want []", init.PendingReaps)
		}
		if init.State != StateBlocked {
			t.Errorf("init.State = %s after the cascade, want BLOCKED (re-blocked after the reap cycle)", init.State)
		}
	}
	if _, exists := k.Processes[pid]; exists {
		t.Errorf("child PID %d still in process table after the reap_cycle phase — runInitReapCycle should have removed it", pid)
	}

	// 3. Terminal HALT present.
	if !k.BootCPU().Halted {
		t.Error("boot CPU not halted after the cascade — terminal HALT missing")
	}
	if len(k.PendingKernelWork) != 0 {
		t.Errorf("PendingKernelWork = %+v after the cascade, want empty", k.PendingKernelWork)
	}

	// 4. A subsequent Step emits nothing material: the simulation is idle.
	evs, err := k.Step()
	if err != nil {
		t.Fatalf("post-exit Step: %v", err)
	}
	for _, e := range evs {
		switch e.(type) {
		case cpu.EventWaitOp, cpu.EventSchedDecision, cpu.EventPCBRestore, cpu.EventCoreAssign, cpu.EventInitReap:
			t.Errorf("post-exit Step emitted %T — cascade drain must have closed cleanly", e)
		}
	}
}

func TestSyscall_WriteRejectsKernelPointer(t *testing.T) {
	// User asks sys_write to read from 0x100000 (inside kernel). The kernel
	// must refuse with -EFAULT and a page fault event, NOT exfiltrate kernel
	// bytes to stdout.
	prog := []byte{
		0xB8, 0x04, 0x00, 0x00, 0x00, // mov eax, 4   (sys_write)
		0xBB, 0x01, 0x00, 0x00, 0x00, // mov ebx, 1   (stdout)
		0xB9, 0x00, 0x00, 0x10, 0x00, // mov ecx, 0x100000  (KERNEL!)
		0xBA, 0x04, 0x00, 0x00, 0x00, // mov edx, 4
		0xCD, 0x80, // int 0x80
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1   (sys_exit)
		0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
		0xCD, 0x80, // int 0x80
	}
	k := newTestKernel(t, prog)
	k.TimerQuantum = 0

	var sawFault bool
	var sawConsole bool
	var syscallRet uint32
	for range 50 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			switch ev := e.(type) {
			case cpu.EventPageFault:
				sawFault = true
				if ev.Addr != 0x100000 {
					t.Errorf("page fault addr = 0x%X, want 0x100000", ev.Addr)
				}
				if ev.PermNeeded != "R" {
					t.Errorf("page fault perm = %q, want R", ev.PermNeeded)
				}
			case cpu.EventConsoleOut:
				sawConsole = true
			case cpu.EventSyscall:
				if ev.Num == 4 {
					syscallRet = ev.Result
				}
			}
		}
		if k.Processes[1].State == StateZombie {
			break
		}
	}
	if !sawFault {
		t.Error("no EventPageFault emitted for kernel-pointer sys_write")
	}
	if sawConsole {
		t.Error("EventConsoleOut emitted — kernel exfiltrated bytes (security bug)")
	}
	if syscallRet != ^uint32(0) {
		t.Errorf("sys_write result = 0x%X, want 0xFFFFFFFF (-EFAULT)", syscallRet)
	}
}

func TestSyscall_ReadRejectsKernelPointer(t *testing.T) {
	// User asks sys_read to write the keyboard byte into 0x100008 (inside
	// kernel). The kernel must refuse with -EFAULT and a page fault event,
	// NOT let user clobber the IDT or kernel handlers.
	prog := []byte{
		0xB8, 0x03, 0x00, 0x00, 0x00, // mov eax, 3   (sys_read)
		0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0   (stdin)
		0xB9, 0x08, 0x00, 0x10, 0x00, // mov ecx, 0x100008 (KERNEL!)
		0xBA, 0x01, 0x00, 0x00, 0x00, // mov edx, 1
		0xCD, 0x80, // int 0x80
		0xB8, 0x01, 0x00, 0x00, 0x00, // mov eax, 1
		0xBB, 0x00, 0x00, 0x00, 0x00, // mov ebx, 0
		0xCD, 0x80, // int 0x80
	}
	k := newTestKernel(t, prog)
	k.TimerQuantum = 0
	// Pre-load a byte in the keyboard buffer so the fast path runs; the fault
	// must fire BEFORE that byte is copied.
	k.KeyboardBuf = append(k.KeyboardBuf, 'X')

	var sawFault bool
	var syscallRet uint32
	for range 50 {
		evs, err := k.Step()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			switch ev := e.(type) {
			case cpu.EventPageFault:
				sawFault = true
				if ev.PermNeeded != "W" {
					t.Errorf("page fault perm = %q, want W", ev.PermNeeded)
				}
			case cpu.EventSyscall:
				if ev.Num == 3 {
					syscallRet = ev.Result
				}
			}
		}
		if k.Processes[1].State == StateZombie {
			break
		}
	}
	if !sawFault {
		t.Error("no EventPageFault emitted for kernel-pointer sys_read")
	}
	if syscallRet != ^uint32(0) {
		t.Errorf("sys_read result = 0x%X, want 0xFFFFFFFF (-EFAULT)", syscallRet)
	}
	// Kernel memory at 0x100008 should still be whatever it was (here zero):
	// the rejected write must not have happened.
	if k.Mem.Bytes[0x100008] == 'X' {
		t.Error("kernel byte was overwritten — read clobbered memory before validating")
	}
}

// TestKillAndNotify_FaultDeathWakesWaitingParent covers the fault-kill path
// after the killAndNotify migration: a process dying by page fault must
// (1) wake its parent if the parent was blocked in waitpid, in the same
// Step as the death, and (2) deliver a wstatus that encodes the terminating
// signal (SIGSEGV in the low 7 bits, WIFSIGNALED convention) so the parent
// reaps a meaningful exit status instead of zero.
//
// We drive the kernel by hand instead of through bytecode to keep the test
// focused on the cleanup path: bytecode fork + waitpid is exercised by
// wait_test.go, what matters here is the new convergence between sys_exit
// and fault-kill on exitProcess.
func TestKillAndNotify_FaultDeathWakesWaitingParent(t *testing.T) {
	k := New(cpu.DefaultMemSize)
	parentPID, err := k.AddProcess("parent", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess parent: %v", err)
	}
	childPID, err := k.AddProcess("child", []byte{0xF4})
	if err != nil {
		t.Fatalf("AddProcess child: %v", err)
	}

	parent := k.Processes[parentPID]
	child := k.Processes[childPID]
	parent.Children = []int{childPID}
	child.ParentPID = parentPID

	// Park the parent in waitpid(-1) so the death wakes it.
	parent.State = StateBlocked
	parent.BlockedOn = waitpidBlockedReason(-1)
	parent.Waiter = -1
	// Clear parent from the ready queue (AddProcess pushed it there).
	k.Ready.Remove(parentPID)

	// Drive the child onto the boot core so killAndNotify acts on it.
	boot := k.BootCore()
	boot.CurrentPID = childPID
	child.State = StateRun
	child.Core = boot.ID
	// Remove child from the ready queue as well — it would not be there
	// while running, but AddProcess pushed it during setup.
	k.Ready.Remove(childPID)

	// Simulate the fault: handler stamps SIGSEGV before delegating to exitProcess.
	k.setFaultExitCode(boot, SIGSEGV)
	evs := k.killAndNotify(boot, "page_fault")
	// killAndNotify now stages the five do_exit phases and drains phase 1
	// (mark_zombie) inline. The remaining phases — notify_parent (which
	// wakes the waiting parent), schedule_out (dispatches it), reap_cycle
	// (drains the pending reap) — fire on subsequent Steps. Drain them
	// here so the assertions below describe the end state of the cascade.
	stepwise := stepUntilWorkDrained(t, k, 6)
	evs = append(evs, flatten(stepwise)...)

	// 1. Parent ended up RUN on the core after the cascade drained: the
	//    notify_parent phase woke it BLOCKED → READY, schedule_out
	//    dispatched it onto the boot core, reap_cycle stamped EAX.
	if parent.State != StateRun {
		t.Errorf("parent.State = %s, want RUN (woken parent dispatched after the cascade)", parent.State)
	}
	if boot.CurrentPID != parentPID {
		t.Errorf("boot.CurrentPID = %d, want %d (parent dispatched to the core)", boot.CurrentPID, parentPID)
	}

	// 2. The reap event carries WIFSIGNALED(SIGSEGV): low 7 bits == 11.
	var reaped *cpu.EventWaitOp
	for i, e := range evs {
		if w, ok := e.(cpu.EventWaitOp); ok && w.Op == "reap" && w.ChildPID == childPID {
			ev := w
			reaped = &ev
			_ = i
			break
		}
	}
	if reaped == nil {
		t.Fatalf("no EventWaitOp{Op:reap} for child PID %d — fault-death must surface as a regular reap to the waiting parent", childPID)
	}
	if reaped.ExitCode&0x7f != SIGSEGV {
		t.Errorf("EventWaitOp.ExitCode = 0x%X, want low 7 bits == SIGSEGV (%d) per WIFSIGNALED convention", reaped.ExitCode, SIGSEGV)
	}
	if reaped.ExitCode&0x80 != 0 {
		t.Errorf("EventWaitOp.ExitCode = 0x%X has WCOREDUMP bit set; simulator does not model core dumps so bit 0x80 must stay clear", reaped.ExitCode)
	}

	// 3. Parent's EAX carries the reaped child PID (waitpid return value),
	//    not the status — that lives in the wstatus / EventWaitOp.ExitCode.
	if parent.Regs.EAX != uint32(childPID) {
		t.Errorf("parent.Regs.EAX = %d, want %d (reaped PID; status goes through EventWaitOp)", parent.Regs.EAX, childPID)
	}

	// 4. Child PCB removed from the table (consumePendingReaps did its job).
	if _, exists := k.Processes[childPID]; exists {
		t.Errorf("child PID %d still in process table after reap", childPID)
	}
}

// TestSyscall_ArgsArityMatchesContract pins the syscall → arity table so a
// regression that re-introduces trailing-register noise in EventSyscall.Args
// fails loudly. The frontend trusts the array length to format the log line;
// padding it with stale ECX/EDX would resurrect the pre-refactor pattern
// "sys_exit(10, 0xa, 0)" that confused the alumno into thinking exit was
// ternary.
func TestSyscall_ArgsArityMatchesContract(t *testing.T) {
	cases := []struct {
		name string
		num  uint32
		want int
	}{
		{"exit", SysExit, 1},
		{"fork", SysFork, 0},
		{"vfork", SysVfork, 0},
		{"pipe", SysPipe, 1},
		{"mknod", SysMknod, 1},
		{"open", SysOpen, 2},
		{"waitpid", SysWaitpid, 2},
		{"write", SysWrite, 3},
		{"read", SysRead, 3},
	}
	for _, tc := range cases {
		if got := len(syscallArgs(tc.num, 0xAA, 0xBB, 0xCC)); got != tc.want {
			t.Errorf("syscall %s (num=%d): len(args) = %d, want %d", tc.name, tc.num, got, tc.want)
		}
	}
}
