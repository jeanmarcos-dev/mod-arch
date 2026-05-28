import { beforeEach, describe, expect, it } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useCpuStore } from './cpu';

describe('cpu store — paused flag', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('defaults to paused=false', () => {
    const cpu = useCpuStore();
    expect(cpu.paused).toBe(false);
  });

  it('reflects state events that include paused', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({ type: 'state', running: false, paused: true });
    expect(cpu.paused).toBe(true);
    cpu.__applyEvent({ type: 'state', running: true, paused: false });
    expect(cpu.paused).toBe(false);
  });

  it('leaves paused untouched when the state event omits it', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({ type: 'state', running: false, paused: true });
    cpu.__applyEvent({ type: 'state', halted: true });
    expect(cpu.paused).toBe(true);
  });

  it('clears paused on reset', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({ type: 'state', running: false, paused: true });
    expect(cpu.paused).toBe(true);
    cpu.__applyEvent({ type: 'reset' });
    expect(cpu.paused).toBe(false);
  });
});
