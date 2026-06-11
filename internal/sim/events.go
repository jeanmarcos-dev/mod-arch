package sim

import (
	"github.com/jeanmarcos/modos-cpu/internal/programs"
)

// SnapshotEvent is the wire format for a simulator snapshot.
type SnapshotEvent struct {
	Type           string                                   `json:"type"`
	Regs           map[string]uint32                        `json:"regs"`
	Segments       map[string]uint16                        `json:"segments"`
	Mode           uint8                                    `json:"mode"`
	Halted         bool                                     `json:"halted"`
	Tick           uint64                                   `json:"tick"`
	TimerQuantum   int                                      `json:"timer_quantum"`
	CurrentPID     int                                      `json:"current_pid"`
	ReadyQueue     []int                                    `json:"ready_queue"`
	WaitQueue      []map[string]any                         `json:"wait_queue"`
	Procs          []map[string]any                         `json:"procs"`
	Sources        map[int]string                           `json:"sources"`
	SourceMaps     map[int]map[string][]programs.SourceLine `json:"source_maps,omitempty"`
	Vars           map[int]map[string]any                   `json:"vars,omitempty"`
	Cores          []CoreSnapshot                           `json:"cores"`
	NumCores       int                                      `json:"num_cores"`
	Running        bool                                     `json:"running"`
	Paused         bool                                     `json:"paused,omitempty"`
	Hz             float64                                  `json:"hz"`
	ClockHz        uint64                                   `json:"clockHz"`
	SpeedMul       float64                                  `json:"speedMul"`
	TickIntervalMs float64                                  `json:"tickIntervalMs"`
}

// CoreSnapshot is the wire format for a single core's state.
type CoreSnapshot struct {
	ID          int               `json:"id"`
	Regs        map[string]uint32 `json:"regs"`
	Segments    map[string]uint16 `json:"segments"`
	Mode        uint8             `json:"mode"`
	Halted      bool              `json:"halted"`
	Tick        uint64            `json:"tick"`
	CurrentPID  int               `json:"current_pid"`
	RunningIdle bool              `json:"running_idle,omitempty"`
	IdleTicks   uint64            `json:"idle_ticks,omitempty"`
}

// LoadedEvent is the wire format broadcast when a program is loaded.
type LoadedEvent struct {
	Type       string                                   `json:"type"`
	Name       string                                   `json:"name"`
	PIDs       []int                                    `json:"pids"`
	Procs      []map[string]any                         `json:"procs"`
	Sources    map[int]string                           `json:"sources"`
	SourceMaps map[int]map[string][]programs.SourceLine `json:"source_maps,omitempty"`
	VarMeta    map[int]map[string]any                   `json:"var_meta,omitempty"`
}

// StateUpdate represents a state change to the CPU.
type StateUpdate struct {
	Halted bool `json:"halted"`
}

// TickEventsEvent is a bundled frame of events broadcasted per-tick.
type TickEventsEvent struct {
	Type      string           `json:"type"`
	Tick      uint64           `json:"tick"`
	TickValue uint64           `json:"tick_value"`
	Events    []map[string]any `json:"events"`
	Vars      map[string]any   `json:"vars,omitempty"`
	Procs     []map[string]any `json:"procs,omitempty"`
	State     *StateUpdate     `json:"state,omitempty"`
}
