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

  describe('state segments', () => {
    it('seeds a live segment per proc on loaded', () => {
      const h = mountComposable();
      h.dispatch({
        type: 'loaded',
        name: 'demo',
        pids: [1, 2],
        procs: [fakePcb(1, 'p1', 'RUN'), fakePcb(2, 'p2', 'READY')],
        sources: {},
      });
      expect(h.api.metrics.stateSegments.length).toBe(0);
      expect(h.api.metrics.liveSegmentByPid[1]?.state).toBe('RUN');
      expect(h.api.metrics.liveSegmentByPid[1]?.startTick).toBe(0);
      expect(h.api.metrics.liveSegmentByPid[1]?.endTick).toBeNull();
      expect(h.api.metrics.liveSegmentByPid[2]?.state).toBe('READY');
      h.unmount();
    });

    it('tracks RUN→BLOCKED→READY→RUN with reason and coreId', () => {
      const h = mountComposable();
      h.dispatch({
        type: 'loaded',
        name: 'demo',
        pids: [1],
        procs: [fakePcb(1, 'p1', 'READY')],
        sources: {},
      });
      // READY 0–10, then RUN on core 0.
      h.dispatch({ type: 'tick', value: 10 });
      h.dispatch({ type: 'core_assign', tick: 0, core_id: 0, pid: 1 });
      h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'RUN' });
      // RUN 10–14, then BLOCKED on mutex 1.
      h.dispatch({ type: 'tick', value: 14 });
      h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'BLOCKED', reason: 'mutex:1' });
      h.dispatch({ type: 'core_release', tick: 0, core_id: 0, pid: 1, reason: 'mutex:1' });
      // BLOCKED 14–22, then READY again.
      h.dispatch({ type: 'tick', value: 22 });
      h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'READY' });
      // READY 22–25, then RUN on core 1 (assign after the state event).
      h.dispatch({ type: 'tick', value: 25 });
      h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'RUN' });
      h.dispatch({ type: 'core_assign', tick: 0, core_id: 1, pid: 1 });

      const segs = h.api.metrics.stateSegments;
      expect(segs.map(s => s.state)).toEqual(['READY', 'RUN', 'BLOCKED', 'READY']);
      expect(segs[0]).toMatchObject({ pid: 1, startTick: 0, endTick: 10 });
      expect(segs[1]).toMatchObject({ startTick: 10, endTick: 14, coreId: 0 });
      expect(segs[2]).toMatchObject({ startTick: 14, endTick: 22, reason: 'mutex:1' });
      expect(segs[3]).toMatchObject({ startTick: 22, endTick: 25 });
      const live = h.api.metrics.liveSegmentByPid[1];
      expect(live).toMatchObject({ state: 'RUN', startTick: 25, endTick: null, coreId: 1 });
      h.unmount();
    });

    it('falls back to the store blocked_on when the event has no reason', () => {
      const h = mountComposable();
      h.dispatch({
        type: 'loaded',
        name: 'demo',
        pids: [1],
        procs: [fakePcb(1, 'p1', 'RUN')],
        sources: {},
      });
      h.dispatch({ type: 'procs', procs: [{ ...fakePcb(1, 'p1', 'RUN'), blocked_on: 'sem:2' }] });
      h.dispatch({ type: 'tick', value: 6 });
      h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'BLOCKED' });
      expect(h.api.metrics.liveSegmentByPid[1]?.reason).toBe('sem:2');
      h.unmount();
    });

    it('ignores repeated process_state with the same state', () => {
      const h = mountComposable();
      h.dispatch({
        type: 'loaded',
        name: 'demo',
        pids: [1],
        procs: [fakePcb(1, 'p1', 'READY')],
        sources: {},
      });
      h.dispatch({ type: 'tick', value: 5 });
      h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'READY' });
      expect(h.api.metrics.stateSegments.length).toBe(0);
      expect(h.api.metrics.liveSegmentByPid[1]?.startTick).toBe(0);
      h.unmount();
    });

    it('closes the live segment on ZOMBIE without opening a new one', () => {
      const h = mountComposable();
      h.dispatch({
        type: 'loaded',
        name: 'demo',
        pids: [1],
        procs: [fakePcb(1, 'p1', 'RUN')],
        sources: {},
      });
      h.dispatch({ type: 'tick', value: 30 });
      h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'ZOMBIE', reason: 'sys_exit' });
      expect(h.api.metrics.liveSegmentByPid[1]).toBeUndefined();
      const segs = h.api.metrics.stateSegments;
      expect(segs.length).toBe(1);
      expect(segs[0]).toMatchObject({ state: 'RUN', startTick: 0, endTick: 30 });
      h.unmount();
    });

    it('clears segments and marks on loaded and reset', () => {
      const h = mountComposable();
      h.dispatch({
        type: 'loaded',
        name: 'demo',
        pids: [1],
        procs: [fakePcb(1, 'p1', 'RUN')],
        sources: {},
      });
      h.dispatch({ type: 'tick', value: 4 });
      h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'READY' });
      h.dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid: 1, mutex_id: 1, op: 'acquired' });
      expect(h.api.metrics.stateSegments.length).toBe(1);
      expect(h.api.metrics.lockMarks.length).toBe(1);
      h.dispatch({
        type: 'loaded',
        name: 'demo2',
        pids: [2],
        procs: [fakePcb(2, 'p2', 'READY')],
        sources: {},
      });
      expect(h.api.metrics.stateSegments.length).toBe(0);
      expect(h.api.metrics.lockMarks.length).toBe(0);
      expect(h.api.metrics.liveSegmentByPid[1]).toBeUndefined();
      expect(h.api.metrics.liveSegmentByPid[2]?.state).toBe('READY');
      h.dispatch({ type: 'reset' });
      expect(Object.keys(h.api.metrics.liveSegmentByPid).length).toBe(0);
      h.unmount();
    });

    it('caps the segments ring buffer at 400', () => {
      const h = mountComposable();
      h.dispatch({
        type: 'loaded',
        name: 'demo',
        pids: [1],
        procs: [fakePcb(1, 'p1', 'READY')],
        sources: {},
      });
      for (let i = 0; i < 230; i++) {
        h.dispatch({ type: 'tick', value: i * 2 + 1 });
        h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'RUN' });
        h.dispatch({ type: 'tick', value: i * 2 + 2 });
        h.dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'READY' });
      }
      expect(h.api.metrics.stateSegments.length).toBe(400);
      h.unmount();
    });
  });

  describe('lock marks', () => {
    it('accumulates marks from mutex_op and sem_op', () => {
      const h = mountComposable();
      h.dispatch({
        type: 'loaded',
        name: 'demo',
        pids: [1, 2, 3],
        procs: [fakePcb(1, 'p1', 'RUN'), fakePcb(2, 'p2', 'READY'), fakePcb(3, 'p3', 'READY')],
        sources: {},
      });
      h.dispatch({ type: 'tick', value: 5 });
      h.dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid: 1, mutex_id: 1, op: 'create' });
      h.dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid: 1, mutex_id: 1, op: 'acquired' });
      h.dispatch({ type: 'tick', value: 8 });
      h.dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid: 2, mutex_id: 1, op: 'waiting', waiters: [2] });
      h.dispatch({ type: 'tick', value: 12 });
      h.dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid: 1, mutex_id: 1, op: 'handoff', next_pid: 2 });
      h.dispatch({ type: 'tick', value: 20 });
      h.dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid: 2, mutex_id: 1, op: 'released' });
      h.dispatch({ type: 'sem_op', core_id: 0, tick: 0, pid: 3, sem_id: 1, op: 'wait_blocked', count: 0 });
      h.dispatch({ type: 'sem_op', core_id: 0, tick: 0, pid: 1, sem_id: 1, op: 'signal', count: 1 });

      const marks = h.api.metrics.lockMarks;
      expect(marks.map(m => [m.pid, m.kind, m.tick])).toEqual([
        [1, 'acquired', 5],
        [2, 'waiting', 8],
        // A single handoff event is both the giver's release and the
        // receiver's acquisition — the kernel emits no separate 'released'.
        [1, 'released', 12],
        [2, 'acquired', 12],
        [2, 'released', 20],
        [3, 'waiting', 20],
        [1, 'released', 20],
      ]);
      h.unmount();
    });
  });
});
