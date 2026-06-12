import { describe, it, expect } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import { useRaceDetector, LOST_CAP } from './useRaceDetector';
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
  api: ReturnType<typeof useRaceDetector>;
  cpu: ReturnType<typeof useCpuStore>;
  dispatch: (ev: CpuEvent) => void;
  unmount: () => void;
}

function mountComposable(): Harness {
  let api!: ReturnType<typeof useRaceDetector>;
  const Host = defineComponent({
    setup() {
      api = useRaceDetector();
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

function load(hh: Harness, varMeta?: Record<number, Record<string, unknown>>) {
  hh.dispatch({
    type: 'loaded',
    name: 'demo',
    pids: [1, 2],
    procs: [fakePcb(1, 'racer_a', 'RUN'), fakePcb(2, 'racer_b', 'READY')],
    sources: {},
    var_meta: varMeta,
  } as CpuEvent);
}

function counterMeta(addr = '0x3FFFE0') {
  return { 1: { globals: [{ name: 'counter', type: 'int', addr, len: 4 }] } };
}

// Binds a PID to a core so subsequent mem events on that core attribute to it.
function assign(hh: Harness, coreId: number, pid: number) {
  hh.dispatch({ type: 'core_assign', core_id: coreId, tick: 0, pid } as CpuEvent);
}

function read(hh: Harness, value: number, tick = 0, coreId = 0, addr = ADDR, size = 4) {
  hh.dispatch({ type: 'mem_read', core_id: coreId, tick, addr, value, size } as CpuEvent);
}

function write(hh: Harness, oldV: number, newV: number, tick = 0, coreId = 0, addr = ADDR, size = 4) {
  hh.dispatch({ type: 'mem_write', core_id: coreId, tick, addr, old: oldV, new: newV, size } as CpuEvent);
}

describe('useRaceDetector', () => {
  it('starts with nothing tracked', () => {
    const hh = mountComposable();
    expect(hh.api.state.trackedAddr).toBeNull();
    expect(hh.api.state.cellValue).toBeNull();
    expect(hh.api.state.lostTotal).toBe(0);
    expect(hh.api.expectedFinal.value).toBeNull();
    hh.unmount();
  });

  it('resolves the tracked address and name from var_meta on loaded', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    expect(hh.api.state.trackedAddr).toBe(ADDR);
    expect(hh.api.state.trackedName).toBe('counter');
    hh.unmount();
  });

  it('prefers a global shared by two PIDs over per-PID singletons', () => {
    const hh = mountComposable();
    load(hh, {
      1: { globals: [
        { name: 'mine', type: 'int', addr: '0x3F0010', len: 4 },
        { name: 'counter', type: 'int', addr: '0x3FFFE0', len: 4 },
      ] },
      2: { globals: [{ name: 'counter', type: 'int', addr: '0x3FFFE0', len: 4 }] },
    });
    expect(hh.api.state.trackedAddr).toBe(ADDR);
    expect(hh.api.state.trackedName).toBe('counter');
    hh.unmount();
  });

  it('falls back to auto-detection: addr read AND written by two distinct PIDs', () => {
    const hh = mountComposable();
    load(hh);
    expect(hh.api.state.trackedAddr).toBeNull();

    assign(hh, 0, 1);
    read(hh, 0);
    write(hh, 0, 1);
    // A second addr touched by P1 only must never be adopted.
    read(hh, 7, 0, 0, 0x300100);
    write(hh, 7, 8, 0, 0, 0x300100);
    expect(hh.api.state.trackedAddr).toBeNull();

    assign(hh, 0, 2);
    read(hh, 1);
    expect(hh.api.state.trackedAddr).toBeNull();
    write(hh, 1, 2);
    expect(hh.api.state.trackedAddr).toBe(ADDR);
    hh.unmount();
  });

  it('never adopts kernel-space addresses in the fallback', () => {
    const hh = mountComposable();
    load(hh);
    // INT entry / IRET traffic lands on the per-core kernel stack, which is
    // shared by every PID: read and written by two PIDs, yet not a counter.
    const kstack = 0x1fffec;
    assign(hh, 0, 1);
    write(hh, 0, 7, 0, 0, kstack);
    assign(hh, 0, 2);
    read(hh, 7, 1, 0, kstack);
    write(hh, 7, 9, 2, 0, kstack);
    assign(hh, 0, 1);
    read(hh, 9, 3, 0, kstack);
    expect(hh.api.state.trackedAddr).toBeNull();
    expect(hh.api.state.cellValue).toBeNull();
    hh.unmount();
  });

  it('never adopts sub-word accesses in the fallback', () => {
    const hh = mountComposable();
    load(hh);
    assign(hh, 0, 1);
    read(hh, 0, 0, 0, ADDR, 1);
    write(hh, 0, 1, 1, 0, ADDR, 1);
    assign(hh, 0, 2);
    read(hh, 1, 2, 0, ADDR, 1);
    write(hh, 1, 2, 3, 0, ADDR, 1);
    expect(hh.api.state.trackedAddr).toBeNull();
    hh.unmount();
  });

  it('seeds the tracked address from the store when mounted after loaded', () => {
    // Apply `loaded` to the store before any detector exists, then mount:
    // the composable must resolve from the stored var metadata immediately.
    const cpu = useCpuStore();
    (cpu as unknown as { __applyEvent: (e: CpuEvent) => void }).__applyEvent({
      type: 'loaded',
      name: 'demo',
      pids: [1, 2],
      procs: [fakePcb(1, 'racer_a', 'RUN'), fakePcb(2, 'racer_b', 'READY')],
      sources: {},
      var_meta: counterMeta(),
    } as CpuEvent);
    const hh = mountComposable();
    expect(hh.api.state.trackedAddr).toBe(ADDR);
    expect(hh.api.state.trackedName).toBe('counter');
    hh.unmount();
  });

  it('detects a lost update on the read-read-write-write interleave', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
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

    expect(hh.api.state.lostUpdates.length).toBe(1);
    const lu = hh.api.state.lostUpdates[0];
    expect(lu.clobberPid).toBe(1);
    expect(lu.victimPid).toBe(2);
    expect(lu.lostValue).toBe(2);
    expect(lu.newValue).toBe(1);
    expect(lu.lostCount).toBe(2); // old(2) - inFlight(0)
    expect(lu.tick).toBe(15);
    expect(hh.api.state.lostTotal).toBe(2);
    expect(hh.api.state.cellValue).toBe(1);
    // expected = actual + lost = every increment actually executed.
    expect(hh.api.expectedFinal.value).toBe(3);
    hh.unmount();
  });

  it('reports no lost update for serialized read-write pairs', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    read(hh, 0, 1);
    write(hh, 0, 1, 2);
    assign(hh, 0, 2);
    read(hh, 1, 3);
    write(hh, 1, 2, 4);
    assign(hh, 0, 1);
    read(hh, 2, 5);
    write(hh, 2, 3, 6);

    expect(hh.api.state.lostUpdates.length).toBe(0);
    expect(hh.api.state.lostTotal).toBe(0);
    expect(hh.api.state.cellValue).toBe(3);
    expect(hh.api.expectedFinal.value).toBe(3);
    hh.unmount();
  });

  it('ignores a store with no prior load (setup writing 0)', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    write(hh, 413, 0, 1);
    expect(hh.api.state.lostUpdates.length).toBe(0);
    expect(hh.api.state.lostTotal).toBe(0);
    expect(hh.api.state.cellValue).toBe(0);
    hh.unmount();
  });

  it('records no lost entry when the cell moved backwards', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    read(hh, 5, 1);
    // old < copy (e.g. a reset write elsewhere): nothing was lost.
    write(hh, 3, 4, 2);
    expect(hh.api.state.lostUpdates.length).toBe(0);
    expect(hh.api.state.lostTotal).toBe(0);
    expect(hh.api.state.openWindows[1]).toBeUndefined();
    hh.unmount();
  });

  it('opens a window on load and closes it on the matching store', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    read(hh, 5, 20);
    const w = hh.api.state.openWindows[1];
    expect(w).toBeDefined();
    expect(w.value).toBe(5);
    expect(w.sinceTick).toBe(20);
    expect(w.preempted).toBe(false);
    write(hh, 5, 6, 21);
    expect(hh.api.state.openWindows[1]).toBeUndefined();
    hh.unmount();
  });

  it('marks the window preempted on sched_decision evicting its owner', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    read(hh, 3, 30);
    hh.dispatch({ type: 'sched_decision', core_id: 0, tick: 31, from_pid: 1, to_pid: 2, reason: 'timer', ready_queue: [] } as CpuEvent);
    expect(hh.api.state.openWindows[1].preempted).toBe(true);
    // A decision evicting a PID without a window is a no-op.
    hh.dispatch({ type: 'sched_decision', core_id: 0, tick: 32, from_pid: 9, to_pid: 1, reason: 'timer', ready_queue: [] } as CpuEvent);
    expect(Object.keys(hh.api.state.openWindows).length).toBe(1);
    hh.unmount();
  });

  it('closes the open window when its owner exits', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    read(hh, 4, 10);
    expect(hh.api.state.openWindows[1]).toBeDefined();
    // Non-exit transitions keep the window open.
    hh.dispatch({ type: 'process_state', core_id: 0, tick: 11, pid: 1, state: 'BLOCKED', reason: 'mutex' } as CpuEvent);
    expect(hh.api.state.openWindows[1]).toBeDefined();
    hh.dispatch({ type: 'process_state', core_id: 0, tick: 12, pid: 1, state: 'ZOMBIE', exit_code: 0 } as CpuEvent);
    expect(hh.api.state.openWindows[1]).toBeUndefined();
    hh.unmount();
  });

  it('attributes accesses through core current_pid on distinct cores', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    assign(hh, 1, 2);
    read(hh, 0, 1, 0);
    read(hh, 0, 2, 1);
    expect(hh.api.state.openWindows[1]).toBeDefined();
    expect(hh.api.state.openWindows[2]).toBeDefined();
    expect(hh.api.state.openWindows[1].sinceTick).toBe(1);
    expect(hh.api.state.openWindows[2].sinceTick).toBe(2);
    hh.unmount();
  });

  it('caps the lost-update ring', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    for (let i = 0; i < LOST_CAP + 10; i++) {
      read(hh, 0, i);
      // old differs from the in-flight value on every store: each is a lost update.
      write(hh, 5, 1, i);
    }
    expect(hh.api.state.lostUpdates.length).toBe(LOST_CAP);
    expect(hh.api.state.lostTotal).toBe((LOST_CAP + 10) * 5);
    hh.unmount();
  });

  it('clears everything on loaded and on reset', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    read(hh, 0, 1);
    write(hh, 9, 1, 2);
    expect(hh.api.state.lostTotal).toBe(9);

    load(hh, counterMeta());
    expect(hh.api.state.lostTotal).toBe(0);
    expect(hh.api.state.lostUpdates).toEqual([]);
    expect(hh.api.state.cellValue).toBeNull();
    expect(hh.api.state.openWindows).toEqual({});
    expect(hh.api.state.trackedAddr).toBe(ADDR);

    hh.dispatch({ type: 'reset' } as CpuEvent);
    expect(hh.api.state.trackedAddr).toBeNull();
    expect(hh.api.state.trackedName).toBeNull();
    hh.unmount();
  });

  it('ignores traffic on other addresses once tracking', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    assign(hh, 0, 1);
    read(hh, 1, 1, 0, 0x200);
    write(hh, 1, 2, 2, 0, 0x200);
    expect(hh.api.state.cellValue).toBeNull();
    expect(Object.keys(hh.api.state.openWindows).length).toBe(0);
    hh.unmount();
  });

  it('tracks the wall tick from tick events', () => {
    const hh = mountComposable();
    load(hh, counterMeta());
    hh.dispatch({ type: 'tick', value: 42 } as CpuEvent);
    expect(hh.api.state.currentTick).toBe(42);
    hh.unmount();
  });
});
