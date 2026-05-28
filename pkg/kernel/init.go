package kernel

import (
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
)

// initProgram is the body of PID 1. Two bytes of `hlt` keep loadCPUFromPCB
// happy (the slot must hold at least one byte of valid code). init never
// actually executes them: SpawnInit parks the PCB in BLOCKED("wait()") right
// after registering and the dispatcher returns init to that state after
// each zombie reap.
var initProgram = []byte{0xF4, 0xF4}

// initBlockedReason names the eternal wait loop init lives in. We use
// "wait(any)" to make the UI distinguish init's universal-reaper role
// from a regular sys_waitpid that targets a specific child (which shows
// up as "wait(P{n})"). sysvinit loops on wait(); systemd uses an epoll
// loop driven by SIGCHLD that is semantically the same thing.
const initBlockedReason = "wait(any)"

// initStackTop is the initial ESP for init's tiny kernel-side stack. Init
// never enters user mode in a way that consumes stack, but loadCPUFromPCB
// still needs a valid ESP. We carve a small region just past initCodeBase
// inside the kernel zone so it does not collide with the per-core kernel
// stacks (which grow downward from KernelStackTop).
const initStackTop uint32 = initCodeBase + 0x100

// SpawnInit allocates the init PCB (always PID 1), parks it in
// BLOCKED("wait()") so the scheduler never picks it, and records its PID
// in k.InitPID. Now invoked unconditionally by sim.LoadProgram so every
// demo carries init in the background.
//
// Unlike AddProcess, SpawnInit does NOT consume a user-code slot — init
// lives at initCodeBase inside the kernel zone, so demo procs loaded
// afterwards keep landing at 0x200000 (slot 0). That preserves hardcoded
// jump addresses inside demo bytecode (e.g. clone() entry points).
//
// Safe to call once per kernel; calling it twice is a no-op that returns
// the existing PID.
func (k *Kernel) SpawnInit() (int, error) {
	if k.InitPID != 0 {
		return k.InitPID, nil
	}
	pid := k.nextPID
	k.nextPID++

	// Load init's two HLT bytes into the reserved kernel-side code region
	// so the CPU has something to read if it ever resumes init. Zeroing the
	// guard area defensively keeps stale bytes from a prior demo out of view.
	for i := initCodeBase; i < initCodeBase+uint32(len(initProgram)); i++ {
		k.Mem.Bytes[i] = 0
	}
	if err := k.Mem.LoadBytes(initCodeBase, initProgram); err != nil {
		return 0, err
	}

	pcb := &PCB{
		PID:       pid,
		Name:      "init",
		State:     StateBlocked,
		BlockedOn: initBlockedReason,
		Mode:      cpu.ModeUser,
		StackTop:  initStackTop,
		Slot:      -1,
		Core:      -1,
		TGID:      pid,
		memSpace:  k.newMemSpace(initCodeBase, uint32(len(initProgram)), initCodeBase+0x80),
		fdSpace:   k.newFDSpace(),
	}
	pcb.Regs.EIP = initCodeBase
	pcb.Regs.ESP = initStackTop
	pcb.Regs.EFLAGS = cpu.FlagIF
	pcb.Regs.CS = cpu.SelUserCode
	pcb.Regs.DS = cpu.SelUserData
	pcb.Regs.SS = cpu.SelUserData

	k.Processes[pid] = pcb
	k.InitPID = pid
	return pid, nil
}

// AdoptUnderInit re-parents a freshly loaded PCB so it appears as a
// child of init in the process tree. Intended for the simulator's demo
// loader (WithInit demos) — AddProcess on its own leaves ParentPID=0,
// which would make the demo proc a sibling of init rather than a child.
//
// Safe to call after AddProcess; idempotent if the relationship already
// exists.
func (k *Kernel) AdoptUnderInit(pid int) {
	if k.InitPID == 0 || pid == k.InitPID {
		return
	}
	child := k.Processes[pid]
	init := k.Processes[k.InitPID]
	if child == nil || init == nil {
		return
	}
	if child.ParentPID == k.InitPID {
		return
	}
	child.ParentPID = k.InitPID
	for _, c := range init.Children {
		if c == pid {
			return
		}
	}
	init.Children = append(init.Children, pid)
}
