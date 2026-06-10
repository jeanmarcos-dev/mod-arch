import { defineStore } from 'pinia';
import { computed, reactive, ref, shallowRef } from 'vue';

// Tipos de eventos que llegan por WS (espejo de internal/cpu/events.go).
// Todos los eventos generados por la CPU llevan core_id: con multicore el
// store mantiene un CoreInfo por core y sólo "espejea" al estado top-level
// (regs, mode, halted, currentPID) cuando el evento viene del core que el
// usuario eligió ver en la SimBar.

import type { CoreInfo, Focus, Presence, ProgramInfo, CategoryMeta, IrqFrame, IdtHandlerHit, LogEntry } from './cpu/types';
export * from './cpu/types';
import { useCpuPresence } from './cpu/presence';
import { useCpuProcesses } from './cpu/processes';
import { useCpuMemory } from './cpu/memory';
import { useCpuTransport } from './cpu/transport';
import { useCpuReducer } from './cpu/reducer';

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
  const presence = useCpuPresence();
  const processes = useCpuProcesses();
  const memory = useCpuMemory();

  // Expose refs natively
  const { sessionInfo, isViewer, lastPresence, handRaiseSignal, lastHandRaiseLabel, ownerGlossaryHover, clearGlossaryStaleTimer, armGlossaryStaleTimer, faultShake, glossarySearch, glossaryOpenSignal, openGlossary } = presence;
  const { procs, readyQueue, waitQueue, mutexes, semaphores, pipes } = processes;
  const { procSources, procSourceMaps, procVarMeta, procVars, pendingMemoryJump } = memory;

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
  const log = shallowRef<LogEntry[]>([]);
  // True after the first successful onopen of this tab session. Lets the UI
  // tell apart "still trying to connect for the first time" (cold start) from
  // "had a live connection that dropped" — the two warrant very different
  // affordances in ConnectionBanner.
  const lastError = ref<string | null>(null);

  // ─── Procesos ───────────────────────────────────────────────────────────
  const currentPID = ref(0);
  // IPC channels created via sys_pipe / sys_mknod; the IpcPanel reads
  // from here and renders the buffer state per channel.

  // Cross-panel intent: when a row of the PCB table requests "jump to my
  // memory", it writes the target address here. Simulator switches the
  // middle tab to memory and MemoryPanel re-centers its custom view on the
  // address, then clears the flag. Kept in the store so the originating
  // component does not need a callback chain to two ancestors.
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
  
  // PIDs que cambiaron en el último evento (para animación flash)
  const flashingPIDs = reactive<Set<number>>(new Set());

  // ─── Consola ────────────────────────────────────────────────────────────
  // Salida acumulada de sys_write. Cada bloque tiene el PID que escribió y el
  // core que originó el write para colorearlos y filtrarlos por core en
  // multicore.
  interface ConsoleChunk { pid: number; coreId?: number; kind?: 'in' | 'out'; text: string; bytes?: number[]; ts: number }
  const consoleChunks = ref<ConsoleChunk[]>([]);

  // ─── Estado del teclado HW ──────────────────────────────────────────────
  // Reflejo del KeyboardBuf del kernel + cola FIFO de procesos bloqueados
  // en sys_read(stdin). Llega vía evento `kbd_state` cada vez que cambia.
  interface KbdReader { pid: number; prog: string }
  const kbdBufUsed = ref(0);
  const kbdBufCap = ref(16);
  const kbdReaders = ref<KbdReader[]>([]);
  // Wall-clock timestamp of the last IRQ 0x21 (keyboard) fire. The Console
  // drawer compares it against its lastOpenedAt to decide whether to flash
  // the IRQ badge.
  const lastIrq21At = ref(0);

  // Helper compartido por la consola y el narrador del teclado.
  function processNameOf(pid: number): string {
    if (pid === 0) return 'kernel';
    return procs.value.find(p => p.pid === pid)?.name ?? '';
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
  const reducerState = {
    activeFrameByCore,
    armGlossaryStaleTimer,
    changedRegs,
    changedRegsByCore,
    clearGlossaryStaleTimer,
    clockHz,
    consoleChunks,
    cores,
    currentPID,
    ensureCoreSlot,
    faultShake,
    flashPID,
    flashReg,
    halted,
    handRaiseSignal,
    isSelected,
    kbdBufCap,
    kbdBufUsed,
    kbdReaders,
    lastError,
    lastFiredHandlerByVector,
    lastHandRaiseLabel,
    lastIrq21At,
    lastMnemonic,
    lastPresence,
    log,
    markCoreActive,
    mode,
    mutexes,
    numCores,
    ownerGlossaryHover,
    paused,
    pendingMemoryJump,
    pipes,
    procs,
    procSourceMaps,
    procSources,
    procVarMeta,
    procVars,
    programs,
    readyQueue,
    reconcileFilterWithCores,
    regs,
    running,
    segments,
    selectedCoreId,
    selectedProgram,
    semaphores,
    sessionInfo,
    speedMul,
    tick,
    timerQuantum,
    waitQueue,
  };

  const { applyEvent, applyVarsPayload, subscribe, pushLog } = useCpuReducer(reducerState);

  const transport = useCpuTransport({
    onFrame: (msg) => {
      if (msg && msg.type === 'tick_events' && Array.isArray(msg.events)) {
        for (const ev of msg.events) applyEvent(ev);
        if (msg.vars !== undefined) applyVarsPayload(msg.vars);
        if (Array.isArray(msg.procs)) applyEvent({ type: 'procs', procs: msg.procs } as any);
        if (typeof msg.tick_value === 'number') applyEvent({ type: 'tick', value: msg.tick_value } as any);
        if (msg.state && typeof msg.state === 'object') applyEvent({ type: 'state', ...msg.state } as any);
        return;
      }
      applyEvent(msg as any);
    },
    onLog: (text, color, scope) => pushLog(text, color, undefined, undefined, scope as any),
    onBeforeConnect: loadCategories,
  });

  const connected = transport.connected;
  const hasEverConnected = transport.hasEverConnected;
  const reconnectAttempts = transport.reconnectAttempts;
  const nextRetryAt = transport.nextRetryAt;
  const connect = transport.connect;
  const reconnectNow = transport.reconnectNow;
  const send = transport.send;

  // applyVarsPayload hydrates procVars from a "vars" map keyed by leader PID.
  // Shared between the legacy `vars_update` frame and the new piggyback in
  // `tick_events.vars` so both paths produce the same store state.





  // Must match the flash-amber keyframe length in CpuPanel.vue and
  // ProcessesPanel.vue so the class drops when the animation ends.
  const FLASH_DURATION_MS = 250;

  function flashReg(name: string, coreId: number = selectedCoreId.value) {
    const slot = ensureChangedSlot(coreId);
    slot.add(name);
    if (isSelected(coreId)) changedRegs.add(name);
    setTimeout(() => {
      slot.delete(name);
      if (isSelected(coreId)) changedRegs.delete(name);
    }, FLASH_DURATION_MS);
  }

  function flashPID(pid: number) {
    flashingPIDs.add(pid);
    setTimeout(() => flashingPIDs.delete(pid), FLASH_DURATION_MS);
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
    procVarMeta, procVars,
    pendingMemoryJump, requestMemoryJump, clearMemoryJump, lastMnemonic,
    cores, numCores, selectedCoreId, selectedCoreIds, visibleCores, isCoreVisible,
    toggleCoreSelection, selectAllCores, selectOnlyCore, setSelectedCore, setVisibleCores,
    mutexes, semaphores, pipes,
    activeFrameByCore, lastFiredHandlerByVector,
    subscribe,
    consoleChunks, faultShake, kbdBufUsed, kbdBufCap, kbdReaders, lastIrq21At, processNameOf,
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
