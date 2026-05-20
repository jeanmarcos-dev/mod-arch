// Package sim contains the CPU + kernel simulator.
//
// A Simulator instance is an isolated machine: its own CPU, kernel, memory,
// loadable program list, and associated client connections. The sessions
// package owns one Simulator per session and multiplexes them.
package sim

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// Demo is a scenario with one or more processes to load.
type Demo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Procs       []Proc `json:"-"`
}

// Proc is a single binary inside a Demo.
type Proc struct {
	Name   string
	Bytes  []byte
	Source string
}

// SendBufferSize is the buffer size of each client channel. If it fills up
// (slow consumer) new messages are dropped instead of blocking the simulator.
const SendBufferSize = 256

// Client is a consumer of simulator events. The sim knows nothing about the
// transport: it just enqueues messages on Send. Whoever creates the Client
// (typically the WS handler) is responsible for draining it.
type Client struct {
	Send chan map[string]any
}

// NewClient builds a Client with the standard buffer size.
func NewClient() *Client {
	return &Client{Send: make(chan map[string]any, SendBufferSize)}
}

// Simulator is an isolated simulator instance.
type Simulator struct {
	mu          sync.Mutex
	kernel      *kernel.Kernel
	programs    map[string]Demo
	procSources map[int]string

	clients  map[*Client]struct{}
	clientMu sync.RWMutex

	running bool
	runStop chan struct{}
	stepHz  int

	logger *slog.Logger
}

// Config holds the construction parameters for a Simulator.
type Config struct {
	Demos    []Demo
	Logger   *slog.Logger
	NumCores int // number of simulated CPU cores; clamped to [1, kernel.MaxCores]
}

// New builds a ready Simulator with no program loaded.
func New(cfg Config) *Simulator {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	progs := make(map[string]Demo, len(cfg.Demos))
	for _, d := range cfg.Demos {
		progs[d.Name] = d
	}
	cores := cfg.NumCores
	if cores < 1 {
		cores = 1
	}
	return &Simulator{
		kernel:   kernel.NewMulti(cpu.DefaultMemSize, cores),
		programs: progs,
		clients:  make(map[*Client]struct{}),
		logger:   cfg.Logger,
		stepHz:   4,
	}
}

// LoadProgram resets the kernel and loads a (potentially multi-process) demo.
//
// All work happens while holding s.mu; the lock is released before
// broadcasting. Broadcasting under the mutex would deadlock against any
// snapshot path that re-acquires it.
func (s *Simulator) LoadProgram(name string) error {
	s.mu.Lock()
	demo, ok := s.programs[name]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("unknown demo: %q", name)
	}
	s.kernel.Reset()
	pids := make([]int, 0, len(demo.Procs))
	sources := make(map[int]string)
	for i, p := range demo.Procs {
		pid, err := s.kernel.AddProcess(p.Name, p.Bytes)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		pids = append(pids, pid)
		sources[pid] = demo.Procs[i].Source
	}
	s.procSources = sources
	snap := s.snapshotLocked()
	procs := s.processListLocked()
	s.mu.Unlock()

	s.Broadcast(map[string]any{
		"type":    "loaded",
		"name":    name,
		"pids":    pids,
		"procs":   procs,
		"sources": sources,
	})
	s.Broadcast(snap)
	return nil
}

// StepOnce executes one instruction and broadcasts the resulting events.
func (s *Simulator) StepOnce() error {
	s.mu.Lock()
	evs, err := s.kernel.Step()
	tick := s.kernel.BootCPU().Tick
	halted := s.kernel.AllHalted()
	s.mu.Unlock()
	if err != nil {
		s.Broadcast(map[string]any{"type": "error", "message": err.Error()})
		return err
	}
	for _, e := range evs {
		s.Broadcast(EventToWire(e))
	}
	s.Broadcast(map[string]any{"type": "tick", "value": tick})
	if halted {
		s.Broadcast(map[string]any{"type": "state", "halted": true})
	}
	return nil
}

// StartRun starts a loop that steps at hz Hz until pause/halt.
func (s *Simulator) StartRun(hz int) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	if hz <= 0 {
		hz = 4
	}
	s.stepHz = hz
	s.running = true
	s.runStop = make(chan struct{})
	stop := s.runStop
	interval := time.Second / time.Duration(hz)
	s.mu.Unlock()

	s.Broadcast(map[string]any{"type": "state", "running": true, "hz": hz})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.mu.Lock()
				halted := s.kernel.AllHalted()
				s.mu.Unlock()
				if halted {
					s.StopRun()
					return
				}
				if err := s.StepOnce(); err != nil {
					s.StopRun()
					return
				}
			}
		}
	}()
}

// StopRun stops the run loop.
func (s *Simulator) StopRun() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.runStop)
	s.runStop = nil
	s.mu.Unlock()
	s.Broadcast(map[string]any{"type": "state", "running": false})
}

// PushKey injects the first byte of key into the kernel keyboard buffer.
func (s *Simulator) PushKey(key string) {
	if key == "" {
		return
	}
	b := key[0]
	s.mu.Lock()
	s.kernel.PushKey(b)
	s.mu.Unlock()
}

// SetTimerQuantum adjusts the timer quantum.
func (s *Simulator) SetTimerQuantum(q int) {
	if q < 0 {
		q = 0
	}
	s.mu.Lock()
	s.kernel.TimerQuantum = q
	snap := s.snapshotLocked()
	s.mu.Unlock()
	s.Broadcast(snap)
}

// Reset clears all simulator state back to the initial values.
func (s *Simulator) Reset() {
	s.mu.Lock()
	wasRunning := s.running
	s.mu.Unlock()
	if wasRunning {
		s.StopRun()
	}
	s.mu.Lock()
	s.kernel.Reset()
	snap := s.snapshotLocked()
	procs := s.processListLocked()
	s.mu.Unlock()

	s.Broadcast(map[string]any{"type": "reset"})
	s.Broadcast(snap)
	s.Broadcast(map[string]any{"type": "procs", "procs": procs})
}

// Snapshot acquires the mutex and returns the current simulator state.
func (s *Simulator) Snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

// snapshotLocked builds a state snapshot. The caller MUST hold s.mu.
//
// The top-level fields (regs, mode, halted, current_pid, …) mirror the boot
// core so single-core demos and the existing frontend keep working unchanged.
// The new "cores" array is the multicore-aware view: one entry per core with
// its own regs/mode/halted/current_pid.
func (s *Simulator) snapshotLocked() map[string]any {
	boot := s.kernel.BootCore()
	c := boot.CPU
	r := c.R
	cores := make([]map[string]any, 0, s.kernel.NumCores())
	for _, core := range s.kernel.Cores {
		cores = append(cores, coreSnapshot(core))
	}
	return map[string]any{
		"type": "snapshot",
		"regs": map[string]uint32{
			"EAX": r.EAX, "EBX": r.EBX, "ECX": r.ECX, "EDX": r.EDX,
			"ESI": r.ESI, "EDI": r.EDI, "ESP": r.ESP, "EBP": r.EBP,
			"EIP": r.EIP, "EFLAGS": r.EFLAGS,
		},
		"segments":      map[string]uint16{"CS": r.CS, "DS": r.DS, "SS": r.SS},
		"mode":          c.Mode,
		"halted":        c.Halted,
		"tick":          c.Tick,
		"timer_quantum": s.kernel.TimerQuantum,
		"current_pid":   boot.CurrentPID,
		"ready_queue":   s.kernel.Ready.Snapshot(),
		"wait_queue":    s.waitQueueLocked(),
		"procs":         s.processListLocked(),
		"sources":       s.procSources,
		"cores":         cores,
		"num_cores":     s.kernel.NumCores(),
	}
}

func coreSnapshot(core *kernel.Core) map[string]any {
	r := core.CPU.R
	return map[string]any{
		"id": core.ID,
		"regs": map[string]uint32{
			"EAX": r.EAX, "EBX": r.EBX, "ECX": r.ECX, "EDX": r.EDX,
			"ESI": r.ESI, "EDI": r.EDI, "ESP": r.ESP, "EBP": r.EBP,
			"EIP": r.EIP, "EFLAGS": r.EFLAGS,
		},
		"segments":    map[string]uint16{"CS": r.CS, "DS": r.DS, "SS": r.SS},
		"mode":        core.CPU.Mode,
		"halted":      core.CPU.Halted,
		"tick":        core.CPU.Tick,
		"current_pid": core.CurrentPID,
	}
}

func (s *Simulator) waitQueueLocked() []map[string]any {
	list := make([]map[string]any, 0)
	maxPID := s.maxKnownPIDLocked()
	for pid := 1; pid <= maxPID; pid++ {
		p, ok := s.kernel.Processes[pid]
		if !ok || p.State != "BLOCKED" {
			continue
		}
		list = append(list, map[string]any{
			"pid":    p.PID,
			"name":   p.Name,
			"reason": p.BlockedOn,
		})
	}
	return list
}

func (s *Simulator) processListLocked() []map[string]any {
	list := make([]map[string]any, 0, len(s.kernel.Processes))
	maxPID := s.maxKnownPIDLocked()
	for pid := 1; pid <= maxPID; pid++ {
		p, ok := s.kernel.Processes[pid]
		if !ok {
			continue
		}
		list = append(list, p.Snapshot())
	}
	return list
}

// maxKnownPIDLocked picks the larger of "highest PID currently running on any
// core" and "size of the process table" so the iteration covers every PID
// the kernel knows about even when some cores are idle.
func (s *Simulator) maxKnownPIDLocked() int {
	m := len(s.kernel.Processes)
	for _, c := range s.kernel.Cores {
		if c.CurrentPID > m {
			m = c.CurrentPID
		}
	}
	return m
}

// MemorySlice returns a copy of the bytes in [addr, addr+length).
func (s *Simulator) MemorySlice(addr, length uint32) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mem := s.kernel.Mem.Bytes
	if int(addr) >= len(mem) {
		return nil, fmt.Errorf("addr out of range")
	}
	end := addr + length
	if int(end) > len(mem) {
		end = uint32(len(mem))
	}
	return append([]byte(nil), mem[addr:end]...), nil
}

// Programs returns the summarized list of loadable demos.
func (s *Simulator) Programs() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]map[string]any, 0, len(s.programs))
	for _, p := range s.programs {
		list = append(list, demoSummary(p))
	}
	return list
}

func demoSummary(d Demo) map[string]any {
	totalSize := 0
	procNames := make([]string, len(d.Procs))
	for i, p := range d.Procs {
		totalSize += len(p.Bytes)
		procNames[i] = p.Name
	}
	return map[string]any{
		"name":        d.Name,
		"description": d.Description,
		"size":        totalSize,
		"procs":       procNames,
	}
}

// AddClient registers a client to receive broadcasts.
func (s *Simulator) AddClient(c *Client) {
	s.clientMu.Lock()
	s.clients[c] = struct{}{}
	s.clientMu.Unlock()
}

// RemoveClient unregisters a client.
func (s *Simulator) RemoveClient(c *Client) {
	s.clientMu.Lock()
	delete(s.clients, c)
	s.clientMu.Unlock()
}

// Broadcast sends a message to every connected client. If a client's channel
// is full the message is dropped, so one slow consumer cannot stall the
// simulator for the rest.
func (s *Simulator) Broadcast(msg map[string]any) {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	for c := range s.clients {
		select {
		case c.Send <- msg:
		default:
		}
	}
}

// EventToWire converts a cpu.Event to its JSON-serializable form for the WS.
func EventToWire(e cpu.Event) map[string]any {
	data, _ := json.Marshal(e)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	m["type"] = e.Tag()
	return m
}
