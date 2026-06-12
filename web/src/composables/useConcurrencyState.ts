import { effectScope, ref } from 'vue';
import { useCpuStore } from '../stores/cpu';
import { useLockGraph } from './useLockGraph';
import { useRaceDetector } from './useRaceDetector';

// One app-level instance of each concurrency detector, shared by every section
// (memory view, topbar chip, tutor context) instead of being tied to the
// concurrency view's lifetime.

let instance: ReturnType<typeof create> | null = null;
let scope: ReturnType<typeof effectScope> | null = null;

function create() {
  // Runs inside a detached effect scope so the inner composables' cpu
  // subscriptions live for the whole session; Pinia is global, so useCpuStore()
  // resolves without an active component instance.
  const cpu = useCpuStore();
  const lockGraph = useLockGraph();
  const raceDetector = useRaceDetector();

  // Absolute tick at which the first deadlock appeared, for display. Latched
  // until a new demo loads or the run resets (a deadlock never self-resolves).
  const deadlockTick = ref<number | null>(null);

  // Latch the rising edge synchronously after each event: hasDeadlock is a
  // computed, so reading it here reflects the store state the event just
  // produced, and cpu.tick is the absolute tick at that moment.
  let wasDeadlocked = lockGraph.hasDeadlock.value;
  if (wasDeadlocked) deadlockTick.value = cpu.tick;
  cpu.subscribe(ev => {
    if (ev.type === 'loaded' || ev.type === 'reset') {
      deadlockTick.value = null;
      wasDeadlocked = lockGraph.hasDeadlock.value;
      return;
    }
    const now = lockGraph.hasDeadlock.value;
    if (now && !wasDeadlocked) deadlockTick.value = cpu.tick;
    wasDeadlocked = now;
  });

  function tutorSummary(): string {
    const parts: string[] = [];

    const rs = raceDetector.state;
    if (rs.trackedAddr !== null && rs.lostTotal > 0) {
      const name = rs.trackedName ?? 'la variable compartida';
      const addr = `0x${rs.trackedAddr.toString(16).toUpperCase()}`;
      parts.push(
        `Condición de carrera en ${name} (${addr}): se han perdido ${rs.lostTotal} incrementos por accesos sin sincronizar.`,
      );
    }

    if (lockGraph.hasDeadlock.value) {
      const cycle = lockGraph.deadlockCycle.value ?? [];
      const procs = cycle
        .filter(id => id.startsWith('p'))
        .map(id => `P${id.slice(1)}`);
      const ring = procs.length > 0 ? [...procs, procs[0]].join(' → ') : '';
      const since = deadlockTick.value !== null ? ` desde el tick ${deadlockTick.value}` : '';
      parts.push(
        `Interbloqueo activo${since}: ciclo de espera ${ring}. Cada proceso retiene un recurso y espera el del siguiente.`,
      );
    }

    return parts.join('\n');
  }

  return { lockGraph, raceDetector, deadlockTick, tutorSummary };
}

export function useConcurrencyState() {
  if (!instance) {
    scope = effectScope(true); // detached: never disposed by a component
    instance = scope.run(() => create())!;
  }
  return instance;
}

// Test seam: dispose the scope and clear the singleton so each test starts fresh.
export function __resetConcurrencyStateForTest() {
  scope?.stop();
  scope = null;
  instance = null;
}
