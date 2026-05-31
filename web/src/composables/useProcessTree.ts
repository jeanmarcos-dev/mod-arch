import { computed, type ComputedRef } from 'vue';
import { useCpuStore, type PCBInfo } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';

export interface ProcTreeNode {
  proc: PCBInfo;
  depth: number;
  realDepth: number;
  isThread: boolean;
}

export interface UseProcessTreeOptions {
  depthCap?: number;
}

export interface UseProcessTreeResult {
  treeNodes: ComputedRef<ProcTreeNode[]>;
  initPID: ComputedRef<number>;
  // idleProcs are the per-core swappers (PID 0, one task per core in our
  // Linux-style model). Pulled out of the regular tree so the panel can
  // render them in a dedicated compact band instead of N top-level nodes.
  idleProcs: ComputedRef<PCBInfo[]>;
  treeParentOf: (p: PCBInfo) => number;
  isThreadOf: (p: PCBInfo) => boolean;
}

// Threads (tgid != pid) collapse under their leader; processes fall back to
// parent_pid so the tree stays hierarchical in both fork-only and threads
// demos.
function treeParentOf(p: PCBInfo): number {
  const tgid = p.tgid ?? p.pid;
  if (tgid !== p.pid) return tgid;
  return p.parent_pid ?? 0;
}

function isThreadOf(p: PCBInfo): boolean {
  const tgid = p.tgid ?? p.pid;
  return tgid !== p.pid;
}

export function useProcessTree(opts: UseProcessTreeOptions = {}): UseProcessTreeResult {
  const cpu = useCpuStore();
  const prefs = usePreferencesStore();
  const cap = opts.depthCap ?? Number.POSITIVE_INFINITY;

  const initPID = computed(() => {
    const p = cpu.procs.find(x => x.name === 'init');
    return p?.pid ?? 0;
  });

  const idleProcs = computed<PCBInfo[]>(() => {
    return cpu.procs.filter(p => p.is_idle === true);
  });

  const treeNodes = computed<ProcTreeNode[]>(() => {
    const hideInit = !prefs.showInit && initPID.value > 0;
    // showInit OFF strips the kernel-side machinery (idle + init + kthreads).
    const visibleProcs = hideInit
      ? cpu.procs.filter(p =>
          p.is_idle !== true &&
          p.pid !== initPID.value &&
          p.is_kthread !== true,
        )
      : cpu.procs.slice();

    // Swappers all share PID 0 so we key them by TID (negative sentinel).
    const keyOf = (p: PCBInfo) => p.is_idle === true ? `t${p.tid ?? 0}` : `p${p.pid}`;

    // swapper/0 (lowest core) acts as init_task: visual root of the tree.
    const swapperCoreOf = (p: PCBInfo): number => {
      const m = /^swapper\/(\d+)$/.exec(p.name ?? '');
      return m ? parseInt(m[1], 10) : Number.POSITIVE_INFINITY;
    };
    let rootSwapper: PCBInfo | null = null;
    if (!hideInit) {
      const swappers = visibleProcs.filter(p => p.is_idle === true);
      swappers.sort((a, b) => swapperCoreOf(a) - swapperCoreOf(b));
      rootSwapper = swappers[0] ?? null;
    }
    const ROOT = '__root__';

    // Threads collapse under their leader. Every swapper is a sibling at
    // ROOT; init and kthreadd hang from swapper/0 (Linux init_task lineage).
    const parentKeyOf = (p: PCBInfo): string => {
      const tgid = p.tgid ?? p.pid;
      if (tgid !== p.pid) return `p${tgid}`;
      if (p.is_idle === true) return ROOT;
      if (rootSwapper && (p.parent_pid ?? 0) === 0) return keyOf(rootSwapper);
      let pp = p.parent_pid ?? 0;
      if (hideInit && pp === initPID.value) pp = 0;
      return `p${pp}`;
    };

    const byParent = new Map<string, PCBInfo[]>();
    for (const p of visibleProcs) {
      const pk = parentKeyOf(p);
      if (!byParent.has(pk)) byParent.set(pk, []);
      byParent.get(pk)!.push(p);
    }
    for (const list of byParent.values()) {
      list.sort((a, b) => {
        if (a.pid !== b.pid) return a.pid - b.pid;
        return (a.tid ?? 0) - (b.tid ?? 0);
      });
    }

    const out: ProcTreeNode[] = [];
    const visited = new Set<string>();
    const visit = (parent: string, depth: number) => {
      const kids = byParent.get(parent);
      if (!kids) return;
      for (const k of kids) {
        const kkey = keyOf(k);
        if (visited.has(kkey)) continue;
        visited.add(kkey);
        out.push({
          proc: k,
          depth: Math.min(depth, cap),
          realDepth: depth,
          isThread: isThreadOf(k),
        });
        visit(kkey, depth + 1);
      }
    };
    if (rootSwapper) {
      visit(ROOT, 0);
    } else {
      visit('p0', 0);
    }
    // Mid-reload fallback: unreachable procs append at depth 0.
    if (out.length < visibleProcs.length) {
      const seen = new Set(out.map(n => keyOf(n.proc)));
      for (const p of visibleProcs) {
        if (!seen.has(keyOf(p))) {
          out.push({ proc: p, depth: 0, realDepth: 0, isThread: isThreadOf(p) });
        }
      }
    }
    return out;
  });

  return { treeNodes, initPID, idleProcs, treeParentOf, isThreadOf };
}
