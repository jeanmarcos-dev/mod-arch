import { describe, it, expect, beforeEach } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { useConcurrencyState, __resetConcurrencyStateForTest } from './useConcurrencyState';
import { useCpuStore, type CpuEvent, type PCBInfo } from '../stores/cpu';

const ADDR = 0x3fffe0;

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
  api: ReturnType<typeof useConcurrencyState>;
  cpu: ReturnType<typeof useCpuStore>;
  dispatch: (ev: CpuEvent) => void;
  unmount: () => void;
}

function mountComposable(): Harness {
  let api!: ReturnType<typeof useConcurrencyState>;
  const Host = defineComponent({
    setup() {
      api = useConcurrencyState();
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

function load(hh: Harness, procs: PCBInfo[], varMeta?: Record<number, Record<string, unknown>>) {
  hh.dispatch({
    type: 'loaded',
    name: 'demo',
    pids: procs.map(p => p.pid),
    procs,
    sources: {},
    var_meta: varMeta,
  } as CpuEvent);
}

function mutexOp(hh: Harness, op: string, mutexId: number, pid: number, waiters: number[] = [], tick = 0) {
  hh.dispatch({ type: 'mutex_op', core_id: 0, tick, pid, mutex_id: mutexId, op, waiters } as CpuEvent);
}

function counterMeta(addr = '0x3FFFE0') {
  return { 1: { globals: [{ name: 'counter', type: 'int', addr, len: 4 }] } };
}

function assign(hh: Harness, coreId: number, pid: number) {
  hh.dispatch({ type: 'core_assign', core_id: coreId, tick: 0, pid } as CpuEvent);
}

function read(hh: Harness, value: number, tick = 0, coreId = 0, addr = ADDR, size = 4) {
  hh.dispatch({ type: 'mem_read', core_id: coreId, tick, addr, value, size } as CpuEvent);
}

function write(hh: Harness, oldV: number, newV: number, tick = 0, coreId = 0, addr = ADDR, size = 4) {
  hh.dispatch({ type: 'mem_write', core_id: coreId, tick, addr, old: oldV, new: newV, size } as CpuEvent);
}

describe('useConcurrencyState', () => {
  beforeEach(() => {
    __resetConcurrencyStateForTest();
    setActivePinia(createPinia());
  });

  it('returns the same singleton instance across calls', () => {
    const hh = mountComposable();
    expect(useConcurrencyState()).toBe(hh.api);
    expect(useConcurrencyState()).toBe(useConcurrencyState());
    hh.unmount();
  });

  it('summarises an active deadlock with the proc ring and tick', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(5, 'f5'), fakePcb(6, 'f6'), fakePcb(7, 'f7')]);
    // Philosophers 3-cycle: P5 holds M1 waits M2, P6 holds M2 waits M3,
    // P7 holds M3 waits M1.
    hh.dispatch({ type: 'tick', value: 42 } as CpuEvent);
    mutexOp(hh, 'acquired', 1, 5);
    mutexOp(hh, 'acquired', 2, 6);
    mutexOp(hh, 'acquired', 3, 7);
    mutexOp(hh, 'waiting', 2, 5, [5]);
    mutexOp(hh, 'waiting', 3, 6, [6]);
    mutexOp(hh, 'waiting', 1, 7, [7]);

    expect(hh.api.lockGraph.hasDeadlock.value).toBe(true);
    const summary = hh.api.tutorSummary();
    expect(summary).toContain('Interbloqueo');
    // The ring is rotation-invariant: whichever proc the cycle search starts
    // from, all three appear and it closes back on its first element.
    const rotations = ['P5 → P6 → P7 → P5', 'P6 → P7 → P5 → P6', 'P7 → P5 → P6 → P7'];
    expect(rotations.some(r => summary.includes(r))).toBe(true);
    expect(hh.api.deadlockTick.value).not.toBeNull();
    hh.unmount();
  });

  it('summarises a lost update as a race condition', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'racer_a', 'RUN'), fakePcb(2, 'racer_b', 'READY')], counterMeta());
    // P1 loads 0; P2 loads 0, stores 1, loads 1, stores 2; P1 stores 1 (stale).
    assign(hh, 0, 1);
    read(hh, 0, 10);
    assign(hh, 0, 2);
    read(hh, 0, 11);
    write(hh, 0, 1, 12);
    read(hh, 1, 13);
    write(hh, 1, 2, 14);
    assign(hh, 0, 1);
    write(hh, 2, 1, 15);

    expect(hh.api.raceDetector.state.lostTotal).toBe(2);
    const summary = hh.api.tutorSummary();
    expect(summary).toContain('Condición de carrera');
    expect(summary).toContain('counter');
    expect(summary).toContain('2 incrementos');
    hh.unmount();
  });

  it('returns an empty summary when nothing notable is happening', () => {
    const hh = mountComposable();
    load(hh, [fakePcb(1, 'a'), fakePcb(2, 'b')]);
    expect(hh.api.tutorSummary()).toBe('');
    hh.unmount();
  });
});
