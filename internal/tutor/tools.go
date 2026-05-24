package tutor

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jeanmarcos/modos-cpu/internal/llm"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
)

// ToolHandler executes one tool invocation. ctx carries cancellation; sess
// is the student's session (and the simulator behind it); args is the raw
// JSON the model emitted for that call. The return value is JSON-serialized
// by the orchestrator before being fed back into the conversation.
type ToolHandler func(ctx context.Context, sess *sessions.Session, args json.RawMessage) (any, error)

// Tool bundles an LLM-facing declaration with the handler that runs when
// the model invokes it.
type Tool struct {
	Def     llm.ToolDef
	Handler ToolHandler
}

// DefaultTools is the toolset the tutor exposes. Six narrow functions that
// together cover the questions a student is likely to ask about a running
// simulation, without making the LLM re-read state it does not need.
func DefaultTools() []Tool {
	return []Tool{
		{
			Def: llm.ToolDef{
				Name:        "get_processes",
				Description: "Lista todos los procesos cargados con su PID, nombre, estado (NEW/READY/RUN/BLOCKED/ZOMBIE), razón de bloqueo y core asignado. Úselo cuando el estudiante pregunte por procesos en general o quiera ver el cuadro completo.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			Handler: handleGetProcesses,
		},
		{
			Def: llm.ToolDef{
				Name:        "get_process",
				Description: "Devuelve los detalles de un proceso específico (registros guardados, ESP, posición en memoria). Úselo cuando el estudiante pregunte por un PID concreto, especialmente para entender por qué está bloqueado o qué estaba ejecutando.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pid": map[string]any{"type": "integer", "description": "PID del proceso a inspeccionar"},
					},
					"required": []string{"pid"},
				},
			},
			Handler: handleGetProcess,
		},
		{
			Def: llm.ToolDef{
				Name:        "get_recent_events",
				Description: "Devuelve hasta last_n eventos recientes del simulador, opcionalmente filtrados por tipo. Tipos útiles: sched_decision (context switch), mode_switch, irq_fire, syscall, page_fault, process_state, pcb_save, pcb_restore, console_out, halt, mutex_op, sem_op. Úselo cuando el estudiante pregunte qué pasó, por qué cambió de proceso, etc.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"types":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "tipos a incluir; vacío = todos"},
						"last_n": map[string]any{"type": "integer", "description": "cuántos eventos devolver, máx 50"},
					},
					"required": []string{"last_n"},
				},
			},
			Handler: handleGetRecentEvents,
		},
		{
			Def: llm.ToolDef{
				Name:        "get_console_output",
				Description: "Devuelve la salida acumulada en la consola del simulador (lo que los procesos imprimieron vía sys_write con FD=1). Úselo cuando el estudiante pregunte qué imprimió el programa.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			Handler: handleGetConsoleOutput,
		},
		{
			Def: llm.ToolDef{
				Name:        "get_scheduler_stats",
				Description: "Cuenta los eventos clave ocurridos en lo que va de la simulación: total de context switches, mode switches, syscalls, IRQs, page faults. Úselo cuando el estudiante pregunte cuántas veces pasó algo.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			Handler: handleGetSchedulerStats,
		},
		{
			Def: llm.ToolDef{
				Name:        "get_program_source",
				Description: "Devuelve el código fuente C original y el assembly desensamblado del programa cargado para un PID. Útil cuando la pregunta sea sobre qué hace el código, por qué falla una instrucción, o cómo se mapea una línea C a la instrucción de máquina ejecutada. Si pid es 0, devuelve todos los programas cargados.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pid": map[string]any{"type": "integer", "description": "PID del proceso (0 para todos los programas cargados)"},
					},
				},
			},
			Handler: handleGetProgramSource,
		},
		{
			Def: llm.ToolDef{
				Name:        "get_memory",
				Description: "Lee `length` bytes de memoria a partir de `addr` (en formato hex). Útil para ver el contenido del stack del proceso actual o de una sección de código. length máximo: 256 bytes.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"addr":   map[string]any{"type": "integer", "description": "dirección de memoria (decimal o hex con prefijo 0x)"},
						"length": map[string]any{"type": "integer", "description": "cuántos bytes leer, máx 256"},
					},
					"required": []string{"addr", "length"},
				},
			},
			Handler: handleGetMemory,
		},
	}
}

// ── Handlers ──────────────────────────────────────────────────────────

func handleGetProcesses(_ context.Context, sess *sessions.Session, _ json.RawMessage) (any, error) {
	if sess == nil || sess.Sim == nil {
		return nil, fmt.Errorf("no session")
	}
	snap := sess.Sim.Snapshot()
	procs, _ := snap["procs"].([]map[string]any)
	out := make([]map[string]any, 0, len(procs))
	for _, p := range procs {
		out = append(out, map[string]any{
			"pid":        p["pid"],
			"name":       p["name"],
			"state":      p["state"],
			"blocked_on": p["blocked_on"],
		})
	}
	return map[string]any{"processes": out, "count": len(out)}, nil
}

func handleGetProcess(_ context.Context, sess *sessions.Session, args json.RawMessage) (any, error) {
	var in struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if sess == nil || sess.Sim == nil {
		return nil, fmt.Errorf("no session")
	}
	snap := sess.Sim.Snapshot()
	procs, _ := snap["procs"].([]map[string]any)
	for _, p := range procs {
		if pid, ok := anyToInt(p["pid"]); ok && pid == in.PID {
			return p, nil
		}
	}
	return nil, fmt.Errorf("pid %d not found", in.PID)
}

func handleGetRecentEvents(_ context.Context, sess *sessions.Session, args json.RawMessage) (any, error) {
	var in struct {
		Types []string `json:"types"`
		LastN int      `json:"last_n"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if in.LastN <= 0 {
		in.LastN = 20
	}
	if in.LastN > 50 {
		in.LastN = 50
	}
	if sess == nil || sess.Sim == nil {
		return nil, fmt.Errorf("no session")
	}
	events := sess.Sim.RecentEvents(in.Types, in.LastN)
	return map[string]any{"events": events, "count": len(events)}, nil
}

func handleGetConsoleOutput(_ context.Context, sess *sessions.Session, _ json.RawMessage) (any, error) {
	if sess == nil || sess.Sim == nil {
		return nil, fmt.Errorf("no session")
	}
	// Pull every console_out event from the ring; rebuild the per-PID streams
	// the front-end would otherwise concatenate visually.
	events := sess.Sim.RecentEvents([]string{"console_out"}, 200)
	if len(events) == 0 {
		return map[string]any{"streams": []any{}, "note": "ningún proceso ha escrito en la consola en lo que va de la simulación"}, nil
	}
	streams := map[int]string{}
	// RecentEvents is newest-first; reverse to chronological for concat.
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		pid, _ := anyToInt(e["pid"])
		text, _ := e["text"].(string)
		streams[pid] += text
	}
	out := make([]map[string]any, 0, len(streams))
	for pid, txt := range streams {
		out = append(out, map[string]any{"pid": pid, "text": txt})
	}
	return map[string]any{"streams": out}, nil
}

func handleGetSchedulerStats(_ context.Context, sess *sessions.Session, _ json.RawMessage) (any, error) {
	if sess == nil || sess.Sim == nil {
		return nil, fmt.Errorf("no session")
	}
	// Pull the entire ring once (max capacity) and count by type. Cheap:
	// ~1000 entries × O(1) string compare each.
	events := sess.Sim.RecentEvents(nil, 10_000)
	counts := map[string]int{
		"context_switches": 0,
		"mode_switches":    0,
		"syscalls":         0,
		"irqs":             0,
		"page_faults":      0,
		"halts":            0,
	}
	for _, e := range events {
		switch e["type"] {
		case "sched_decision":
			counts["context_switches"]++
		case "mode_switch":
			counts["mode_switches"]++
		case "syscall":
			counts["syscalls"]++
		case "irq_fire":
			counts["irqs"]++
		case "page_fault":
			counts["page_faults"]++
		case "halt":
			counts["halts"]++
		}
	}
	counts["events_in_ring"] = len(events)
	return counts, nil
}

func handleGetProgramSource(_ context.Context, sess *sessions.Session, args json.RawMessage) (any, error) {
	var in struct {
		PID int `json:"pid"`
	}
	_ = json.Unmarshal(args, &in)
	if sess == nil || sess.Sim == nil {
		return nil, fmt.Errorf("no session")
	}
	// Round-trip the snapshot through JSON so we get uniform map[string]any
	// shapes regardless of the concrete types Snapshot() returns (sources is
	// map[int]string keyed by ints, which JSON turns into string keys).
	raw, err := json.Marshal(sess.Sim.Snapshot())
	if err != nil {
		return nil, fmt.Errorf("snapshot marshal: %w", err)
	}
	var snap struct {
		Sources    map[string]string `json:"sources"`
		SourceMaps map[string]any    `json:"source_maps"`
		Procs      []map[string]any  `json:"procs"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("snapshot unmarshal: %w", err)
	}

	nameByPID := map[string]string{}
	for _, p := range snap.Procs {
		if pid, ok := anyToInt(p["pid"]); ok {
			if name, ok := p["name"].(string); ok {
				nameByPID[fmt.Sprintf("%d", pid)] = name
			}
		}
	}

	buildEntry := func(pidKey string) map[string]any {
		pidInt, _ := anyToInt(pidKey)
		if pidInt == 0 {
			_, _ = fmt.Sscanf(pidKey, "%d", &pidInt)
		}
		entry := map[string]any{
			"pid":  pidInt,
			"name": nameByPID[pidKey],
		}
		if src, ok := snap.Sources[pidKey]; ok && src != "" {
			entry["c_source"] = src
		}
		if sm, ok := snap.SourceMaps[pidKey]; ok {
			entry["source_map"] = sm
		}
		return entry
	}

	if in.PID > 0 {
		key := fmt.Sprintf("%d", in.PID)
		if _, ok := snap.Sources[key]; !ok {
			return nil, fmt.Errorf("pid %d has no source loaded", in.PID)
		}
		return buildEntry(key), nil
	}
	all := make([]map[string]any, 0, len(snap.Sources))
	for pidKey := range snap.Sources {
		all = append(all, buildEntry(pidKey))
	}
	return map[string]any{"programs": all, "count": len(all)}, nil
}

func handleGetMemory(_ context.Context, sess *sessions.Session, args json.RawMessage) (any, error) {
	var in struct {
		Addr   uint64 `json:"addr"`
		Length uint64 `json:"length"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if in.Length == 0 {
		in.Length = 64
	}
	if in.Length > 256 {
		in.Length = 256
	}
	if sess == nil || sess.Sim == nil {
		return nil, fmt.Errorf("no session")
	}
	bytes, err := sess.Sim.MemorySlice(uint32(in.Addr), uint32(in.Length))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"addr":   fmt.Sprintf("0x%08x", in.Addr),
		"length": len(bytes),
		"hex":    hex.EncodeToString(bytes),
	}, nil
}
