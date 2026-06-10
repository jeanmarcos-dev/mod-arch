import { describe, it, expect } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import { useSchedulerMetrics } from './useSchedulerMetrics';
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
  api: ReturnType<typeof useSchedulerMetrics>;
  cpu: ReturnType<typeof useCpuStore>;
  dispatch: (ev: CpuEvent) => void;
  unmount: () => void;
}

function mountComposable(): Harness {
  let api!: ReturnType<typeof useSchedulerMetrics>;
  const Host = defineComponent({
    setup() {
      api = useSchedulerMetrics();
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

describe('useSchedulerMetrics', () => {
  it('initialises empty', () => {
    const h = mountComposable();
    expect(Object.keys(h.api.metrics.byPid).length).toBe(0);
    expect(h.api.metrics.slices.length).toBe(0);
    expect(h.api.metrics.completedProcs).toBe(0);
    expect(h.api.currentTickRel.value).toBe(0);
    expect(h.api.throughputPerKTick.value).toBe(0);
    h.unmount();
  });

  it('resets and seeds PIDs on loaded', () => {
    const h = mountComposable();
    h.dispatch({ type: 'tick', value: 50 });
    h.dispatch({
      type: 'loaded',
      name: 'demo',
      pids: [1, 2],
      procs: [fakePcb(1, 'p1', 'RUN'), fakePcb(2, 'p2', 'READY')],
      sources: {},
    });
    // baselineTick = 50 → all relative times start at 0.
    expect(h.api.currentTickRel.value).toBe(0);
    expect(h.api.metrics.byPid[1]?.name).toBe('p1');
    expect(h.api.metrics.byPid[1]?.createdAtTick).toBe(0);
    expect(h.api.metrics.byPid[2]?.createdAtTick).toBe(0);
    h.unmount();
  });

  it('counts completed quantum on timer-driven sched_decision', () => {
    const h = mountComposable();
    h.dispatch({
      type: 'loaded',
      name: 'demo',
      pids: [1, 2],
      procs: [fakePcb(1, 'p1', 'RUN'), fakePcb(2, 'p2', 'READY')],
      sources: {},
    });
    h.dispatch({ type: 'core_assign', tick: 0, core_id: 0, pid: 1 });
    h.dispatch({ type: 'tick', value: 8 });
    h.dispatch({ type: 'core_release', tick: 0, core_id: 0, pid: 1, reason: 'preempted' });
    h.dispatch({ type: 'sched_decision', core_id: 0, tick: 0, from_pid: 1, to_pid: 2, reason: 'timer', ready_queue: [] });
    expect(h.api.metrics.byPid[1].quantumsCompleted).toBe(1);
    expect(h.api.metrics.byPid[1].quantumsPartial).toBe(0);
    expect(h.api.metrics.byPid[1].totalRunTicks).toBe(8);
    expect(h.api.metrics.byPid[1].firstRunAtTick).toBe(0);
    expect(h.api.metrics.slices.length).toBe(1);
    expect(h.api.metrics.slices[0].endTick).toBe(8);
    h.unmount();
  });

  it('counts partial quantum on block-driven sched_decision', () => {
    const h = mountComposable();
    h.dispatch({
      type: 'loaded',
      name: 'demo',
      pids: [1, 2],
      procs: [fakePcb(1, 'p1', 'RUN'), fakePcb(2, 'p2', 'READY')],
      sources: {},
    });
    h.dispatch({ type: 'core_assign', tick: 0, core_id: 0, pid: 1 });
    h.dispatch({ type: 'tick', value: 3 });
    h.dispatch({ type: 'core_release', tick: 0, core_id: 0, pid: 1, reason: 'mutex:1' });
    h.dispatch({ type: 'sched_decision', core_id: 0, tick: 0, from_pid: 1, to_pid: 2, reason: 'block', ready_queue: [] });
    expect(h.api.metrics.byPid[1].quantumsCompleted).toBe(0);
    expect(h.api.metrics.byPid[1].quantumsPartial).toBe(1);
    expect(h.api.metrics.byPid[1].totalRunTicks).toBe(3);
    h.unmount();
  });

  it('ignores wake-ups onto idle cores (from_pid=0)', () => {
    const h = mountComposable();
    h.dispatch({
      type: 'loaded',
      name: 'demo',
      pids: [1],
      procs: [fakePcb(1, 'p1', 'READY')],
      sources: {},
    });
    h.dispatch({ type: 'sched_decision', core_id: 0, tick: 0, from_pid: 0, to_pid: 1, reason: 'wake', ready_queue: [] });
    expect(h.api.metrics.byPid[1]?.quantumsCompleted ?? 0).toBe(0);
    expect(h.api.metrics.byPid[1]?.quantumsPartial ?? 0).toBe(0);
    h.unmount();
  });

  it('accumulates ready and blocked durations from process_state transitions', () => {
    const h = mountComposable();
    h.dispatch({
      type: 'loaded',
      name: 'demo',
      pids: [1],
      procs: [fakePcb(1, 'p1', 'READY')],
      sources: {},
    });
    // READY (tick 0) → RUN (tick 10) ⇒ +10 ready ticks
    h.dispatch({ type: 'tick', value: 10 });
    h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'RUN' });
    // RUN (not counted via process_state) → BLOCKED (tick 14)
    h.dispatch({ type: 'tick', value: 14 });
    h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'BLOCKED', reason: 'mutex:1' });
    // BLOCKED (tick 14) → READY (tick 22) ⇒ +8 blocked ticks
    h.dispatch({ type: 'tick', value: 22 });
    h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'READY' });
    expect(h.api.metrics.byPid[1].totalReadyTicks).toBe(10);
    expect(h.api.metrics.byPid[1].totalBlockedTicks).toBe(8);
    h.unmount();
  });

  it('marks zombie and updates throughput', () => {
    const h = mountComposable();
    h.dispatch({
      type: 'loaded',
      name: 'demo',
      pids: [1, 2],
      procs: [fakePcb(1, 'p1', 'RUN'), fakePcb(2, 'p2', 'READY')],
      sources: {},
    });
    h.dispatch({ type: 'tick', value: 100 });
    h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'ZOMBIE', reason: 'sys_exit' });
    expect(h.api.metrics.byPid[1].endedAtTick).toBe(100);
    expect(h.api.metrics.completedProcs).toBe(1);
    // throughput = 1 * 1000 / 100 = 10 procs per 1000 ticks
    expect(h.api.throughputPerKTick.value).toBe(10);
    h.unmount();
  });

  it('caps the slices ring buffer at 300', () => {
    const h = mountComposable();
    h.dispatch({
      type: 'loaded',
      name: 'demo',
      pids: [1],
      procs: [fakePcb(1, 'p1', 'READY')],
      sources: {},
    });
    for (let i = 0; i < 350; i++) {
      h.dispatch({ type: 'tick', value: i * 2 });
      h.dispatch({ type: 'core_assign', tick: 0, core_id: 0, pid: 1 });
      h.dispatch({ type: 'tick', value: i * 2 + 1 });
      h.dispatch({ type: 'core_release', tick: 0, core_id: 0, pid: 1, reason: 'preempted' });
    }
    expect(h.api.metrics.slices.length).toBe(300);
    h.unmount();
  });

  it('resets metrics on reset event', () => {
    const h = mountComposable();
    h.dispatch({
      type: 'loaded',
      name: 'demo',
      pids: [1],
      procs: [fakePcb(1, 'p1', 'RUN')],
      sources: {},
    });
    h.dispatch({ type: 'core_assign', tick: 0, core_id: 0, pid: 1 });
    h.dispatch({ type: 'tick', value: 5 });
    h.dispatch({ type: 'core_release', tick: 0, core_id: 0, pid: 1, reason: 'preempted' });
    expect(h.api.metrics.slices.length).toBe(1);
    h.dispatch({ type: 'reset' });
    expect(Object.keys(h.api.metrics.byPid).length).toBe(0);
    expect(h.api.metrics.slices.length).toBe(0);
    expect(h.api.metrics.completedProcs).toBe(0);
    h.unmount();
  });
});
