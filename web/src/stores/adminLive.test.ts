import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { normalizeLive, useAdminStore, type LiveStats } from './admin';

function payload(overrides: Partial<LiveStats> = {}): LiveStats {
  return {
    generated_at: 1,
    window_minutes: 120,
    now: {
      sessions_active: 3, ws_connections: 4, sessions_with_user: 2, sessions_anonymous: 1,
      peak_sessions: 5, sessions_created_total: 9, sections: [], demos: [],
    },
    window: { unique_users: 2, unique_sessions: 3, page_views: [], demos: [], activity_per_minute: [] },
    tutor: {
      questions: 1, askers: 1, latency_p50_ms: 10, latency_p95_ms: 20, degraded: 0,
      thumbs_up: 0, thumbs_down: 0, top_topics: [], recent: [],
    },
    countries: [],
    ...overrides,
  };
}

describe('admin store — live', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.restoreAllMocks();
  });

  it('requests /admin/api/live with the window and stores the payload', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => payload() });
    vi.stubGlobal('fetch', fetchMock);
    const admin = useAdminStore();
    await admin.loadLive(30);
    expect(fetchMock.mock.calls[0][0]).toBe('/admin/api/live?window=30');
    expect(admin.live?.now.sessions_active).toBe(3);
  });

  it('turns null lists from the server into empty arrays', () => {
    const raw = payload();
    (raw.now as unknown as { sections: unknown }).sections = null;
    (raw.window as unknown as { page_views: unknown }).page_views = null;
    (raw.tutor as unknown as { recent: unknown }).recent = null;
    (raw as unknown as { countries: unknown }).countries = null;
    const out = normalizeLive(raw);
    expect(out.now.sections).toEqual([]);
    expect(out.window.page_views).toEqual([]);
    expect(out.tutor.recent).toEqual([]);
    expect(out.countries).toEqual([]);
  });

  it('keeps the previous data when a refresh fails', async () => {
    const admin = useAdminStore();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => payload() }));
    await admin.loadLive();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 404, json: async () => ({ error: 'nope' }) }));
    await expect(admin.loadLive()).rejects.toThrow();
    expect(admin.live?.now.sessions_active).toBe(3);
  });
});
