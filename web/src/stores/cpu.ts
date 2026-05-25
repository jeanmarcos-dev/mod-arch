import { defineStore } from 'pinia';
import { computed, reactive, ref } from 'vue';

// Tipos de eventos que llegan por WS (espejo de internal/cpu/events.go).
// Todos los eventos generados por la CPU llevan core_id: con multicore el
// store mantiene un CoreInfo por core y sólo "espejea" al estado top-level
// (regs, mode, halted, currentPID) cuando el evento viene del core que el
// usuario eligió ver en la SimBar.
export type CpuEvent =
  | { type: 'instruction'; core_id: number; pid: number; eip: number; mnemonic: string; bytes: number[] }
  | { type: 'reg_write'; core_id: number; reg: string; old: number; new: number }
  | { type: 'mem_read'; core_id: number; addr: number; value: number; size: number }
  | { type: 'mem_write'; core_id: number; addr: number; old: number; new: number; size: number }
  | { type: 'mode_switch'; core_id: number; from: number; to: number; cause: string }
  | { type: 'irq_fire'; core_id: number; vector: number; source: string; handler: number; from_mode: number; saved_eip?: number; saved_cs?: number; saved_eflags?: number }
  | { type: 'page_fault'; core_id: number; addr: number; mode: number; perm_needed: string }
  | { type: 'halt'; core_id: number; pid?: number }
  | { type: 'tick'; value: number }
  | { type: 'state'; halted?: boolean; running?: boolean; hz?: number }
  | { type: 'snapshot'; regs: Record<string, number>; segments: Record<string, number>; mode: number; halted: boolean; tick: number; timer_quantum?: number; current_pid?: number; ready_queue?: number[]; wait_queue?: WaitInfo[]; procs?: PCBInfo[]; sources?: Record<string, string>; source_maps?: Record<string, SourceLine[]>; cores?: CoreInfo[]; num_cores?: number; running?: boolean; hz?: number }
  | { type: 'programs'; list: ProgramInfo[] }
  | { type: 'loaded'; name: string; pids: number[]; procs: PCBInfo[]; sources: Record<string, string>; source_maps?: Record<string, SourceLine[]> }
  | { type: 'reset' }
  | { type: 'procs'; procs: PCBInfo[] }
  | { type: 'pcb_save'; core_id: number; pid: number; regs: Record<string, number> }
  | { type: 'pcb_restore'; core_id: number; pid: number; regs: Record<string, number> }
  | { type: 'sched_decision'; from_pid: number; to_pid: number; reason?: string; ready_queue: number[] }
  | { type: 'process_state'; pid: number; state: string; reason?: string }
  | { type: 'syscall'; pid: number; num: number; name: string; args: number[]; result: number }
  | { type: 'console_out'; core_id?: number; pid: number; bytes: number[]; text: string }
  | { type: 'core_assign'; core_id: number; pid: number }
  | { type: 'core_release'; core_id: number; pid: number; reason?: string }
  | { type: 'mutex_op'; core_id: number; pid: number; mutex_id: number; op: 'create' | 'acquired' | 'waiting' | 'released' | 'handoff' | 'rejected'; next_pid?: number; waiters?: number[] }
  | { type: 'sem_op'; core_id: number; pid: number; sem_id: number; op: 'create' | 'wait_immediate' | 'wait_blocked' | 'signal' | 'handoff'; count: number; next_pid?: number; waiters?: number[] }
  | { type: 'session_info'; id: string; mode: string; role: string; label: string; view_token: string; user_id?: string; user_name?: string; user_email?: string; presence?: { section: string; cores: number[]; mode?: string; narrators?: boolean; focus?: Focus } }
  | { type: 'presence'; section: string; cores: number[]; mode?: string; narrators?: boolean; focus?: Focus; at?: number }
  | { type: 'hand_raise'; label: string; at?: number }
  | { type: 'owner_pointer'; anchor?: string; offset_x?: number; offset_y?: number; route?: string; ts?: number; off?: boolean }
  | { type: 'owner_click'; anchor?: string; offset_x?: number; offset_y?: number; route?: string; ts?: number }
  | { type: 'owner_glossary_hover'; term?: string; off?: boolean; ts?: number }
  | { type: 'error'; message: string };

export interface CoreInfo {
  id: number;
  regs: Record<string, number>;
  segments: Record<string, number>;
  mode: number;
  halted: boolean;
  tick: number;
  current_pid: number;
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

export interface ProgramInfo {
  name: string;
  description: string;
  category?: string;
  size: number;
  procs?: string[];
}

export interface PCBInfo {
  pid: number;
  name: string;
  state: string;
  blocked_on?: string;
  regs: Record<string, number>;
  saved_user_esp: number;
  code_base: number;
  code_size: number;
  stack_top: number;
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
}

export interface LogEntry {
  ts: number;
  text: string;
  color: string;
  // Optional core that produced the event. Undefined for global events
  // (scheduler decisions, syscalls, sync ops) so they remain visible
  // regardless of which cores the user has filtered in the SimBar.
  coreId?: number;
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
  const tick = ref(0);
  const stepHz = ref(4);
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
  interface ConsoleChunk { pid: number; coreId?: number; text: string }
  const consoleChunks = ref<ConsoleChunk[]>([]);

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
  function connect() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
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
      pushLog('⚠ desconectado', 'red');
      scheduleReconnect();
    };
    ws.onerror = () => {
      pushLog('⚠ error de conexión WS', 'red');
      // onerror precedes onclose in the WebSocket lifecycle; rely on onclose
      // to trigger the backoff so we do not double-schedule.
    };
    ws.onmessage = (e) => {
      try {
        const ev = JSON.parse(e.data) as CpuEvent;
        applyEvent(ev);
      } catch (err) {
        pushLog(`⚠ json: ${err}`, 'red');
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
        if (ev.hz !== undefined) stepHz.value = ev.hz;
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
        // EventsLog renders the per-core chip from coreId so the text itself
        // stays clean and identical across single- and multi-core sessions.
        pushLog(`[${ev.eip.toString(16).padStart(6, '0')}]  ${ev.mnemonic}`, 'amber', ev.core_id);
        if (isSelected(ev.core_id)) regs.EIP = nextEIP;
        break;
      }
      case 'tick':
        tick.value = ev.value;
        break;
      case 'mode_switch': {
        const c = ensureCoreSlot(ev.core_id);
        c.mode = ev.to;
        pushLog(`▸ mode ${ev.from === 3 ? 'USER' : 'KERNEL'} → ${ev.to === 3 ? 'USER' : 'KERNEL'} (${ev.cause})`, 'red', ev.core_id);
        if (isSelected(ev.core_id)) mode.value = ev.to;
        // IRET pops the interrupt frame; the panel needs to clear that core
        // so it goes back to "— sin handler activo —". Defer the mutation
        // with queueMicrotask so subscribers fired below (in particular the
        // InterruptNarrator, which reads the stack-frame DOM slots to spawn
        // the IRET mini-arrows) still see the slots mounted. Vue will flush
        // the unmount in the next microtask, after the narrator captured its
        // coordinates.
        if (ev.cause === 'IRET') {
          const coreId = ev.core_id;
          queueMicrotask(() => {
            activeFrameByCore.value = { ...activeFrameByCore.value, [coreId]: null };
          });
        }
        break;
      }
      case 'irq_fire': {
        pushLog(`◉ IRQ 0x${ev.vector.toString(16).padStart(2, '0')} (${ev.source})`, 'yellow', ev.core_id);
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
        pushLog(`✗ PAGE FAULT @ 0x${ev.addr.toString(16)} (need ${ev.perm_needed} in PL${ev.mode})`, 'red', ev.core_id);
        faultShake.value = true;
        setTimeout(() => { faultShake.value = false; }, 500);
        break;
      }
      case 'halt': {
        const c = ensureCoreSlot(ev.core_id);
        c.halted = true;
        pushLog(`▸ HALT`, 'red', ev.core_id);
        if (isSelected(ev.core_id)) halted.value = true;
        break;
      }
      case 'state':
        if (ev.halted !== undefined) halted.value = ev.halted;
        if (ev.running !== undefined) running.value = ev.running;
        if (ev.hz !== undefined) stepHz.value = ev.hz;
        break;
      case 'loaded':
        pushLog(`▸ loaded ${ev.name} (${ev.pids.length} proc) PIDs=${ev.pids.join(',')}`, 'green');
        procs.value = ev.procs;
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
      case 'reset':
        pushLog('▸ reset', 'cyan');
        changedRegs.clear();
        for (const k of Object.keys(changedRegsByCore)) delete changedRegsByCore[Number(k)];
        procs.value = [];
        readyQueue.value = [];
        waitQueue.value = [];
        currentPID.value = 0;
        consoleChunks.value = [];
        procSources.value = {};
        procSourceMaps.value = {};
        mutexes.value = [];
        semaphores.value = [];
        activeFrameByCore.value = {};
        lastFiredHandlerByVector.value = {};
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
        break;
      case 'procs':
        procs.value = ev.procs;
        break;
      case 'pcb_save': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) p.regs = ev.regs;
        flashPID(ev.pid);
        pushLog(`◍ PCB save  PID=${ev.pid}  EIP=0x${(ev.regs.EIP || 0).toString(16)}`, 'red');
        break;
      }
      case 'pcb_restore': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) p.regs = ev.regs;
        flashPID(ev.pid);
        pushLog(`◌ PCB restore PID=${ev.pid}  EIP=0x${(ev.regs.EIP || 0).toString(16)}`, 'green');
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
        const reasonTag = ev.reason ? ` (${ev.reason})` : '';
        pushLog(`⇄ schedule: P${ev.from_pid} → P${ev.to_pid}${reasonTag}  ready=[${rq.join(',')}]`, 'yellow');
        break;
      }
      case 'process_state': {
        const p = procs.value.find(x => x.pid === ev.pid);
        if (p) {
          p.state = ev.state;
          if (ev.reason) p.blocked_on = ev.reason;
        }
        // Mantener wait_queue sincronizada con BLOCKED ⇄ otros estados.
        if (ev.state === 'BLOCKED') {
          if (!waitQueue.value.find(w => w.pid === ev.pid)) {
            waitQueue.value.push({ pid: ev.pid, name: p?.name ?? `P${ev.pid}`, reason: ev.reason ?? '' });
          }
        } else {
          waitQueue.value = waitQueue.value.filter(w => w.pid !== ev.pid);
        }
        break;
      }
      case 'syscall':
        pushLog(`⚙ sys_${ev.name}(${ev.args[0]}, 0x${(ev.args[1]>>>0).toString(16)}, ${ev.args[2]}) → ${ev.result}  [P${ev.pid}]`, 'green');
        break;
      case 'console_out':
        consoleChunks.value.push({ pid: ev.pid, coreId: ev.core_id, text: ev.text });
        if (consoleChunks.value.length > 200) consoleChunks.value.splice(0, consoleChunks.value.length - 200);
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
          pushLog(`▸ conectado como ESPECTADOR de "${ev.label || ev.id.slice(0, 6)}"`, 'cyan');
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
        pushLog(`⚠ ${ev.message}`, 'red');
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
        markCoreActive(c, ev.core_id);
        pushLog(`◈ core ${ev.core_id} ← P${ev.pid}`, 'amber', ev.core_id);
        if (isSelected(ev.core_id)) currentPID.value = ev.pid;
        break;
      }
      case 'core_release': {
        const c = ensureCoreSlot(ev.core_id);
        c.current_pid = 0;
        pushLog(`◇ core ${ev.core_id} ↛ P${ev.pid}${ev.reason ? ' (' + ev.reason + ')' : ''}`, 'amber', ev.core_id);
        if (isSelected(ev.core_id)) currentPID.value = 0;
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
            pushLog(`🔒 mutex ${ev.mutex_id} creado por P${ev.pid}`, 'green');
            break;
          case 'acquired':
            m.holder = ev.pid;
            m.waiters = ev.waiters ?? [];
            pushLog(`🔒 mutex ${ev.mutex_id} adquirido por P${ev.pid}`, 'green');
            break;
          case 'waiting':
            m.waiters = ev.waiters ?? [];
            pushLog(`⏸ P${ev.pid} esperando mutex ${ev.mutex_id}`, 'yellow');
            break;
          case 'released':
            m.holder = -1;
            m.waiters = ev.waiters ?? [];
            pushLog(`🔓 mutex ${ev.mutex_id} liberado por P${ev.pid}`, 'cyan');
            break;
          case 'handoff':
            m.holder = ev.next_pid ?? -1;
            m.waiters = ev.waiters ?? [];
            pushLog(`↪ mutex ${ev.mutex_id}: P${ev.pid} → P${ev.next_pid}`, 'green');
            break;
          case 'rejected':
            pushLog(`✗ mutex ${ev.mutex_id}: P${ev.pid} intentó liberarlo sin poseerlo (rechazado)`, 'red');
            break;
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
            pushLog(`◯ semáforo ${ev.sem_id} creado (count=${ev.count})`, 'green');
            break;
          case 'wait_immediate':
            pushLog(`↓ P${ev.pid} wait sem ${ev.sem_id} (count=${ev.count})`, 'cyan');
            break;
          case 'wait_blocked':
            pushLog(`⏸ P${ev.pid} bloqueado en sem ${ev.sem_id}`, 'yellow');
            break;
          case 'signal':
            pushLog(`↑ P${ev.pid} signal sem ${ev.sem_id} (count=${ev.count})`, 'green');
            break;
          case 'handoff':
            pushLog(`↪ sem ${ev.sem_id}: P${ev.pid} → P${ev.next_pid}`, 'green');
            break;
        }
        break;
      }
    }
    // Fan-out to subscribers AFTER the store has reacted, so they can read
    // the up-to-date snapshot in their handler.
    if (subscribers.size > 0) {
      for (const fn of subscribers) {
        try { fn(ev); } catch (err) { pushLog(`⚠ subscriber: ${err}`, 'red'); }
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

  function pushLog(text: string, color: string, coreId?: number) {
    log.value.push({ ts: Date.now(), text, color, coreId });
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
  function setSpeed(hz: number) {
    stepHz.value = hz;
    send({ cmd: 'set_speed', speed: hz });
  }
  function pushKey(key: string) { send({ cmd: 'push_key', key }); }
  function share() { send({ cmd: 'share' }); }
  function unshare() { send({ cmd: 'unshare' }); }
  function setAlias(alias: string) { send({ cmd: 'set_alias', alias }); }
  function setNumCores(n: number) { send({ cmd: 'set_num_cores', cores: n }); }

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
    regs, segments, mode, halted, running, tick, stepHz, timerQuantum,
    changedRegs, changedRegsByCore,
    programs, selectedProgram, log, connected, hasEverConnected, lastError,
    reconnectAttempts, nextRetryAt, reconnectNow,
    procs, currentPID, readyQueue, waitQueue, flashingPIDs, procSources, procSourceMaps,
    cores, numCores, selectedCoreId, selectedCoreIds, isCoreVisible,
    toggleCoreSelection, selectAllCores, selectOnlyCore, setSelectedCore, setVisibleCores,
    mutexes, semaphores,
    activeFrameByCore, lastFiredHandlerByVector,
    subscribe,
    consoleChunks, faultShake,
    glossarySearch, glossaryOpenSignal,
    sessionInfo, isViewer, viewerLink,
    lastPresence, handRaiseSignal, lastHandRaiseLabel, ownerGlossaryHover,
    connect, load, step, run, pause, reset, clearLog, setQuantum, setSpeed, pushKey, openGlossary,
    share, unshare, setAlias, setNumCores, sendPresence, sendFocus, raiseHand, sendPointer, sendPointerOff, sendClick, sendGlossaryHover, sendGlossaryHoverOff,
    // Tutor: keep the wire shape private to this store so consumers do not
    // have to know the WS command name.
    sendTutorAsk: (question: string, requestId: string) => send({ cmd: 'ask', question, request_id: requestId }),
    // Test seam: vitest fakes the WebSocket by dispatching events directly
    // through this entry point. Production code uses the real ws.onmessage.
    __applyEvent: applyEvent,
  };
});
