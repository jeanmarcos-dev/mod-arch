// Build the "owners of memory" list rendered in the memory views. The
// simulator has a single global RAM (kernel.Mem.Bytes on the Go side); the
// real owners are the kernel and each live process. Cores are not owners —
// they are executors that, at this instant, happen to be reading from some
// address.
//
// The helper centralizes that crosswalk so the page view, the simulator
// panel and any future memory consumer all describe ownership the same
// way.

import type { CoreInfo, PCBInfo } from '../stores/cpu';
import { MEMORY_REGIONS } from './memoryLayout';

export type OwnerKind = 'kernel' | 'process';

/** Cap mirrored from ProcessesPanel.vue so both panes indent the same way. */
export const OWNER_DEPTH_CAP = 4;

/** Per-task user stack size on the Go side (pkg/kernel/kernel.go: ProcStackSize). */
export const PROC_STACK_SIZE = 0x40000;

export interface ThreadInfo {
  /** PID of the thread (its kernel-side task id). */
  pid: number;
  /** Optional tid surfaced by the backend; falls back to pid when absent. */
  tid?: number;
  name: string;
  state: string;
  /** Core currently executing this thread, if any. */
  runningOnCore?: number;
  /** Thread's own stack top (CLONE_THREAD threads share code with the leader
   *  but get a separate stack slot — see pkg/kernel/clone.go:220). */
  stackTop: number;
}

export interface MemoryOwner {
  kind: OwnerKind;
  /** Stable id for v-for keys. 'kernel' or `pid-<n>`. */
  id: string;
  /** PID for processes; undefined for the kernel. */
  pid?: number;
  /** Display name. For the kernel it is just the literal 'kernel'. */
  name: string;
  /** Inclusive start of the owner's primary code range. */
  codeBase: number;
  /** Length in bytes of the code range. */
  codeSize: number;
  /** Top-of-stack pointer (highest address; stack grows down). */
  stackTop: number;
  /** Process state for chip styling (running/ready/blocked/...); kernel = undefined. */
  state?: string;
  /** Core id currently executing this owner's PID, if any. Kernel owners
   *  surface the lowest core running in kernel mode for the same hint. */
  runningOnCore?: number;
  /** Thread group leader pid; undefined for the kernel and stand-alone procs. */
  tgid?: number;
  /** Parent pid (fork tree). 0 / undefined means top-level (init or kernel). */
  parentPid?: number;
  /** Depth in the fork tree, clamped to OWNER_DEPTH_CAP. */
  depth: number;
  /** Real depth before clamping; used by the chip to surface a `↳dN` badge
   *  when the tree is too deep for a clean indent. */
  realDepth: number;
  /** Threads that share this owner's address space (CLONE_THREAD). The
   *  leader's own task is NOT included. Empty for non-leaders and procs
   *  without children threads. */
  threads: ThreadInfo[];
}

const KERNEL_REGION = MEMORY_REGIONS.find(r => r.name === 'kernel');

function isThreadOf(p: PCBInfo): boolean {
  return p.tgid !== undefined && p.tgid !== p.pid;
}

/**
 * Build the ordered list of memory owners. Layout:
 *
 *   [ kernel,                            // always first
 *     ...leaders in fork-tree DFS order  // threads collapsed inside leaders
 *   ]
 *
 * Threads (tgid !== pid) are NOT emitted as separate owners — they share
 * the leader's code/stack so duplicating the entry would mislead. Each
 * leader instead carries a `threads[]` list which the chip renders as a
 * collapsible "HILOS (n)" section.
 */
export function buildOwners(procs: PCBInfo[], cores: CoreInfo[]): MemoryOwner[] {
  // Swappers (one per core, all sharing PID 0 — see idle_task.go) are part
  // of the kernel band, not standalone owners. They also have ParentPID=0,
  // which would put them in byParent[0] AND make them kids of themselves,
  // turning the DFS below into infinite recursion.
  procs = procs.filter(p => !p.is_idle);

  const owners: MemoryOwner[] = [];

  // Find the first core running in kernel mode; surface it on the kernel chip
  // so the student sees "kernel is on Cx right now" the same way a process
  // chip says it.
  const kernelCore = cores.find(c => c.mode !== 3 && !c.halted);
  owners.push({
    kind: 'kernel',
    id: 'kernel',
    name: 'kernel',
    codeBase: KERNEL_REGION?.start ?? 0x100000,
    codeSize: KERNEL_REGION ? KERNEL_REGION.end - KERNEL_REGION.start + 1 : 0x100000,
    stackTop: KERNEL_REGION?.end ?? 0x1FFFFF,
    runningOnCore: kernelCore?.id,
    depth: 0,
    realDepth: 0,
    threads: [],
  });

  if (procs.length === 0) return owners;

  // Split tasks into leaders (own their address space) and threads (share
  // with a leader via tgid). Stand-alone processes count as their own leader.
  const leaders: PCBInfo[] = [];
  const threadsByTgid = new Map<number, PCBInfo[]>();
  for (const p of procs) {
    if (isThreadOf(p)) {
      const tgid = p.tgid as number;
      const list = threadsByTgid.get(tgid);
      if (list) list.push(p);
      else threadsByTgid.set(tgid, [p]);
    } else {
      leaders.push(p);
    }
  }
  for (const list of threadsByTgid.values()) list.sort((a, b) => a.pid - b.pid);

  // Group leaders by parent_pid so a single DFS walk emits them in fork
  // order. parent_pid === 0 means "top level" (typically init).
  const byParent = new Map<number, PCBInfo[]>();
  for (const l of leaders) {
    const pp = l.parent_pid ?? 0;
    const list = byParent.get(pp);
    if (list) list.push(l);
    else byParent.set(pp, [l]);
  }
  for (const list of byParent.values()) list.sort((a, b) => a.pid - b.pid);

  const visit = (parent: number, depth: number) => {
    const kids = byParent.get(parent);
    if (!kids) return;
    for (const k of kids) {
      const runningCore = cores.find(c => c.current_pid === k.pid && !c.halted);
      const threadList = threadsByTgid.get(k.pid) ?? [];
      const threads: ThreadInfo[] = threadList.map(t => {
        const tcore = cores.find(c => c.current_pid === t.pid && !c.halted);
        return {
          pid: t.pid,
          tid: t.tid,
          name: t.name,
          state: t.state,
          runningOnCore: tcore?.id,
          stackTop: t.stack_top,
        };
      });
      owners.push({
        kind: 'process',
        id: `pid-${k.pid}`,
        pid: k.pid,
        name: k.name,
        codeBase: k.code_base,
        codeSize: k.code_size,
        stackTop: k.stack_top,
        state: k.state,
        runningOnCore: runningCore?.id,
        tgid: k.tgid,
        parentPid: k.parent_pid,
        depth: Math.min(depth, OWNER_DEPTH_CAP),
        realDepth: depth,
        threads,
      });
      visit(k.pid, depth + 1);
    }
  };
  visit(0, 0);

  // Safety net: any leader whose parent_pid points at a non-existent task
  // (snapshot races, hidden ancestors) would never be visited above. Emit
  // it at depth 0 so it stays visible.
  const emittedIds = new Set(owners.filter(o => o.kind === 'process').map(o => o.pid));
  for (const l of leaders) {
    if (emittedIds.has(l.pid)) continue;
    const runningCore = cores.find(c => c.current_pid === l.pid && !c.halted);
    const threadList = threadsByTgid.get(l.pid) ?? [];
    const threads: ThreadInfo[] = threadList.map(t => {
      const tcore = cores.find(c => c.current_pid === t.pid && !c.halted);
      return {
        pid: t.pid,
        tid: t.tid,
        name: t.name,
        state: t.state,
        runningOnCore: tcore?.id,
        stackTop: t.stack_top,
      };
    });
    owners.push({
      kind: 'process',
      id: `pid-${l.pid}`,
      pid: l.pid,
      name: l.name,
      codeBase: l.code_base,
      codeSize: l.code_size,
      stackTop: l.stack_top,
      state: l.state,
      runningOnCore: runningCore?.id,
      tgid: l.tgid,
      parentPid: l.parent_pid,
      depth: 0,
      realDepth: 0,
      threads,
    });
  }

  return owners;
}

/**
 * Markers describing where each core is looking inside the global memory.
 * The page/panel uses these to paint floating chips next to the right cells
 * of the hex grid. The marker carries the executing PID for context so the
 * UI can show "C0 · PID 3" instead of just "C0".
 */
export interface CoreMarker {
  coreId: number;
  kind: 'EIP' | 'ESP';
  addr: number;
  pid: number;
}

/**
 * Resolve the owner of a single byte. `null` if the address falls in the
 * reserved region or outside any known task. Used by the hex grid popover
 * so the student sees "kernel" vs "P3 stack" instead of having to map
 * ranges in their head.
 */
export interface AddrOwner {
  /** PID of the task that owns this byte; undefined for the kernel. */
  pid?: number;
  /** Display name (kernel, init, worker, ...). */
  name: string;
  /** Whether the byte sits in the code segment or in the stack slot. */
  role: 'code' | 'stack' | 'kernel';
  /** Leader pid when the byte is the stack of a CLONE_THREAD thread. */
  leaderPid?: number;
  /** Threads sharing this code range (only set when role === 'code' and the
   *  owner is a leader with one or more CLONE_THREAD children). */
  sharedWithPids?: number[];
}

export function ownerAt(addr: number, owners: MemoryOwner[]): AddrOwner | null {
  for (const o of owners) {
    if (o.kind === 'kernel') {
      if (addr >= o.codeBase && addr < o.codeBase + o.codeSize) {
        return { name: o.name, role: 'kernel' };
      }
      continue;
    }
    if (addr >= o.codeBase && addr < o.codeBase + o.codeSize) {
      return {
        pid: o.pid,
        name: o.name,
        role: 'code',
        sharedWithPids: o.threads.length > 0 ? o.threads.map(t => t.pid) : undefined,
      };
    }
    // Stack range: stack_top is the highest valid byte + 0x10 (see
    // pkg/kernel/kernel.go:297 — stackBase = stackTop + 0x10 - ProcStackSize).
    const stackEnd = o.stackTop + 0x10;
    const stackBase = stackEnd - PROC_STACK_SIZE;
    if (addr >= stackBase && addr < stackEnd) {
      return { pid: o.pid, name: o.name, role: 'stack' };
    }
    // CLONE_THREAD threads have their own stack slot.
    for (const t of o.threads) {
      const tEnd = t.stackTop + 0x10;
      const tBase = tEnd - PROC_STACK_SIZE;
      if (addr >= tBase && addr < tEnd) {
        return { pid: t.pid, name: t.name, role: 'stack', leaderPid: o.pid };
      }
    }
  }
  return null;
}

export function buildCoreMarkers(cores: CoreInfo[], visibleIds: Set<number>): CoreMarker[] {
  const out: CoreMarker[] = [];
  for (const c of cores) {
    if (!visibleIds.has(c.id)) continue;
    if (c.halted) continue;
    const eip = (c.regs.EIP ?? 0) >>> 0;
    const esp = (c.regs.ESP ?? 0) >>> 0;
    out.push({ coreId: c.id, kind: 'EIP', addr: eip, pid: c.current_pid });
    out.push({ coreId: c.id, kind: 'ESP', addr: esp, pid: c.current_pid });
  }
  return out;
}
