import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { DEFAULT_ROUTE_NAME, useAppConfigStore } from './appConfig';

const originalFetch = globalThis.fetch;

function mockFetchOnce(body: unknown, ok = true, status = 200) {
  globalThis.fetch = vi.fn(() =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        statusText: ok ? 'OK' : 'Bad',
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  ) as typeof globalThis.fetch;
}

describe('appConfig store', () => {
  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  beforeEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('parses every disabled-set and default route from /api/config', async () => {
    mockFetchOnce({
      turnstile_sitekey: 'key',
      disabled_routes: ['processes', 'memory'],
      disabled_panels: ['events'],
      disabled_demos: ['race_counter'],
      default_route: 'memory',
    });
    const cfg = useAppConfigStore();
    await cfg.load();
    expect(cfg.turnstileSitekey).toBe('key');
    expect(cfg.disabledRoutes.has('processes')).toBe(true);
    expect(cfg.disabledRoutes.has('memory')).toBe(true);
    expect(cfg.disabledPanels.has('events')).toBe(true);
    expect(cfg.disabledDemos.has('race_counter')).toBe(true);
    expect(cfg.defaultRoute).toBe('memory');
  });

  it('falls back to defaults when fields are missing', async () => {
    mockFetchOnce({ turnstile_sitekey: '' });
    const cfg = useAppConfigStore();
    await cfg.load();
    expect(cfg.disabledRoutes.size).toBe(0);
    expect(cfg.disabledPanels.size).toBe(0);
    expect(cfg.disabledDemos.size).toBe(0);
    expect(cfg.defaultRoute).toBe(DEFAULT_ROUTE_NAME);
  });

  it('ignores non-string entries in arrays', async () => {
    mockFetchOnce({
      turnstile_sitekey: '',
      disabled_routes: ['memory', 123, null, 'processes'],
      default_route: '',
    });
    const cfg = useAppConfigStore();
    await cfg.load();
    expect(cfg.disabledRoutes.size).toBe(2);
    expect(cfg.disabledRoutes.has('memory')).toBe(true);
    expect(cfg.disabledRoutes.has('processes')).toBe(true);
    expect(cfg.defaultRoute).toBe(DEFAULT_ROUTE_NAME);
  });

  it('does not re-fetch when already loaded', async () => {
    const fetchSpy = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ turnstile_sitekey: '' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    );
    globalThis.fetch = fetchSpy as typeof globalThis.fetch;
    const cfg = useAppConfigStore();
    await cfg.load();
    await cfg.load();
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it('honors force-reload', async () => {
    const fetchSpy = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ turnstile_sitekey: '' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    );
    globalThis.fetch = fetchSpy as typeof globalThis.fetch;
    const cfg = useAppConfigStore();
    await cfg.load();
    await cfg.load(true);
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });
});
