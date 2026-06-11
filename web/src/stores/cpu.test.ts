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

describe('cpu store — idle task (PID 0)', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('keeps is_idle and running_idle from the snapshot payload', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({
      type: 'snapshot',
      regs: {}, segments: {}, mode: 0, halted: false, tick: 0,
      procs: [
        { pid: 0, name: 'swapper', state: 'IDLE', regs: {}, is_idle: true },
        { pid: 1, name: 'init', state: 'BLOCKED', regs: {} },
      ],
      cores: [
        { id: 0, regs: {}, segments: {}, mode: 0, halted: true, tick: 0, current_pid: 0, running_idle: true },
      ],
    } as any);

    const swapper = cpu.procs.find(p => p.pid === 0);
    expect(swapper?.is_idle).toBe(true);
    const core0 = cpu.cores.find(c => c.id === 0);
    expect(core0?.running_idle).toBe(true);
  });

  it('flips running_idle on the right core when core_idle fires', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({
      type: 'snapshot',
      regs: {}, segments: {}, mode: 0, halted: false, tick: 0,
      procs: [],
      cores: [
        { id: 0, regs: {}, segments: {}, mode: 0, halted: false, tick: 0, current_pid: 0, running_idle: false },
        { id: 1, regs: {}, segments: {}, mode: 0, halted: false, tick: 0, current_pid: 0, running_idle: false },
      ],
    } as any);

    cpu.__applyEvent({ type: 'core_idle', core_id: 1, tick: 5 } as any);

    expect(cpu.cores.find(c => c.id === 0)?.running_idle).toBe(false);
    expect(cpu.cores.find(c => c.id === 1)?.running_idle).toBe(true);
  });

  it('clears running_idle when a real proc takes the core', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({
      type: 'snapshot',
      regs: {}, segments: {}, mode: 0, halted: false, tick: 0,
      procs: [],
      cores: [
        { id: 0, regs: {}, segments: {}, mode: 0, halted: false, tick: 0, current_pid: 0, running_idle: true },
      ],
    } as any);

    cpu.__applyEvent({ type: 'core_assign', core_id: 0, pid: 7 } as any);

    expect(cpu.cores.find(c => c.id === 0)?.running_idle).toBe(false);
  });

  it('idleTaskEnabled mirrors the presence of PID 0 in procs', () => {
    const cpu = useCpuStore();
    expect(cpu.idleTaskEnabled).toBe(false);

    cpu.__applyEvent({
      type: 'procs',
      procs: [
        { pid: 0, name: 'swapper', state: 'IDLE', regs: {}, is_idle: true },
        { pid: 1, name: 'init', state: 'BLOCKED', regs: {} },
      ],
    } as any);
    expect(cpu.idleTaskEnabled).toBe(true);

    cpu.__applyEvent({
      type: 'procs',
      procs: [
        { pid: 1, name: 'init', state: 'BLOCKED', regs: {} },
      ],
    } as any);
    expect(cpu.idleTaskEnabled).toBe(false);
  });
});

describe('cpu store — halt event formatting', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('formats a swapper HLT (pid=0) as "HLT idle" with cyan color and leaves machine.halted untouched', () => {
    const cpu = useCpuStore();
    const before = cpu.log.length;
    cpu.__applyEvent({ type: 'halt', core_id: 0, pid: 0, tick: 5 } as any);
    const entry = cpu.log[cpu.log.length - 1];
    expect(cpu.log.length).toBe(before + 1);
    expect(entry.text).toContain('HLT idle');
    expect(entry.text).not.toContain('HALT (');
    expect(entry.color).toBe('cyan');
    expect(cpu.halted).toBe(false);
  });

  it('formats a process-death HLT (pid>0) as "HALT (Pn)" with red color and freezes halted', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({ type: 'halt', core_id: 0, pid: 3, tick: 9 } as any);
    const entry = cpu.log[cpu.log.length - 1];
    expect(entry.text).toContain('HALT (P3)');
    expect(entry.text).not.toContain('HLT idle');
    expect(entry.color).toBe('red');
    expect(cpu.halted).toBe(true);
  });
});

describe('cpu store — event atomicity roles', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('tags the dispatch sequence as one primary headline plus details', () => {
    const cpu = useCpuStore();
    cpu.__applyEvent({
      type: 'loaded',
      name: 'demo',
      pids: [5],
      procs: [{ pid: 5, name: 'demo', state: 'READY', parent_pid: 1, regs: {} }],
      sources: {},
    } as any);
    const before = cpu.log.length;
    // A dispatch tick: sched_decision is the atomic action; pcb_restore and
    // core_assign elaborate it.
    cpu.__applyEvent({ type: 'sched_decision', core_id: 0, tick: 0, from_pid: 0, to_pid: 5, reason: 'dispatch', ready_queue: [] } as any);
    cpu.__applyEvent({ type: 'pcb_restore', core_id: 0, tick: 0, pid: 5, regs: { EIP: 0x200000 } } as any);
    cpu.__applyEvent({ type: 'core_assign', core_id: 0, tick: 0, pid: 5 } as any);

    const added = cpu.log.slice(before);
    const byTag = (tag: string) => added.find(e => e.tag === tag);
    expect(byTag('sched_decision')?.role).toBe('primary');
    expect(byTag('pcb_restore')?.role).toBe('detail');
    expect(byTag('core_assign')?.role).toBe('detail');
    // Exactly one primary lands on this (tick, core).
    const primaries = added.filter(e => e.coreId === 0 && e.tick === 0 && e.role === 'primary');
    expect(primaries).toHaveLength(1);
  });

  it('marks a plain instruction as primary and does not duplicate it', () => {
    const cpu = useCpuStore();
    const before = cpu.log.length;
    cpu.__applyEvent({ type: 'instruction', core_id: 1, tick: 3, eip: 0x200005, mnemonic: 'MOV EBX, 0x20', bytes: [0, 0, 0, 0, 0] } as any);
    const added = cpu.log.slice(before).filter(e => e.tick === 3 && e.coreId === 1);
    expect(added).toHaveLength(1);
    expect(added[0].role).toBe('primary');
    expect(added[0].tag).toBe('instruction');
  });
});
