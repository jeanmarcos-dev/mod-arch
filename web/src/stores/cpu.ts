import { defineStore } from 'pinia';
import { computed, reactive, ref } from 'vue';
import { formatLog } from '../i18n/formatLog';
import { usePreferencesStore } from './preferences';

// Tipos de eventos que llegan por WS (espejo de internal/cpu/events.go).
// Todos los eventos generados por la CPU llevan core_id: con multicore el
// store mantiene un CoreInfo por core y sólo "espejea" al estado top-level
// (regs, mode, halted, currentPID) cuando el evento viene del core que el
// usuario eligió ver en la SimBar.
export type CpuEvent =
  | { type: 'instruction'; core_id: number; tick?: number; pid: number; eip: number; mnemonic: string; bytes: number[] }
  | { type: 'reg_write'; core_id: number; tick?: number; reg: string; old: number; new: number }
  | { type: 'mem_read'; core_id: number; tick?: number; addr: number; value: number; size: number }
  | { type: 'mem_write'; core_id: number; tick?: number; addr: number; old: number; new: number; size: number }
  | { type: 'mode_switch'; core_id: number; tick?: number; from: number; to: number; cause: string; new_cs?: number; new_ds?: number; new_ss?: number; new_esp?: number }
  | { type: 'irq_fire'; core_id: number; tick?: number; vector: number; source: string; handler: number; from_mode: number; saved_eip?: number; saved_cs?: number; saved_eflags?: number }
  | { type: 'page_fault'; core_id: number; tick?: number; addr: number; mode: number; perm_needed: string }
  | { type: 'halt'; core_id: number; tick?: number; pid?: number }
  | { type: 'tick'; value: number }
  | { type: 'state'; halted?: boolean; running?: boolean; paused?: boolean; hz?: number; clockHz?: number; speedMul?: number; tickIntervalMs?: number }
  | { type: 'snapshot'; regs: Record<string, number>; segments: Record<string, number>; mode: number; halted: boolean; tick: number; timer_quantum?: number; current_pid?: number; ready_queue?: number[]; wait_queue?: WaitInfo[]; procs?: PCBInfo[]; sources?: Record<string, string>; source_maps?: Record<string, SourceLine[]>; cores?: CoreInfo[]; num_cores?: number; running?: boolean; paused?: boolean; hz?: number; clockHz?: number; speedMul?: number; tickIntervalMs?: number }
  | { type: 'programs'; list: ProgramInfo[] }
  | { type: 'loaded'; name: string; pids: number[]; procs: PCBInfo[]; sources: Record<string, string>; source_maps?: Record<string, SourceLine[]> }
  | { type: 'reset' }
  | { type: 'procs'; procs: PCBInfo[] }
  | { type: 'pipe_create'; core_id?: number; tick?: number; pipe_id: number; owner_pid: number; fd_r: number; fd_w: number; is_fifo: boolean; name?: string }
  | { type: 'pipe_op'; core_id?: number; tick?: number; pipe_id: number; pid: number; op: 'read' | 'write'; bytes?: number[]; buf_level: number; buf_cap: number }
  | { type: 'fork'; core_id?: number; tick?: number; parent_pid: number; child_pid: number; is_vfork: boolean }
  | { type: 'reparent'; core_id?: number; tick?: number; pid: number; old_parent: number; new_parent: number }
  | { type: 'wait_op'; core_id?: number; tick?: number; pid: number; child_pid: number; op: string; exit_code?: number }
  | { type: 'thread_create'; core_id?: number; tick?: number; tid: number; tgid: number; leader_pid: number; creator_pid: number; fn_eip: number; stack_top: number }
  | { type: 'thread_exit'; core_id?: number; tick?: number; tid: number; tgid: number; retval: number }
  | { type: 'pcb_save'; core_id: number; tick?: number; pid: number; regs: Record<string, number> }
  | { type: 'pcb_restore'; core_id: number; tick?: number; pid: number; regs: Record<string, number> }
  | { type: 'sched_decision'; core_id?: number; tick?: number; from_pid: number; to_pid: number; reason?: string; ready_queue: number[] }
  | { type: 'wake'; core_id?: number; tick?: number; pid: number; from_reason: string; ready_queue: number[] }
  | { type: 'init_reap'; core_id?: number; tick?: number; init_pid: number; reaps: { child_pid: number; exit_code: number; reparented: boolean }[]; ready_queue: number[] }
  | { type: 'process_state'; core_id?: number; tick?: number; pid: number; state: string; reason?: string; exit_code?: number }
  | { type: 'syscall'; core_id?: number; tick?: number; pid: number; num: number; name: string; args: number[]; result: number }
  | { type: 'console_out'; core_id?: number; tick?: number; pid: number; kind?: 'in' | 'out'; bytes: number[]; text: string }
  | { type: 'kbd_state'; buf_used: number; buf_cap: number; readers: { pid: number; prog: string }[] }
  | { type: 'core_assign'; core_id: number; tick?: number; pid: number }
  | { type: 'core_release'; core_id: number; tick?: number; pid: number; reason?: string }
  | { type: 'core_idle'; core_id: number; tick: number }
  | { type: 'mutex_op'; core_id: number; tick?: number; pid: number; mutex_id: number; op: 'create' | 'acquired' | 'waiting' | 'released' | 'handoff' | 'rejected'; next_pid?: number; waiters?: number[] }
  | { type: 'sem_op'; core_id: number; tick?: number; pid: number; sem_id: number; op: 'create' | 'wait_immediate' | 'wait_blocked' | 'signal' | 'handoff'; count: number; next_pid?: number; waiters?: number[] }
  | { type: 'session_info'; id: string; mode: string; role: string; label: string; view_token: string; user_id?: string; user_name?: string; user_email?: string; presence?: { section: string; cores: number[]; mode?: string; narrators?: boolean; focus?: Focus } }
  | { type: 'presence'; section: string; cores: number[]; mode?: string; narrators?: boolean; focus?: Focus; at?: number }
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

export interface MutexState {
  id: number;
  holder: number;
  waiters: number[];
}

export interface SemaphoreState {
  id: number;
  count: number;
  waiters: number[];
}

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
}

const CORES_FILTER_KEY = 'modos-cpu:cores-filter:v1';
const CORES_FILTER_TOUCHED_KEY = 'modos-cpu:cores-filter-touched:v1';

// Returns the persisted filter, or an empty Set when the user has never made
// a manual choice. An empty initial Set is the signal for
// reconcileFilterWithCores to auto-populate with every core present in the
// first snapshot — so a fresh session with N cores shows N cores by default.
function loadCoresFilter(): Set<number> {
  if (typeof window === 'undefined') return new Set();
  try {
    const raw = window.localStorage.getItem(CORES_FILTER_KEY);
    if (!raw) return new Set();
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      const ids = parsed.filter((x): x is number => typeof x === 'number' && x >= 0);
      if (ids.length > 0) return new Set(ids);
    }
  } catch { /* corrupt entry, fall back to empty */ }
  return new Set();
}

function loadCoresFilterTouched(): boolean {
  if (typeof window === 'undefined') return false;
  return window.localStorage.getItem(CORES_FILTER_TOUCHED_KEY) === '1';
}

export const REGISTERS = [
  'EAX', 'EBX', 'ECX', 'EDX',
  'ESI', 'EDI', 'ESP', 'EBP',
  'EIP', 'EFLAGS',
] as const;

export const useCpuStore = defineStore('cpu', () => {
  // ─── Estado del CPU ─────────────────────────────────────────────────────
  const regs = reactive<Record<string, number>>(Object.fromEntries(REGISTERS.map(r => [r, 0])));
  const segments = reactive<Record<string, number>>({ CS: 0, DS: 0, SS: 0 });
  const mode = ref(3); // arranca en user
  const halted = ref(false);
  const running = ref(false);
  // paused distinguishes "user-pressed-pause" from "system idle waiting for
  // IRQ". Both produce running=false, but only paused=true should freeze
  // visual narrators mid-animation; idle should let them complete naturally.
  const paused = ref(false);
  const tick = ref(0);
  // Speed model: clockHz is the simulated CPU clock (integer Hz the kernel
  // would advertise); speedMul is the wall-clock playback multiplier the user
  // drives from the TopBar. effectiveHz = clockHz × speedMul. stepHz is kept
  // as a derived view so legacy consumers (SimBar bindings, snapshots) keep
  // reading the same shape without knowing about the split.
  const clockHz = ref(2);
  const speedMul = ref(1.0);
  const stepHz = computed(() => clockHz.value * speedMul.value);
  const tickIntervalMs = computed(() => {
    const hz = stepHz.value;
    return hz > 0 ? Math.round(1000 / hz) : 10000;
  });
  const timerQuantum = ref(8);

  // Registros que cambiaron en el último evento, para animar flash. Per-core:
  // every reg_write stamps the (coreId, reg) pair into the right set so a
  // change on C1 flashes inside the C1 strip even when C0 is the focused
  // core. The legacy `changedRegs` set is kept as a thin view over the
  // focused core so any pre-multicore consumer keeps reading the same shape.
  const changedRegsByCore = reactive<Record<number, Set<string>>>({});
  const changedRegs = reactive<Set<string>>(new Set());

  function ensureChangedSlot(coreId: number): Set<string> {
    let s = changedRegsByCore[coreId];
    if (!s) {
      s = reactive(new Set<string>());
      changedRegsByCore[coreId] = s;
    }
    return s;
  }

  // ─── Programas y log ────────────────────────────────────────────────────
  const programs = ref<ProgramInfo[]>([]);
  // Catalogue category metadata fetched from /api/categories. Populated once
  // per session; the picker reads order + labels + descriptions from here.
  const categories = ref<CategoryMeta[]>([]);
  const selectedProgram = ref<string | null>(null);
  const log = ref<LogEntry[]>([]);
  const connected = ref(false);
  // True after the first successful onopen of this tab session. Lets the UI
  // tell apart "still trying to connect for the first time" (cold start) from
  // "had a live connection that dropped" — the two warrant very different
  // affordances in ConnectionBanner.
  const hasEverConnected = ref(false);
  const lastError = ref<string | null>(null);

  // ─── Procesos ───────────────────────────────────────────────────────────
  const procs = ref<PCBInfo[]>([]);
  const currentPID = ref(0);
  const readyQueue = ref<number[]>([]);
  const waitQueue = ref<WaitInfo[]>([]);
  // IPC channels created via sys_pipe / sys_mknod; the IpcPanel reads
  // from here and renders the buffer state per channel.
  const pipes = ref<PipeInfo[]>([]);

  // Cross-panel intent: when a row of the PCB table requests "jump to my
  // memory", it writes the target address here. Simulator switches the
  // middle tab to memory and MemoryPanel re-centers its custom view on the
  // address, then clears the flag. Kept in the store so the originating
  // component does not need a callback chain to two ancestors.
  const pendingMemoryJump = ref<number | null>(null);
  function requestMemoryJump(addr: number) {
    pendingMemoryJump.value = addr >>> 0;
  }
  function clearMemoryJump() {
    pendingMemoryJump.value = null;
  }

  // Latest decoded mnemonic per core. Fed by the 'instruction' event so the
  // PCB panel can show "next/last instruction" for the running process
  // without re-requesting it on every render.
  const lastMnemonic = reactive<Record<number, string>>({});

  // ─── Multicore + sync primitives ─────────────────────────────────────────
  // cores tracks per-core state. numCores comes from the snapshot. mutexes
  // and semaphores are kept up-to-date via the mutex_op / sem_op events so
  // the /concurrencia view can render the lock tables without re-querying.
  //
  // selectedCoreIds is a multi-select "visibility filter" that rules every
  // pane in the app: any core not in the set is hidden from grids, narrators
  // and the event log. Default: all cores visible. The set is never empty —
  // toggle helpers below preserve at least one core so the UI always has
  // something to paint.
  //
  // selectedCoreId stays exposed as a *computed* "focus" = first id in the
  // visible set. It keeps the top-level regs/mode/halted/currentPID mirrors
  // working for components that were not migrated to per-core state (CpuPanel
  // fallback, MemoryPanel, several narrators).
  const cores = ref<CoreInfo[]>([]);
  const numCores = ref(1);
  const selectedCoreIds = ref<Set<number>>(loadCoresFilter());
  // Tracks whether the user has explicitly chosen which cores to view. When
  // false (fresh install or post-clear), reconcileFilterWithCores auto-adds
  // every core present in the snapshot so multicore demos are visible without
  // the user discovering the C0/C1/... toggles. Flips to true on the first
  // call to toggle/selectAll/selectOnly/setVisibleCores.
  const userTouchedCoresFilter = ref<boolean>(loadCoresFilterTouched());
  const mutexes = ref<MutexState[]>([]);
  const semaphores = ref<SemaphoreState[]>([]);

  // Active interrupt frame per core. Populated when an irq_fire arrives from
  // user mode (the HW pushed EIP/CS/EFLAGS to the kernel stack), cleared on
  // the matching iret. The /interrupciones view reads this map to render the
  // "Stack de retorno por core" panel.
  const activeFrameByCore = ref<Record<number, IrqFrame | null>>({});
  // Most-recent fire per IDT vector, used to populate the "Handler @" column
  // and to highlight the row when the same vector fires again.
  const lastFiredHandlerByVector = ref<Record<number, IdtHandlerHit>>({});

  const selectedCoreId = computed<number>(() => {
    const it = selectedCoreIds.value.values().next();
    return it.done ? 0 : (it.value as number);
  });

  // Cores currently visible (selected in the filter), sorted by id so any
  // consumer iterates them as C0..C3 left → right. Components that need to
  // render one row per visible core (MemoryPanel acordeón, EventLog chips,
  // narrators) should read this instead of filtering cores manually.
  const visibleCores = computed<CoreInfo[]>(() =>
    cores.value
      .filter(c => selectedCoreIds.value.has(c.id))
      .slice()
      .sort((a, b) => a.id - b.id),
  );

  function ensureCoreSlot(id: number): CoreInfo {
    let c = cores.value.find(x => x.id === id);
    if (!c) {
      c = {
        id,
        regs: Object.fromEntries(REGISTERS.map(r => [r, 0])),
        segments: { CS: 0, DS: 0, SS: 0 },
        mode: 3,
        halted: false,
        tick: 0,
        current_pid: 0,
      };
      cores.value.push(c);
      cores.value.sort((a, b) => a.id - b.id);
    }
    return c;
  }

  function isCoreVisible(coreId: number): boolean {
    return selectedCoreIds.value.has(coreId);
  }
  // Legacy single-select check used by mirror-update logic.
  function isSelected(coreId: number): boolean {
    return coreId === selectedCoreId.value;
  }

  function mirrorFocus() {
    const c = cores.value.find(x => x.id === selectedCoreId.value);
    if (!c) return;
    for (const k of Object.keys(regs)) regs[k] = c.regs[k] ?? 0;
    for (const k of Object.keys(segments)) segments[k] = c.segments[k] ?? 0;
    mode.value = c.mode;
    halted.value = c.halted;
    currentPID.value = c.current_pid;
  }

  // HALT in x86 is "park until next IRQ", not "permanently dead". The
  // backend emits `halt` on HLT but no explicit `wake` event when an IRQ
  // restarts the core — so any subsequent activity event must clear the
  // stale halted flag. Without this the chip badge sticks at HALT while
  // the core happily issues writes/reads.
  function markCoreActive(c: CoreInfo, coreId: number) {
    if (!c.halted) return;
    c.halted = false;
    if (isSelected(coreId)) halted.value = false;
  }

  function persistCoresFilter() {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage.setItem(
        CORES_FILTER_KEY,
        JSON.stringify([...selectedCoreIds.value]),
      );
    } catch { /* quota / private mode → keep in memory only */ }
  }

  function persistFilterTouched() {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage.setItem(CORES_FILTER_TOUCHED_KEY, '1');
    } catch { /* ignore */ }
  }

  // Marks the filter as user-controlled so future snapshots stop auto-adding
  // newly-discovered cores. Called from every entry point a user can trigger:
  // toggle/selectAll/selectOnly/setVisibleCores.
  function markUserTouchedFilter() {
    if (userTouchedCoresFilter.value) return;
    userTouchedCoresFilter.value = true;
    persistFilterTouched();
  }

  function commitFilter(next: Set<number>, opts: { userInitiated?: boolean } = {}) {
    // Never accept an empty set — fall back to {focus} or {0}.
    if (next.size === 0) next = new Set([selectedCoreId.value || 0]);
    selectedCoreIds.value = next;
    mirrorFocus();
    persistCoresFilter();
    if (opts.userInitiated) markUserTouchedFilter();
  }

  function toggleCoreSelection(id: number) {
    const next = new Set(selectedCoreIds.value);
    if (next.has(id)) {
      if (next.size === 1) return; // would leave the filter empty
      next.delete(id);
    } else {
      next.add(id);
    }
    commitFilter(next, { userInitiated: true });
  }

  function selectAllCores() {
    const ids = cores.value.length > 0 ? cores.value.map(c => c.id) : [0];
    commitFilter(new Set(ids), { userInitiated: true });
  }

  function selectOnlyCore(id: number) {
    commitFilter(new Set([id]), { userInitiated: true });
  }

  // Replace the entire visible-cores set in one shot. Used by the viewer
  // when a presence broadcast arrives so the filter never goes through the
  // transient empty state that clear()+add() would create, and so the
  // mutation goes through commitFilter exactly like every other path
  // (consistent reactivity, persistence, and mirrorFocus side effects).
  // Presence-driven calls do not count as a user-initiated decision, so the
  // host's filter stays in auto-populate mode if the viewer never touched it.
  function setVisibleCores(ids: number[]) {
    if (!Array.isArray(ids) || ids.length === 0) return;
    commitFilter(new Set(ids), { userInitiated: true });
  }

  // Back-compat alias. Old code (and the alias kept on the public store
  // surface) treats "set selected core" as "focus this core exclusively".
  function setSelectedCore(id: number) {
    selectOnlyCore(id);
  }

  // Reconcile the filter with the available cores after a snapshot. Removes
  // ids that no longer exist (e.g. user just shrank num_cores from 4 to 2)
  // and, when the user has not yet made an explicit choice, ALSO adds every
  // available core to the filter so multicore demos render fully on first
  // load. If the user has touched the filter (userTouchedCoresFilter=true),
  // their selection is preserved verbatim and only invalid ids are dropped.
  function reconcileFilterWithCores(available: number[]) {
    if (available.length === 0) return;
    const allowed = new Set(available);
    const next = new Set<number>();
    for (const id of selectedCoreIds.value) {
      if (allowed.has(id)) next.add(id);
    }
    if (!userTouchedCoresFilter.value) {
      // Auto-populate: every available core becomes visible, regardless of
      // what was persisted. This is what the user expects on a fresh install.
      for (const id of allowed) next.add(id);
    }
    if (next.size === 0) for (const id of allowed) next.add(id);
    if (next.size !== selectedCoreIds.value.size
      || [...next].some(id => !selectedCoreIds.value.has(id))) {
      commitFilter(next);
    }
  }
  // Fuente C (escrita a mano) por PID, para mostrar en el panel CÓDIGO.
  const procSources = ref<Record<number, string>>({});
  // Mapping line-C ↔ offset-byte para cada PID. Empty record = no bridge
  // declared (legacy demo); the UI degrades to plain Source display.
  const procSourceMaps = ref<Record<number, SourceLine[]>>({});
  // PIDs que cambiaron en el último evento (para animación flash)
  const flashingPIDs = reactive<Set<number>>(new Set());

  // ─── Consola ────────────────────────────────────────────────────────────
  // Salida acumulada de sys_write. Cada bloque tiene el PID que escribió y el
  // core que originó el write para colorearlos y filtrarlos por core en
  // multicore.
  interface ConsoleChunk { pid: number; coreId?: number; kind?: 'in' | 'out'; text: string; bytes?: number[] }
  const consoleChunks = ref<ConsoleChunk[]>([]);

  // ─── Estado del teclado HW ──────────────────────────────────────────────
  // Reflejo del KeyboardBuf del kernel + cola FIFO de procesos bloqueados
  // en sys_read(stdin). Llega vía evento `kbd_state` cada vez que cambia.
  interface KbdReader { pid: number; prog: string }
  const kbdBufUsed = ref(0);
  const kbdBufCap = ref(16);
  const kbdReaders = ref<KbdReader[]>([]);

  // Helper compartido por la consola y el narrador del teclado.
  function processNameOf(pid: number): string {
    if (pid === 0) return 'kernel';
    return procs.value.find(p => p.pid === pid)?.name ?? '';
  }

  // ─── Sesión multi-tenant ────────────────────────────────────────────────
  // Llega vía evento session_info al conectar + cada vez que cambia (share/unshare).
  const sessionInfo = ref<SessionInfo>({ id: '', mode: 'unknown', role: 'none', label: '', view_token: '', user_id: '', user_name: '', user_email: '' });
  // Computed cómodos para el resto de la app.
  const isViewer = () => sessionInfo.value.role === 'viewer';

  // Last presence snapshot the host has broadcast (section + visible cores
  // + presentation mode + narrators toggle). Updated by 'presence' events
  // and the initial session_info payload. The viewer shell watches it to
  // apply autofollow; the host writes to it implicitly via sendPresence
  // below.
  const lastPresence = ref<Presence>({ section: '', cores: [], mode: '', narrators: false });
  // Bumped every time a hand_raise event arrives so the host UI can react
  // without subscribing to the raw event channel.
  const handRaiseSignal = ref(0);
  const lastHandRaiseLabel = ref('');

  // Viewer-side mirror of the owner's currently-hovered glossary term. When
  // non-null, every <Term> instance whose term matches lights up its tooltip
  // alongside the owner's. Cleared by an owner_glossary_hover with off=true
  // or by the staleness timer below (5s) so a disconnected owner does not
  // leave a zombie tooltip open.
  const ownerGlossaryHover = ref<string | null>(null);
  const GLOSSARY_STALE_MS = 5000;
  let glossaryStaleTimer: number | null = null;
  function clearGlossaryStaleTimer() {
    if (glossaryStaleTimer !== null) {
      window.clearTimeout(glossaryStaleTimer);
      glossaryStaleTimer = null;
    }
  }
  function armGlossaryStaleTimer() {
    clearGlossaryStaleTimer();
    glossaryStaleTimer = window.setTimeout(() => {
      ownerGlossaryHover.value = null;
      glossaryStaleTimer = null;
    }, GLOSSARY_STALE_MS);
  }

  // ─── Animaciones globales ──────────────────────────────────────────────
  // shakeUntil es un timestamp; mientras Date.now() < shakeUntil, la UI shakea.
  const faultShake = ref(false);

  // ─── Glosario global ────────────────────────────────────────────────────
  // El componente App escucha glossaryOpenSignal para abrir el modal con la
  // búsqueda prefilteada. Cualquier elemento clickeable en la UI puede llamar
  // openGlossary("EAX") y mostrar info inmediatamente.
  const glossarySearch = ref('');
  const glossaryOpenSignal = ref(0);
  function openGlossary(term?: string) {
    glossarySearch.value = term ?? '';
    glossaryOpenSignal.value++;
  }

  // ─── WS ─────────────────────────────────────────────────────────────────
  let ws: WebSocket | null = null;

  // Auto-reconnect bookkeeping. The backoff schedule is fixed instead of a
  // pure exponential so the first retry is fast (a passing network hiccup
  // recovers in 1s) but the last buckets cap at 30s — well below the user
  // patience threshold during a class without hammering the server when it
  // is genuinely down.
  const BACKOFF_MS = [1000, 2000, 5000, 10000, 30000];
  const reconnectAttempts = ref(0);
  const nextRetryAt = ref<number | null>(null);
  let retryTimer: number | null = null;

  // Event subscribers fan out raw CpuEvents to anything that wants to react —
  // narrators, panels, debugging overlays. The store's own applyEvent runs
  // first to keep its state consistent; subscribers fire after, so by the
  // time they read regs/procs/etc the store already reflects the event.
  type Subscriber = (ev: CpuEvent) => void;
  const subscribers = new Set<Subscriber>();
  function subscribe(fn: Subscriber): () => void {
    subscribers.add(fn);
    return () => subscribers.delete(fn);
  }

  function clearRetryTimer() {
    if (retryTimer !== null) {
      window.clearTimeout(retryTimer);
      retryTimer = null;
    }
    nextRetryAt.value = null;
  }

  function scheduleReconnect() {
    if (retryTimer !== null) return;
    const idx = Math.min(reconnectAttempts.value, BACKOFF_MS.length - 1);
    const delay = BACKOFF_MS[idx];
    nextRetryAt.value = Date.now() + delay;
    retryTimer = window.setTimeout(() => {
      retryTimer = null;
      nextRetryAt.value = null;
      reconnectAttempts.value += 1;
      connect();
    }, delay);
  }

  // Force an immediate reconnect, cancelling any pending scheduled retry.
  // Surfaced through the public API for the ConnectionBanner "reconnect now"
  // button — the user gets to bypass the backoff when they know the server
  // is back up.
  function reconnectNow() {
    clearRetryTimer();
    reconnectAttempts.value += 1;
    connect();
  }

  // connect is idempotent. Re-mounting components that call connect() — for
  // example navigating away from the Simulator and back through the shell —
  // would otherwise leak a WebSocket per visit, each with its own onmessage
  // handler pushing events to the log a second, third, fourth time.
  // loadCategories fetches the catalogue's section metadata once per tab.
  // The picker uses it to render section headers in canonical order; a
  // network blip falls back to deriving sections from programs[].category.
  async function loadCategories(): Promise<void> {
    if (categories.value.length > 0) return;
    try {
      const r = await fetch('/api/categories');
      if (r.ok) {
        const data = (await r.json()) as CategoryMeta[];
        if (Array.isArray(data)) categories.value = data;
      }
    } catch {
      /* keep empty; ProgramPicker falls back to ad-hoc grouping */
    }
  }

  // bootstrapSession ensures the backend has created the mc_sid cookie before
  // we open the WebSocket. In production the cookie is already set by the Go
  // handler that served index.html, but during `make watch` the HTML comes
  // from Vite and never touches that handler — so without this call the WS
  // upgrade returns 401 and the client falls into the reconnect backoff.
  let sessionBootstrapped = false;
  async function bootstrapSession(): Promise<void> {
    if (sessionBootstrapped) return;
    try {
      const r = await fetch('/api/session', { credentials: 'same-origin' });
      if (r.ok) sessionBootstrapped = true;
    } catch {
      /* swallow: WS open will fail and trigger the normal reconnect backoff */
    }
  }

  async function connect() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    await bootstrapSession();
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    void loadCategories();
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${location.host}/ws`;
    ws = new WebSocket(url);
    ws.onopen = () => {
      connected.value = true;
      hasEverConnected.value = true;
      reconnectAttempts.value = 0;
      clearRetryTimer();
      // Connection state is already surfaced by the WS pill in the SimStatusRow.
      // Don't push it as an EventLog entry — the log is reserved for simulator
      // events (instructions, mode switches, syscalls) so a fresh session
      // shouldn't read like something just happened.
    };
    ws.onclose = () => {
      connected.value = false;
      ws = null;
      pushLog(formatLog.wsDisconnected(), 'red', undefined, undefined, 'global');
      scheduleReconnect();
    };
    ws.onerror = () => {
      pushLog(formatLog.wsError(), 'red', undefined, undefined, 'global');
      // onerror precedes onclose in the WebSocket lifecycle; rely on onclose
      // to trigger the backoff so we do not double-schedule.
    };
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        // The backend bundles every event produced in the same simulator tick
        // into one frame ({type:"tick_events", events:[...]}). Applying the
        // whole batch inside a single sync callback keeps every log.value
        // mutation in the same reactive turn, so gridRows (and any other
        // computed downstream) only re-runs once per tick.
        if (msg && msg.type === 'tick_events' && Array.isArray(msg.events)) {
          for (const ev of msg.events as CpuEvent[]) applyEvent(ev);
        } else {
          applyEvent(msg as CpuEvent);
        }
      } catch (err) {
        pushLog(formatLog.wsJsonErr(err), 'red', undefined, undefined, 'global');
      }
    };
  }

  function send(cmd: object) {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(cmd));
    }
  }

  function applyEvent(ev: CpuEvent) {
    // The tutor store listens via a window CustomEvent so cpu.ts does not
    // grow a hard dependency on it. Any payload typed as 'tutor_answer' is
    // forwarded verbatim; the tutor handler decides what to do with it.
    if ((ev as { type?: string }).type === 'tutor_answer') {
      window.dispatchEvent(new CustomEvent('modos:tutor-answer', { detail: ev }));
      return;
    }
    if ((ev as { type?: string }).type === 'tutor_progress') {
      window.dispatchEvent(new CustomEvent('modos:tutor-progress', { detail: ev }));
      return;
    }
    switch (ev.type) {
      case 'snapshot':
        if (ev.cores) cores.value = ev.cores;
        if (ev.num_cores !== undefined) numCores.value = ev.num_cores;
        // Drop filter ids that no longer exist (after a SetNumCores shrink);
        // repopulate with all available ones if the filter would otherwise
        // be empty.
        reconcileFilterWithCores(cores.value.map(c => c.id));
        // Mirror the selected core's view into the top-level refs so the
        // panels that read cpu.regs/mode/etc keep working. Falls back to
        // the payload's boot-core copy if cores[] is missing for some
        // reason (legacy snapshot format).
        const sel = cores.value.find(x => x.id === selectedCoreId.value);
        if (sel) {
          for (const k of Object.keys(regs)) regs[k] = sel.regs[k] ?? 0;
          for (const k of Object.keys(segments)) segments[k] = sel.segments[k] ?? 0;
          mode.value = sel.mode;
          halted.value = sel.halted;
          currentPID.value = sel.current_pid;
        } else {
          for (const k of Object.keys(regs)) regs[k] = ev.regs[k] ?? 0;
          for (const k of Object.keys(segments)) segments[k] = ev.segments[k] ?? 0;
          mode.value = ev.mode;
          halted.value = ev.halted;
          if (ev.current_pid !== undefined) currentPID.value = ev.current_pid;
        }
        tick.value = ev.tick;
        if (ev.timer_quantum !== undefined) timerQuantum.value = ev.timer_quantum;
        if (ev.running !== undefined) running.value = ev.running;
        if (ev.paused !== undefined) paused.value = ev.paused;
        if (ev.clockHz !== undefined) clockHz.value = ev.clockHz;
        if (ev.speedMul !== undefined) speedMul.value = ev.speedMul;
        if (ev.ready_queue) readyQueue.value = ev.ready_queue;
        if (ev.wait_queue) waitQueue.value = ev.wait_queue;
        if (ev.procs) procs.value = ev.procs;
        if (ev.sources) {
          procSources.value = {};
          for (const [pid, src] of Object.entries(ev.sources)) {
            procSources.value[Number(pid)] = src;
          }
        }
        if (ev.source_maps) {
          procSourceMaps.value = {};
          for (const [pid, lines] of Object.entries(ev.source_maps)) {
            procSourceMaps.value[Number(pid)] = lines;
          }
        }
        changedRegs.clear();
        for (const k of Object.keys(changedRegsByCore)) delete changedRegsByCore[Number(k)];
        break;
      case 'programs':
        programs.value = ev.list;
        if (!selectedProgram.value && ev.list[0]) selectedProgram.value = ev.list[0].name;
        break;
      case 'reg_write': {
        const c = ensureCoreSlot(ev.core_id);
        c.regs[ev.reg] = ev.new;
        markCoreActive(c, ev.core_id);
        // Stamp the per-core flash unconditionally — every visible core lights
        // up its own register strip on a write, not only the focused one.
        flashReg(ev.reg, ev.core_id);
        if (isSelected(ev.core_id)) {
          regs[ev.reg] = ev.new;
        }
        break;
      }
      case 'instruction': {
        const c = ensureCoreSlot(ev.core_id);
        const nextEIP = ev.eip + ev.bytes.length;
        c.regs.EIP = nextEIP;
        markCoreActive(c, ev.core_id);
        lastMnemonic[ev.core_id] = ev.mnemonic;
        // EventsLog renders the per-core chip from coreId so the text itself
        // stays clean and identical across single- and multi-core sessions.
        pushLog(formatLog.instruction(ev.eip, ev.mnemonic), 'amber', ev.core_id, ev.tick);
        if (isSelected(ev.core_id)) regs.EIP = nextEIP;
        break;
      }
      case 'tick':
        tick.value = ev.value;
        // Optimistically bump idle_ticks for every core currently bound to
        // the swapper so the UI sees a live heartbeat without waiting for
        // the next full snapshot. Snapshots from the server overwrite the
        // value when they arrive, keeping the local counter in sync.
        for (const c of cores.value) {
          if (c.running_idle) c.idle_ticks = (c.idle_ticks ?? 0) + 1;
        }
        break;
      case 'mode_switch': {
        const c = ensureCoreSlot(ev.core_id);
        c.mode = ev.to;
        pushLog(formatLog.modeSwitch(ev.from, ev.to, ev.cause), 'red', ev.core_id, ev.tick);
        // K = kernel code; mode_switch is HW, so strip it from the entry.
        const lastMS = log.value[log.value.length - 1];
        if (lastMS) lastMS.inKernel = false;
        if (isSelected(ev.core_id)) mode.value = ev.to;
        if (ev.new_cs  !== undefined) c.segments.CS = ev.new_cs;
        if (ev.new_ds  !== undefined) c.segments.DS = ev.new_ds;
        if (ev.new_ss  !== undefined) c.segments.SS = ev.new_ss;
        if (ev.new_esp !== undefined) c.regs.ESP    = ev.new_esp;
        if (isSelected(ev.core_id)) {
          if (ev.new_cs  !== undefined) segments.CS = ev.new_cs;
          if (ev.new_ds  !== undefined) segments.DS = ev.new_ds;
          if (ev.new_ss  !== undefined) segments.SS = ev.new_ss;
          if (ev.new_esp !== undefined) regs.ESP    = ev.new_esp;
        }
        delete lastMnemonic[ev.core_id];
        // Defer the frame-slot unmount so the InterruptNarrator can still
        // read DOM coordinates for the IRET mini-arrows before Vue flushes.
        if (ev.cause === 'IRET') {
          const coreId = ev.core_id;
          queueMicrotask(() => {
            activeFrameByCore.value = { ...activeFrameByCore.value, [coreId]: null };
          });
        }
        break;
      }
      case 'irq_fire': {
        pushLog(formatLog.irq(ev.vector, ev.source), 'yellow', ev.core_id, ev.tick);
        // K = kernel code; HW IRQ delivery is not, strip it.
        const lastIRQ = log.value[log.value.length - 1];
        if (lastIRQ) lastIRQ.inKernel = false;
        lastFiredHandlerByVector.value = {
          ...lastFiredHandlerByVector.value,
          [ev.vector]: {
            coreId: ev.core_id,
            handler: ev.handler ?? 0,
            source: ev.source,
            ts: Date.now(),
          },
        };
        // Only user→kernel traps push a frame. Kernel→kernel IRQs (timer
        // arriving while already in kernel mode) leave saved_* undefined.
        if (ev.from_mode === 3 && ev.saved_eip !== undefined) {
          activeFrameByCore.value = {
            ...activeFrameByCore.value,
            [ev.core_id]: {
              vector: ev.vector,
              handler: ev.handler ?? 0,
              savedEIP: ev.saved_eip,
              savedCS: ev.saved_cs ?? 0,
              savedEFLAGS: ev.saved_eflags ?? 0,
              fromMode: ev.from_mode,
              capturedAt: Date.now(),
            },
          };
        }
        break;
      }
      case 'page_fault': {
        pushLog(formatLog.pageFault(ev.addr, ev.perm_needed, ev.mode), 'red', ev.core_id, ev.tick);
        faultShake.value = true;
        setTimeout(() => { faultShake.value = false; }, 500);
        break;
      }
      case 'halt': {
        const c = ensureCoreSlot(ev.core_id);
        c.halted = true;
        const pid = (ev as { pid?: number }).pid ?? -1;
        // PID 0 is the swapper parking on HLT between IRQs — non-fatal, cyan.
        // Any other PID is the terminal cascade (process death) — red.
        const isIdle = pid === 0;
        pushLog(formatLog.halt(pid), isIdle ? 'cyan' : 'red', ev.core_id, ev.tick);
        // Only the cascade halt freezes the UI as "machine halted". Swapper
        // HLT keeps the run loop alive — the next IRQ wakes it back up.
        if (!isIdle && isSelected(ev.core_id)) halted.value = true;
        break;
      }
      case 'state':
        if (ev.halted !== undefined) halted.value = ev.halted;
        if (ev.running !== undefined) running.value = ev.running;
        if (ev.paused !== undefined) paused.value = ev.paused;
        if (ev.clockHz !== undefined) clockHz.value = ev.clockHz;
        if (ev.speedMul !== undefined) speedMul.value = ev.speedMul;
        break;
      case 'loaded': {
        pushLog(formatLog.loaded(ev.name, ev.pids.length, ev.pids), 'green', undefined, undefined, 'global');
        procs.value = ev.procs;
        // Pick a sensible default for the "Mostrar init" toggle based on how
        // many user-space procs the demo carries (init excluded). Single-proc
        // demos like ADD start with init hidden; fork/threads demos reveal it.
        const prefs = usePreferencesStore();
        const userProcs = ev.procs.filter(p => p.name !== 'init').length;
        prefs.applyShowInitDefault(userProcs);
        // A new demo starts with no open IPC channels — the kernel
        // resets its Pipes registry alongside its process table.
        pipes.value = [];
        if (ev.sources) {
          procSources.value = {};
          for (const [pid, src] of Object.entries(ev.sources)) {
            procSources.value[Number(pid)] = src;
          }
        }
        if (ev.source_maps) {
          procSourceMaps.value = {};
          for (const [pid, lines] of Object.entries(ev.source_maps)) {
            procSourceMaps.value[Number(pid)] = lines;
          }
        } else {
          procSourceMaps.value = {};
        }
        break;
      }
      case 'reset':
        pushLog(formatLog.reset(), 'cyan', undefined, undefined, 'global');
        changedRegs.clear();
        for (const k of Object.keys(changedRegsByCore)) delete changedRegsByCore[Number(k)];
        procs.value = [];
        readyQueue.value = [];
        waitQueue.value = [];
        currentPID.value = 0;
        consoleChunks.value = [];
        kbdBufUsed.value = 0;
        kbdReaders.value = [];
        procSources.value = {};
        procSourceMaps.value = {};
        mutexes.value = [];
        semaphores.value = [];
        pipes.value = [];
        activeFrameByCore.value = {};
        lastFiredHandlerByVector.value = {};
        pendingMemoryJump.value = null;
        for (const k of Object.keys(lastMnemonic)) delete lastMnemonic[Number(k)];
        // Reset per-core register/state caches so the next snapshot starts
        // from a clean slate. Without this, stale values from the previous
        // demo would survive a reset on cores that the new demo does not
        // touch (smaller process count than core count).
        for (const c of cores.value) {
          for (const k of Object.keys(c.regs)) c.regs[k] = 0;
          for (const k of Object.keys(c.segments)) c.segments[k] = 0;
          c.mode = 3;
          c.halted = false;
          c.tick = 0;
          c.current_pid = 0;
        }
        for (const k of Object.keys(regs)) regs[k] = 0;
        for (const k of Object.keys(segments)) segments[k] = 0;
        mode.value = 3;
        halted.value = false;
        paused.value = false;
        break;
      case 'procs':
        procs.value = ev.procs;
        break;
      case 'pcb_save': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) p.regs = ev.regs;
        flashPID(ev.pid);
        pushLog(formatLog.pcbSave(ev.pid, ev.regs.EIP || 0), 'red', ev.core_id, ev.tick, undefined, 'scheduler');
        break;
      }
      case 'pcb_restore': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) p.regs = ev.regs;
        // Inside a handler (c.mode === 0) the ring-dependent regs live in
        // the patched trap frame and become live only at the IRET
        // mode_switch — never overwrite them here. Ring-independent GP
        // regs are safe either way.
        const c = ensureCoreSlot(ev.core_id);
        const ringIndependent = ['EAX', 'EBX', 'ECX', 'EDX', 'ESI', 'EDI', 'EBP', 'EFLAGS'] as const;
        for (const k of ringIndependent) {
          if (k in ev.regs) c.regs[k] = ev.regs[k];
        }
        if (c.mode !== 0) {
          if ('EIP' in ev.regs) c.regs.EIP = ev.regs.EIP;
          if ('ESP' in ev.regs) c.regs.ESP = ev.regs.ESP;
          for (const k of ['CS', 'DS', 'SS'] as const) {
            if (k in ev.regs) c.segments[k] = ev.regs[k];
          }
        }
        if (isSelected(ev.core_id)) {
          for (const k of ringIndependent) {
            if (k in ev.regs) regs[k] = ev.regs[k];
          }
          if (c.mode !== 0) {
            if ('EIP' in ev.regs) regs.EIP = ev.regs.EIP;
            if ('ESP' in ev.regs) regs.ESP = ev.regs.ESP;
            for (const k of ['CS', 'DS', 'SS'] as const) {
              if (k in ev.regs) segments[k] = ev.regs[k];
            }
          }
        }
        flashPID(ev.pid);
        pushLog(formatLog.pcbRestore(ev.pid, ev.regs.EIP || 0), 'green', ev.core_id, ev.tick, undefined, 'scheduler');
        break;
      }
      case 'sched_decision': {
        // currentPID top-level is owned by core_assign/core_release so the
        // SimBar mirrors the *selected* core only. Updating it here would
        // pull the PID of whichever core just scheduled, regardless of
        // which one the user is watching.
        // ready_queue arrives as null when the kernel's queue is empty;
        // normalize so consumers see an empty array.
        const rq = ev.ready_queue ?? [];
        readyQueue.value = rq;
        pushLog(formatLog.schedule(ev.from_pid, ev.to_pid, ev.reason, rq), 'yellow', ev.core_id, ev.tick, undefined, 'scheduler');
        break;
      }
      case 'wake': {
        // BLOCKED → READY transition. Sync the ready queue so the haltedDead
        // heuristic in SimBar (halted && readyQueue.empty) does not stay
        // stuck after a child sys_exit wakes a parent. The state change
        // itself travels in the accompanying EventProcessState; this event
        // exists so wakes are not conflated with scheduling decisions.
        const rq = ev.ready_queue ?? [];
        readyQueue.value = rq;
        pushLog(formatLog.wake(ev.pid, ev.from_reason, rq), 'yellow', ev.core_id, ev.tick, undefined, 'scheduler');
        break;
      }
      case 'init_reap': {
        // Kernel-emulated reap cycle of PID 1: init does not occupy a core,
        // it drains every PendingReap and goes back to wait(any) without
        // executing user code. The event carries one ReapDetail per zombie
        // collected (with Reparented telling apart genuine orphans from
        // birth-adopted children of init).
        const rq = ev.ready_queue ?? [];
        readyQueue.value = rq;
        for (const r of ev.reaps) {
          // Remove the reaped child from the local procs mirror; the
          // backend already dropped it from k.Processes.
          const idx = procs.value.findIndex(x => x.pid === r.child_pid);
          if (idx >= 0) procs.value.splice(idx, 1);
        }
        pushLog(formatLog.initReap(ev.init_pid, ev.reaps), 'yellow', ev.core_id, ev.tick, undefined, 'scheduler');
        break;
      }
      case 'process_state': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) {
          p.state = ev.state;
          if (ev.reason) p.blocked_on = ev.reason;
          // The kernel attaches exit_code on the ZOMBIE transition so the
          // panel does not need to wait for the next full procs snapshot.
          if (ev.exit_code !== undefined) p.exit_code = ev.exit_code;
          // Stamp the local tick when entering BLOCKED so the Concurrency view
          // can render "esperando hace N ticks" without waiting for the next
          // snapshot. Clear on any other transition to avoid stale residue.
          if (ev.state === 'BLOCKED') {
            p.blocked_since = tick.value;
          } else {
            p.blocked_since = undefined;
          }
        }
        // Mantener wait_queue sincronizada con BLOCKED ⇄ otros estados.
        if (ev.state === 'BLOCKED') {
          if (!waitQueue.value.find(w => w.pid === ev.pid)) {
            waitQueue.value.push({
              pid: ev.pid,
              name: p?.name ?? `P${ev.pid}`,
              reason: ev.reason ?? '',
              blocked_since: tick.value,
            });
          }
        } else {
          waitQueue.value = waitQueue.value.filter(w => w.pid !== ev.pid);
        }
        break;
      }
      case 'syscall':
        pushLog(formatLog.syscall(ev.name, ev.args, ev.result, ev.pid), 'green', ev.core_id, ev.tick);
        break;
      case 'console_out':
        consoleChunks.value.push({
          pid: ev.pid,
          coreId: ev.core_id,
          kind: ev.kind ?? (ev.pid === 0 ? 'in' : 'out'),
          text: ev.text,
          bytes: ev.bytes,
        });
        if (consoleChunks.value.length > 200) consoleChunks.value.splice(0, consoleChunks.value.length - 200);
        break;
      case 'kbd_state':
        kbdBufUsed.value = ev.buf_used ?? 0;
        kbdBufCap.value = ev.buf_cap ?? 16;
        kbdReaders.value = Array.isArray(ev.readers) ? ev.readers : [];
        break;
      case 'session_info':
        sessionInfo.value = {
          id: ev.id,
          mode: ev.mode as SessionInfo['mode'],
          role: ev.role as SessionInfo['role'],
          label: ev.label,
          view_token: ev.view_token,
          user_id: ev.user_id ?? '',
          user_name: ev.user_name ?? '',
          user_email: ev.user_email ?? '',
        };
        if (ev.presence && ev.presence.section) {
          lastPresence.value = {
            section: ev.presence.section,
            cores: ev.presence.cores ?? [],
            mode: (ev.presence.mode as Presence['mode']) ?? '',
            narrators: Boolean(ev.presence.narrators),
            focus: ev.presence.focus ?? {},
          };
        }
        if (ev.role === 'viewer') {
          pushLog(formatLog.spectatorMode(ev.label || ev.id.slice(0, 6)), 'cyan', undefined, undefined, 'global');
        }
        break;
      case 'presence':
        lastPresence.value = {
          section: ev.section,
          cores: ev.cores ?? [],
          mode: (ev.mode as Presence['mode']) ?? '',
          narrators: Boolean(ev.narrators),
          focus: ev.focus ?? {},
        };
        break;
      case 'hand_raise':
        lastHandRaiseLabel.value = ev.label;
        handRaiseSignal.value++;
        break;
      case 'owner_glossary_hover':
        if (ev.off) {
          ownerGlossaryHover.value = null;
          clearGlossaryStaleTimer();
        } else if (typeof ev.term === 'string' && ev.term.length > 0) {
          ownerGlossaryHover.value = ev.term;
          armGlossaryStaleTimer();
        }
        break;
      case 'error':
        lastError.value = ev.message;
        if (ev.code) {
          pushLog(formatLog.backendError(ev.code, ev.params, ev.message), 'red', undefined, undefined, 'global');
        } else {
          pushLog(formatLog.genericErr(ev.message), 'red', undefined, undefined, 'global');
        }
        break;
      case 'mem_read':
      case 'mem_write': {
        // Memory traffic is consumed exclusively by subscribers (narrators,
        // memory panel filters, etc.) — the store carries no aggregated
        // counter for it. We only use it as a heartbeat to clear a stale
        // HALT flag when an IRQ has actually woken the core back up.
        const c = cores.value.find(x => x.id === ev.core_id);
        if (c) markCoreActive(c, ev.core_id);
        break;
      }
      case 'core_assign': {
        const c = ensureCoreSlot(ev.core_id);
        c.current_pid = ev.pid;
        // Any non-zero PID assignment unbinds the swapper from this core.
        // PID 0 also lands here when the kernel uses EventCoreAssign for
        // the idle case; the dedicated core_idle event sets the flag back.
        if (ev.pid !== 0) c.running_idle = false;
        markCoreActive(c, ev.core_id);
        pushLog(formatLog.coreAssign(ev.core_id, ev.pid), 'amber', ev.core_id, ev.tick);
        if (isSelected(ev.core_id)) currentPID.value = ev.pid;
        break;
      }
      case 'core_release': {
        const c = ensureCoreSlot(ev.core_id);
        c.current_pid = 0;
        c.running_idle = false;
        pushLog(formatLog.coreRelease(ev.core_id, ev.pid, ev.reason), 'amber', ev.core_id, ev.tick);
        if (isSelected(ev.core_id)) currentPID.value = 0;
        break;
      }
      case 'core_idle': {
        const c = ensureCoreSlot(ev.core_id);
        c.running_idle = true;
        break;
      }
      case 'mutex_op': {
        let m = mutexes.value.find(x => x.id === ev.mutex_id);
        if (!m) {
          m = { id: ev.mutex_id, holder: -1, waiters: [] };
          mutexes.value.push(m);
        }
        switch (ev.op) {
          case 'create':
            // Object already added above; nothing else to track.
            pushLog(formatLog.mutexCreated(ev.mutex_id, ev.pid), 'green', ev.core_id, ev.tick);
            break;
          case 'acquired':
            m.holder = ev.pid;
            m.waiters = ev.waiters ?? [];
            pushLog(formatLog.mutexAcquired(ev.mutex_id, ev.pid), 'green', ev.core_id, ev.tick);
            break;
          case 'waiting':
            m.waiters = ev.waiters ?? [];
            pushLog(formatLog.mutexWaiting(ev.mutex_id, ev.pid), 'yellow', ev.core_id, ev.tick);
            break;
          case 'released':
            m.holder = -1;
            m.waiters = ev.waiters ?? [];
            pushLog(formatLog.mutexReleased(ev.mutex_id, ev.pid), 'cyan', ev.core_id, ev.tick);
            break;
          case 'handoff':
            m.holder = ev.next_pid ?? -1;
            m.waiters = ev.waiters ?? [];
            pushLog(formatLog.mutexHandoff(ev.mutex_id, ev.pid, ev.next_pid ?? -1), 'green', ev.core_id, ev.tick);
            break;
          case 'rejected':
            pushLog(formatLog.mutexRejected(ev.mutex_id, ev.pid), 'red', ev.core_id, ev.tick);
            break;
        }
        break;
      }
      case 'pipe_create': {
        // Avoid duplicates if the server replays the snapshot after a
        // reconnect — the kernel guarantees pipe_id is monotonic per
        // session, so an existing entry just refreshes its metadata.
        const existing = pipes.value.find(p => p.pipe_id === ev.pipe_id);
        if (existing) {
          existing.owner_pid = ev.owner_pid;
          existing.fd_r = ev.fd_r;
          existing.fd_w = ev.fd_w;
          existing.is_fifo = ev.is_fifo;
          existing.name = ev.name;
        } else {
          pipes.value.push({
            pipe_id: ev.pipe_id, owner_pid: ev.owner_pid,
            fd_r: ev.fd_r, fd_w: ev.fd_w,
            is_fifo: ev.is_fifo, name: ev.name,
            buf_level: 0, buf_cap: 16,
          });
        }
        break;
      }
      case 'pipe_op': {
        const p = pipes.value.find(x => x.pipe_id === ev.pipe_id);
        if (p) {
          p.buf_level = ev.buf_level;
          p.buf_cap = ev.buf_cap;
          p.last_bytes = ev.bytes;
          p.last_op = ev.op;
          p.last_pid = ev.pid;
        }
        break;
      }
      case 'fork': {
        // Tree shape is rebuilt from the next 'procs' snapshot; here we
        // just flash both PCBs so the user notices the relationship.
        flashPID(ev.parent_pid);
        flashPID(ev.child_pid);
        pushLog(formatLog.forkSpawn(ev.parent_pid, ev.child_pid, ev.is_vfork), 'cyan', ev.core_id, ev.tick);
        break;
      }
      case 'reparent': {
        // Same as fork: the periodic procs snapshot carries the new
        // parent_pid. We flash the affected PCB so the move is visible.
        flashPID(ev.pid);
        // Update parent_pid locally so the tree re-renders without
        // waiting for the next procs snapshot.
        const moved = procs.value.find(x => x.pid === ev.pid);
        if (moved) moved.parent_pid = ev.new_parent;
        pushLog(formatLog.reparent(ev.pid, ev.old_parent, ev.new_parent), 'cyan', ev.core_id, ev.tick);
        break;
      }
      case 'thread_create': {
        // A sibling thread was spawned via clone(CLONE_THREAD). Flash both
        // creator and new task so the user sees them tied to the same
        // address space; the next 'procs' snapshot fills in tgid/tid for
        // the panel to fold the new TID under its leader.
        flashPID(ev.creator_pid);
        flashPID(ev.tid);
        pushLog(formatLog.threadCreate(ev.tid, ev.tgid, ev.creator_pid), 'cyan', ev.core_id, ev.tick);
        break;
      }
      case 'thread_exit': {
        flashPID(ev.tid);
        pushLog(formatLog.threadExit(ev.tid, ev.tgid, ev.retval), 'yellow', ev.core_id, ev.tick);
        break;
      }
      case 'wait_op': {
        if (ev.op === 'reap') {
          // Drop the reaped zombie from the local snapshot so the panel
          // empties at the same tick the kernel emits the event. Without
          // this the zombie sticks around until the next procs broadcast
          // (which never comes when AllHalted fires immediately after).
          procs.value = procs.value.filter(x => x.pid !== ev.child_pid);
          // Drop the orphan from the parent's children list too.
          const parent = procs.value.find(x => x.pid === ev.pid);
          if (parent && Array.isArray(parent.children)) {
            parent.children = parent.children.filter(c => c !== ev.child_pid);
          }
          flashPID(ev.pid);
          // Init reaps no longer travel as EventWaitOp — they fire as the
          // consolidated EventInitReap, formatted by formatLog.initReap.
          // So a wait_op event reaching here is always a regular parent
          // collecting through sys_waitpid.
          pushLog(formatLog.waitReap(ev.pid, ev.child_pid, ev.exit_code ?? 0), 'yellow', ev.core_id, ev.tick);
        } else if (ev.op === 'call') {
          pushLog(formatLog.waitCall(ev.pid, ev.child_pid), 'cyan', ev.core_id, ev.tick);
        }
        break;
      }
      case 'sem_op': {
        let s = semaphores.value.find(x => x.id === ev.sem_id);
        if (!s) {
          s = { id: ev.sem_id, count: ev.count, waiters: [] };
          semaphores.value.push(s);
        }
        s.count = ev.count;
        s.waiters = ev.waiters ?? [];
        switch (ev.op) {
          case 'create':
            pushLog(formatLog.semCreated(ev.sem_id, ev.count), 'green', ev.core_id, ev.tick);
            break;
          case 'wait_immediate':
            pushLog(formatLog.semWait(ev.sem_id, ev.pid, ev.count), 'cyan', ev.core_id, ev.tick);
            break;
          case 'wait_blocked':
            pushLog(formatLog.semBlocked(ev.sem_id, ev.pid), 'yellow', ev.core_id, ev.tick);
            break;
          case 'signal':
            pushLog(formatLog.semSignal(ev.sem_id, ev.pid, ev.count), 'green', ev.core_id, ev.tick);
            break;
          case 'handoff':
            pushLog(formatLog.semHandoff(ev.sem_id, ev.pid, ev.next_pid ?? -1), 'green', ev.core_id, ev.tick);
            break;
        }
        break;
      }
    }
    // Fan-out to subscribers AFTER the store has reacted, so they can read
    // the up-to-date snapshot in their handler.
    if (subscribers.size > 0) {
      for (const fn of subscribers) {
        try { fn(ev); } catch (err) { pushLog(formatLog.subscriberErr(err), 'red'); }
      }
    }
  }

  function flashReg(name: string, coreId: number = selectedCoreId.value) {
    const slot = ensureChangedSlot(coreId);
    slot.add(name);
    if (isSelected(coreId)) changedRegs.add(name);
    setTimeout(() => {
      slot.delete(name);
      if (isSelected(coreId)) changedRegs.delete(name);
    }, 800);
  }

  function flashPID(pid: number) {
    flashingPIDs.add(pid);
    setTimeout(() => flashingPIDs.delete(pid), 800);
  }

  function pushLog(
    text: string,
    color: string,
    coreId?: number,
    tick?: number,
    scope?: 'global',
    kind?: 'scheduler',
  ) {
    // Derive inKernel from the live per-core mode tracked in cores.value.
    // mode_switch events update that value before subsequent pushLog calls
    // (the WS handler dispatches events in arrival order), so a kernel-mode
    // instruction that arrives right after mode_switch{to:0} is correctly
    // tagged. Mode 0 = PL0 (kernel), 3 = PL3 (user).
    let inKernel: boolean | undefined;
    if (coreId !== undefined) {
      const c = cores.value.find(x => x.id === coreId);
      if (c) inKernel = c.mode === 0;
    }
    log.value.push({ ts: Date.now(), text, color, coreId, tick, scope, inKernel, kind });
    if (log.value.length > 500) log.value.splice(0, log.value.length - 500);
  }

  // ─── Comandos públicos ──────────────────────────────────────────────────
  function load(name: string) { send({ cmd: 'load', program: name }); }
  function step() { send({ cmd: 'step' }); }
  function run(speed: number) { send({ cmd: 'run', speed }); }
  function pause() { send({ cmd: 'pause' }); }
  function reset() {
    // Reset es "borrar todo": el backend resetea kernel/CPU/procesos y nosotros
    // wipeamos el log y consola para empezar de cero visualmente.
    send({ cmd: 'reset' });
    log.value = [];
    consoleChunks.value = [];
    kbdBufUsed.value = 0;
    kbdReaders.value = [];
  }
  function clearLog() {
    // Limpia solo la vista (log + consola) sin tocar el estado del simulador.
    // Útil durante una demo para "limpiar la pizarra" sin perder el programa cargado.
    log.value = [];
    consoleChunks.value = [];
  }
  function setQuantum(q: number) {
    timerQuantum.value = q;
    send({ cmd: 'timer_quantum', quantum: q });
  }
  function setClockHz(hz: number) {
    // The backend clamps to [1, 60] too; doing it here keeps the optimistic
    // UI in sync with what the server will accept.
    const clamped = Math.max(1, Math.min(60, Math.round(hz)));
    clockHz.value = clamped;
    send({ cmd: 'set_clock_hz', clockHz: clamped });
  }
  function setSpeedMul(mul: number) {
    const clamped = Math.max(0.1, Math.min(5.0, mul));
    speedMul.value = clamped;
    send({ cmd: 'set_speed_mul', speedMul: clamped });
  }
  // setSpeed is the deprecated single-knob API; routes to setClockHz so the
  // run loop's effective rate keeps matching the user's input until the last
  // caller migrates.
  function setSpeed(hz: number) {
    setClockHz(hz);
  }
  function pushKey(key: string) { send({ cmd: 'push_key', key }); }
  function share() { send({ cmd: 'share' }); }
  function unshare() { send({ cmd: 'unshare' }); }
  function setAlias(alias: string) { send({ cmd: 'set_alias', alias }); }
  function setNumCores(n: number) {
    send({ cmd: 'set_num_cores', cores: n });
    setVisibleCores(Array.from({ length: Math.max(1, n) }, (_, i) => i));
  }
  function setIdleTaskEnabled(enabled: boolean) { send({ cmd: 'set_idle_task', idle_task: enabled }); }

  // idleTaskEnabled is derived from the process table: PID 0 only appears
  // when the kernel toggle is on (LoadProgram / SetIdleTaskEnabled spawn
  // it, SetIdleTaskEnabled(false) does NOT despawn but the broadcast
  // procs list excludes it via processListLocked). Using the snapshot as
  // single source of truth avoids drifting state when reconnecting.
  const idleTaskEnabled = computed(() => procs.value.some(p => p.is_idle === true));

  // Presence is owner-only: send the current section + visible cores so
  // autofollowing viewers can mirror them. Debounced so a flurry of route
  // changes (or a select-all toggle) collapses into a single broadcast.
  let presenceTimer: number | null = null;
  const PRESENCE_DEBOUNCE_MS = 300;
  // Owner-only helper: ship a partial focus update without forcing the caller
  // to re-supply section/cores/mode/narrators. The current snapshot in
  // lastPresence.value is merged with the delta so a component only worries
  // about the focus field it actually owns (middleTab, pinnedPID, etc.).
  function sendFocus(delta: Partial<Focus>) {
    if (sessionInfo.value.role !== 'owner') return;
    const current = lastPresence.value;
    if (!current.section) return; // No section yet — the next presence will carry the focus.
    const mergedFocus: Focus = { ...(current.focus ?? {}), ...delta };
    sendPresence({
      ...current,
      focus: mergedFocus,
    });
  }

  function sendPresence(p: Presence) {
    if (sessionInfo.value.role !== 'owner') return;
    const focus: Focus | undefined = p.focus ? { ...p.focus } : undefined;
    // Track locally too — keeps the owner's own state in lockstep with what
    // viewers receive, and lets late subscribers read it directly.
    lastPresence.value = {
      section: p.section,
      cores: [...p.cores],
      mode: p.mode,
      narrators: p.narrators,
      focus,
    };
    if (presenceTimer !== null) {
      window.clearTimeout(presenceTimer);
    }
    presenceTimer = window.setTimeout(() => {
      presenceTimer = null;
      send({
        cmd: 'presence',
        section: p.section,
        visible_cores: p.cores,
        mode: p.mode,
        narrators: p.narrators,
        focus,
      });
    }, PRESENCE_DEBOUNCE_MS);
  }
  // Viewer-only: tap the host on the shoulder. Backend forwards a hand_raise
  // event to the owner with our session label so they see who knocked.
  function raiseHand() {
    if (sessionInfo.value.role !== 'viewer') return;
    send({ cmd: 'hand_raise' });
  }

  // Owner-only: broadcast the local pointer to every viewer. Coordinates are
  // DOM-anchored — the caller resolves the [data-pizarra-anchor] element
  // under the mouse and emits {anchor, offsetX, offsetY} normalized to that
  // element. The server validates anchor shape and offset range.
  function sendPointer(anchor: string, offsetX: number, offsetY: number, routePath: string) {
    if (sessionInfo.value.role !== 'owner') return;
    send({ cmd: 'pointer', anchor, offset_x: offsetX, offset_y: offsetY, route: routePath });
  }
  function sendPointerOff() {
    if (sessionInfo.value.role !== 'owner') return;
    send({ cmd: 'pointer_off' });
  }
  // Owner-only: announce a click-ripple at a specific anchor + offset. The
  // server validates the anchor regex and offset range, then broadcasts an
  // owner_click event to viewers; the WhiteboardClickRipple overlay renders
  // a short-lived pulse on the same anchor in the viewer's DOM.
  function sendClick(anchor: string, offsetX: number, offsetY: number, routePath: string) {
    if (sessionInfo.value.role !== 'owner') return;
    send({ cmd: 'click', anchor, offset_x: offsetX, offset_y: offsetY, route: routePath });
  }
  // Owner-only: tell every viewer that the host just opened the glossary
  // tooltip for a given term. The viewer side opens its own tooltip on any
  // <Term> with the same canonical term string. Only one term is hovered at
  // a time so we ignore re-entries.
  function sendGlossaryHover(term: string) {
    if (sessionInfo.value.role !== 'owner') return;
    if (!term) return;
    send({ cmd: 'glossary_hover', term });
  }
  function sendGlossaryHoverOff() {
    if (sessionInfo.value.role !== 'owner') return;
    send({ cmd: 'glossary_hover_off' });
  }

  // viewerLink arma la URL completa que el owner puede copiar para compartir.
  function viewerLink(): string {
    const t = sessionInfo.value.view_token;
    if (!t) return '';
    return `${location.origin}/v/${t}`;
  }

  return {
    regs, segments, mode, halted, running, paused, tick,
    stepHz, clockHz, speedMul, tickIntervalMs, timerQuantum,
    changedRegs, changedRegsByCore,
    programs, categories, loadCategories, selectedProgram, log, connected, hasEverConnected, lastError,
    reconnectAttempts, nextRetryAt, reconnectNow,
    procs, currentPID, readyQueue, waitQueue, flashingPIDs, procSources, procSourceMaps,
    pendingMemoryJump, requestMemoryJump, clearMemoryJump, lastMnemonic,
    cores, numCores, selectedCoreId, selectedCoreIds, visibleCores, isCoreVisible,
    toggleCoreSelection, selectAllCores, selectOnlyCore, setSelectedCore, setVisibleCores,
    mutexes, semaphores, pipes,
    activeFrameByCore, lastFiredHandlerByVector,
    subscribe,
    consoleChunks, faultShake, kbdBufUsed, kbdBufCap, kbdReaders, processNameOf,
    glossarySearch, glossaryOpenSignal,
    sessionInfo, isViewer, viewerLink,
    lastPresence, handRaiseSignal, lastHandRaiseLabel, ownerGlossaryHover,
    connect, load, step, run, pause, reset, clearLog, setQuantum,
    setSpeed, setClockHz, setSpeedMul, pushKey, openGlossary,
    share, unshare, setAlias, setNumCores, setIdleTaskEnabled, idleTaskEnabled, sendPresence, sendFocus, raiseHand, sendPointer, sendPointerOff, sendClick, sendGlossaryHover, sendGlossaryHoverOff,
    // Tutor: keep the wire shape private to this store so consumers do not
    // have to know the WS command name.
    sendTutorAsk: (
      question: string,
      requestId: string,
      history?: { role: 'user' | 'assistant'; text: string }[],
    ) => send({ cmd: 'ask', question, request_id: requestId, history }),
    // Test seam: vitest fakes the WebSocket by dispatching events directly
    // through this entry point. Production code uses the real ws.onmessage.
    __applyEvent: applyEvent,
  };
});
