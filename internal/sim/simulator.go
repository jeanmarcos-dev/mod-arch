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

	"github.com/jeanmarcos-dev/mod-arch/internal/telemetry"
	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
	"github.com/jeanmarcos-dev/mod-arch/pkg/kernel"
)

// Demo is a scenario with one or more processes to load.
//
// Cores is optional: when greater than zero, LoadProgram switches the
// underlying kernel to that core count before loading the procs. Default-zero
// leaves the current configuration untouched, so legacy single-core demos
// behave as before.
type Demo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Category     string   `json:"category,omitempty"`
	Cores        int      `json:"cores,omitempty"`
	Difficulty   string   `json:"difficulty,omitempty"`
	Topics       []string `json:"topics,omitempty"`
	SyscallsUsed []string `json:"syscalls_used,omitempty"`
	Procs        []Proc   `json:"-"`
}

// Proc is a single binary inside a Demo.
type Proc struct {
	Name      string
	Bytes     []byte
	Source    string
	SourceMap []SourceLine
	Globals   []Global
	Locals    []Local
	Funcs     []Func
}

// SourceLine pins one machine instruction to the C line that motivated it.
// Mirrors programs.SourceLine; lives here so the sim layer stays independent
// of the programs package on its outbound DTO.
type SourceLine struct {
	Offset   uint32 `json:"offset"`
	Mnemonic string `json:"mnemonic"`
	CLine    int    `json:"c_line"`
}

// VarType, VarLocKind, VarLoc, Global, Local, Func mirror the same-named
// types in internal/programs. The sim layer keeps its own copy so its public
// API stays independent of the programs package; main.go translates across
// the boundary when registering demos.
type VarType string

const (
	VarInt  VarType = "int"
	VarUint VarType = "uint"
	VarPtr  VarType = "ptr"
	VarChar VarType = "char"
)

type VarLocKind string

const (
	VarLocMem   VarLocKind = "mem"
	VarLocReg   VarLocKind = "reg"
	VarLocStack VarLocKind = "stack"
)

type VarLoc struct {
	Kind   VarLocKind
	Addr   uint32
	Reg    string
	Offset int32
}

type Global struct {
	Name string
	Type VarType
	Addr uint32
	Len  uint32
}

type Local struct {
	Name string
	Type VarType
	Loc  VarLoc
	Func string
}

type Func struct {
	Name        string
	StartOffset uint32
	EndOffset   uint32
}

// SendBufferSize is the buffer size of each client channel. If it fills up
// (slow consumer) new messages are dropped instead of blocking the simulator.
const SendBufferSize = 256

// ClientRole tags a Client with its role so the sim can fanout selectively
// (e.g. broadcast presence updates only to viewers, hand_raise pings only to
// the owner). The sim does not know about the sessions package, so it uses
// its own enum that the WS handler maps from sessions.Role.
type ClientRole int

const (
	// RoleAny is the default tag for clients whose role is irrelevant.
	RoleAny ClientRole = iota
	// RoleOwner tags the session owner.
	RoleOwner
	// RoleViewer tags a read-only viewer connected via /v/<token>.
	RoleViewer
)

// Client is a consumer of simulator events. The sim knows nothing about the
// transport: it just enqueues messages on Send. Whoever creates the Client
// (typically the WS handler) is responsible for draining it.
type Client struct {
	Send chan any
	Role ClientRole
}

// NewClient builds a Client with the standard buffer size and role.
func NewClient(role ClientRole) *Client {
	return &Client{
		Send: make(chan any, SendBufferSize),
		Role: role,
	}
}

// Simulator is an isolated simulator instance.
type Simulator struct {
	mu             sync.Mutex
	kernel         *kernel.Kernel
	programs       map[string]Demo
	procSources    map[int]string
	procSourceMaps map[int][]SourceLine

	// procGlobals/procLocals/procFuncs hold variable metadata declared by
	// each demo's Proc, keyed by the leader PID of the loaded program. Live
	// values are resolved in snapshotLocked() against the current kernel
	// state; threads of the same process inherit their leader's metadata
	// because clone(CLONE_VM|CLONE_THREAD) shares the address space.
	procGlobals map[int][]Global
	procLocals  map[int][]Local
	procFuncs   map[int][]Func

	clients  map[*Client]struct{}
	clientMu sync.RWMutex

	running bool
	// userPaused tracks whether the most recent stop was an explicit user
	// pause (cmd:pause) as opposed to an idle autostop (HasWork=false). It is
	// emitted alongside running in state events so the UI can distinguish
	// "frozen on purpose" from "system idle waiting for IRQ" and only freeze
	// visual narrators in the former case.
	userPaused bool
	// autoPauseOnIdle parks the run loop once no user work remains, so a
	// finished demo settles instead of streaming idle ticks. Default on.
	autoPauseOnIdle bool
	runStop         chan struct{}
	clockHz         int
	speedMul        float64
	intervalChange  chan time.Duration

	// events is a rolling history of the most recent CPU events. Consumed by
	// the tutor's tools so the LLM can ask "what happened in the last second?"
	// without us shipping the full event stream in the prompt.
	events *EventRing

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
	cores := max(cfg.NumCores, 1)
	k := kernel.NewMulti(cpu.DefaultMemSize, cores)
	// Default the swapper on so the simulator behaves like a real OS out of
	// the box: every demo runs against a kernel that always has a process
	// to schedule (PID 0 idle threads on otherwise-free cores), the run
	// loop never auto-stops, and the student sees the classical "what does
	// the CPU do with nothing to run?" answer — Silberschatz §5.5 calls
	// this the "kernel idle task" on SMP. The toggle remains so a class
	// can flip it off to compare against the legacy auto-stop model.
	k.IdleTaskEnabled = true
	return &Simulator{
		kernel:          k,
		programs:        progs,
		clients:         make(map[*Client]struct{}),
		events:          NewEventRing(DefaultEventRingCapacity),
		logger:          cfg.Logger,
		clockHz:         2,
		speedMul:        1.0,
		autoPauseOnIdle: true,
	}
}

// RecentEvents returns the latest CPU events kept in the ring buffer,
// optionally filtered by type (e.g. []string{"sched_decision", "mode_switch"}).
// Newest-first ordering. Exposed for the tutor's get_recent_events tool.
func (s *Simulator) RecentEvents(types []string, n int) []map[string]any {
	return s.events.Recent(types, n)
}

// LoadProgram resets the kernel and loads a (potentially multi-process) demo.
//
// All work happens while holding s.mu; the lock is released before
// broadcasting. Broadcasting under the mutex would deadlock against any
// snapshot path that re-acquires it.
func (s *Simulator) LoadProgram(name string) error {
	s.mu.Lock()
	demo, ok := s.programs[name]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown demo: %q", name)
	}
	if demo.Cores > 0 {
		// SetNumCores is a no-op when the kernel already has the requested
		// count; otherwise it rebuilds the kernel with a fresh layout. Either
		// way it acquires s.mu internally, so it must run outside our lock.
		if err := s.SetNumCores(demo.Cores); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.kernel.Reset()
	// New demo restarts the clock at 0; drop stale-tick history from the ring.
	s.events.Clear()
	// Init (PID 1) is spawned for every demo so the process tree always has
	// a root and orphan zombies have somewhere to land. Demos that only care
	// about a single process keep working: the UI hides PID 1 by default
	// (preferences.showInit). See the ZOMBIE entry in the glossary for the
	// pedagogical rationale.
	if _, err := s.kernel.SpawnInit(); err != nil {
		s.mu.Unlock()
		return err
	}
	// kthreadd (PID 2) + one kernel/N task per core hang off swapper (PID 0)
	// the same way Linux ships them. They are never scheduled — the frontend
	// derives their RUN/SLEEP appearance from each core's live mode — but
	// having them as real PCBs gives the process tree a single ancestor
	// chain: swapper → {init → user procs, kthreadd → kernel/N}.
	s.kernel.SpawnKThreadd()
	s.kernel.SpawnKernelThreads()
	// Swapper (PID 0) is spawned alongside init when the toggle is on so
	// the very first DispatchIdleCores call can bind it to whatever cores
	// remain idle. With the toggle off this is skipped, leaving the
	// legacy auto-stop semantics untouched.
	if s.kernel.IdleTaskEnabled {
		if err := s.kernel.SpawnIdle(); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	pids := make([]int, 0, len(demo.Procs))
	sources := make(map[int]string)
	sourceMaps := make(map[int][]SourceLine)
	globalsByPID := make(map[int][]Global)
	localsByPID := make(map[int][]Local)
	funcsByPID := make(map[int][]Func)
	for i, p := range demo.Procs {
		pid, err := s.kernel.AddProcess(p.Name, p.Bytes)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.kernel.AdoptUnderInit(pid)
		pids = append(pids, pid)
		sources[pid] = demo.Procs[i].Source
		if len(demo.Procs[i].SourceMap) > 0 {
			sourceMaps[pid] = demo.Procs[i].SourceMap
		}
		if len(demo.Procs[i].Globals) > 0 {
			globalsByPID[pid] = demo.Procs[i].Globals
		}
		if len(demo.Procs[i].Locals) > 0 {
			localsByPID[pid] = demo.Procs[i].Locals
		}
		if len(demo.Procs[i].Funcs) > 0 {
			funcsByPID[pid] = demo.Procs[i].Funcs
		}
	}
	s.procSources = sources
	s.procSourceMaps = sourceMaps
	s.procGlobals = globalsByPID
	s.procLocals = localsByPID
	s.procFuncs = funcsByPID
	// Boot bind: park every core on its swapper so the snapshot reflects the
	// idle state. These events are NOT broadcast as a tick-0 frame: load does
	// not step the CPU, so tick 0 belongs to the first STEP/RUN. Emitting them
	// here would later merge with that step's dispatch and make the tick-0 row
	// mutate. The first DispatchIdleCores re-surfaces the swapper→user transition.
	s.kernel.BindIdleCores()
	snap := s.snapshotLocked()
	procs := s.processListLocked()
	s.mu.Unlock()

	s.Broadcast(map[string]any{
		"type":        "loaded",
		"name":        name,
		"pids":        pids,
		"procs":       procs,
		"sources":     sources,
		"source_maps": sourceMaps,
		"var_meta":    varMetaPayload(globalsByPID, localsByPID, funcsByPID),
	})
	s.Broadcast(snap)
	return nil
}

// StepOnce executes one instruction and broadcasts the resulting events.
func (s *Simulator) StepOnce() error {
	s.mu.Lock()
	// Boot dispatch covers the case where the user steps without having
	// pressed RUN first: idle cores pull from the ready queue before the
	// kernel ticks. Idempotent when every core already has a CurrentPID.
	// Capture the tick BEFORE Step so the value we report matches the tick
	// the just-executed events were stamped with (CPU.Step emits its events
	// against the current Tick value and then increments at the end). Reading
	// after Step would surface "next tick to execute", which made the header
	// pill in the UI run one ahead of the last row in the event log.
	tick := s.kernel.Clock
	bootEvs := s.kernel.DispatchIdleCores()
	evs, err := s.kernel.Step()
	halted := s.kernel.AllHalted()
	idleEnabled := s.kernel.IdleTaskEnabled
	// Process-table mutations (fork, clone-thread, exit, reparent, reap) only
	// reach the frontend through individual events whose handlers expect the
	// PCB to already exist in the local list. Without a fresh "procs"
	// broadcast the new tasks never appear in the panel and subsequent
	// pcb_save / process_state events for them are dropped silently.
	var procsSnap []map[string]any
	if procTableMutated(bootEvs) || procTableMutated(evs) {
		procsSnap = s.processListLocked()
	}
	// Compute vars under the same lock as Step() so the snapshot captured
	// matches exactly the post-step register file and memory. Bundling vars
	// into the tick_events frame below also makes delivery atomic: a slow
	// WS consumer either gets the new events AND the new vars or drops
	// both, which prevents the cursor from running ahead of the VARS panel.
	varsPayload := s.varsLocked()
	s.mu.Unlock()
	if err != nil {
		s.Broadcast(map[string]any{"type": "error", "message": err.Error()})
		return err
	}
	// Fuse boot dispatch and the tick's actual events into one batch so the
	// client gets a single reactive update with the full picture of the tick.
	tickEvs := bootEvs
	if len(evs) > 0 {
		tickEvs = append(tickEvs, evs...)
	}
	haltedNotice := halted && !idleEnabled
	s.broadcastTickFrame(tick, tickEvs, varsPayload, procsSnap, haltedNotice)
	return nil
}

// broadcastTickEvents bundles every cpu.Event produced in the same tick into
// a single WS frame. Emitting events one-by-one used to make the frontend
// re-render each row twice — once with only the first core filled in and
// again once the rest of the tick arrived — so we batch them here so the
// client can apply them atomically (one reactive update per tick).
func (s *Simulator) broadcastTickEvents(tick uint64, evs []cpu.Event) {
	s.broadcastTickFrame(tick, evs, nil, nil, false)
}

// broadcastTickFrame bundles every per-tick payload (events, vars, procs,
// halt notice) into a single WS frame so the client pays one parse + one
// reactive flush per tick. vars/procs/haltedNotice may be nil/false.
func (s *Simulator) broadcastTickFrame(
	tick uint64,
	evs []cpu.Event,
	vars map[string]any,
	procs []map[string]any,
	haltedNotice bool,
) {
	if len(evs) == 0 && vars == nil && procs == nil && !haltedNotice {
		return
	}
	wire := make([]map[string]any, 0, len(evs))
	for _, e := range evs {
		s.events.push(e, tick)
		wire = append(wire, EventToWire(e))
	}
	msg := TickEventsEvent{
		Type:      "tick_events",
		Tick:      tick,
		TickValue: tick,
		Events:    wire,
		Vars:      vars,
		Procs:     procs,
	}
	if haltedNotice {
		msg.State = &StateUpdate{Halted: true}
	}
	s.Broadcast(msg)
}

// procTableMutated reports whether any event in the slice changed the set of
// processes the kernel tracks (or their parent/zombie status). The frontend
// keeps its own copy of the process list and only refreshes it on a
// {type:"procs"} broadcast, so any of these events must be paired with one.
func procTableMutated(evs []cpu.Event) bool {
	for _, e := range evs {
		switch ev := e.(type) {
		case cpu.EventFork, cpu.EventThreadCreate, cpu.EventThreadExit, cpu.EventReparent, cpu.EventInitReap:
			return true
		case cpu.EventWaitOp:
			if ev.Op == "reap" {
				return true
			}
		}
	}
	return false
}

// StartRun starts the auto-step loop. If hz > 0 it overrides the stored
// clockHz before starting; pass 0 to honor the current clockHz × speedMul.
// Loop cadence is intervalChange-driven so SetClockHz / SetSpeedMul can
// retune the ticker without restarting the goroutine.
func (s *Simulator) StartRun(hz int) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	if hz > 0 {
		s.clockHz = clampClockHz(hz)
	}
	s.running = true
	// Starting the loop clears any sticky user-pause flag so a fresh run is
	// always emitted as paused:false even if the previous stop was manual.
	s.userPaused = false
	s.runStop = make(chan struct{})
	s.intervalChange = make(chan time.Duration, 1)
	stop := s.runStop
	intCh := s.intervalChange
	interval := tickInterval(s.clockHz, s.speedMul)
	clockHz := s.clockHz
	speedMul := s.speedMul
	s.mu.Unlock()

	s.broadcastSpeedState(true, false, clockHz, speedMul, interval)

	// Step-then-wait: the first StepOnce runs immediately so RUN feels
	// responsive, and crucially it is the sole owner of tick 0. A separate
	// boot-dispatch broadcast here would land a second frame on the same tick
	// and make the event log's tick-0 row mutate; StepOnce already does its own
	// DispatchIdleCores + Step, so tick 0 matches what STEP produces.
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			s.mu.Lock()
			hasWork := s.kernel.HasWork()
			// HasWork is always true with the swapper on, so a finished demo
			// would stream idle ticks forever; autoPauseOnIdle parks the loop
			// once no user work remains. A future PushKey rearms us via StepOnce.
			idleDone := s.autoPauseOnIdle && !s.kernel.HasUserWork()
			s.mu.Unlock()
			if !hasWork || idleDone {
				s.StopRun()
				return
			}
			if err := s.StepOnce(); err != nil {
				s.StopRun()
				return
			}
			// Wait for the next tick, honoring stop and live retunes without
			// stepping again.
			waiting := true
			for waiting {
				select {
				case <-stop:
					return
				case newInterval := <-intCh:
					if newInterval > 0 {
						ticker.Reset(newInterval)
					}
				case <-ticker.C:
					waiting = false
				}
			}
		}
	}()
}

// SetClockHz changes the simulated CPU clock frequency (integer Hz, clamped to
// [1, 60]). If the run loop is active the ticker is retuned without restarting
// the goroutine; otherwise the new value is just stored for the next StartRun.
// A `state` event broadcasts clockHz, speedMul and the resulting tickIntervalMs
// so every client can re-render its speed UI consistently.
func (s *Simulator) SetClockHz(hz int) {
	hz = clampClockHz(hz)
	s.mu.Lock()
	if s.clockHz == hz {
		s.mu.Unlock()
		return
	}
	s.clockHz = hz
	mul := s.speedMul
	interval := tickInterval(hz, mul)
	ch := s.intervalChange
	running := s.running
	paused := s.userPaused
	s.mu.Unlock()
	if running && ch != nil {
		// Non-blocking send: the loop drains one value per iteration, and a
		// previous unread value already reflects user intent close enough.
		select {
		case ch <- interval:
		default:
		}
	}
	s.broadcastSpeedState(running, paused, hz, mul, interval)
}

// SetSpeedMul changes the wall-clock multiplier (float, clamped to
// [0.1, 5.0]). Same retune semantics as SetClockHz.
func (s *Simulator) SetSpeedMul(mul float64) {
	mul = clampSpeedMul(mul)
	s.mu.Lock()
	if s.speedMul == mul {
		s.mu.Unlock()
		return
	}
	s.speedMul = mul
	hz := s.clockHz
	interval := tickInterval(hz, mul)
	ch := s.intervalChange
	running := s.running
	paused := s.userPaused
	s.mu.Unlock()
	if running && ch != nil {
		select {
		case ch <- interval:
		default:
		}
	}
	s.broadcastSpeedState(running, paused, hz, mul, interval)
}

// SetSpeed is the legacy entry point kept around so the WS handler keeps
// compiling; it forwards to SetClockHz. Remove once the frontend stops sending
// the `set_speed` command.
func (s *Simulator) SetSpeed(hz int) {
	s.SetClockHz(hz)
}

// broadcastSpeedState fans out the speed-related fields. tickIntervalMs is
// rounded to an int so the wire format stays small; the legacy `hz` field is
// the effective rate (clockHz × speedMul) so old clients still see something
// sensible without knowing about the multiplier.
func (s *Simulator) broadcastSpeedState(running, paused bool, hz int, mul float64, interval time.Duration) {
	tickMs := int(interval / time.Millisecond)
	if tickMs <= 0 {
		tickMs = 1
	}
	effective := float64(hz) * mul
	state := map[string]any{
		"type":           "state",
		"clockHz":        hz,
		"speedMul":       mul,
		"tickIntervalMs": tickMs,
		"hz":             effective,
		"paused":         paused,
	}
	if running {
		state["running"] = true
	}
	s.Broadcast(state)
}

// tickInterval converts (clockHz, speedMul) into the time.Duration consumed by
// the ticker. Caps the effective rate at 0.1 Hz on the low end to keep the
// duration finite when callers pass a stale zero.
func tickInterval(hz int, mul float64) time.Duration {
	effective := float64(hz) * mul
	if effective < 0.1 {
		effective = 0.1
	}
	return time.Duration(float64(time.Second) / effective)
}

func clampClockHz(hz int) int {
	if hz < 1 {
		return 1
	}
	if hz > 60 {
		return 60
	}
	return hz
}

func clampSpeedMul(mul float64) float64 {
	if mul < 0.1 {
		return 0.1
	}
	if mul > 5.0 {
		return 5.0
	}
	return mul
}

// StopRun stops the run loop. It is used both for the user-driven cmd:pause
// path (via Pause) and the run-loop's own idle autostop (HasWork=false).
// The emitted state event carries the current userPaused flag so the UI can
// distinguish the two: Pause sets userPaused=true beforehand; autostop leaves
// it at the value StartRun cleared to false.
func (s *Simulator) StopRun() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.runStop)
	s.runStop = nil
	s.intervalChange = nil
	paused := s.userPaused
	s.mu.Unlock()
	s.Broadcast(map[string]any{"type": "state", "running": false, "paused": paused})
}

// Pause is the explicit user-initiated pause (cmd:pause). It marks the
// simulator as user-paused and then stops the run loop; the resulting state
// event broadcasts paused:true so the frontend freezes visual narrators
// mid-animation. Idle autostops go through StopRun directly and keep
// paused:false so animations complete naturally during HALT-waiting-IRQ.
//
// Pause can also be invoked while the simulator is already stopped — for
// example when the user presses PAUSE during idle-wait (every core HALT,
// blocked on a key). In that case StopRun is a no-op, but we still need
// to broadcast the paused transition so the UI flips from "running, will
// resume on key" to "user-paused, key buffers without resuming".
func (s *Simulator) Pause() {
	s.mu.Lock()
	wasRunning := s.running
	s.userPaused = true
	s.mu.Unlock()
	if wasRunning {
		s.StopRun()
		return
	}
	s.Broadcast(map[string]any{"type": "state", "running": false, "paused": true})
}

// PushKey injects the first byte of key into the kernel keyboard buffer.
//
// PushKey models the keyboard controller raising IRQ 0x21. When the run loop
// is idle (all cores parked in HLT waiting for input), the IRQ would sit in
// PendingIRQs forever because no ticker is firing. We drive one StepOnce
// inline so the boot core dispatches the interrupt immediately, the blocked
// reader wakes up, and — if the simulation has work queued and the user did
// not explicitly pause — the loop is resumed so the wakened process keeps
// executing without a second click on RUN. When the user did pause, the
// byte and IRQ stay buffered and the sim remains frozen.
func (s *Simulator) PushKey(key string) {
	if key == "" {
		return
	}
	b := key[0]
	s.mu.Lock()
	evs := s.kernel.PushKey(b)
	tick := s.kernel.Clock
	running := s.running
	userPaused := s.userPaused
	exitDraining := len(s.kernel.PendingKernelWork) > 0
	s.mu.Unlock()
	s.broadcastTickEvents(tick, evs)
	if running || userPaused {
		return
	}
	// If the kernel is mid-do_exit (phases queued on PendingKernelWork),
	// stepping inline here would consume an exit phase on behalf of a
	// keyboard IRQ — which would hide the death cascade the user is
	// observing click by click. Leave the byte and the IRQ buffered;
	// they will dispatch the moment the cascade drains and the user (or
	// the run loop) takes the next Step.
	if exitDraining {
		return
	}
	// Idle autostop case: step once so the IRQ dispatches immediately and
	// the wake transitions are visible, then resume the run loop if there
	// is still work to do (a tight readkey loop keeps HasWork true; a
	// program that exits after reading one byte will not).
	if err := s.StepOnce(); err != nil {
		return
	}
	s.mu.Lock()
	hasWork := s.kernel.HasWork()
	alreadyRunning := s.running
	s.mu.Unlock()
	if hasWork && !alreadyRunning {
		s.StartRun(0)
	}
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

// SetNumCores rebuilds the underlying kernel with a fresh layout of n cores,
// clamped to [1, kernel.MaxCores]. The simulator must lose all program state
// because rescaling cores on the fly would mean redistributing PIDs, register
// files, and kernel-stack frames — out of scope for a teaching simulator.
//
// If a run loop is active it is stopped first; the function broadcasts a
// fresh snapshot so connected clients see the new core count immediately.
func (s *Simulator) SetNumCores(n int) error {
	if n < 1 {
		return fmt.Errorf("núcleos inválido (%d): debe ser >= 1", n)
	}
	if n > kernel.MaxCores {
		return fmt.Errorf("núcleos inválido (%d): el máximo soportado es %d", n, kernel.MaxCores)
	}

	s.mu.Lock()
	wasRunning := s.running
	s.mu.Unlock()
	if wasRunning {
		s.StopRun()
	}

	s.mu.Lock()
	if s.kernel.NumCores() == n {
		s.mu.Unlock()
		return nil
	}
	// Preserve the idle-task preference across the rebuild: the toggle is
	// a user setting, not a per-kernel-instance attribute. Without this,
	// changing core count would silently flip idle off.
	wasIdleEnabled := s.kernel.IdleTaskEnabled
	s.kernel = kernel.NewMulti(cpu.DefaultMemSize, n)
	s.kernel.IdleTaskEnabled = wasIdleEnabled
	s.procSources = nil
	s.procSourceMaps = nil
	s.procGlobals = nil
	s.procLocals = nil
	s.procFuncs = nil
	snap := s.snapshotLocked()
	procs := s.processListLocked()
	s.mu.Unlock()

	s.Broadcast(map[string]any{"type": "reset"})
	s.Broadcast(map[string]any{"type": "procs", "procs": procs})
	s.Broadcast(snap)
	return nil
}

// SetAutoPauseOnIdle toggles parking the run loop once no user work remains.
// Takes effect on the next ticker iteration; no restart needed.
func (s *Simulator) SetAutoPauseOnIdle(enabled bool) {
	s.mu.Lock()
	s.autoPauseOnIdle = enabled
	s.mu.Unlock()
}

// SetIdleTaskEnabled toggles the kernel's swapper (PID 0). When enabling on
// a loaded demo the PCB is spawned immediately so the next dispatch can bind
// it. When disabling, the PCB stays in the table but every core bound to it
// is released so the legacy auto-stop semantics resume. Callers (typically
// the WS dispatcher) hold no kernel locks; the simulator serializes with its
// own mutex.
func (s *Simulator) SetIdleTaskEnabled(enabled bool) error {
	s.mu.Lock()
	if s.kernel.IdleTaskEnabled == enabled {
		s.mu.Unlock()
		return nil
	}
	s.kernel.IdleTaskEnabled = enabled

	var evs []map[string]any
	if enabled {
		// Spawn the swapper only if a demo has already been loaded —
		// otherwise the PCB would sit in an otherwise-empty kernel
		// and confuse the snapshot the next LoadProgram broadcasts.
		// InitPID being non-zero is the proxy for "demo loaded".
		if s.kernel.InitPID != 0 {
			if err := s.kernel.SpawnIdle(); err != nil {
				s.mu.Unlock()
				return err
			}
			for _, e := range s.kernel.DispatchIdleCores() {
				evs = append(evs, EventToWire(e))
			}
		}
	} else {
		// Release any core currently bound to PID 0 so the legacy
		// auto-stop kicks in once the ready queue drains. The PCB
		// itself stays in the table to avoid spawn/teardown thrash
		// when the user flips the toggle again.
		for _, c := range s.kernel.Cores {
			if c.RunningIdle {
				evs = append(evs, EventToWire(cpu.EventCoreRelease{
					CoreID: c.ID, PID: 0, Reason: "idle disabled",
				}))
				c.RunningIdle = false
				c.CurrentPID = 0
				c.CPU.Halted = true
			}
		}
	}
	procs := s.processListLocked()
	snap := s.snapshotLocked()
	s.mu.Unlock()

	for _, e := range evs {
		s.Broadcast(e)
	}
	s.Broadcast(map[string]any{"type": "procs", "procs": procs})
	s.Broadcast(snap)
	return nil
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
	s.events.Clear()
	snap := s.snapshotLocked()
	procs := s.processListLocked()
	s.mu.Unlock()

	s.Broadcast(map[string]any{"type": "reset"})
	s.Broadcast(snap)
	s.Broadcast(map[string]any{"type": "procs", "procs": procs})
}

// Snapshot acquires the mutex and returns the current simulator state.
func (s *Simulator) Snapshot() map[string]any {
	start := time.Now()
	defer telemetry.TrackSnapshotLatency(start)

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
		cores = append(cores, coreSnapshot(core, s.kernel.Clock))
	}
	return map[string]any{
		"type": "snapshot",
		"regs": map[string]uint32{
			"EAX": r.EAX, "EBX": r.EBX, "ECX": r.ECX, "EDX": r.EDX,
			"ESI": r.ESI, "EDI": r.EDI, "ESP": r.ESP, "EBP": r.EBP,
			"EIP": r.EIP, "EFLAGS": r.EFLAGS,
		},
		"segments":       map[string]uint16{"CS": r.CS, "DS": r.DS, "SS": r.SS},
		"mode":           c.Mode,
		"halted":         c.Halted,
		"tick":           s.kernel.Clock,
		"timer_quantum":  s.kernel.TimerQuantum,
		"current_pid":    boot.CurrentPID,
		"ready_queue":    s.kernel.Ready.Snapshot(),
		"wait_queue":     s.waitQueueLocked(),
		"procs":          s.processListLocked(),
		"sources":        s.procSources,
		"source_maps":    s.procSourceMaps,
		"vars":           s.varsLocked(),
		"mutexes":        s.mutexesLocked(),
		"semaphores":     s.semaphoresLocked(),
		"cores":          cores,
		"num_cores":      s.kernel.NumCores(),
		"running":        s.running,
		"hz":             float64(s.clockHz) * s.speedMul,
		"clockHz":        s.clockHz,
		"speedMul":       s.speedMul,
		"tickIntervalMs": tickIntervalMs(s.clockHz, s.speedMul),
	}
}

// mutexesLocked serializes the kernel's mutex registry for the snapshot so a
// client connecting mid-demo sees the same state it would have built from the
// mutex_op event stream. Waiters are copied: the snapshot may be encoded
// after s.mu is released, while the kernel keeps mutating the live slices.
func (s *Simulator) mutexesLocked() []MutexSnapshot {
	list := make([]MutexSnapshot, 0, len(s.kernel.Mutexes))
	for _, m := range s.kernel.Mutexes {
		list = append(list, MutexSnapshot{
			ID:      m.ID,
			Holder:  m.Holder,
			Waiters: append([]int{}, m.Waiters...),
		})
	}
	return list
}

// semaphoresLocked is the semaphore counterpart of mutexesLocked.
func (s *Simulator) semaphoresLocked() []SemaphoreSnapshot {
	list := make([]SemaphoreSnapshot, 0, len(s.kernel.Semaphores))
	for _, sem := range s.kernel.Semaphores {
		list = append(list, SemaphoreSnapshot{
			ID:      sem.ID,
			Count:   sem.Count,
			Waiters: append([]int{}, sem.Waiters...),
		})
	}
	return list
}

// tickIntervalMs returns the snapshot-friendly rounded millisecond value
// derived from the same formula the ticker uses.
func tickIntervalMs(hz int, mul float64) int {
	ms := int(tickInterval(hz, mul) / time.Millisecond)
	if ms <= 0 {
		ms = 1
	}
	return ms
}

func coreSnapshot(core *kernel.Core, clock uint64) map[string]any {
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
		"tick":        clock,
		"current_pid": core.CurrentPID,
		// running_idle disambiguates CurrentPID==0: legacy meaning is
		// "no proc", but the frontend keys its dedicated HLT-loop badge
		// off this flag when the swapper is bound. Default false keeps
		// older clients unaffected.
		"running_idle": core.RunningIdle,
		// idle_ticks counts cycles the per-core swapper has spent in HLT.
		// The frontend renders this as the visible heartbeat of the
		// idle thread so the student sees activity without the log
		// drowning in repeated halt entries.
		"idle_ticks": core.IdleTicks,
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
			"pid":           p.PID,
			"name":          p.Name,
			"reason":        p.BlockedOn,
			"blocked_since": p.BlockedSince,
		})
	}
	return list
}

func (s *Simulator) processListLocked() []map[string]any {
	list := make([]map[string]any, 0, len(s.kernel.Processes)+len(s.kernel.IdleByCore))
	// Per-core swappers live in IdleByCore (one PCB per core, all sharing
	// PID 0 with negative TIDs to disambiguate). Emit them first so the
	// frontend can render the compact kernel-idle band above the regular
	// process tree. Skipped entirely when the toggle is off so legacy
	// snapshots keep their exact shape.
	if s.kernel.IdleTaskEnabled {
		for _, pcb := range s.kernel.IdleByCore {
			if pcb != nil {
				list = append(list, pcb.Snapshot())
			}
		}
	}
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
	difficulty := d.Difficulty
	if difficulty == "" {
		difficulty = "basic"
	}
	topics := d.Topics
	if topics == nil {
		topics = []string{}
	}
	syscalls := d.SyscallsUsed
	if syscalls == nil {
		syscalls = []string{}
	}
	return map[string]any{
		"name":          d.Name,
		"description":   d.Description,
		"category":      d.Category,
		"size":          totalSize,
		"procs":         procNames,
		"difficulty":    difficulty,
		"topics":        topics,
		"syscalls_used": syscalls,
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
func (s *Simulator) Broadcast(msg any) {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	for c := range s.clients {
		select {
		case c.Send <- msg:
		default:
		}
	}
}

// BroadcastTo sends a message only to clients tagged with one of the given
// roles. Used for presence (viewers) and hand_raise (owner) so each side gets
// only the events it cares about.
func (s *Simulator) BroadcastTo(msg any, roles ...ClientRole) {
	if len(roles) == 0 {
		return
	}
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	for c := range s.clients {
		match := false
		for _, r := range roles {
			if c.Role == r {
				match = true
				break
			}
		}
		if !match {
			continue
		}
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
