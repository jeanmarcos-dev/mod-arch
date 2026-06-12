import { computed, onBeforeUnmount, reactive, type ComputedRef } from 'vue';
import { useCpuStore, type CpuEvent, type VarMeta } from '../stores/cpu';

// Ring cap. Small on purpose: the lens only narrates the recent past.
export const LOST_CAP = 50;

// Upper bound for the fallback candidate table so a long unattended session
// can never grow it without limit (stack traffic creates many addresses).
const CANDIDATE_CAP = 1024;

// Start of user memory (AddrUserBase in the memory layout). Interrupt entry
// and IRET push/pop frames on the per-core kernel stack below this base, and
// those accesses emit mem events too; they must never become candidates.
const USER_BASE = 0x200000;

// One stale store: clobberPid wrote newValue on top of lostValue while it
// held a copy read before others advanced the cell. lostCount is the exact
// number of increments by others that the stale store erased (old - copy).
export interface LostUpdate {
  addr: number;
  victimPid: number;
  clobberPid: number;
  lostValue: number;
  newValue: number;
  lostCount: number;
  tick: number;
  seq: number;
}

// An in-flight load: the PID read `value` and has not stored yet. While the
// window is open any store by another PID makes this copy stale.
export interface OpenWindow {
  pid: number;
  value: number;
  sinceTick: number;
  preempted: boolean;
}

export interface RaceDetectorState {
  trackedAddr: number | null;
  trackedName: string | null;
  cellValue: number | null;
  openWindows: Record<number, OpenWindow>;
  lostUpdates: LostUpdate[];
  lostTotal: number;
  currentTick: number;
}

export interface UseRaceDetectorReturn {
  state: RaceDetectorState;
  // actual + lost = the count every executed increment would have produced.
  expectedFinal: ComputedRef<number | null>;
  reset(): void;
}

function parseAddr(addr: unknown): number | null {
  const n = Number(addr);
  return Number.isFinite(n) ? n : null;
}

// Picks the shared counter out of the static var metadata: a global address
// declared by two or more PIDs wins; a single global in the whole demo is
// unambiguous too. Anything else stays unresolved and the runtime fallback
// (an address read AND written by >=2 PIDs) takes over.
function resolveFromVarMeta(varMeta: Record<number, VarMeta>): { addr: number; name: string } | null {
  const byAddr = new Map<number, { name: string; pids: number }>();
  for (const meta of Object.values(varMeta)) {
    for (const g of meta.globals ?? []) {
      const addr = parseAddr(g.addr);
      if (addr === null) continue;
      const entry = byAddr.get(addr);
      if (entry) entry.pids += 1;
      else byAddr.set(addr, { name: g.name, pids: 1 });
    }
  }
  let best: { addr: number; name: string } | null = null;
  for (const [addr, e] of byAddr) {
    if (e.pids >= 2 && (best === null || e.pids > byAddr.get(best.addr)!.pids)) {
      best = { addr, name: e.name };
    }
  }
  if (best) return best;
  if (byAddr.size === 1) {
    const [addr, e] = byAddr.entries().next().value as [number, { name: string }];
    return { addr, name: e.name };
  }
  return null;
}

export function useRaceDetector(): UseRaceDetectorReturn {
  const cpu = useCpuStore();

  const state = reactive<RaceDetectorState>({
    trackedAddr: null,
    trackedName: null,
    cellValue: null,
    openWindows: {},
    lostUpdates: [],
    lostTotal: 0,
    currentTick: 0,
  });

  // A component can mount after `loaded` was applied; the store keeps the
  // var metadata, so resolve from it immediately instead of waiting for the
  // runtime fallback.
  const seeded = resolveFromVarMeta(cpu.procVarMeta);
  if (seeded) {
    state.trackedAddr = seeded.addr;
    state.trackedName = seeded.name;
  }

  // Hot-path bookkeeping stays out of the reactive graph: every mem event
  // passes through here, but only tracked-address traffic touches `state`.
  let candidates = new Map<number, Map<number, { r: boolean; w: boolean }>>();
  let lastWriterPid: number | null = null;
  let seq = 0;

  function reset() {
    state.trackedAddr = null;
    state.trackedName = null;
    state.cellValue = null;
    state.openWindows = {};
    state.lostUpdates = [];
    state.lostTotal = 0;
    candidates = new Map();
    lastWriterPid = null;
  }

  function pidForCore(coreId: number): number {
    return cpu.cores.find(c => c.id === coreId)?.current_pid ?? cpu.currentPID;
  }

  // Fallback resolution: adopt the first word-sized user address that two
  // distinct PIDs have both read and written — kernel stacks, private user
  // stacks and setup-only writes never match.
  function trackCandidate(addr: number, pid: number, op: 'r' | 'w', size: number) {
    if (addr < USER_BASE || size !== 4) return;
    let byPid = candidates.get(addr);
    if (!byPid) {
      if (candidates.size >= CANDIDATE_CAP) return;
      byPid = new Map();
      candidates.set(addr, byPid);
    }
    let flags = byPid.get(pid);
    if (!flags) {
      flags = { r: false, w: false };
      byPid.set(pid, flags);
    }
    flags[op] = true;
    let full = 0;
    for (const f of byPid.values()) if (f.r && f.w) full += 1;
    if (full >= 2) {
      state.trackedAddr = addr;
      candidates = new Map();
    }
  }

  function handleAccess(ev: CpuEvent & { type: 'mem_read' | 'mem_write' }) {
    // Cheap address check first: while tracking, almost every mem event is
    // foreign traffic and must bail before the core lookup below.
    if (state.trackedAddr !== null && ev.addr !== state.trackedAddr) return;
    const pid = pidForCore(ev.core_id);
    if (pid <= 0) return;
    if (state.trackedAddr === null) {
      trackCandidate(ev.addr, pid, ev.type === 'mem_read' ? 'r' : 'w', ev.size);
      if (ev.addr !== state.trackedAddr) return;
    }

    if (ev.tick > state.currentTick) state.currentTick = ev.tick;
    seq += 1;

    if (ev.type === 'mem_read') {
      state.cellValue = ev.value;
      state.openWindows[pid] = { pid, value: ev.value, sinceTick: ev.tick, preempted: false };
      return;
    }

    state.cellValue = ev.new;
    const win = state.openWindows[pid];
    if (win) {
      const lost = ev.old - win.value;
      if (lost > 0) {
        // Exact lost update: the cell moved from win.value to ev.old while
        // this PID held its copy; the store just erased that progress.
        state.lostUpdates.push({
          addr: ev.addr,
          victimPid: lastWriterPid ?? pid,
          clobberPid: pid,
          lostValue: ev.old,
          newValue: ev.new,
          lostCount: lost,
          tick: ev.tick,
          seq,
        });
        if (state.lostUpdates.length > LOST_CAP) {
          state.lostUpdates.splice(0, state.lostUpdates.length - LOST_CAP);
        }
        state.lostTotal += lost;
      }
      delete state.openWindows[pid];
    }
    lastWriterPid = pid;
  }

  const unsub = cpu.subscribe((ev: CpuEvent) => {
    switch (ev.type) {
      case 'tick':
        state.currentTick = ev.value;
        return;
      case 'loaded': {
        reset();
        if (ev.var_meta) {
          const resolved = resolveFromVarMeta(ev.var_meta as Record<number, VarMeta>);
          if (resolved) {
            state.trackedAddr = resolved.addr;
            state.trackedName = resolved.name;
          }
        }
        return;
      }
      case 'reset':
        reset();
        return;
      case 'sched_decision': {
        const win = state.openWindows[ev.from_pid];
        if (win) win.preempted = true;
        return;
      }
      case 'process_state':
        // A PID can die between its load and store; drop the window so the
        // lens does not show a copy that can never be stored.
        if (ev.state === 'ZOMBIE') delete state.openWindows[ev.pid];
        return;
      case 'mem_read':
      case 'mem_write':
        handleAccess(ev);
        return;
      default:
        return;
    }
  });

  onBeforeUnmount(() => unsub());

  const expectedFinal = computed(() =>
    state.cellValue === null ? null : state.cellValue + state.lostTotal,
  );

  return { state, expectedFinal, reset };
}
