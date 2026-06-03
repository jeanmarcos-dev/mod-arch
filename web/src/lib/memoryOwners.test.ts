import { describe, it, expect } from 'vitest';
import { buildOwners } from './memoryOwners';
import type { CoreInfo, PCBInfo } from '../stores/cpu';

function pcb(p: Partial<PCBInfo> & { pid: number; name: string }): PCBInfo {
  return {
    pid: p.pid,
    name: p.name,
    state: p.state ?? 'READY',
    regs: {},
    saved_user_esp: 0,
    code_base: p.code_base ?? 0,
    code_size: p.code_size ?? 0,
    stack_top: p.stack_top ?? 0,
    parent_pid: p.parent_pid,
    tgid: p.tgid,
    tid: p.tid,
    is_idle: p.is_idle,
  };
}

function core(id: number): CoreInfo {
  return {
    id,
    regs: {},
    segments: {},
    mode: 3,
    halted: true,
    tick: 0,
    current_pid: 0,
  };
}

describe('buildOwners', () => {
  // Reproduces a RangeError that hits the /memory panel on the first load /
  // demo load: every core spawns a swapper PCB with PID=0 and ParentPID=0
  // (idle_task.go:58-68 — Linux's "PID 0 is reserved for the swapper" rule).
  // buildOwners groups leaders by parent_pid, then DFS-recurses via
  // visit(k.pid, ...); a swapper sits in byParent[0] *and* is itself a kid
  // of 0, so the recursion never reaches a base case → stack overflow.
  it('does not stack-overflow when swappers (PID 0, ParentPID 0) are present', () => {
    const cores = [core(0), core(1)];
    const procs: PCBInfo[] = [
      pcb({ pid: 0, name: 'swapper/0', parent_pid: 0, is_idle: true }),
      pcb({ pid: 0, name: 'swapper/1', parent_pid: 0, is_idle: true }),
      pcb({ pid: 1, name: 'init', parent_pid: 0 }),
      pcb({ pid: 4, name: 'badmem', parent_pid: 1 }),
    ];

    const owners = buildOwners(procs, cores);

    // Sanity: the kernel chip is always first, then the real user-visible
    // processes. Swappers belong to the kernel band, not the owners list.
    expect(owners[0]?.kind).toBe('kernel');
    const pids = owners.filter(o => o.kind === 'process').map(o => o.pid);
    expect(pids).toEqual(expect.arrayContaining([1, 4]));
    expect(pids).not.toContain(0);
  });
});
