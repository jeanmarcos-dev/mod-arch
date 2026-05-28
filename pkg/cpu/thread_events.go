package cpu

// EventThreadCreate is emitted when clone() runs with CLONE_THREAD: the new
// task is a sibling thread within the same thread group, not a child process.
// The frontend uses it to fold the new TID into the leader's subtree of the
// process panel instead of rendering it as a new fork branch.
type EventThreadCreate struct {
	TID       int    `json:"tid"`
	TGID      int    `json:"tgid"`
	LeaderPID int    `json:"leader_pid"`
	CreatorID int    `json:"creator_pid"`
	FnEIP     uint32 `json:"fn_eip"`
	StackTop  uint32 `json:"stack_top"`
}

// Tag returns the event tag.
func (EventThreadCreate) Tag() string { return "thread_create" }

// EventThreadExit is emitted when a task whose TGID differs from its PID
// terminates — i.e., when a clone(CLONE_THREAD)-spawned sibling reaches
// sys_exit. EventProcessState{State:ZOMBIE} still fires alongside; the
// dedicated event lets the UI distinguish a thread terminating from a
// full process exiting.
type EventThreadExit struct {
	TID    int    `json:"tid"`
	TGID   int    `json:"tgid"`
	Retval uint32 `json:"retval"`
}

// Tag returns the event tag.
func (EventThreadExit) Tag() string { return "thread_exit" }
