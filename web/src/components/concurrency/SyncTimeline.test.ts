import { describe, it, expect, beforeEach } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import SyncTimeline from './SyncTimeline.vue';
import { useSchedulerMetrics, type UseSchedulerMetricsReturn } from '../../composables/useSchedulerMetrics';
import { i18n } from '../../i18n';
import { useCpuStore, type CpuEvent, type PCBInfo } from '../../stores/cpu';

beforeEach(() => {
  (i18n.global.locale as unknown as { value: string }).value = 'es';
});

function dispatch(ev: CpuEvent) {
  (useCpuStore() as unknown as { __applyEvent: (e: CpuEvent) => void }).__applyEvent(ev);
}

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

function loadDemo() {
  dispatch({
    type: 'loaded',
    name: 'demo',
    pids: [1, 2],
    procs: [fakePcb(1, 'filosofo_a', 'RUN'), fakePcb(2, 'filosofo_b', 'READY')],
    sources: {},
  } as CpuEvent);
}

describe('SyncTimeline', () => {
  it('shows the empty hint before any segment exists', () => {
    const w = mount(SyncTimeline);
    expect(w.find('svg').exists()).toBe(false);
    expect(w.find('.st__empty').exists()).toBe(true);
    expect(w.text()).toContain('Aún no hay transiciones de estado');
  });

  it('renders one lane per user proc with its segments', async () => {
    const w = mount(SyncTimeline);
    loadDemo();
    dispatch({ type: 'tick', value: 10 });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'BLOCKED', reason: 'mutex:1' });
    dispatch({ type: 'tick', value: 30 });
    dispatch({ type: 'core_assign', tick: 0, core_id: 0, pid: 2 });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 2, state: 'RUN' });
    await w.vm.$nextTick();
    expect(w.find('svg').exists()).toBe(true);
    const labels = w.findAll('.st__lane-label');
    expect(labels.length).toBe(2);
    expect(labels[0].text()).toContain('P1 filosofo_a');
    // P1: closed RUN 0–10 + live BLOCKED; P2: closed READY 0–30 + live RUN.
    expect(w.findAll('.st__seg').length).toBe(4);
    expect(w.findAll('.st__seg--blocked').length).toBe(1);
    expect(w.find('.st__lock-tag').text()).toBe('M1');
    expect(w.find('.st__legend').exists()).toBe(true);
  });

  it('excludes kernel procs from the lanes', async () => {
    const w = mount(SyncTimeline);
    loadDemo();
    dispatch({
      type: 'procs',
      procs: [
        { ...fakePcb(0, 'swapper/0', 'RUN'), is_idle: true },
        { ...fakePcb(1, 'init', 'BLOCKED') },
        { ...fakePcb(2, 'kernel/0', 'BLOCKED'), is_kthread: true },
        fakePcb(3, 'worker', 'READY'),
      ],
    } as CpuEvent);
    dispatch({ type: 'tick', value: 5 });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'BLOCKED', reason: 'wait(any)' });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 2, state: 'BLOCKED' });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 3, state: 'RUN' });
    await w.vm.$nextTick();
    const labels = w.findAll('.st__lane-label');
    expect(labels.length).toBe(1);
    expect(labels[0].text()).toContain('P3 worker');
  });

  it('renders lock markers from mutex ops', async () => {
    const w = mount(SyncTimeline);
    loadDemo();
    dispatch({ type: 'tick', value: 5 });
    dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid: 1, mutex_id: 1, op: 'acquired' } as CpuEvent);
    dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid: 2, mutex_id: 1, op: 'waiting', waiters: [2] } as CpuEvent);
    await w.vm.$nextTick();
    const marks = w.findAll('.st__mark');
    expect(marks.length).toBe(2);
    expect(marks[0].text()).toBe('▼');
    expect(marks[1].text()).toBe('✖');
  });

  it('draws the deadlock line and label when the prop is set', async () => {
    const w = mount(SyncTimeline, { props: { deadlockAtTick: 12 } });
    loadDemo();
    dispatch({ type: 'tick', value: 20 });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'BLOCKED', reason: 'mutex:1' });
    await w.vm.$nextTick();
    expect(w.find('.st__deadlock-line').exists()).toBe(true);
    expect(w.find('.st__deadlock-label').text()).toContain('interbloqueo');
  });

  it('consumes a shared metrics instance passed via the metrics prop', async () => {
    let shared!: UseSchedulerMetricsReturn;
    const Host = defineComponent({
      setup() {
        shared = useSchedulerMetrics();
        return () => h(SyncTimeline, { metrics: shared });
      },
    });
    const w = mount(Host);
    loadDemo();
    dispatch({ type: 'tick', value: 10 });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'BLOCKED', reason: 'mutex:1' });
    await w.vm.$nextTick();
    // The lanes must come from the shared accumulator, not a private one.
    expect(Object.keys(shared.metrics.liveSegmentByPid).length).toBeGreaterThan(0);
    expect(w.findAll('.st__lane-label').length).toBe(2);
    expect(w.findAll('.st__seg--blocked').length).toBe(1);
  });

  it('omits the deadlock line when the prop is null', async () => {
    const w = mount(SyncTimeline, { props: { deadlockAtTick: null } });
    loadDemo();
    dispatch({ type: 'tick', value: 20 });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 1, state: 'BLOCKED', reason: 'mutex:1' });
    await w.vm.$nextTick();
    expect(w.find('.st__deadlock-line').exists()).toBe(false);
  });
});
