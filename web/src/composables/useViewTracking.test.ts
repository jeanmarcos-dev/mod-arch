import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import { useViewTracking } from './useViewTracking';
import { useCpuStore } from '../stores/cpu';

const Blank = defineComponent({ render: () => h('div') });

const Host = defineComponent({
  setup() {
    useViewTracking();
    return () => h('div');
  },
});

function visibility(state: 'hidden' | 'visible') {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  document.dispatchEvent(new Event('visibilitychange'));
}

describe('useViewTracking', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-14T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
    visibility('visible');
  });

  async function setup() {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: Blank },
        { path: '/procesos', component: Blank },
        { path: '/memoria', component: Blank },
        { path: '/admin/live', component: Blank },
      ],
    });
    router.push('/procesos');
    await router.isReady();
    const wrapper = mount(Host, { global: { plugins: [router] } });
    const cpu = useCpuStore();
    const sendTrack = vi.spyOn(cpu, 'sendTrack').mockImplementation(() => undefined);
    return { router, wrapper, sendTrack };
  }

  it('sends the elapsed time of a route when navigating away', async () => {
    const { router, sendTrack } = await setup();
    vi.advanceTimersByTime(40_000);
    await router.push('/memoria');
    expect(sendTrack).toHaveBeenCalledWith({
      cmd: 'track',
      event: 'page_view',
      name: '/procesos',
      duration_ms: 40_000,
    });
  });

  it('closes the route on visibilitychange and reopens it without hidden time', async () => {
    const { router, sendTrack } = await setup();
    vi.advanceTimersByTime(10_000);
    visibility('hidden');
    vi.advanceTimersByTime(300_000);
    visibility('visible');
    vi.advanceTimersByTime(3_000);
    await router.push('/memoria');
    const durations = sendTrack.mock.calls.map(([c]) => (c as { duration_ms: number }).duration_ms);
    expect(durations).toEqual([10_000, 3_000]);
  });

  it('closes the current route on pagehide', async () => {
    const { sendTrack } = await setup();
    vi.advanceTimersByTime(7_000);
    window.dispatchEvent(new Event('pagehide'));
    expect(sendTrack).toHaveBeenCalledTimes(1);
    expect(sendTrack.mock.calls[0][0]).toMatchObject({ name: '/procesos', duration_ms: 7_000 });
  });

  it('does not report admin routes', async () => {
    const { router, sendTrack } = await setup();
    await router.push('/admin/live');
    sendTrack.mockClear();
    vi.advanceTimersByTime(5_000);
    await router.push('/memoria');
    expect(sendTrack).not.toHaveBeenCalled();
  });
});
