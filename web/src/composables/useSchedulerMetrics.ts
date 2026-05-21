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

export interface SchedulerMetricsState {
  byPid: Record<number, PidStats>;
  slices: TimelineSlice[];
  liveSlicesByCore: Record<number, TimelineSlice>;
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
    completedProcs: 0,
    currentTick: 0,
    baselineTick: 0,
  };
}

export function useSchedulerMetrics(): UseSchedulerMetricsReturn {
  const cpu = useCpuStore();
  const metrics = reactive<SchedulerMetricsState>(freshMetrics());

  // Per-PID bookkeeping that is internal to the accumulator. Kept outside
  // the reactive state so frequent mutations do not trigger renders on
  // subscribers that only care about derived totals.
  let lastState = new Map<number, { state: string; tick: number }>();
  let sliceSeq = 0;

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
    metrics.completedProcs = 0;
    metrics.baselineTick = metrics.currentTick;
    lastState = new Map();
  }

  function pushSlice(s: TimelineSlice) {
    metrics.slices.push(s);
    if (metrics.slices.length > SLICE_CAP) {
      metrics.slices.splice(0, metrics.slices.length - SLICE_CAP);
    }
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
        if (ev.state === 'ZOMBIE' && stats.endedAtTick === null) {
          stats.endedAtTick = t;
          metrics.completedProcs += 1;
        }
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
