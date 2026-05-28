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
  /** Thread group leader pid for clone-thread children; undefined for processes. */
  tgid?: number;
  /** When true the owner is a thread sharing memory with its tgid leader. */
  isThread?: boolean;
}

const KERNEL_REGION = MEMORY_REGIONS.find(r => r.name === 'kernel');

/**
 * Build [kernel, ...processes] enriched with the core that is currently
 * running each process (if any). Processes are returned in pid order so the
 * list does not visually shuffle between snapshots.
 */
export function buildOwners(procs: PCBInfo[], cores: CoreInfo[]): MemoryOwner[] {
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
  });

  const sorted = [...procs].sort((a, b) => a.pid - b.pid);
  for (const p of sorted) {
    const core = cores.find(c => c.current_pid === p.pid && !c.halted);
    const isThread = p.tgid !== undefined && p.tgid !== p.pid;
    owners.push({
      kind: 'process',
      id: `pid-${p.pid}`,
      pid: p.pid,
      name: p.name,
      codeBase: p.code_base,
      codeSize: p.code_size,
      stackTop: p.stack_top,
      state: p.state,
      runningOnCore: core?.id,
      tgid: p.tgid,
      isThread,
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
