package sim

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// varsLocked builds the per-program variable payload that ships in every
// snapshot. The caller MUST hold s.mu.
//
// Shape of the returned map:
//
//	{
//	  "<pid>": {                       // leader PID of the program
//	    "globals": [ {name,type,addr,len,value_hex,value_dec,unmapped}, ... ],
//	    "threads": [
//	      { tid, label, name, func, in_scope, ebp, regs:{EAX,EBX,...},
//	        locals: [ {name,type,loc:{kind,addr,reg,offset},value_hex,value_dec,
//	                    unset,unmapped} ] }
//	    ]
//	  }
//	}
//
// Empty when no demos with variable metadata are loaded.
func (s *Simulator) varsLocked() map[string]any {
	if len(s.procGlobals) == 0 && len(s.procLocals) == 0 {
		return nil
	}
	leaders := make([]int, 0, len(s.procGlobals)+len(s.procLocals))
	seen := map[int]bool{}
	for pid := range s.procGlobals {
		if !seen[pid] {
			leaders = append(leaders, pid)
			seen[pid] = true
		}
	}
	for pid := range s.procLocals {
		if !seen[pid] {
			leaders = append(leaders, pid)
			seen[pid] = true
		}
	}
	sort.Ints(leaders)

	out := make(map[string]any, len(leaders))
	for _, leaderPID := range leaders {
		entry := map[string]any{
			"globals": s.globalSnapshotsLocked(s.procGlobals[leaderPID]),
			"threads": s.threadSnapshotsLocked(leaderPID),
		}
		out[fmt.Sprintf("%d", leaderPID)] = entry
	}
	return out
}

// globalSnapshotsLocked reads the current value of each declared global from
// physical memory. The caller MUST hold s.mu.
func (s *Simulator) globalSnapshotsLocked(globals []Global) []map[string]any {
	out := make([]map[string]any, 0, len(globals))
	for _, g := range globals {
		entry := map[string]any{
			"name": g.Name,
			"type": string(g.Type),
			"addr": fmt.Sprintf("0x%X", g.Addr),
			"len":  g.Len,
		}
		bytes, ok := s.readMemLocked(g.Addr, g.Len)
		if !ok {
			entry["unmapped"] = true
		} else {
			hex, dec := formatBytes(bytes, g.Type)
			entry["value_hex"] = hex
			entry["value_dec"] = dec
		}
		out = append(out, entry)
	}
	return out
}

// threadSnapshotsLocked emits one entry per PCB belonging to the leader's
// thread group, ordered by PID. The caller MUST hold s.mu.
func (s *Simulator) threadSnapshotsLocked(leaderPID int) []map[string]any {
	tgid := s.thgroupOfLocked(leaderPID)
	threads := s.threadsInGroupLocked(tgid)
	locals := s.procLocals[leaderPID]
	funcs := s.procFuncs[leaderPID]

	// One PCB in the group means "ordinary process"; the chip should read
	// "P<pid>". Two or more is "thread group"; siblings deserve "T<pid>" so
	// the student can see they share an address space.
	isThreadGroup := len(threads) > 1
	out := make([]map[string]any, 0, len(threads))
	for _, p := range threads {
		out = append(out, s.threadSnapshotLocked(p, locals, funcs, isThreadGroup))
	}
	return out
}

// threadSnapshotLocked builds one thread entry: which function it is in,
// whether the corresponding scope is active, and the resolved value of every
// local declared in that scope.
func (s *Simulator) threadSnapshotLocked(p *kernel.PCB, locals []Local, funcs []Func, isThreadGroup bool) map[string]any {
	regs, codeBase := s.threadRegsLocked(p)
	currentEIP := regs.EIP

	fn, inScope := funcFor(currentEIP, codeBase, funcs)
	entry := map[string]any{
		"tid":      p.PID,
		"label":    threadLabel(p, isThreadGroup),
		"name":     p.Name,
		"in_scope": inScope,
		"ebp":      fmt.Sprintf("0x%X", regs.EBP),
		"regs": map[string]uint32{
			"EAX": regs.EAX, "EBX": regs.EBX, "ECX": regs.ECX, "EDX": regs.EDX,
			"ESI": regs.ESI, "EDI": regs.EDI, "EBP": regs.EBP, "ESP": regs.ESP,
			"EIP": regs.EIP, "EFLAGS": regs.EFLAGS,
		},
	}
	if inScope {
		entry["func"] = fn.Name
	}

	var scopedLocals []Local
	if inScope {
		for _, l := range locals {
			if l.Func == fn.Name {
				scopedLocals = append(scopedLocals, l)
			}
		}
	}
	entry["locals"] = s.localSnapshotsLocked(scopedLocals, regs, inScope)
	return entry
}

// localSnapshotsLocked resolves each local to its current value using the
// thread's registers and the kernel memory image.
func (s *Simulator) localSnapshotsLocked(locals []Local, regs cpu.Registers, inScope bool) []map[string]any {
	out := make([]map[string]any, 0, len(locals))
	for _, l := range locals {
		entry := map[string]any{
			"name": l.Name,
			"type": string(l.Type),
			"func": l.Func,
			"loc": map[string]any{
				"kind":   string(l.Loc.Kind),
				"addr":   l.Loc.Addr,
				"reg":    l.Loc.Reg,
				"offset": l.Loc.Offset,
			},
		}
		if !inScope {
			entry["unset"] = true
			out = append(out, entry)
			continue
		}
		switch l.Loc.Kind {
		case VarLocReg:
			val, ok := readRegByName(regs, l.Loc.Reg)
			if !ok {
				entry["unmapped"] = true
				break
			}
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, val)
			hex, dec := formatBytes(b, l.Type)
			entry["value_hex"] = hex
			entry["value_dec"] = dec
		case VarLocMem:
			b, ok := s.readMemLocked(l.Loc.Addr, sizeOf(l.Type))
			if !ok {
				entry["unmapped"] = true
				break
			}
			hex, dec := formatBytes(b, l.Type)
			entry["value_hex"] = hex
			entry["value_dec"] = dec
		case VarLocStack:
			if regs.EBP == 0 {
				entry["unset"] = true
				break
			}
			addr := uint32(int64(regs.EBP) + int64(l.Loc.Offset))
			b, ok := s.readMemLocked(addr, sizeOf(l.Type))
			if !ok {
				entry["unmapped"] = true
				break
			}
			entry["resolved_addr"] = fmt.Sprintf("0x%X", addr)
			hex, dec := formatBytes(b, l.Type)
			entry["value_hex"] = hex
			entry["value_dec"] = dec
		}
		out = append(out, entry)
	}
	return out
}

// threadRegsLocked returns the live register file for a PCB. When the thread
// is currently RUN on some core, the live values are in core.CPU.R; otherwise
// the PCB's saved Regs are authoritative. ESP follows the same dual rule.
func (s *Simulator) threadRegsLocked(p *kernel.PCB) (cpu.Registers, uint32) {
	var codeBase uint32
	if ms := p.MemSpace(); ms != nil {
		codeBase = ms.CodeBase
	}
	for _, core := range s.kernel.Cores {
		if core.CurrentPID == p.PID && !core.RunningIdle {
			return core.CPU.R, codeBase
		}
	}
	regs := p.Regs
	if regs.ESP == 0 {
		regs.ESP = p.SavedUserESP
	}
	return regs, codeBase
}

// thgroupOfLocked returns the TGID associated with a PCB; falls back to the
// PID itself when the PCB is missing (a load+immediate-reset edge case).
func (s *Simulator) thgroupOfLocked(pid int) int {
	if p, ok := s.kernel.Processes[pid]; ok && p.TGID != 0 {
		return p.TGID
	}
	return pid
}

// threadsInGroupLocked returns every PCB whose TGID matches the leader's,
// ordered by PID so the UI gets a stable thread list.
func (s *Simulator) threadsInGroupLocked(tgid int) []*kernel.PCB {
	out := make([]*kernel.PCB, 0)
	for pid, p := range s.kernel.Processes {
		if p == nil {
			continue
		}
		if p.TGID == tgid || (p.TGID == 0 && pid == tgid) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// readMemLocked copies length bytes starting at addr from the kernel's memory
// image. The caller MUST hold s.mu. Returns false when the range exceeds the
// mapped memory (e.g. uninitialised globals past the demo's last byte).
func (s *Simulator) readMemLocked(addr, length uint32) ([]byte, bool) {
	if length == 0 {
		return nil, false
	}
	mem := s.kernel.Mem.Bytes
	if int(addr) >= len(mem) {
		return nil, false
	}
	end := uint64(addr) + uint64(length)
	if end > uint64(len(mem)) {
		return nil, false
	}
	return append([]byte(nil), mem[addr:end]...), true
}

// funcFor finds the Func whose [StartOffset, EndOffset) range contains the
// current relative EIP. Returns false when EIP is outside the program's code
// (kernel mode, post-exit, etc.) or no Funcs are declared.
func funcFor(eip, codeBase uint32, funcs []Func) (Func, bool) {
	if codeBase == 0 || len(funcs) == 0 {
		return Func{}, false
	}
	if eip < codeBase {
		return Func{}, false
	}
	rel := eip - codeBase
	for _, f := range funcs {
		end := f.EndOffset
		if end == 0 || end <= f.StartOffset {
			continue
		}
		if rel >= f.StartOffset && rel < end {
			return f, true
		}
	}
	return Func{}, false
}

// readRegByName resolves a canonical register name (uppercase: "EAX", "EBX",
// …) against a Registers value. Unknown names return false.
func readRegByName(r cpu.Registers, name string) (uint32, bool) {
	switch name {
	case "EAX":
		return r.EAX, true
	case "EBX":
		return r.EBX, true
	case "ECX":
		return r.ECX, true
	case "EDX":
		return r.EDX, true
	case "ESI":
		return r.ESI, true
	case "EDI":
		return r.EDI, true
	case "EBP":
		return r.EBP, true
	case "ESP":
		return r.ESP, true
	}
	return 0, false
}

// sizeOf returns the number of bytes the simulator reads for the given type.
// Anything larger than 4 bytes is currently out of scope.
func sizeOf(t VarType) uint32 {
	switch t {
	case VarChar:
		return 1
	default:
		return 4
	}
}

// formatBytes converts a byte slice to a (hex, decimal) display pair based on
// the declared C type. Hex is always little-endian-decoded to a fixed-width
// representation; the decimal channel carries the type-appropriate reading
// (signed for int, unsigned for uint/ptr, ASCII glyph for char).
func formatBytes(b []byte, t VarType) (string, string) {
	switch t {
	case VarChar:
		if len(b) == 0 {
			return "0x00", ""
		}
		ch := b[0]
		dec := fmt.Sprintf("%d", ch)
		if ch >= 32 && ch < 127 {
			dec = fmt.Sprintf("'%c' (%d)", ch, ch)
		}
		return fmt.Sprintf("0x%02X", ch), dec
	}
	var u uint32
	for i := 0; i < len(b) && i < 4; i++ {
		u |= uint32(b[i]) << (8 * uint(i))
	}
	hex := fmt.Sprintf("0x%08X", u)
	switch t {
	case VarInt:
		return hex, fmt.Sprintf("%d", int32(u))
	case VarPtr:
		return hex, hex
	default: // VarUint and unknowns
		return hex, fmt.Sprintf("%d", u)
	}
}

// threadLabel returns the chip label the UI uses. Plain processes (single
// PCB) read "P<pid>" so the panel doesn't lie about there being threads
// involved; multi-PCB groups read "T<pid>" to surface the thread-of-process
// distinction. Idle and kernel-side tasks always use "P<pid>".
func threadLabel(p *kernel.PCB, isThreadGroup bool) string {
	if p.IsIdle() || p.IsKernelThread() {
		return fmt.Sprintf("P%d", p.PID)
	}
	if isThreadGroup {
		return fmt.Sprintf("T%d", p.PID)
	}
	return fmt.Sprintf("P%d", p.PID)
}

// varMetaPayload emits a static description of every declared variable so the
// frontend can tokenize the C source against the known names even before the
// first runtime snapshot arrives. Shape:
//
//	{ "<leader_pid>": {
//	    "globals": [ {name,type,addr,len} ],
//	    "funcs":   [ {name,start_offset,end_offset} ],
//	    "locals":  [ {name,type,func,loc:{kind,addr,reg,offset}} ]
//	  } }
//
// Returns nil when no PID has any metadata, mirroring varsLocked.
func varMetaPayload(globals map[int][]Global, locals map[int][]Local, funcs map[int][]Func) map[string]any {
	if len(globals) == 0 && len(locals) == 0 && len(funcs) == 0 {
		return nil
	}
	pids := make([]int, 0, len(globals)+len(locals)+len(funcs))
	seen := map[int]bool{}
	for pid := range globals {
		if !seen[pid] {
			pids = append(pids, pid)
			seen[pid] = true
		}
	}
	for pid := range locals {
		if !seen[pid] {
			pids = append(pids, pid)
			seen[pid] = true
		}
	}
	for pid := range funcs {
		if !seen[pid] {
			pids = append(pids, pid)
			seen[pid] = true
		}
	}
	sort.Ints(pids)

	out := make(map[string]any, len(pids))
	for _, pid := range pids {
		entry := map[string]any{}
		if gs := globals[pid]; len(gs) > 0 {
			arr := make([]map[string]any, 0, len(gs))
			for _, g := range gs {
				arr = append(arr, map[string]any{
					"name": g.Name,
					"type": string(g.Type),
					"addr": fmt.Sprintf("0x%X", g.Addr),
					"len":  g.Len,
				})
			}
			entry["globals"] = arr
		}
		if ls := locals[pid]; len(ls) > 0 {
			arr := make([]map[string]any, 0, len(ls))
			for _, l := range ls {
				arr = append(arr, map[string]any{
					"name": l.Name,
					"type": string(l.Type),
					"func": l.Func,
					"loc": map[string]any{
						"kind":   string(l.Loc.Kind),
						"addr":   l.Loc.Addr,
						"reg":    l.Loc.Reg,
						"offset": l.Loc.Offset,
					},
				})
			}
			entry["locals"] = arr
		}
		if fs := funcs[pid]; len(fs) > 0 {
			arr := make([]map[string]any, 0, len(fs))
			for _, f := range fs {
				arr = append(arr, map[string]any{
					"name":         f.Name,
					"start_offset": f.StartOffset,
					"end_offset":   f.EndOffset,
				})
			}
			entry["funcs"] = arr
		}
		out[fmt.Sprintf("%d", pid)] = entry
	}
	return out
}
