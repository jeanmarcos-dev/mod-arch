import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useAdminStore } from './admin';

function respond(map: Record<string, unknown>) {
  return vi.fn(async (url: string) => {
    const path = url.split('?')[0];
    if (!(path in map)) return { ok: false, status: 404, json: async () => ({ error: 'nf' }) };
    return { ok: true, status: 200, json: async () => map[path] };
  });
}

const me = { user: 'root', role: 'admin', via_env: true, stats: { full: 0, privileged: 0, max_public: 1, max_privileged: 1, total_created: 0, peak: 0 } };

describe('admin store — refresh cycle', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('pulls fresh telemetry metrics on every refresh', async () => {
    const metric = (n: number) => ({ active_ws_connections: n, total_ws_connections: n, llm_queries: 0, llm_errors: 0, avg_llm_latency_ms: 0 });
    const admin = useAdminStore();
    vi.stubGlobal('fetch', respond({ '/admin/api/me': me, '/admin/api/metrics': metric(4) }));
    await admin.refresh();
    expect(admin.metrics?.active_ws_connections).toBe(4);
    vi.stubGlobal('fetch', respond({ '/admin/api/me': me, '/admin/api/metrics': metric(9) }));
    await admin.refresh();
    expect(admin.metrics?.active_ws_connections).toBe(9);
  });

  it('keeps the last metrics when the endpoint is not available', async () => {
    const admin = useAdminStore();
    vi.stubGlobal('fetch', respond({ '/admin/api/me': me, '/admin/api/metrics': { active_ws_connections: 4, total_ws_connections: 4, llm_queries: 0, llm_errors: 0, avg_llm_latency_ms: 0 } }));
    await admin.refresh();
    vi.stubGlobal('fetch', respond({ '/admin/api/me': me }));
    await admin.refresh();
    expect(admin.metrics?.active_ws_connections).toBe(4);
  });
});
