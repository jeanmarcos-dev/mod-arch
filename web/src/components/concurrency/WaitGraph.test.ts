import { describe, it, expect, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import WaitGraph from './WaitGraph.vue';
import { i18n } from '../../i18n';
import { useCpuStore, type CpuEvent } from '../../stores/cpu';

beforeEach(() => {
  (i18n.global.locale as unknown as { value: string }).value = 'es';
});

function dispatch(ev: CpuEvent) {
  (useCpuStore() as unknown as { __applyEvent: (e: CpuEvent) => void }).__applyEvent(ev);
}

function mutexOp(op: string, mutexId: number, pid: number, waiters: number[] = []) {
  dispatch({ type: 'mutex_op', core_id: 0, tick: 0, pid, mutex_id: mutexId, op, waiters } as CpuEvent);
}

describe('WaitGraph', () => {
  it('shows the didactic empty state when the demo has no locks', () => {
    const w = mount(WaitGraph);
    expect(w.find('svg').exists()).toBe(false);
    expect(w.text()).toContain('Ningún proceso espera por un recurso.');
    expect(w.find('.wg__empty-hint').exists()).toBe(true);
  });

  it('renders nodes, edges and the legend once locks exist', async () => {
    const w = mount(WaitGraph);
    mutexOp('acquired', 1, 1);
    mutexOp('waiting', 1, 2, [2]);
    await w.vm.$nextTick();
    expect(w.find('svg').exists()).toBe(true);
    expect(w.findAll('.wg__proc').length).toBe(2);
    expect(w.findAll('.wg__lock').length).toBe(1);
    expect(w.findAll('.wg__edge--holds').length).toBeGreaterThanOrEqual(1);
    expect(w.findAll('.wg__edge--waits').length).toBeGreaterThanOrEqual(1);
    expect(w.find('.wg__legend').exists()).toBe(true);
    expect(w.find('.wg__banner').exists()).toBe(false);
  });

  it('truncates long proc names and keeps the full label as tooltip', async () => {
    const w = mount(WaitGraph);
    dispatch({
      type: 'procs',
      procs: [{
        pid: 1,
        name: 'productor_consumidor',
        state: 'RUN',
        regs: {},
        saved_user_esp: 0,
        code_base: 0,
        code_size: 0,
        stack_top: 0,
      }],
    });
    mutexOp('acquired', 1, 1);
    await w.vm.$nextTick();
    expect(w.find('.wg__proc-label').text()).toBe('P1 producto…');
    expect(w.find('.wg__proc title').text()).toBe('P1 productor_consumidor');
  });

  it('shows the deadlock banner with the interpolated cycle', async () => {
    const w = mount(WaitGraph);
    mutexOp('acquired', 1, 1);
    mutexOp('acquired', 2, 2);
    mutexOp('waiting', 2, 1, [1]);
    mutexOp('waiting', 1, 2, [2]);
    await w.vm.$nextTick();
    const banner = w.find('.wg__banner');
    expect(banner.exists()).toBe(true);
    expect(banner.text()).toContain('INTERBLOQUEO DETECTADO');
    expect(banner.text()).toMatch(/P\d → P\d → P\d/);
    expect(w.findAll('.wg__edge--cycle').length).toBe(4);
  });
});
