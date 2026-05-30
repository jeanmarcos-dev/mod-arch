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
    // Visibility model:
    //   showInit ON  → render swapper (PID 0, one task per core), kthreadd
    //                  (PID 2), the kernel/N siblings, init (PID 1) and
    //                  every user proc as a single hierarchical tree.
    //   showInit OFF → strip kernel-side machinery (idle + init + kthreads),
    //                  leaving only the user-proc subtree as standalone
    //                  roots — matches the introductory demo experience.
    const visibleProcs = hideInit
      ? cpu.procs.filter(p =>
          p.is_idle !== true &&
          p.pid !== initPID.value &&
          p.is_kthread !== true,
        )
      : cpu.procs.slice();
    const byParent = new Map<number, PCBInfo[]>();
    for (const p of visibleProcs) {
      let pp = treeParentOf(p);
      // When init is hidden, promote its direct children to top-level so
      // they appear as roots instead of orphaned under a missing node.
      if (hideInit && pp === initPID.value) pp = 0;
      if (!byParent.has(pp)) byParent.set(pp, []);
      byParent.get(pp)!.push(p);
    }
    // Within each bucket: swappers (is_idle, all share PID 0) sort by tid so
    // swapper/0 always comes before swapper/1. Everyone else sorts by PID.
    for (const list of byParent.values()) {
      list.sort((a, b) => {
        if (a.pid !== b.pid) return a.pid - b.pid;
        return (a.tid ?? 0) - (b.tid ?? 0);
      });
    }
    // Identity key: swappers all share PID 0, so we key them by TID (each
    // swapper carries a unique negative TID assigned by the backend). Every
    // other PCB uses its PID directly.
    const keyOf = (p: PCBInfo) => p.is_idle === true ? `t${p.tid ?? 0}` : `p${p.pid}`;
    const out: ProcTreeNode[] = [];
    // Guard against revisiting PID 0: the swapper's PID would otherwise loop
    // the recursion back to the root bucket. Swappers are leaves anyway.
    const visited = new Set<number>([0]);
    const visit = (parent: number, depth: number) => {
      const kids = byParent.get(parent);
      if (!kids) return;
      for (const k of kids) {
        out.push({
          proc: k,
          depth: Math.min(depth, cap),
          realDepth: depth,
          isThread: isThreadOf(k),
        });
        if (k.is_idle === true) continue;
        if (visited.has(k.pid)) continue;
        visited.add(k.pid);
        visit(k.pid, depth + 1);
      }
    };
    visit(0, 0);
    // Fallback for snapshots arriving mid-reload where some proc may lack a
    // resolvable parent — append them so they remain visible.
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
