import { describe, it, expect, beforeEach } from 'vitest';
import { mount, RouterLinkStub } from '@vue/test-utils';
import DeadlockChip from './DeadlockChip.vue';
import { i18n } from '../../i18n';

beforeEach(() => {
  // Pin the catalog so the Spanish copy assertions are deterministic.
  (i18n.global.locale as unknown as { value: string }).value = 'es';
});

function mountChip(props: { hasDeadlock: boolean; tick: number | null }) {
  return mount(DeadlockChip, {
    props,
    global: { stubs: { RouterLink: RouterLinkStub } },
  });
}

describe('DeadlockChip', () => {
  it('is absent when there is no deadlock', () => {
    const w = mountChip({ hasDeadlock: false, tick: null });
    expect(w.find('.deadlock-chip').exists()).toBe(false);
    expect(w.findComponent(RouterLinkStub).exists()).toBe(false);
  });

  it('renders with the tick and links to the concurrency view', () => {
    const w = mountChip({ hasDeadlock: true, tick: 42 });
    const link = w.findComponent(RouterLinkStub);
    expect(link.exists()).toBe(true);
    expect(link.props('to')).toBe('/concurrencia');
    expect(w.text()).toContain('Posible interbloqueo (tick 42)');
  });

  it('omits the tick number when the tick is null', () => {
    const w = mountChip({ hasDeadlock: true, tick: null });
    expect(w.find('.deadlock-chip').exists()).toBe(true);
    expect(w.text()).toContain('Posible interbloqueo');
    expect(w.text()).not.toContain('tick');
  });
});
