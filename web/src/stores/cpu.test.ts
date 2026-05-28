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

describe('cpu store — exit_code propagation', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('updates proc.exit_code when process_state carries one', () => {
    const cpu = useCpuStore();
    // Seed a proc as if it had just been loaded.
    cpu.__applyEvent({
      type: 'loaded',
      name: 'add',
      pids: [2],
      procs: [{ pid: 2, name: 'add', state: 'READY', parent_pid: 1, regs: {} } as any],
      sources: {},
    } as any);

    cpu.__applyEvent({
      type: 'process_state',
      pid: 2,
      state: 'ZOMBIE',
      reason: 'exit',
      exit_code: 42,
    } as any);

    const p = cpu.procs.find(x => x.pid === 2);
    expect(p?.state).toBe('ZOMBIE');
    expect(p?.exit_code).toBe(42);
  });

  it('does not overwrite exit_code when process_state omits it', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({
      type: 'loaded',
      name: 'demo',
      pids: [2],
      procs: [{ pid: 2, name: 'demo', state: 'READY', parent_pid: 1, regs: {}, exit_code: 7 } as any],
      sources: {},
    } as any);

    cpu.__applyEvent({ type: 'process_state', pid: 2, state: 'BLOCKED', reason: 'keyboard' } as any);

    const p = cpu.procs.find(x => x.pid === 2);
    expect(p?.exit_code).toBe(7);
  });
});
