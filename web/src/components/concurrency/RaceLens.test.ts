import { describe, it, expect, beforeEach } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import RaceLens from './RaceLens.vue';
import { i18n } from '../../i18n';
import { useRaceDetector } from '../../composables/useRaceDetector';
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

const ADDR = 0x3fffe0;

function loadRaceDemo() {
  dispatch({
    type: 'loaded',
    name: 'threads_race',
    pids: [1, 2],
    procs: [fakePcb(1, 'racer_a', 'RUN'), fakePcb(2, 'racer_b', 'READY')],
    sources: {},
    var_meta: { 1: { globals: [{ name: 'counter', type: 'int', addr: '0x3FFFE0', len: 4 }] } },
  } as CpuEvent);
}

function assign(coreId: number, pid: number) {
  dispatch({ type: 'core_assign', core_id: coreId, tick: 0, pid } as CpuEvent);
}

function read(value: number, tick = 0) {
  dispatch({ type: 'mem_read', core_id: 0, tick, addr: ADDR, value, size: 4 } as CpuEvent);
}

function write(oldV: number, newV: number, tick = 0) {
  dispatch({ type: 'mem_write', core_id: 0, tick, addr: ADDR, old: oldV, new: newV, size: 4 } as CpuEvent);
}

describe('RaceLens', () => {
  it('shows the empty hint while no address is tracked', () => {
    const w = mount(RaceLens);
    expect(w.find('.rl__empty').exists()).toBe(true);
    expect(w.text()).toContain('Ninguna variable compartida');
  });

  it('renders the shared cell with value, name and address', async () => {
    const w = mount(RaceLens);
    loadRaceDemo();
    assign(0, 1);
    read(57, 5);
    await w.vm.$nextTick();
    expect(w.find('.rl__cell-value').text()).toBe('57');
    expect(w.find('.rl__cell-label').text()).toBe('counter');
    expect(w.find('.rl__cell-addr').text()).toBe('0x3FFFE0');
    // P1 holds an open window: its chip shows the local copy.
    const chip = w.find('.rl__chip');
    expect(chip.exists()).toBe(true);
    expect(chip.text()).toContain('copia local counter=57');
    // No losses yet: the scoreboard shows the green zero.
    expect(w.find('.rl__stat--good').text()).toContain('0 perdidas');
    expect(w.find('.rl__stat--bad').exists()).toBe(false);
  });

  it('shows the preempted badge when the window owner is evicted', async () => {
    const w = mount(RaceLens);
    loadRaceDemo();
    assign(0, 1);
    read(3, 5);
    dispatch({ type: 'sched_decision', core_id: 0, tick: 6, from_pid: 1, to_pid: 2, reason: 'timer', ready_queue: [] } as CpuEvent);
    await w.vm.$nextTick();
    expect(w.find('.rl__chip-badge').text()).toBe('desalojado');
  });

  it('increments the scoreboard and lists the entry on a lost update', async () => {
    const w = mount(RaceLens);
    loadRaceDemo();
    // P1 loads 0; P2 increments twice; P1 stores its stale copy.
    assign(0, 1);
    read(0, 10);
    assign(0, 2);
    read(0, 11);
    write(0, 1, 12);
    read(1, 13);
    write(1, 2, 14);
    assign(0, 1);
    write(2, 1, 15);
    await w.vm.$nextTick();

    const bad = w.find('.rl__stat--bad');
    expect(bad.exists()).toBe(true);
    expect(bad.text()).toContain('PERDIDAS');
    expect(bad.text()).toContain('2');
    // expected = actual(1) + lost(2)
    expect(w.text()).toContain('ESPERADO');
    expect(w.findAll('.rl__stat-value').map(s => s.text())).toEqual(['1', '2', '3']);

    const item = w.find('.rl__lost-item');
    expect(item.exists()).toBe(true);
    expect(item.text()).toBe('P1 escribió 1 sobre 2 — incrementos perdidos: 2');
    // The overwritten value falls away crossed out and the cell flashes.
    expect(w.find('.rl__fall').text()).toBe('2');
    expect(w.find('.rl__flash').exists()).toBe(true);
  });

  it('accepts a shared detector instance via prop', async () => {
    let detector!: ReturnType<typeof useRaceDetector>;
    const Host = defineComponent({
      setup() {
        detector = useRaceDetector();
        return () => h(RaceLens, { detector });
      },
    });
    const w = mount(Host);
    loadRaceDemo();
    assign(0, 1);
    read(9, 1);
    await w.vm.$nextTick();
    expect(detector.state.cellValue).toBe(9);
    expect(w.find('.rl__cell-value').text()).toBe('9');
  });
});
