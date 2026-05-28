package kernel

// initProgram is the user-mode body of PID 1. Two bytes of `hlt` keep
// AddProcess happy (the slot must hold at least one byte of valid code).
// Init never actually executes them: SpawnInit parks the PCB in
// BLOCKED("wait()") right after registering and the dispatcher returns
// init to that state after each zombie reap.
var initProgram = []byte{0xF4, 0xF4}

// initBlockedReason mirrors the wait() syscall name a real init would
// spend its life inside (sysvinit loops on wait(); systemd uses an
// epoll loop driven by SIGCHLD that semantically does the same thing).
// Sharing the string with blockedOnWait keeps the UI honest about what
// state init is in.
const initBlockedReason = "wait()"

// SpawnInit allocates the init PCB (typically PID 1), parks it in
// BLOCKED("idle") so the scheduler never picks it, and records its PID
// in k.InitPID. Demos that exercise fork / vfork / pipes call this
// before loading their own processes so the tree always has a root and
// so re-parenting in killCurrent has a target.
//
// Safe to call once per kernel; calling it twice is a no-op that returns
// the existing PID.
func (k *Kernel) SpawnInit() (int, error) {
	if k.InitPID != 0 {
		return k.InitPID, nil
	}
	pid, err := k.AddProcess("init", initProgram)
	if err != nil {
		return 0, err
	}
	k.InitPID = pid

	// Park init: BLOCKED keeps the scheduler from ever choosing it and
	// makes AllHalted reachable once user demos finish. Without this,
	// init's HLT loop would still occupy the boot core forever.
	init := k.Processes[pid]
	if init != nil {
		init.State = StateBlocked
		init.BlockedOn = initBlockedReason
	}
	k.Ready.Remove(pid)
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
