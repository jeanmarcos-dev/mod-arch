package tutor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
)

// SnapshotView is the compact, LLM-friendly summary of the simulator's state
// that the tutor sends in every prompt. Building a struct (instead of a raw
// string) lets us hash the same fields for the answer cache key.
//
// This view is intentionally minimal: per-process detail, event history,
// console output and memory live behind tutor tools the LLM calls on demand,
// so a long-running demo does not blow up the prompt with thousands of
// events the model may not care about.
type SnapshotView struct {
	Section      string
	Program      string
	Mode         string
	CurrentPID   int
	NumCores     int
	Running      bool
	Halted       bool
	Tick         uint64
	EIP          uint32
	ESP          uint32
	EFLAGS       uint32
	ReadyQueue   []int
	WaitQueue    []int
	TimerQuantum int
}

// BuildSnapshot extracts a SnapshotView from a session. It deliberately drops
// memory contents, source maps and per-core arrays — those would blow up the
// prompt token budget without adding pedagogical value for a chat question.
func BuildSnapshot(sess *sessions.Session) SnapshotView {
	v := SnapshotView{}
	if sess == nil {
		return v
	}
	v.Section = sess.Presence().Section
	if sess.Sim == nil {
		return v
	}
	snap := sess.Sim.Snapshot()
	if s, ok := snap["mode"].(int); ok {
		v.Mode = modeLabel(s)
	} else if s, ok := snap["mode"].(uint32); ok {
		v.Mode = modeLabel(int(s))
	}
	v.CurrentPID, _ = anyToInt(snap["current_pid"])
	v.NumCores, _ = anyToInt(snap["num_cores"])
	v.Running, _ = snap["running"].(bool)
	v.Halted, _ = snap["halted"].(bool)
	v.TimerQuantum, _ = anyToInt(snap["timer_quantum"])
	if t, ok := snap["tick"].(uint64); ok {
		v.Tick = t
	} else if t, ok := anyToInt(snap["tick"]); ok {
		v.Tick = uint64(t)
	}
	if regs, ok := snap["regs"].(map[string]uint32); ok {
		v.EIP = regs["EIP"]
		v.ESP = regs["ESP"]
		v.EFLAGS = regs["EFLAGS"]
	}
	if ready, ok := snap["ready_queue"].([]int); ok {
		v.ReadyQueue = ready
	}
	if waits, ok := snap["wait_queue"].([]map[string]any); ok {
		for _, w := range waits {
			if pid, ok := anyToInt(w["pid"]); ok {
				v.WaitQueue = append(v.WaitQueue, pid)
			}
		}
	}
	if procs, ok := snap["procs"].([]map[string]any); ok {
		for _, p := range procs {
			if pid, _ := anyToInt(p["pid"]); pid == v.CurrentPID {
				if name, ok := p["name"].(string); ok {
					v.Program = name
				}
			}
		}
	}
	return v
}

// Render produces the prompt-ready text. ~200 tokens of structured facts.
func (v SnapshotView) Render() string {
	if v.isZero() {
		return ""
	}
	var b strings.Builder
	w := func(key, val string) {
		if val == "" {
			return
		}
		fmt.Fprintf(&b, "- %s: %s\n", key, val)
	}
	if v.Section != "" {
		w("Sección visible", v.Section)
	}
	if v.Program != "" {
		w("Programa cargado", v.Program)
	}
	w("Modo CPU", v.Mode)
	if v.CurrentPID > 0 {
		w("PID en ejecución", fmt.Sprintf("%d", v.CurrentPID))
	}
	if v.NumCores > 0 {
		w("Núcleos", fmt.Sprintf("%d", v.NumCores))
	}
	w("Estado", runStateLabel(v.Running, v.Halted))
	if v.TimerQuantum > 0 {
		w("Quantum del scheduler", fmt.Sprintf("%d ticks", v.TimerQuantum))
	}
	w("Tick", fmt.Sprintf("%d", v.Tick))
	w("Registros clave", fmt.Sprintf("EIP=0x%08x ESP=0x%08x EFLAGS=0x%08x", v.EIP, v.ESP, v.EFLAGS))
	if len(v.ReadyQueue) > 0 {
		w("Cola READY (PIDs)", fmt.Sprintf("%v", v.ReadyQueue))
	}
	if len(v.WaitQueue) > 0 {
		w("Cola WAIT (PIDs)", fmt.Sprintf("%v", v.WaitQueue))
	}
	return strings.TrimSpace(b.String())
}

// Digest returns a stable hex digest of the snapshot. Used in answer-cache
// keys so the same question asked in different states does not collide.
func (v SnapshotView) Digest() string {
	h := sha256.New()
	fmt.Fprintf(h, "section=%s|program=%s|mode=%s|pid=%d|cores=%d|running=%v|halted=%v|qtm=%d|tick=%d|eip=%d|esp=%d|eflags=%d|ready=%v|wait=%v",
		v.Section, v.Program, v.Mode, v.CurrentPID, v.NumCores, v.Running, v.Halted,
		v.TimerQuantum, v.Tick, v.EIP, v.ESP, v.EFLAGS, v.ReadyQueue, v.WaitQueue,
	)
	return hex.EncodeToString(h.Sum(nil))
}

// isZero reports whether the view carries no useful information. Used by
// Render to short-circuit when the simulator has nothing to say yet.
func (v SnapshotView) isZero() bool {
	return v.Section == "" && v.Program == "" && v.Mode == "" &&
		v.CurrentPID == 0 && v.NumCores == 0 && !v.Running && !v.Halted &&
		v.Tick == 0 && v.EIP == 0 && v.ESP == 0 && v.EFLAGS == 0 &&
		len(v.ReadyQueue) == 0 && len(v.WaitQueue) == 0 && v.TimerQuantum == 0
}

func modeLabel(mode int) string {
	switch mode {
	case 0:
		return "PL0 (kernel)"
	case 3:
		return "PL3 (user)"
	default:
		return fmt.Sprintf("PL%d", mode)
	}
}

func runStateLabel(running, halted bool) string {
	switch {
	case halted:
		return "detenido (HLT)"
	case running:
		return "ejecutando"
	default:
		return "pausado"
	}
}

// anyToInt coerces interface{} values that may arrive as int, uint32, uint64,
// or float64 (the JSON path) into an int. Returns ok=false when the value is
// nil or of a non-numeric type.
func anyToInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		return int(x), true
	case uint64:
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	default:
		return 0, false
	}
}
