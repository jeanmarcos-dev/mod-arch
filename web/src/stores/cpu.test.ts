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

describe('cpu store — wake and init_reap events', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('updates readyQueue from EventWake (BLOCKED→READY transition)', () => {
    const cpu = useCpuStore();
    expect(cpu.readyQueue).toEqual([]);
    cpu.__applyEvent({
      type: 'wake',
      pid: 1,
      from_reason: 'wait(any)',
      ready_queue: [1],
    } as any);
    expect(cpu.readyQueue).toEqual([1]);
  });

  it('removes reaped children from procs when EventInitReap fires', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({
      type: 'loaded',
      name: 'demo',
      pids: [1, 2],
      procs: [
        { pid: 1, name: 'init', state: 'BLOCKED', parent_pid: 0, regs: {} },
        { pid: 2, name: 'add', state: 'ZOMBIE', parent_pid: 1, regs: {}, exit_code: 42 },
      ],
      sources: {},
    } as any);

    cpu.__applyEvent({
      type: 'init_reap',
      init_pid: 1,
      reaps: [{ child_pid: 2, exit_code: 42, reparented: false }],
      ready_queue: [],
    } as any);

    expect(cpu.procs.find(x => x.pid === 2)).toBeUndefined();
    expect(cpu.readyQueue).toEqual([]);
  });
});
