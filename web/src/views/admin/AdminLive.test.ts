import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import AdminLive from './AdminLive.vue';
import { normalizeLive, useAdminStore, type LiveStats } from '../../stores/admin';
import { LIVE_REFRESH_MS, LIVE_STORAGE_KEY } from '../../composables/useLiveDashboard';
import { i18n } from '../../i18n';

const QUESTION = '¿Por qué se bloquean los filósofos?';

function payload(): LiveStats {
  return normalizeLive({
    generated_at: 1760410000000,
    window_minutes: 120,
    now: {
      sessions_active: 64, ws_connections: 71, sessions_with_user: 40, sessions_anonymous: 24,
      peak_sessions: 70, sessions_created_total: 112,
      sections: [{ name: '/concurrencia', count: 21 }],
      demos: [{ name: 'philos_deadlock', count: 12 }],
    },
    window: {
      unique_users: 52, unique_sessions: 90,
      page_views: [{ name: '/procesos', views: 140, avg_ms: 61000 }],
      demos: [{ name: 'race_counter', loads: 33, runs: 41 }],
      activity_per_minute: [
        { minute: 1760409900000, events: 20, questions: 1 },
        { minute: 1760409960000, events: 35, questions: 4 },
      ],
    },
    tutor: {
      questions: 87, askers: 31, latency_p50_ms: 2900, latency_p95_ms: 6100, degraded: 2,
      thumbs_up: 25, thumbs_down: 3,
      top_topics: [{ label: '[Glosario: Deadlock]', count: 9 }],
      recent: [{ ts: 1760409990000, question: QUESTION, rating: 1, degraded: false }],
    },
    countries: [{ code: 'VE', count: 58 }],
  });
}

async function mountLive() {
  const admin = useAdminStore();
  const load = vi.spyOn(admin, 'loadLive').mockImplementation(async () => {
    admin.live = payload();
    return admin.live;
  });
  const wrapper = mount(AdminLive);
  await flushPromises();
  return { wrapper, admin, load };
}

describe('AdminLive', () => {
  beforeEach(() => {
    window.localStorage.clear();
    (i18n.global.locale as unknown as { value: string }).value = 'es';
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows the headline numbers', async () => {
    const { wrapper } = await mountLive();
    expect(wrapper.get('[data-testid="live-connected"]').text()).toBe('64');
    expect(wrapper.get('[data-testid="live-with-account"]').text()).toBe('40');
    expect(wrapper.get('[data-testid="live-peak"]').text()).toBe('70');
    expect(wrapper.get('[data-testid="live-questions"]').text()).toBe('87');
  });

  it('starts in presentation mode and hides the question text', async () => {
    const { wrapper } = await mountLive();
    expect(wrapper.find('[data-testid="live-recent"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain(QUESTION);
    expect(wrapper.find('[data-testid="live-presentation-hint"]').exists()).toBe(true);
    expect(wrapper.text()).toContain('Glosario: Deadlock');
  });

  it('shows the recent questions once presentation mode is turned off', async () => {
    const { wrapper } = await mountLive();
    await wrapper.get('[data-testid="live-presentation-toggle"]').trigger('click');
    expect(wrapper.get('[data-testid="live-recent"]').text()).toContain(QUESTION);
    await wrapper.get('[data-testid="live-presentation-toggle"]').trigger('click');
    expect(wrapper.text()).not.toContain(QUESTION);
  });

  it('remembers presentation mode being off', async () => {
    window.localStorage.setItem(LIVE_STORAGE_KEY, JSON.stringify({ presentation: false, windowMinutes: 30 }));
    const { wrapper, load } = await mountLive();
    expect(wrapper.text()).toContain(QUESTION);
    expect(load).toHaveBeenCalledWith(30);
  });

  it('persists the choice when toggled', async () => {
    const { wrapper } = await mountLive();
    await wrapper.get('[data-testid="live-presentation-toggle"]').trigger('click');
    expect(JSON.parse(window.localStorage.getItem(LIVE_STORAGE_KEY) ?? '{}').presentation).toBe(false);
  });

  it('never renders personal data fields', async () => {
    const { wrapper } = await mountLive();
    await wrapper.get('[data-testid="live-presentation-toggle"]').trigger('click');
    expect(wrapper.html()).not.toMatch(/@|\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/);
  });

  it('refreshes every three seconds', async () => {
    vi.useFakeTimers();
    const { load } = await mountLive();
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(LIVE_REFRESH_MS);
    expect(load).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(LIVE_REFRESH_MS * 2);
    expect(load).toHaveBeenCalledTimes(4);
  });

  it('reloads immediately with the chosen window', async () => {
    const { wrapper, load } = await mountLive();
    await wrapper.get('[data-window="1440"]').trigger('click');
    await flushPromises();
    expect(load).toHaveBeenLastCalledWith(1440);
  });

  it('keeps the data and flags it as stale when a refresh fails', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 9, 14, 10, 30, 15));
    const { wrapper, load } = await mountLive();
    expect(wrapper.get('[data-testid="live-status"]').text()).toContain('10:30:15');
    expect(wrapper.get('[data-testid="live-status"]').classes()).not.toContain('live__status--stale');

    load.mockRejectedValue(new Error('boom'));
    await vi.advanceTimersByTimeAsync(LIVE_REFRESH_MS);
    const status = wrapper.get('[data-testid="live-status"]');
    expect(status.text()).toContain('sin actualizar desde 10:30:15');
    expect(status.classes()).toContain('live__status--stale');
    expect(wrapper.get('[data-testid="live-connected"]').text()).toBe('64');

    load.mockImplementation(async () => payload());
    await vi.advanceTimersByTimeAsync(LIVE_REFRESH_MS);
    expect(wrapper.get('[data-testid="live-status"]').classes()).not.toContain('live__status--stale');
  });

  it('stops polling when unmounted', async () => {
    vi.useFakeTimers();
    const { wrapper, load } = await mountLive();
    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(LIVE_REFRESH_MS * 3);
    expect(load).toHaveBeenCalledTimes(1);
  });

  it('is translated to English', async () => {
    (i18n.global.locale as unknown as { value: string }).value = 'en';
    const { wrapper } = await mountLive();
    expect(wrapper.text()).toContain('Connected now');
    expect(wrapper.text()).toContain('Presentation mode');
  });
});
