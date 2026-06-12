import type { CpuEventGo, SnapshotEvent, LoadedEvent, SessionInfoEvent, PresenceEvent, TickEventsEvent, MutexSnapshot, SemaphoreSnapshot } from '../../types/generated';

export type CpuEvent =
  | CpuEventGo
  | { type: 'tick'; value: number }
  | { type: 'state'; halted?: boolean; running?: boolean; paused?: boolean; hz?: number; clockHz?: number; speedMul?: number; tickIntervalMs?: number }
  | SnapshotEvent
  | { type: 'programs'; list: ProgramInfo[] }
  | LoadedEvent
  | { type: 'vars_update'; vars: Record<string, VarSnapshot> | null }
  | { type: 'reset' }
  | { type: 'procs'; procs: PCBInfo[] }
  | SessionInfoEvent
  | PresenceEvent
  | TickEventsEvent
  | { type: 'hand_raise'; label: string; at?: number }
  | { type: 'owner_pointer'; anchor?: string; offset_x?: number; offset_y?: number; route?: string; ts?: number; off?: boolean }
  | { type: 'owner_click'; anchor?: string; offset_x?: number; offset_y?: number; route?: string; ts?: number }
  | { type: 'owner_glossary_hover'; term?: string; off?: boolean; ts?: number }
  | { type: 'error'; message: string; code?: string; params?: Record<string, string> };

export interface CoreInfo {
  id: number;
  regs: Record<string, number>;
  segments: Record<string, number>;
  mode: number;
  halted: boolean;
  tick: number;
  current_pid: number;
  // running_idle disambiguates current_pid===0: legacy "no proc" vs the
  // kernel swapper (PID 0) being bound. Drives the dedicated HLT badge.
  running_idle?: boolean;
  // idle_ticks counts the cycles this core has spent in the swapper's HLT
  // loop. Updates every tick while RunningIdle so the UI can show a live
  // heartbeat without the event log being flooded.
  idle_ticks?: number;
}

// Lock state is built from mutex_op/sem_op events and resynced from
// snapshots; both sources share the generated wire shape.
export type MutexState = MutexSnapshot;

export type SemaphoreState = SemaphoreSnapshot;

export interface SessionInfo {
  id: string;
  mode: 'full' | 'privileged' | 'unknown';
  role: 'owner' | 'viewer' | 'none';
  label: string;
  view_token: string;
  user_id: string;
  user_name: string;
  user_email: string;
}

// Presence is what the host is currently presenting: section, visible
// cores, presentation mode preset and narrators toggle. Viewers with
// autofollow on mirror these values. Owner-side, it is set on every
// navigation or preference change so the server can rebroadcast.
//
// Focus is the inner-panel state of the host (which middle tab is active,
// where memory is centered, etc.). Every field is optional — only the ones
// the owner has actually set are present, and the server merges deltas so
// the client can send partial updates.
export type PresentationMode = 'compact' | 'pedagogic' | 'projector';
export type MiddleTab = 'code' | 'events' | 'memory' | 'console';
export type MobileTab = 'cpu' | 'code' | 'events' | 'memory' | 'procs' | 'console';
export type SimBarMode = 'docked' | 'floating';

export interface Focus {
  middle_tab?: MiddleTab;
  mobile_tab?: MobileTab;
  memory_base_addr?: number;
  memory_auto_follow?: boolean;
  pinned_pid?: number;
  sim_bar_mode?: SimBarMode;
  sidebar_collapsed?: boolean;
  is_mobile?: boolean;
}

export interface Presence {
  section: string;
  cores: number[];
  mode: PresentationMode | '';
  narrators: boolean;
  focus?: Focus;
}

export type Difficulty = 'basic' | 'intermediate' | 'advanced';

export interface ProgramInfo {
  name: string;
  description: string;
  category?: string;
  size: number;
  procs?: string[];
  difficulty?: Difficulty;
  topics?: string[];
  syscalls_used?: string[];
}

export interface CategoryMeta {
  slug: string;
  label: string;
  description: string;
  order: number;
}

export interface PCBInfo {
  pid: number;
  name: string;
  state: string;
  blocked_on?: string;
  blocked_since?: number;
  regs: Record<string, number>;
  saved_user_esp: number;
  code_base: number;
  code_size: number;
  stack_top: number;
  // Process-tree relationships introduced with sys_fork / sys_vfork.
  // parent_pid = 0 means "no parent" (top-level process, e.g. init).
  parent_pid?: number;
  children?: number[];
  exit_code?: number;
  vfork_parent?: number;
  // Thread group identity. tgid groups tasks that share an address space
  // through clone(CLONE_THREAD); is_leader marks the original task that
  // owns the memory and renders as the "process" head. For fork-only
  // demos these fields are still populated (tgid == pid, is_leader true)
  // so panels can switch to TGID-based grouping unconditionally.
  tgid?: number;
  tid?: number;
  is_leader?: boolean;
  // is_idle flags the kernel swapper (PID 0). Pulled out of the regular
  // process tree and rendered as the compact kernel-idle band instead.
  is_idle?: boolean;
  // is_kthread marks the kernel-thread hierarchy: kthreadd (PID 2) and the
  // per-core kernel/N tasks that hang from it. These PCBs are never
  // scheduled; their RUN/SLEEP appearance is derived from cores[N].mode.
  is_kthread?: boolean;
}

// One IPC channel (pipe or FIFO) tracked in the store. Buffer is held
// modulo the cap; level is decoupled from buffer.length to keep the
// UI rendering simple even when there is no pipe_op event in the last
// window. Name is only set for FIFOs.
export interface PipeInfo {
  pipe_id: number;
  owner_pid: number;
  fd_r: number;
  fd_w: number;
  is_fifo: boolean;
  name?: string;
  buf_level: number;
  buf_cap: number;
  last_bytes?: number[];
  last_op?: 'read' | 'write';
  last_pid?: number;
}

// SourceLine bridges one machine instruction back to its C line of origin.
// Comes from the backend `source_maps` payload on `loaded` and `snapshot`.
export interface SourceLine {
  offset: number;
  mnemonic: string;
  c_line: number;
}

// VarType, VarLocKind and VarLoc mirror their Go counterparts in
// internal/sim. The VARS panel uses Loc to render a label like "[EAX]" or
// "[EBP-4]" next to the value, and the popover uses it to resolve a token
// in the C source against the right thread.
export type VarType = 'int' | 'uint' | 'ptr' | 'char';
export type VarLocKind = 'mem' | 'reg' | 'stack';
export interface VarLoc {
  kind: VarLocKind;
  addr?: number;
  reg?: string;
  offset?: number;
}

// VarMetaGlobal / VarMetaLocal / VarMetaFunc are the static slice declared
// by a demo (name, type, address) — they arrive once in the `loaded` event
// and never change while the demo is running. Lets the UI tokenize the C
// source even before the first snapshot arrives.
export interface VarMetaGlobal {
  name: string;
  type: VarType;
  addr: string; // backend ships hex string ("0x3F0000")
  len: number;
}
export interface VarMetaLocal {
  name: string;
  type: VarType;
  func: string;
  loc: VarLoc;
}
export interface VarMetaFunc {
  name: string;
  start_offset: number;
  end_offset: number;
}
export interface VarMeta {
  globals?: VarMetaGlobal[];
  locals?: VarMetaLocal[];
  funcs?: VarMetaFunc[];
}

// GlobalSnapshot / LocalSnapshot / ThreadVarSnapshot / VarSnapshot are the
// per-tick values resolved from kernel memory + register file. value_hex /
// value_dec are pre-formatted on the backend so the UI does not have to know
// signed-vs-unsigned semantics.
export interface GlobalSnapshot {
  name: string;
  type: VarType;
  addr: string;
  len: number;
  value_hex?: string;
  value_dec?: string;
  unmapped?: boolean;
}
export interface LocalSnapshot {
  name: string;
  type: VarType;
  func: string;
  loc: VarLoc;
  resolved_addr?: string;
  value_hex?: string;
  value_dec?: string;
  unset?: boolean;
  unmapped?: boolean;
}
export interface ThreadVarSnapshot {
  tid: number;
  label: string;
  name: string;
  func?: string;
  in_scope: boolean;
  ebp: string;
  regs: Record<string, number>;
  locals: LocalSnapshot[];
}
export interface VarSnapshot {
  globals: GlobalSnapshot[];
  threads: ThreadVarSnapshot[];
}

// IrqFrame is the three-word stack frame the CPU HW pushed to the kernel
// stack when it took an interrupt from user mode. The Interrupts view renders
// it while the handler is running and clears it on IRET.
export interface IrqFrame {
  vector: number;
  handler: number;
  savedEIP: number;
  savedCS: number;
  savedEFLAGS: number;
  fromMode: number;
  capturedAt: number;
}

// IdtHandlerHit records the last time a given vector fired: which core took
// it, what handler addr the CPU jumped to, and when. Drives both the IDT
// table highlight and the "Handler @" column.
export interface IdtHandlerHit {
  coreId: number;
  handler: number;
  source: string;
  ts: number;
}

export interface WaitInfo {
  pid: number;
  name: string;
  reason: string;
  // blocked_since carries the CPU tick at which the proc entered BLOCKED.
  // Optional because EventProcessState doesn't transport it yet — only the
  // snapshot path populates it; live wakes/blocks may leave it undefined
  // until the next snapshot lands. Concurrency.vue degrades gracefully.
  blocked_since?: number;
}

export interface LogEntry {
  ts: number;
  text: string;
  color: string;
  // Optional core that produced the event. Undefined for global events
  // (loaded, reset, ws_*) so they remain visible regardless of which cores
  // the user has filtered in the SimBar.
  coreId?: number;
  // Optional simulator tick at which the event occurred. The multi-column
  // log groups rows by this value so the user can read parallelism line by
  // line. Undefined for genuinely global events that have no per-core tick.
  tick?: number;
  // 'global' marks events that span every core (loaded, reset, ws_*). The
  // grid renders them as a full-width row separator instead of placing them
  // in any single column.
  scope?: 'global';
  // True when the event executed in PL0 (kernel mode). The grid uses it to
  // tint the cell so the student can see at a glance which slices of each
  // core's timeline ran in supervisor mode — Silberschatz's "the kernel runs
  // in the context of the trapped process on the trapping core" made
  // concrete in the visual.
  inKernel?: boolean;
  // 'scheduler' tags events that are specifically the scheduler acting:
  // sched_decision, wake, init_reap, pcb_save/restore. The grid uses this
  // to surface a dedicated chip on those cells, so the alumno can tell
  // "the kernel ran here" (inKernel) apart from "the scheduler ran here"
  // (kind=='scheduler') — they are not the same thing.
  kind?: 'scheduler';
  // Leading icon rendered before the text. PCB save/restore use Lucide SVGs
  // (see EventsLog) instead of the Unicode glyph the other events carry in
  // their `text`, because JetBrains Mono lacks the original ◍/◌ at this
  // size and the system fallback breaks vertical alignment.
  glyph?: 'pcbSave' | 'pcbRestore';
  // Atomicity role: the event log shows one 'primary' headline per cell and
  // tucks 'detail' lines behind an expander. Undefined for global rows.
  role?: 'primary' | 'detail';
  // Originating event tag, used to pick the headline when two primaries share
  // a cell (an `INT 0x80` tick emits both 'instruction' and 'syscall').
  tag?: string;
  // Run epoch, bumped on every load/reset; the log groups by (epoch, tick) so
  // a new demo's tick 0 never merges into the previous one.
  epoch?: number;
}

