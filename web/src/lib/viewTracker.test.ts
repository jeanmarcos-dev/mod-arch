import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createViewTracker,
  isTrackedPath,
  MAX_VIEW_DURATION_MS,
  type PageViewCommand,
} from './viewTracker';

describe('viewTracker', () => {
  let clock: number;
  let sent: PageViewCommand[];
  const send = (c: PageViewCommand) => sent.push(c);
  const now = () => clock;

  beforeEach(() => {
    clock = 1_000;
    sent = [];
  });

  it('reports the previous route with its duration on navigation', () => {
    const t = createViewTracker(send, now);
    t.enter('/procesos');
    clock += 40_213;
    t.enter('/memoria');
    expect(sent).toEqual([{ cmd: 'track', event: 'page_view', name: '/procesos', duration_ms: 40_213 }]);
  });

  it('reports the accumulated time when the tab is hidden', () => {
    const t = createViewTracker(send, now);
    t.enter('/memoria');
    clock += 5_000;
    t.hide();
    expect(sent).toEqual([{ cmd: 'track', event: 'page_view', name: '/memoria', duration_ms: 5_000 }]);
  });

  it('does not count hidden time after returning to the tab', () => {
    const t = createViewTracker(send, now);
    t.enter('/memoria');
    clock += 5_000;
    t.hide();
    clock += 600_000;
    t.show();
    clock += 2_000;
    t.enter('/procesos');
    expect(sent.map((c) => c.duration_ms)).toEqual([5_000, 2_000]);
    expect(sent.map((c) => c.name)).toEqual(['/memoria', '/memoria']);
  });

  it('does not send twice when hidden repeatedly', () => {
    const t = createViewTracker(send, now);
    t.enter('/memoria');
    t.hide();
    t.hide();
    expect(sent).toHaveLength(1);
  });

  it('ignores show before any route is entered', () => {
    const t = createViewTracker(send, now);
    t.show();
    t.hide();
    expect(sent).toHaveLength(0);
  });

  it('clamps the duration to six hours', () => {
    const t = createViewTracker(send, now);
    t.enter('/procesos');
    clock += MAX_VIEW_DURATION_MS * 2;
    t.hide();
    expect(sent[0].duration_ms).toBe(MAX_VIEW_DURATION_MS);
  });

  it('never goes negative when the clock moves backwards', () => {
    const t = createViewTracker(send, now);
    t.enter('/procesos');
    clock -= 500;
    t.hide();
    expect(sent[0].duration_ms).toBe(0);
  });

  it('does not measure admin, login or viewer-token routes', () => {
    const t = createViewTracker(send, now);
    for (const p of ['/admin', '/admin/live', '/login', '/v/abc123']) {
      t.enter(p);
      clock += 1_000;
      t.hide();
    }
    expect(sent).toHaveLength(0);
  });

  it('stops measuring after navigating to an untracked route', () => {
    const t = createViewTracker(send, now);
    t.enter('/procesos');
    clock += 1_000;
    t.enter('/admin');
    clock += 9_000;
    t.show();
    t.hide();
    expect(sent).toHaveLength(1);
    expect(sent[0].name).toBe('/procesos');
  });

  it('validates tracked paths', () => {
    expect(isTrackedPath('/')).toBe(true);
    expect(isTrackedPath('/concurrencia')).toBe(true);
    expect(isTrackedPath('/adminx')).toBe(true);
    expect(isTrackedPath('/admin/usuarios')).toBe(false);
    expect(isTrackedPath('/login')).toBe(false);
    expect(isTrackedPath('/v/token')).toBe(false);
    expect(isTrackedPath('sin-barra')).toBe(false);
    expect(isTrackedPath(`/${'a'.repeat(100)}`)).toBe(false);
  });

  it('keeps the injected sender decoupled from the socket', () => {
    const spy = vi.fn();
    const t = createViewTracker(spy, now);
    t.enter('/x86');
    t.hide();
    expect(spy).toHaveBeenCalledTimes(1);
  });
});
