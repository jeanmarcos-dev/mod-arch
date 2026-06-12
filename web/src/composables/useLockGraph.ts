import { computed, onBeforeUnmount, ref, type ComputedRef } from 'vue';
import { useCpuStore, type CpuEvent } from '../stores/cpu';

export interface GraphNode {
  id: string;
  kind: 'proc' | 'mutex' | 'sem';
  refId: number;
  x: number;
  y: number;
  label: string;
  inCycle: boolean;
}

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
  kind: 'holds' | 'waits';
  inCycle: boolean;
}

export interface UseLockGraphReturn {
  nodes: ComputedRef<GraphNode[]>;
  edges: ComputedRef<GraphEdge[]>;
  deadlockCycle: ComputedRef<string[] | null>;
  hasDeadlock: ComputedRef<boolean>;
}

// Fixed viewBox of the resource-allocation graph.
export const GRAPH_W = 400;
export const GRAPH_H = 300;

// Deterministic slot positions: locks in a center column, procs alternating
// left/right. A node keeps its slot for the whole run so the graph does not
// reshuffle every time a lock changes hands.
const LOCK_SLOTS = [
  { x: 200, y: 75 },
  { x: 200, y: 150 },
  { x: 200, y: 225 },
] as const;

const PROC_SLOTS = [
  { x: 80, y: 60 },
  { x: 320, y: 60 },
  { x: 80, y: 150 },
  { x: 320, y: 150 },
  { x: 80, y: 240 },
  { x: 320, y: 240 },
] as const;

export function useLockGraph(): UseLockGraphReturn {
  const cpu = useCpuStore();

  // Slot index per node id, assigned on first appearance and never
  // reassigned while the demo runs. Assignment happens in the event
  // subscriber — not inside the computeds — so reads stay side-effect free;
  // slotEpoch forces re-evaluation whenever the maps change.
  const lockSlots = new Map<string, number>();
  const procSlots = new Map<string, number>();
  const slotEpoch = ref(0);

  // Procs that hold or wait on a sync object, deduped, in first-seen order.
  function collectInvolvedPids(): number[] {
    const out: number[] = [];
    const seen = new Set<number>();
    const add = (pid: number) => {
      if (pid >= 0 && !seen.has(pid)) {
        seen.add(pid);
        out.push(pid);
      }
    };
    for (const m of cpu.mutexes) {
      if (m.holder >= 0) add(m.holder);
      for (const w of m.waiters) add(w);
    }
    for (const s of cpu.semaphores) for (const w of s.waiters) add(w);
    return out;
  }

  function assignSlots() {
    let created = false;
    const ensure = (map: Map<string, number>, id: string) => {
      if (!map.has(id)) {
        map.set(id, map.size);
        created = true;
      }
    };
    for (const m of cpu.mutexes) ensure(lockSlots, `m${m.id}`);
    for (const s of cpu.semaphores) ensure(lockSlots, `s${s.id}`);
    for (const pid of collectInvolvedPids()) ensure(procSlots, `p${pid}`);
    if (created) slotEpoch.value++;
  }

  const unsub = cpu.subscribe((ev: CpuEvent) => {
    if (ev.type === 'loaded' || ev.type === 'reset') {
      lockSlots.clear();
      procSlots.clear();
      slotEpoch.value++;
      return;
    }
    // Subscribers run after the reducer applied the event, so the store
    // already reflects any lock/proc the event introduced. These are the
    // only event types that can add sync objects or participants.
    if (ev.type === 'mutex_op' || ev.type === 'sem_op' || ev.type === 'snapshot') {
      assignSlots();
    }
  });
  onBeforeUnmount(() => unsub());

  // Cover locks that already existed before this consumer mounted.
  assignSlots();

  // Deadlock detection on the wait-for graph between processes: each waiter
  // points at the current holder of the mutex it waits on. Semaphores are
  // deliberately excluded — a semaphore has no single owner, so no ownership
  // edge can be derived; deadlocks involving only semaphores are invisible
  // to this detector. Returns the cycle as alternating proc/mutex node ids
  // (['p1','m2','p2','m1'] = P1 waits M2 held by P2, P2 waits M1 held by P1).
  // Only the FIRST cycle found is reported — multiple disjoint deadlocks at
  // once are out of scope for the visualization.
  const deadlockCycle = computed<string[] | null>(() => {
    const adj = new Map<number, { to: number; mutexId: number }[]>();
    for (const m of cpu.mutexes) {
      if (m.holder < 0) continue;
      for (const w of new Set(m.waiters)) {
        if (w === m.holder) continue;
        let list = adj.get(w);
        if (!list) {
          list = [];
          adj.set(w, list);
        }
        list.push({ to: m.holder, mutexId: m.id });
      }
    }

    // DFS with visit colors. pathPids is the gray chain; viaMutex[i] is the
    // mutex through which pathPids[i] waits for pathPids[i+1].
    const color = new Map<number, 1 | 2>();
    const pathPids: number[] = [];
    const viaMutex: number[] = [];

    function visit(u: number): string[] | null {
      color.set(u, 1);
      pathPids.push(u);
      for (const e of adj.get(u) ?? []) {
        const c = color.get(e.to);
        if (c === 1) {
          const start = pathPids.indexOf(e.to);
          const ids: string[] = [];
          for (let i = start; i < pathPids.length; i++) {
            ids.push(`p${pathPids[i]}`);
            ids.push(`m${i < pathPids.length - 1 ? viaMutex[i] : e.mutexId}`);
          }
          return ids;
        }
        if (c === undefined) {
          viaMutex.push(e.mutexId);
          const found = visit(e.to);
          if (found) return found;
          viaMutex.pop();
        }
      }
      pathPids.pop();
      color.set(u, 2);
      return null;
    }

    for (const pid of adj.keys()) {
      if (color.has(pid)) continue;
      const found = visit(pid);
      if (found) return found;
    }
    return null;
  });

  const hasDeadlock = computed(() => deadlockCycle.value !== null);

  const nodes = computed<GraphNode[]>(() => {
    void slotEpoch.value;
    const cycleIds = new Set(deadlockCycle.value ?? []);

    const locks: { id: string; kind: 'mutex' | 'sem'; refId: number }[] = [];
    for (const m of cpu.mutexes) locks.push({ id: `m${m.id}`, kind: 'mutex', refId: m.id });
    for (const s of cpu.semaphores) locks.push({ id: `s${s.id}`, kind: 'sem', refId: s.id });

    // Escape hatch: with more nodes than slots, place everything on an
    // ellipse ordered by slot index (locks first) — still deterministic.
    const overflow = lockSlots.size > LOCK_SLOTS.length || procSlots.size > PROC_SLOTS.length;
    const pos = (slot: number, isLock: boolean): { x: number; y: number } => {
      if (!overflow) return isLock ? LOCK_SLOTS[slot] : PROC_SLOTS[slot];
      const i = isLock ? slot : lockSlots.size + slot;
      const n = Math.max(1, lockSlots.size + procSlots.size);
      const a = -Math.PI / 2 + (2 * Math.PI * i) / n;
      return {
        x: Math.round(GRAPH_W / 2 + 150 * Math.cos(a)),
        y: Math.round(GRAPH_H / 2 + 110 * Math.sin(a)),
      };
    };

    const out: GraphNode[] = [];
    for (const l of locks) {
      // No slot yet means the lock is stale leftovers between a loaded
      // event and the snapshot that resyncs the store — skip it.
      const slot = lockSlots.get(l.id);
      if (slot === undefined) continue;
      const p = pos(slot, true);
      out.push({
        id: l.id,
        kind: l.kind,
        refId: l.refId,
        x: p.x,
        y: p.y,
        label: `${l.kind === 'mutex' ? 'M' : 'S'}${l.refId}`,
        inCycle: cycleIds.has(l.id),
      });
    }
    for (const pid of collectInvolvedPids()) {
      const id = `p${pid}`;
      const slot = procSlots.get(id);
      if (slot === undefined) continue;
      const p = pos(slot, false);
      const name = cpu.procs.find(x => x.pid === pid)?.name;
      out.push({
        id,
        kind: 'proc',
        refId: pid,
        x: p.x,
        y: p.y,
        label: name ? `P${pid} ${name}` : `P${pid}`,
        inCycle: cycleIds.has(id),
      });
    }
    return out;
  });

  const edges = computed<GraphEdge[]>(() => {
    // Consecutive ids in the cycle (wrapping) are exactly the edges of the
    // deadlock circuit, in edge direction: proc→mutex is a waits edge,
    // mutex→proc is a holds edge.
    const cyc = deadlockCycle.value;
    const cyclePairs = new Set<string>();
    if (cyc) {
      for (let i = 0; i < cyc.length; i++) {
        cyclePairs.add(`${cyc[i]}>${cyc[(i + 1) % cyc.length]}`);
      }
    }

    const out: GraphEdge[] = [];
    for (const m of cpu.mutexes) {
      const mid = `m${m.id}`;
      if (m.holder >= 0) {
        const to = `p${m.holder}`;
        out.push({ id: `h:${mid}>${to}`, from: mid, to, kind: 'holds', inCycle: cyclePairs.has(`${mid}>${to}`) });
      }
      // Set() dedupes defensively so anomalous state never yields edges
      // with duplicate v-for keys downstream.
      for (const w of new Set(m.waiters)) {
        const from = `p${w}`;
        out.push({ id: `w:${from}>${mid}`, from, to: mid, kind: 'waits', inCycle: cyclePairs.has(`${from}>${mid}`) });
      }
    }
    for (const s of cpu.semaphores) {
      const sid = `s${s.id}`;
      for (const w of new Set(s.waiters)) {
        out.push({ id: `w:p${w}>${sid}`, from: `p${w}`, to: sid, kind: 'waits', inCycle: false });
      }
    }
    return out;
  });

  return { nodes, edges, deadlockCycle, hasDeadlock };
}
