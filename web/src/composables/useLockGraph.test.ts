import { describe, it, expect } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import { useLockGraph, GRAPH_W, GRAPH_H } from './useLockGraph';
import { useCpuStore, type CpuEvent, type PCBInfo } from '../stores/cpu';

function fakePcb(pid: number, name: string, state = 'READY'): PCBInfo {
  return {
    pid,
    name,
    state,
    regs: {},
    saved_user_esp: 0,
    code_base: 0,
    code_size: 0,
    stack_top: 0,
  };
}

interface Harness {
  api: ReturnType<typeof useLockGraph>;
  cpu: ReturnType<typeof useCpuStore>;
  dispatch: (ev: CpuEvent) => void;
  unmount: () => void;
}

function mountComposable(): Harness {
  let api!: ReturnType<typeof useLockGraph>;
  const Host = defineComponent({
    setup() {
      api = useLockGraph();
      return () => h('div');
    },
  });
  const wrapper = mount(Host);
  const cpu = useCpuStore();
  return {
    api,
    cpu,
    dispatch: (ev) => (cpu as unknown as { __applyEvent: (e: CpuEvent) => void }).__applyEvent(ev),
    unmount: () => wrapper.unmount(),
  };
}

function load(hh: Harness, procs: PCBInfo[]) {
  hh.dispatch({
    type: 'loaded',
    name: 'demo',
    pids: procs.map(p => p.pid),
    procs,
    sources: {},
  });
}

function mutexOp(hh: Harness, op: string, mutexId: number, pid: number, waiters: number[] = []) {
  hh.dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid, mutex_id: mutexId, op, waiters } as CpuEvent);
}

function semOp(hh: Harness, op: string, semId: number, pid: number, count: number, waiters: number[] = []) {
  hh.dispatch({ type: 'sem_op', core_id: 0, tick: 0, pid, sem_id: semId, op, count, waiters } as CpuEvent);
}

function node(hh: Harness, id: string) {
  return hh.api.nodes.value.find(n => n.id === id);
}

function edge(hh: Harness, from: string, to: string) {
  return hh.api.edges.value.find(e => e.from === from && e.to === to);
}

describe('useLockGraph', () => {
  it('starts empty', () => {
    const hh = mountComposable();
    expect(hh.api.nodes.value).toEqual([]);
    expect(hh.api.edges.value).toEqual([]);
    expect(hh.api.deadlockCycle.value).toBeNull();
    expect(hh.api.hasDeadlock.value).toBe(false);
    hh.unmount();
  });

  it('builds holds/waits edges and only includes involved procs', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'hilo_a', 'RUN'), fakePcb(2, 'hilo_b', 'BLOCKED'), fakePcb(3, 'ajeno', 'READY')]);
    mutexOp(hh, 'create', 1, 1);
    mutexOp(hh, 'acquired', 1, 1);
    mutexOp(hh, 'waiting', 1, 2, [2]);

    const ids = hh.api.nodes.value.map(n => n.id).sort();
    expect(ids).toEqual(['m1', 'p1', 'p2']);
    expect(node(hh, 'p3')).toBeUndefined();

    const holds = edge(hh, 'm1', 'p1');
    expect(holds?.kind).toBe('holds');
    const waits = edge(hh, 'p2', 'm1');
    expect(waits?.kind).toBe('waits');
    expect(hh.api.edges.value.length).toBe(2);

    // Labels: procs carry their name, locks just M{id}/S{id}.
    expect(node(hh, 'p1')?.label).toBe('P1 hilo_a');
    expect(node(hh, 'm1')?.label).toBe('M1');
    expect(node(hh, 'm1')?.kind).toBe('mutex');
    expect(node(hh, 'p1')?.refId).toBe(1);

    // Every node sits inside the fixed viewBox.
    for (const n of hh.api.nodes.value) {
      expect(n.x).toBeGreaterThanOrEqual(0);
      expect(n.x).toBeLessThanOrEqual(GRAPH_W);
      expect(n.y).toBeGreaterThanOrEqual(0);
      expect(n.y).toBeLessThanOrEqual(GRAPH_H);
    }
    expect(hh.api.hasDeadlock.value).toBe(false);
    hh.unmount();
  });

  it('detects a 2-cycle deadlock and marks the full circuit', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'a', 'BLOCKED'), fakePcb(2, 'b', 'BLOCKED')]);
    // P1 holds M1 and waits M2; P2 holds M2 and waits M1.
    mutexOp(hh, 'acquired', 1, 1);
    mutexOp(hh, 'acquired', 2, 2);
    mutexOp(hh, 'waiting', 2, 1, [1]);
    mutexOp(hh, 'waiting', 1, 2, [2]);

    expect(hh.api.hasDeadlock.value).toBe(true);
    const cycle = hh.api.deadlockCycle.value!;
    expect(cycle.length).toBe(4);
    expect([...cycle].sort()).toEqual(['m1', 'm2', 'p1', 'p2']);
    // Alternating proc/mutex and semantically consistent: proc at i waits on
    // the mutex at i+1, which is held by the proc at i+2 (wrapping).
    const mutexById = (id: string) => hh.cpu.mutexes.find(m => `m${m.id}` === id)!;
    for (let i = 0; i < cycle.length; i += 2) {
      expect(cycle[i][0]).toBe('p');
      expect(cycle[i + 1][0]).toBe('m');
      const waiter = Number(cycle[i].slice(1));
      const m = mutexById(cycle[i + 1]);
      expect(m.waiters).toContain(waiter);
      expect(`p${m.holder}`).toBe(cycle[(i + 2) % cycle.length]);
    }

    for (const id of ['p1', 'p2', 'm1', 'm2']) expect(node(hh, id)?.inCycle).toBe(true);
    expect(edge(hh, 'p1', 'm2')?.inCycle).toBe(true);
    expect(edge(hh, 'm2', 'p2')?.inCycle).toBe(true);
    expect(edge(hh, 'p2', 'm1')?.inCycle).toBe(true);
    expect(edge(hh, 'm1', 'p1')?.inCycle).toBe(true);
    hh.unmount();
  });

  it('detects the philosophers 3-cycle and leaves outsiders unmarked', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'f1'), fakePcb(2, 'f2'), fakePcb(3, 'f3'), fakePcb(4, 'extra')]);
    mutexOp(hh, 'acquired', 1, 1);
    mutexOp(hh, 'acquired', 2, 2);
    mutexOp(hh, 'acquired', 3, 3);
    mutexOp(hh, 'waiting', 2, 1, [1]);
    mutexOp(hh, 'waiting', 3, 2, [2]);
    // P4 also waits on M1 but is not part of the cycle.
    mutexOp(hh, 'waiting', 1, 3, [3, 4]);

    expect(hh.api.hasDeadlock.value).toBe(true);
    const cycle = hh.api.deadlockCycle.value!;
    expect(cycle.length).toBe(6);
    expect([...cycle].sort()).toEqual(['m1', 'm2', 'm3', 'p1', 'p2', 'p3']);
    expect(node(hh, 'p4')?.inCycle).toBe(false);
    expect(edge(hh, 'p4', 'm1')?.inCycle).toBe(false);
    expect(edge(hh, 'p3', 'm1')?.inCycle).toBe(true);
    hh.unmount();
  });

  it('does not flag a waiting chain without a cycle', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'a'), fakePcb(2, 'b'), fakePcb(3, 'c')]);
    // P1 holds M1; P2 waits M1 and holds M2; P3 waits M2. No cycle.
    mutexOp(hh, 'acquired', 1, 1);
    mutexOp(hh, 'acquired', 2, 2);
    mutexOp(hh, 'waiting', 1, 2, [2]);
    mutexOp(hh, 'waiting', 2, 3, [3]);

    expect(hh.api.hasDeadlock.value).toBe(false);
    expect(hh.api.deadlockCycle.value).toBeNull();
    expect(hh.api.nodes.value.every(n => !n.inCycle)).toBe(true);
    expect(hh.api.edges.value.every(e => !e.inCycle)).toBe(true);
    hh.unmount();
  });

  it('semaphore waits create edges but never holds nor false cycles', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'a'), fakePcb(2, 'b')]);
    // P1 holds M1 and blocks on S1; P2 waits M1. Would be a cycle if S1
    // had an ownership edge back to P2 — it must not.
    mutexOp(hh, 'acquired', 1, 1);
    mutexOp(hh, 'waiting', 1, 2, [2]);
    semOp(hh, 'create', 1, 0, 0);
    semOp(hh, 'wait_blocked', 1, 1, 0, [1]);

    expect(hh.api.hasDeadlock.value).toBe(false);
    const sWaits = edge(hh, 'p1', 's1');
    expect(sWaits?.kind).toBe('waits');
    expect(node(hh, 's1')?.kind).toBe('sem');
    expect(node(hh, 's1')?.label).toBe('S1');
    // No edge ever leaves a semaphore node.
    expect(hh.api.edges.value.some(e => e.from === 's1')).toBe(false);
    hh.unmount();
  });

  it('assigns slots from events so late-mounted consumers see identical positions', () => {
    const h1 = mountComposable();
    load(h1, [fakePcb(1, 'a'), fakePcb(2, 'b')]);
    mutexOp(h1, 'acquired', 1, 1);
    mutexOp(h1, 'waiting', 1, 2, [2]);
    semOp(h1, 'create', 1, 0, 0);

    // Second consumer mounts after the events already happened (no further
    // dispatches): it must derive the exact same layout from store state.
    const h2 = mountComposable();
    for (const id of ['m1', 's1', 'p1', 'p2']) {
      const a = node(h1, id)!;
      const b = node(h2, id)!;
      expect(b).toBeDefined();
      expect(`${b.x},${b.y}`).toBe(`${a.x},${a.y}`);
    }
    h2.unmount();
    h1.unmount();
  });

  it('dedupes anomalous duplicate waiters in edges and nodes', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'a'), fakePcb(2, 'b')]);
    mutexOp(hh, 'acquired', 1, 1);
    mutexOp(hh, 'waiting', 1, 2, [2, 2]);
    expect(hh.api.edges.value.filter(e => e.from === 'p2').length).toBe(1);
    expect(hh.api.nodes.value.filter(n => n.id === 'p2').length).toBe(1);
    hh.unmount();
  });

  it('keeps slots stable across release and re-acquire', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'a'), fakePcb(2, 'b')]);
    mutexOp(hh, 'create', 1, 1);
    mutexOp(hh, 'acquired', 1, 1);
    const p1Pos = { x: node(hh, 'p1')!.x, y: node(hh, 'p1')!.y };
    const m1Pos = { x: node(hh, 'm1')!.x, y: node(hh, 'm1')!.y };

    mutexOp(hh, 'released', 1, 1);
    expect(node(hh, 'p1')).toBeUndefined();

    // P2 appears for the first time: gets a NEW slot, not P1's.
    mutexOp(hh, 'acquired', 1, 2);
    const p2 = node(hh, 'p2')!;
    expect(p2.x === p1Pos.x && p2.y === p1Pos.y).toBe(false);

    // P1 comes back: same slot as before.
    mutexOp(hh, 'released', 1, 2);
    mutexOp(hh, 'acquired', 1, 1);
    expect(node(hh, 'p1')!.x).toBe(p1Pos.x);
    expect(node(hh, 'p1')!.y).toBe(p1Pos.y);
    expect(node(hh, 'm1')!.x).toBe(m1Pos.x);
    expect(node(hh, 'm1')!.y).toBe(m1Pos.y);
    hh.unmount();
  });

  it('clears slot assignments on loaded', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'a')]);
    mutexOp(hh, 'acquired', 1, 1);
    mutexOp(hh, 'create', 2, 1);
    const m1Pos = { x: node(hh, 'm1')!.x, y: node(hh, 'm1')!.y };
    const m2Pos = { x: node(hh, 'm2')!.x, y: node(hh, 'm2')!.y };
    expect(m2Pos).not.toEqual(m1Pos);

    // New demo: M2 is now the only lock and must claim the first slot.
    load(hh, [fakePcb(1, 'a')]);
    hh.dispatch({
      type: 'snapshot',
      regs: {},
      segments: {},
      mode: 3,
      halted: false,
      tick: 0,
      mutexes: [{ id: 2, holder: 1, waiters: [] }],
      semaphores: [],
    } as unknown as CpuEvent);
    expect(node(hh, 'm1')).toBeUndefined();
    expect(node(hh, 'm2')!.x).toBe(m1Pos.x);
    expect(node(hh, 'm2')!.y).toBe(m1Pos.y);
    hh.unmount();
  });

  it('clears everything on reset', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'a'), fakePcb(2, 'b')]);
    mutexOp(hh, 'acquired', 1, 1);
    mutexOp(hh, 'acquired', 2, 2);
    const m1Pos = { x: node(hh, 'm1')!.x, y: node(hh, 'm1')!.y };
    const p1Pos = { x: node(hh, 'p1')!.x, y: node(hh, 'p1')!.y };

    hh.dispatch({ type: 'reset' });
    expect(hh.api.nodes.value).toEqual([]);
    expect(hh.api.edges.value).toEqual([]);

    // First lock/proc of the next run reclaim the first slots.
    mutexOp(hh, 'acquired', 9, 2);
    expect(node(hh, 'm9')!.x).toBe(m1Pos.x);
    expect(node(hh, 'm9')!.y).toBe(m1Pos.y);
    expect(node(hh, 'p2')!.x).toBe(p1Pos.x);
    expect(node(hh, 'p2')!.y).toBe(p1Pos.y);
    hh.unmount();
  });

  it('falls back to a circular layout when slots run out', () => {
    const hh = mountComposable();
    load(hh, [1, 2, 3, 4, 5, 6, 7].map(i => fakePcb(i, `w${i}`)));
    // Four locks exceed the 3 center slots; positions must still be unique
    // and inside the viewBox.
    for (let i = 1; i <= 4; i++) mutexOp(hh, 'acquired', i, i);
    const seen = new Set(hh.api.nodes.value.map(n => `${n.x},${n.y}`));
    expect(seen.size).toBe(hh.api.nodes.value.length);
    for (const n of hh.api.nodes.value) {
      expect(n.x).toBeGreaterThanOrEqual(0);
      expect(n.x).toBeLessThanOrEqual(GRAPH_W);
      expect(n.y).toBeGreaterThanOrEqual(0);
      expect(n.y).toBeLessThanOrEqual(GRAPH_H);
    }
    hh.unmount();
  });
});
