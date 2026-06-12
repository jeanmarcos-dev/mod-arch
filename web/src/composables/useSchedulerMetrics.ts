import { computed, onBeforeUnmount, reactive, type ComputedRef } from 'vue';
import { useCpuStore, type CpuEvent } from '../stores/cpu';

// Didactic clock: pretend every simulated tick equals 1 ms of wall time.
// Pure cosmetic; metrics themselves are measured in ticks. Used only to
// translate ticks into a "1 quantum ≈ N ms" hint that ties the demo to
// the real Linux scheduler ranges (CFS uses 1–10 ms slices).
export const HZ_SIM = 1000;

// Cap on the ring buffer of historic time slices. 300 covers ~37 quantums
// per core at the default TimerQuantum=8 — plenty for the 200-tick visible
// window of the timeline plus some scrollback history.
const SLICE_CAP = 300;

// Caps for the per-process state segments and lock-op marks consumed by the
// SyncTimeline. Sized like SLICE_CAP: enough history for the 200-tick window.
const SEGMENT_CAP = 400;
const MARK_CAP = 200;

export interface PidStats {
  pid: number;
  name: string;
  state: string;
  createdAtTick: number;
  endedAtTick: number | null;
  firstRunAtTick: number | null;
  totalRunTicks: number;
  totalReadyTicks: number;
  totalBlockedTicks: number;
  quantumsCompleted: number;
  quantumsPartial: number;
}

export interface TimelineSlice {
  seq: number;
  coreId: number;
  pid: number;
  startTick: number;
  endTick: number | null;
  exitReason: string | null;
}

// One contiguous span of a process lifecycle state, for the per-PID lanes
// of the SyncTimeline. RUN segments carry the core; BLOCKED segments carry
// the cause as the kernel reports it ("mutex:1", "sem:2", "keyboard"...).
export interface StateSegment {
  seq: number;
  pid: number;
  state: 'RUN' | 'READY' | 'BLOCKED';
  coreId?: number;
  reason?: string;
  startTick: number;
  endTick: number | null; // null = still live
}

export interface LockMark {
  seq: number;
  pid: number;
  tick: number;
  kind: 'acquired' | 'waiting' | 'released';
}

export interface SchedulerMetricsState {
  byPid: Record<number, PidStats>;
  slices: TimelineSlice[];
  liveSlicesByCore: Record<number, TimelineSlice>;
  stateSegments: StateSegment[];
  liveSegmentByPid: Record<number, StateSegment>;
  lockMarks: LockMark[];
  completedProcs: number;
  currentTick: number;     // wall tick from the wire
  baselineTick: number;    // tick at the moment the current demo loaded
}

export interface UseSchedulerMetricsReturn {
  metrics: SchedulerMetricsState;
  currentTickRel: ComputedRef<number>;
  throughputPerKTick: ComputedRef<number>;
  hzSim: typeof HZ_SIM;
  quantumMsHint: ComputedRef<number>;
}

function freshMetrics(): SchedulerMetricsState {
  return {
    byPid: {},
    slices: [],
    liveSlicesByCore: {},
    stateSegments: [],
    liveSegmentByPid: {},
    lockMarks: [],
    completedProcs: 0,
    currentTick: 0,
    baselineTick: 0,
  };
}

// Translates a mutex_op/sem_op into the marker drawn on the owning lane.
// handoff is handled by the caller — it yields two marks. create and
// rejected produce no marker.
function markKindFor(op: string): LockMark['kind'] | null {
  switch (op) {
    case 'acquired':
    case 'wait_immediate':
      return 'acquired';
    case 'waiting':
    case 'wait_blocked':
      return 'waiting';
    case 'released':
    case 'signal':
      return 'released';
    default:
      return null;
  }
}

export function useSchedulerMetrics(): UseSchedulerMetricsReturn {
  const cpu = useCpuStore();
  const metrics = reactive<SchedulerMetricsState>(freshMetrics());

  // Per-PID bookkeeping that is internal to the accumulator. Kept outside
  // the reactive state so frequent mutations do not trigger renders on
  // subscribers that only care about derived totals.
  let lastState = new Map<number, { state: string; tick: number }>();
  let sliceSeq = 0;
  let segSeq = 0;
  let markSeq = 0;
  // Core currently bound to each PID, fed by core_assign/core_release.
  // process_state RUN events do not carry a reliable core id, so RUN
  // segments take theirs from here regardless of event ordering.
  let coreByPid = new Map<number, number>();

  function relTick(): number {
    return Math.max(0, metrics.currentTick - metrics.baselineTick);
  }

  function ensurePid(pid: number, name?: string, state?: string): PidStats {
    let p = metrics.byPid[pid];
    if (!p) {
      p = {
        pid,
        name: name ?? `P${pid}`,
        state: state ?? 'READY',
        createdAtTick: relTick(),
        endedAtTick: null,
        firstRunAtTick: null,
        totalRunTicks: 0,
        totalReadyTicks: 0,
        totalBlockedTicks: 0,
        quantumsCompleted: 0,
        quantumsPartial: 0,
      };
      metrics.byPid[pid] = p;
    } else {
      if (name) p.name = name;
      if (state) p.state = state;
    }
    return p;
  }

  function resetAll() {
    metrics.byPid = {};
    metrics.slices = [];
    metrics.liveSlicesByCore = {};
    metrics.stateSegments = [];
    metrics.liveSegmentByPid = {};
    metrics.lockMarks = [];
    metrics.completedProcs = 0;
    metrics.baselineTick = metrics.currentTick;
    lastState = new Map();
    coreByPid = new Map();
  }

  function pushSlice(s: TimelineSlice) {
    metrics.slices.push(s);
    if (metrics.slices.length > SLICE_CAP) {
      metrics.slices.splice(0, metrics.slices.length - SLICE_CAP);
    }
  }

  function pushMark(pid: number, kind: LockMark['kind'], tick: number) {
    if (pid <= 0) return;
    metrics.lockMarks.push({ seq: ++markSeq, pid, tick, kind });
    if (metrics.lockMarks.length > MARK_CAP) {
      metrics.lockMarks.splice(0, metrics.lockMarks.length - MARK_CAP);
    }
  }

  function closeSegment(pid: number, tick: number) {
    const live = metrics.liveSegmentByPid[pid];
    if (!live) return;
    live.endTick = tick;
    metrics.stateSegments.push(live);
    if (metrics.stateSegments.length > SEGMENT_CAP) {
      metrics.stateSegments.splice(0, metrics.stateSegments.length - SEGMENT_CAP);
    }
    delete metrics.liveSegmentByPid[pid];
  }

  // States other than RUN/READY/BLOCKED (ZOMBIE, NEW...) open no segment:
  // the lane simply ends where the last segment closed.
  function openSegment(pid: number, state: string, tick: number, reason?: string) {
    if (state !== 'RUN' && state !== 'READY' && state !== 'BLOCKED') return;
    const seg: StateSegment = { seq: ++segSeq, pid, state, startTick: tick, endTick: null };
    if (state === 'RUN') {
      const core = coreByPid.get(pid);
      if (core !== undefined) seg.coreId = core;
    } else if (state === 'BLOCKED') {
      // The kernel ships the cause already normalized ("mutex:1", "sem:2",
      // "keyboard"...). When the event omits it, the reducer-maintained
      // blocked_on of the PCB is the next best source.
      const r = reason ?? cpu.procs.find(p => p.pid === pid)?.blocked_on;
      if (r) seg.reason = r;
    }
    metrics.liveSegmentByPid[pid] = seg;
  }

  const unsub = cpu.subscribe((ev: CpuEvent) => {
    switch (ev.type) {
      case 'tick':
        metrics.currentTick = ev.value;
        return;
      case 'snapshot':
        metrics.currentTick = ev.tick;
        return;
      case 'loaded': {
        // A new demo restarts the metric clock. Use baselineTick so the
        // currentTick reading from the wire (which keeps advancing) still
        // translates to "ticks since this demo started" via relTick().
        resetAll();
        for (const p of ev.procs ?? []) {
          const stats = ensurePid(p.pid, p.name, p.state);
          stats.createdAtTick = 0;
          lastState.set(p.pid, { state: p.state, tick: 0 });
          openSegment(p.pid, p.state, 0, p.blocked_on);
        }
        return;
      }
      case 'reset':
        resetAll();
        return;
      case 'core_assign': {
        const t = relTick();
        const stats = ensurePid(ev.pid);
        if (stats.firstRunAtTick === null) stats.firstRunAtTick = t;
        // Defensive: close any slice that was never released on this core
        // (would indicate a missed core_release event). Mark it displaced.
        const stale = metrics.liveSlicesByCore[ev.core_id];
        if (stale) {
          stale.endTick = t;
          stale.exitReason = 'displaced';
          pushSlice(stale);
        }
        const slice: TimelineSlice = {
          seq: ++sliceSeq,
          coreId: ev.core_id,
          pid: ev.pid,
          startTick: t,
          endTick: null,
          exitReason: null,
        };
        metrics.liveSlicesByCore[ev.core_id] = slice;
        coreByPid.set(ev.pid, ev.core_id);
        // Patch the RUN segment when its process_state arrived first.
        const seg = metrics.liveSegmentByPid[ev.pid];
        if (seg && seg.state === 'RUN' && seg.coreId === undefined) {
          seg.coreId = ev.core_id;
        }
        return;
      }
      case 'core_release': {
        const t = relTick();
        const live = metrics.liveSlicesByCore[ev.core_id];
        if (live && live.pid === ev.pid) {
          live.endTick = t;
          live.exitReason = ev.reason ?? null;
          const stats = ensurePid(ev.pid);
          stats.totalRunTicks += Math.max(0, t - live.startTick);
          pushSlice(live);
          delete metrics.liveSlicesByCore[ev.core_id];
        }
        // Only drop the binding this release refers to: an out-of-order
        // release after a re-assign must not erase the fresh core.
        if (coreByPid.get(ev.pid) === ev.core_id) coreByPid.delete(ev.pid);
        return;
      }
      case 'sched_decision': {
        // FromPID=0 is a wake-up onto an idle core; nothing to attribute.
        if (ev.from_pid > 0 && ev.reason) {
          const stats = ensurePid(ev.from_pid);
          if (ev.reason === 'timer') stats.quantumsCompleted += 1;
          else stats.quantumsPartial += 1;
        }
        return;
      }
      case 'process_state': {
        const t = relTick();
        const prev = lastState.get(ev.pid);
        const stats = ensurePid(ev.pid, undefined, ev.state);
        if (prev) {
          const dur = Math.max(0, t - prev.tick);
          // RUN time is owned by core_assign/release to avoid double-counting:
          // process_state only contributes READY/BLOCKED durations here.
          if (prev.state === 'READY') stats.totalReadyTicks += dur;
          else if (prev.state === 'BLOCKED') stats.totalBlockedTicks += dur;
        }
        lastState.set(ev.pid, { state: ev.state, tick: t });
        const seg = metrics.liveSegmentByPid[ev.pid];
        if (!seg || seg.state !== ev.state) {
          closeSegment(ev.pid, t);
          openSegment(ev.pid, ev.state, t, ev.reason);
        }
        if (ev.state === 'ZOMBIE' && stats.endedAtTick === null) {
          stats.endedAtTick = t;
          metrics.completedProcs += 1;
        }
        return;
      }
      case 'mutex_op':
      case 'sem_op': {
        const t = relTick();
        if (ev.op === 'handoff') {
          // The kernel emits a single handoff when a lock is released with
          // waiters — no separate 'released' follows. It is both the giver's
          // release and the receiver's acquisition.
          pushMark(ev.pid, 'released', t);
          if (ev.next_pid !== undefined) pushMark(ev.next_pid, 'acquired', t);
          return;
        }
        const kind = markKindFor(ev.op);
        if (kind) pushMark(ev.pid, kind, t);
        return;
      }
      default:
        return;
    }
  });

  onBeforeUnmount(() => unsub());

  const currentTickRel = computed(() => relTick());
  const throughputPerKTick = computed(() => {
    const t = currentTickRel.value;
    return t > 0 ? (metrics.completedProcs * 1000) / t : 0;
  });
  const quantumMsHint = computed(() => (cpu.timerQuantum * 1000) / HZ_SIM);

  return {
    metrics,
    currentTickRel,
    throughputPerKTick,
    hzSim: HZ_SIM,
    quantumMsHint,
  };
}
