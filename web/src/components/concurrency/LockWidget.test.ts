import { describe, it, expect, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import LockWidget from './LockWidget.vue';
import { i18n } from '../../i18n';
import { useCpuStore, type CpuEvent, type PCBInfo } from '../../stores/cpu';

beforeEach(() => {
  // Pin the catalog so the copy assertions below are deterministic — jsdom
  // reports navigator.language as en-US, which would flip the default.
  (i18n.global.locale as unknown as { value: string }).value = 'es';
});

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

function dispatch(ev: CpuEvent) {
  (useCpuStore() as unknown as { __applyEvent: (e: CpuEvent) => void }).__applyEvent(ev);
}

describe('LockWidget', () => {
  it('renders a free mutex with its anchor and key on the hook', () => {
    const w = mount(LockWidget, {
      props: { kind: 'mutex' as const, lock: { id: 1, holder: -1, waiters: [] } },
    });
    expect(w.attributes('data-narrator-anchor')).toBe('mutex-1');
    expect(w.classes()).toContain('lw--free');
    expect(w.text()).toContain('M1');
    expect(w.text()).toContain('LIBRE');
    expect(w.find('[data-lock-key]').exists()).toBe(true);
    expect(w.find('.lw__hook').exists()).toBe(true);
    expect(w.find('.lw__capsule').exists()).toBe(false);
  });

  it('renders a held mutex with the holder name inside a capsule', () => {
    dispatch({ type: 'procs', procs: [fakePcb(2, 'worker', 'RUN')] });
    const w = mount(LockWidget, {
      props: { kind: 'mutex' as const, lock: { id: 3, holder: 2, waiters: [] } },
    });
    expect(w.classes()).toContain('lw--held');
    expect(w.text()).toContain('EN PODER DE P2 (worker)');
    expect(w.find('.lw__capsule').exists()).toBe(true);
    expect(w.find('[data-lock-key]').exists()).toBe(true);
  });

  it('renders the FIFO waiter pills with blocked ticks and pulsing head', () => {
    dispatch({ type: 'tick', value: 10 });
    dispatch({ type: 'process_state', core_id: 0, tick: 0, pid: 3, state: 'BLOCKED', reason: 'mutex:1' } as CpuEvent);
    dispatch({ type: 'tick', value: 22 });
    const w = mount(LockWidget, {
      props: { kind: 'mutex' as const, lock: { id: 1, holder: 2, waiters: [3, 4] } },
    });
    expect(w.classes()).toContain('lw--contended');
    const pills = w.findAll('.lw__waiter');
    expect(pills.length).toBe(2);
    expect(pills[0].classes()).toContain('lw__waiter--head');
    expect(pills[1].classes()).not.toContain('lw__waiter--head');
    expect(pills[0].text()).toContain('P3');
    expect(pills[0].text()).toContain('~12 ticks');
    // P4 never got a BLOCKED stamp → no tick counter for it.
    expect(pills[1].text()).toContain('P4');
    expect(pills[1].text()).not.toContain('ticks');
  });

  it('renders semaphore tokens and remembers the highest count as cap', async () => {
    const w = mount(LockWidget, {
      props: { kind: 'sem' as const, lock: { id: 1, count: 3, waiters: [] } },
    });
    expect(w.attributes('data-narrator-anchor')).toBe('sem-1');
    expect(w.text()).toContain('S1');
    expect(w.findAll('.lw__token').length).toBe(3);
    expect(w.findAll('.lw__token--on').length).toBe(3);

    await w.setProps({ lock: { id: 1, count: 1, waiters: [] } });
    expect(w.findAll('.lw__token').length).toBe(3);
    expect(w.findAll('.lw__token--on').length).toBe(1);
    expect(w.find('.lw__count').text()).toBe('1');
  });

  it('shows the numeric value even when no token cap is known', () => {
    const w = mount(LockWidget, {
      props: { kind: 'sem' as const, lock: { id: 2, count: 0, waiters: [5] } },
    });
    expect(w.findAll('.lw__token').length).toBe(0);
    expect(w.find('.lw__count').text()).toBe('0');
    expect(w.classes()).toContain('lw--contended');
    expect(w.findAll('.lw__waiter').length).toBe(1);
  });

  it('applies the cycle alarm class when inCycle', () => {
    const w = mount(LockWidget, {
      props: { kind: 'mutex' as const, lock: { id: 1, holder: 1, waiters: [2] }, inCycle: true },
    });
    expect(w.classes()).toContain('lw--cycle');
  });
});
