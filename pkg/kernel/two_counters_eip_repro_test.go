package kernel

import (
	"fmt"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

type instrLogRepro struct {
	tick     uint64
	coreID   int
	eip      uint32
	mnemonic string
	bytes    []byte
	modeAt   uint8
}

// TestTwoCounters_HandlerEIPStaysAt101020 reproduces the two_counters demo
// (two spin-counters on two cores) and asserts that every NOP emitted in
// kernel mode lands at the timer-handler entry address 0x101020 — never at
// 0x101023 or anything else inside the kernel-text region. The frontend
// reported `EIP=0x101024 INSTR=NOP` on C0, which would imply a NOP at
// 0x101023; this test exists to catch any such anomaly at the source.
func TestTwoCounters_HandlerEIPStaysAt101020(t *testing.T) {
	k := NewMulti(cpu.DefaultMemSize, 2)
	k.SpawnInit()
	k.SpawnKThreadd()
	k.SpawnKernelThreads()

	// two_counters byte-for-byte: P1 inc EAX in a loop, P2 inc EBX in a loop.
	// MOV r32, 0  (5 bytes B8/BB + imm32)
	// loop: INC r32  (1 byte 40/43)
	//       JMP loop (2 bytes EB FD)  ; -3 offset
	p1Bytes := []byte{
		0xB8, 0x00, 0x00, 0x00, 0x00, // MOV EAX, 0
		0x40,       // INC EAX
		0xEB, 0xFD, // JMP -3
	}
	p2Bytes := []byte{
		0xBB, 0x00, 0x00, 0x00, 0x00, // MOV EBX, 0
		0x43,       // INC EBX
		0xEB, 0xFD, // JMP -3
	}

	if _, err := k.AddProcess("p1_count_eax", p1Bytes); err != nil {
		t.Fatalf("AddProcess p1: %v", err)
	}
	if _, err := k.AddProcess("p2_count_ebx", p2Bytes); err != nil {
		t.Fatalf("AddProcess p2: %v", err)
	}

	// Dump kernel-text bytes before stepping. If any path mutates them at
	// runtime we will spot the diff later.
	kernelTextBefore := append([]byte(nil), k.Mem.Bytes[cpu.AddrKernelText:cpu.AddrKernelText+0x40]...)
	t.Logf("kernel-text[0x101000..0x101040] BEFORE: % X", kernelTextBefore)

	const totalTicks = 250

	var logs []instrLogRepro

	// Capture every event that touches EIP, indexed in arrival order so we can
	// see exactly what the frontend would consume.
	type eipTouch struct {
		tick    uint64
		coreID  int
		source  string // "instruction" | "reg_write" | "pcb_save" | "pcb_restore" | "mode_switch"
		eipOld  uint32
		eipNew  uint32 // for instruction this is ev.eip + len(ev.bytes)
		extra   string
	}
	var touches []eipTouch

	processEvent := func(e cpu.Event) {
		recordInstr(&logs, e, k)
		switch v := e.(type) {
		case cpu.EventInstruction:
			next := v.EIP + uint32(len(v.Bytes))
			touches = append(touches, eipTouch{
				tick: v.Tick, coreID: v.CoreID, source: "instruction",
				eipNew: next,
				extra: fmt.Sprintf("eip=0x%08X mnem=%s bytes=%s", v.EIP, v.Mnemonic, formatBytes(v.Bytes)),
			})
		case cpu.EventRegWrite:
			if v.Reg != "EIP" {
				return
			}
			touches = append(touches, eipTouch{
				tick: v.Tick, coreID: v.CoreID, source: "reg_write",
				eipOld: v.Old, eipNew: v.New,
			})
		case cpu.EventPCBSave:
			if eip, ok := v.Regs["EIP"]; ok {
				touches = append(touches, eipTouch{
					tick: v.Tick, coreID: v.CoreID, source: "pcb_save",
					eipNew: eip,
					extra: fmt.Sprintf("pid=%d", v.PID),
				})
			}
		case cpu.EventPCBRestore:
			if eip, ok := v.Regs["EIP"]; ok {
				touches = append(touches, eipTouch{
					tick: v.Tick, coreID: v.CoreID, source: "pcb_restore",
					eipNew: eip,
					extra: fmt.Sprintf("pid=%d", v.PID),
				})
			}
		}
	}

	for _, e := range k.DispatchIdleCores() {
		processEvent(e)
	}

	for i := 0; i < totalTicks; i++ {
		evs, err := k.Step()
		if err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		for _, e := range evs {
			processEvent(e)
		}
	}

	// Only flag EIPs the frontend could PERSIST. Inside the IRET sequence
	// the frontend transiently sees nextEIP=0x101022 (from
	// `instruction(IRET @0x101021, bytes=[CF])` → eip+len) but the very next
	// event is `reg_write(EIP, new=user_eip)` from setEIP(eip_from_frame),
	// which overwrites it immediately — the frontend never settles at
	// 0x101022. The genuine anomaly to catch is 0x101024 (the symptom
	// reported by the user) or anything else in the kernel text dead zones.
	anomalies := 0
	for _, tc := range touches {
		// Allow the legitimate kernel addresses: 0x101000 (divide-error iret),
		// 0x101010/0x101011, 0x101020/0x101021, 0x101030/0x101031,
		// 0x101040/0x101041/0x101042, 0x101050/0x101051/0x101052,
		// 0x101060/0x101061, plus the natural "+1 after IRET" landing points.
		eip := tc.eipNew
		validKernelEIPs := map[uint32]bool{
			0x00101000: true, 0x00101001: true, // divide error IRET
			0x00101010: true, 0x00101011: true, 0x00101012: true, // page fault NOP+IRET
			0x00101020: true, 0x00101021: true, 0x00101022: true, // timer NOP+IRET (post-IRET transient)
			0x00101030: true, 0x00101031: true, 0x00101032: true, // keyboard
			0x00101040: true, 0x00101041: true, 0x00101042: true, 0x00101043: true, // syscall NOP NOP IRET
			0x00101050: true, 0x00101051: true, 0x00101052: true, 0x00101053: true, // sync
			0x00101060: true, 0x00101061: true, 0x00101062: true, // gp
		}
		if eip >= 0x00101000 && eip < 0x00101070 && !validKernelEIPs[eip] {
			anomalies++
			t.Errorf("[ANOMALY] tick=%d core=%d source=%s eip_old=0x%08X eip_new=0x%08X %s",
				tc.tick, tc.coreID, tc.source, tc.eipOld, tc.eipNew, tc.extra)
		}
	}
	t.Logf("Total EIP touches: %d, anomalies inside kernel-text dead-zones: %d", len(touches), anomalies)

	// Dump every EIP touch on core 0 around any kernel-text address so we can
	// trace what the frontend sees.
	t.Logf("--- All EIP touches on core 0 inside kernel text ---")
	for _, tc := range touches {
		if tc.coreID != 0 {
			continue
		}
		if tc.eipNew < 0x00101000 || tc.eipNew > 0x00101080 {
			continue
		}
		t.Logf("  tick=%4d source=%-12s old=0x%08X new=0x%08X  %s",
			tc.tick, tc.source, tc.eipOld, tc.eipNew, tc.extra)
	}

	// Memory diff after the run — nothing should have written into kernel text.
	kernelTextAfter := k.Mem.Bytes[cpu.AddrKernelText : cpu.AddrKernelText+0x40]
	if string(kernelTextBefore) != string(kernelTextAfter) {
		t.Errorf("kernel text mutated during run\n  before: % X\n  after:  % X",
			kernelTextBefore, kernelTextAfter)
	}
	t.Logf("kernel-text[0x101000..0x101040] AFTER : % X", kernelTextAfter)

	// Per-core final state.
	for _, core := range k.Cores {
		t.Logf("Core %d final: Mode=%d EIP=0x%08X ESP=0x%08X CS=0x%04X SS=0x%04X DS=0x%04X PID=%d",
			core.ID, core.CPU.Mode, core.CPU.R.EIP, core.CPU.R.ESP,
			core.CPU.R.CS, core.CPU.R.SS, core.CPU.R.DS, core.CurrentPID)
	}

	// Audit: every NOP emitted with EIP inside kernel text must be at
	// AddrKernelText + 0x20 (= 0x101020, timer handler entry). Anything
	// else means the dispatcher landed on a stray byte.
	const timerHandler = cpu.AddrKernelText + 0x20
	violations := 0
	nopByCore := map[int]int{}
	for _, l := range logs {
		if l.mnemonic != "NOP" {
			continue
		}
		// Only flag NOPs inside kernel text (user procs do not have NOPs).
		if l.eip < cpu.AddrKernelText || l.eip >= cpu.AddrKernelText+0x70 {
			continue
		}
		nopByCore[l.coreID]++
		if l.eip != timerHandler {
			violations++
			t.Errorf("[VIOLATION] tick=%d core=%d emitted NOP at EIP=0x%08X (expected 0x%08X); bytes=% X",
				l.tick, l.coreID, l.eip, timerHandler, l.bytes)
		}
	}
	t.Logf("NOPs emitted: core0=%d core1=%d (total kernel NOPs: %d, violations: %d)",
		nopByCore[0], nopByCore[1], nopByCore[0]+nopByCore[1], violations)

	// Print the first 25 instructions of each core in chronological order to
	// see the boot + early loop + first IRQ entry.
	type coreView struct {
		id    int
		items []instrLogRepro
	}
	views := map[int]*coreView{}
	for _, l := range logs {
		v, ok := views[l.coreID]
		if !ok {
			v = &coreView{id: l.coreID}
			views[l.coreID] = v
		}
		v.items = append(v.items, l)
	}
	for _, id := range []int{0, 1} {
		v := views[id]
		if v == nil {
			continue
		}
		n := len(v.items)
		if n > 40 {
			n = 40
		}
		t.Logf("--- Core %d, first %d instructions emitted ---", id, n)
		for i := 0; i < n; i++ {
			l := v.items[i]
			t.Logf("  tick=%4d mode=%d eip=0x%08X bytes=%s  %s",
				l.tick, l.modeAt, l.eip, formatBytes(l.bytes), l.mnemonic)
		}
	}
}

func recordInstr(logs *[]instrLogRepro, e cpu.Event, k *Kernel) {
	ev, ok := e.(cpu.EventInstruction)
	if !ok {
		return
	}
	mode := uint8(0)
	for _, c := range k.Cores {
		if c.ID == ev.CoreID {
			mode = c.CPU.Mode
			break
		}
	}
	bytesCopy := append([]byte(nil), ev.Bytes...)
	*logs = append(*logs, instrLogRepro{
		tick:     ev.Tick,
		coreID:   ev.CoreID,
		eip:      ev.EIP,
		mnemonic: ev.Mnemonic,
		bytes:    bytesCopy,
		modeAt:   mode,
	})
}

func formatBytes(bs []byte) string {
	if len(bs) == 0 {
		return "[]"
	}
	out := "["
	for i, b := range bs {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprintf("%02X", b)
	}
	return out + "]"
}
