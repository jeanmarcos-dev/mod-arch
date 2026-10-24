import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import AdminOverview from './AdminOverview.vue';
import { useAdminStore, type AdminStats, type SessionHistoryPage } from '../../stores/admin';
import { i18n } from '../../i18n';

const stats: AdminStats = { full: 1, privileged: 0, max_public: 10, max_privileged: 2, total_created: 5, peak: 3 };

function mountOverview() {
  const admin = useAdminStore();
  admin.stats = stats;
  admin.metrics = {
    active_ws_connections: 12,
    total_ws_connections: 40,
    llm_queries: 7,
    llm_errors: 0,
    avg_llm_latency_ms: 2900.4,
  };
  vi.spyOn(admin, 'loadSessionHistory').mockResolvedValue({ rows: [], total: 0 } as unknown as SessionHistoryPage);
  return mount(AdminOverview, { global: { stubs: { RouterLink: true } } });
}

describe('AdminOverview', () => {
  beforeEach(() => {
    (i18n.global.locale as unknown as { value: string }).value = 'es';
  });

  it('translates the metric labels', async () => {
    const w = mountOverview();
    await flushPromises();
    expect(w.text()).toContain('Conexiones WS');
    expect(w.text()).toContain('Latencia del LLM (ms)');
    expect(w.text()).not.toContain('WS Connections');
    expect(w.text()).not.toContain('LLM Latency (ms)');
  });

  it.each([
    ['en', 'WS connections', 'LLM latency (ms)'],
    ['pt', 'Conexões WS', 'Latência do LLM (ms)'],
  ])('uses the %s catalog', async (locale, ws, llm) => {
    (i18n.global.locale as unknown as { value: string }).value = locale;
    const w = mountOverview();
    await flushPromises();
    expect(w.text()).toContain(ws);
    expect(w.text()).toContain(llm);
  });

  it('reflects metrics that change between refresh cycles', async () => {
    const w = mountOverview();
    const admin = useAdminStore();
    await flushPromises();
    expect(w.text()).toContain('12');
    admin.metrics = { ...admin.metrics!, active_ws_connections: 31 };
    await flushPromises();
    expect(w.text()).toContain('31');
  });
});
