package cpu

import "testing"

func TestEventTags(t *testing.T) {
	cases := []struct {
		ev   Event
		want string
	}{
		{EventInstruction{}, "instruction"},
		{EventRegWrite{}, "reg_write"},
		{EventMemRead{}, "mem_read"},
		{EventMemWrite{}, "mem_write"},
		{EventModeSwitch{}, "mode_switch"},
		{EventIRQFire{}, "irq_fire"},
		{EventPageFault{}, "page_fault"},
		{EventHalt{}, "halt"},
		{EventPCBSave{}, "pcb_save"},
		{EventPCBRestore{}, "pcb_restore"},
		{EventSchedDecision{}, "sched_decision"},
		{EventProcessState{}, "process_state"},
		{EventSyscall{}, "syscall"},
		{EventConsoleOut{}, "console_out"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.ev.Tag(); got != tc.want {
				t.Errorf("%T.Tag() = %q, want %q", tc.ev, got, tc.want)
			}
		})
	}
}
